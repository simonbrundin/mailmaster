package main

import (
	"context"
	"flag"
	"fmt"
	"mailagent/actions"
	"mailagent/ai"
	"mailagent/config"
	"mailagent/imap"
	"mailagent/logging"
	"mailagent/mcp"
	"mailagent/rules"
	"mailagent/server"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

var (
	configPath  = flag.String("config", "config/config.yaml", "Path to configuration file")
	rulesPath   = flag.String("rules", "rules.md", "Path to rules markdown file")
	listen      = flag.Bool("listen", false, "Listen for new emails continuously")
	limit       = flag.Uint("limit", 10, "Number of emails to process")
	once        = flag.Bool("once", false, "Process emails once and exit")
	listFolders = flag.Bool("list-folders", false, "List available folders and exit")
	accountName = flag.String("account", "", "Process only this account (by name)")
	apiAddr     = flag.String("api", "", "Start HTTP API server on this address (e.g., :8080)")
	mcpMode     = flag.Bool("mcp", false, "Run as MCP server (stdio)")
)

func main() {
	flag.Parse()

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log, err := logging.New(cfg.Logging.Level, cfg.Logging.File)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Close()

	// Load rules
	ruleContent, err := os.ReadFile(*rulesPath)
	if err != nil {
		log.LogError("load_rules", err, nil)
		os.Exit(1)
	}

	ruleSet, err := rules.ParseMarkdown(string(ruleContent))
	if err != nil {
		log.LogError("parse_rules", err, nil)
		os.Exit(1)
	}

	// Get AI API key
	apiKey, _ := cfg.GetAPIKey()

	// Initialize AI provider
	aiProvider := ai.NewOpenAIProvider(&cfg.AI, apiKey)

	// Start MCP server if --mcp flag is set
	if *mcpMode {
		mcpServer := mcp.NewServer(cfg, aiProvider, ruleSet)
		log.Info("mcp_server_starting", "mode", "stdio")
		if err := mcpServer.Start(ctx); err != nil {
			log.LogError("mcp_server", err, nil)
			os.Exit(1)
		}
		return
	}

	// Start API server if --api flag is set
	if *apiAddr != "" {
		apiServer := server.New(cfg, aiProvider, ruleSet)
		log.Info("api_server_starting", "addr", *apiAddr)
		go func() {
			if err := apiServer.Start(*apiAddr); err != nil && err != http.ErrServerClosed {
				log.LogError("api_server", err, nil)
			}
		}()
		defer apiServer.Shutdown(context.Background())

		// Only run API server, wait for shutdown signal
		<-ctx.Done()
		return
	}

	// Filter accounts if --account flag is set
	accounts := cfg.Mail
	if *accountName != "" {
		filtered := []config.MailAccount{}
		for _, acc := range cfg.Mail {
			if acc.Name == *accountName {
				filtered = append(filtered, acc)
				break
			}
		}
		if len(filtered) == 0 {
			fmt.Fprintf(os.Stderr, "Account '%s' not found. Available accounts:\n", *accountName)
			for _, acc := range cfg.Mail {
				fmt.Printf("  - %s (%s)\n", acc.Name, acc.Username)
			}
			os.Exit(1)
		}
		accounts = filtered
	}

	if len(accounts) == 0 {
		fmt.Fprintf(os.Stderr, "No mail accounts configured\n")
		os.Exit(1)
	}

	log.Info("mailagent_started",
		"accounts", len(accounts),
		"rules", *rulesPath,
		"version", "1.0.0",
	)

	// List folders for all accounts if requested
	if *listFolders {
		for _, acc := range accounts {
			fmt.Printf("\n=== %s (%s) ===\n", acc.Name, acc.Username)
			listAccountFolders(&acc, log)
		}
		return
	}

	// Process all accounts
	var wg sync.WaitGroup
	for _, acc := range accounts {
		wg.Add(1)
		go func(account config.MailAccount) {
			defer wg.Done()
			processAccount(ctx, log, &account, aiProvider, ruleSet)
		}(acc)
	}

	// Wait for all accounts
	wg.Wait()
}

func getAccountCredential(acc *config.MailAccount) (string, error) {
	if acc.UseOAuth2 {
		return ai.GetOAuthAccessToken(ai.OAuthConfig{
			ClientID:    acc.AzureClientID,
			TenantID:    acc.AzureTenantID,
			Username:    acc.Username,
			RedirectURI: acc.OAuthRedirectURI,
			CachePath:   acc.OAuthCachePath,
		})
	}
	return acc.GetPassword()
}

func listAccountFolders(acc *config.MailAccount, log *logging.Logger) {
	// Create IMAP client
	imapClient, err := imap.NewClient(acc)
	if err != nil {
		log.LogError("create_imap_client", err, map[string]interface{}{
			"account": acc.Name,
		})
		return
	}

	// Get password or OAuth2 access token
	password, err := getAccountCredential(acc)
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
		return
	}

	// Connect
	if err := imapClient.Connect(password); err != nil {
		log.LogError("imap_connect", err, map[string]interface{}{
			"account": acc.Name,
			"server":  acc.Server,
		})
		return
	}
	defer imapClient.Close()

	// List folders
	folders, err := imapClient.ListFolders()
	if err != nil {
		log.LogError("list_folders", err, nil)
		return
	}

	fmt.Println("Available folders:")
	for _, f := range folders {
		fmt.Printf("  - %s\n", f)
	}
}

func processAccount(ctx context.Context, log *logging.Logger, acc *config.MailAccount, aiProv *ai.OpenAIProvider, ruleSet *rules.RuleSet) {
	// Create IMAP client
	imapClient, err := imap.NewClient(acc)
	if err != nil {
		log.LogError("create_imap_client", err, map[string]interface{}{
			"account": acc.Name,
		})
		return
	}

	// Get password or OAuth2 access token (used for IMAP and SMTP)
	password, err := getAccountCredential(acc)
	if err != nil {
		log.LogError("get_password", err, map[string]interface{}{
			"account": acc.Name,
		})
		return
	}

	// Connect
	if err := imapClient.Connect(password); err != nil {
		log.LogError("imap_connect", err, map[string]interface{}{
			"account": acc.Name,
			"server":  acc.Server,
		})
		return
	}
	defer imapClient.Close()
	log.LogConnection(acc.Server, "connected", "account", acc.Name)

	// Initialize action executor (same password for IMAP and SMTP)
	fallbackCfg := &config.FallbackConfig{
		Enabled:     true,
		NotifyEmail: "michael@brundins.se", // Fallback notification email
	}

	executor := actions.NewExecutor(imapClient, fallbackCfg)
	executor.SetSMTPCredentials(password)

	log.Info("processing_account",
		"account", acc.Name,
		"folder", acc.Folder,
	)

	// Process emails
	if *listen {
		processLoop(ctx, log, imapClient, aiProv, executor, ruleSet, acc.Name, acc.Folder)
	} else {
		processOnce(log, imapClient, aiProv, executor, ruleSet, acc.Name, acc.Folder, *limit)
		if *once {
			return
		}
	}
}

func processLoop(ctx context.Context, log *logging.Logger, client *imap.Client, aiProv *ai.OpenAIProvider, executor *actions.Executor, ruleSet *rules.RuleSet, accountName, folder string) {
	log.Info("Starting continuous email monitoring",
		"account", accountName,
		"folder", folder,
	)

	// Process existing emails first
	processOnce(log, client, aiProv, executor, ruleSet, accountName, folder, 10)

	// Then wait for new emails
	for {
		select {
		case <-ctx.Done():
			log.Info("stopping_account", "account", accountName)
			return
		default:
			err := client.WaitForNewEmails(ctx, folder)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.LogError("idle", err, map[string]interface{}{"account": accountName})
				time.Sleep(5 * time.Second) // Wait before retry
				continue
			}

			// New email received
			processOnce(log, client, aiProv, executor, ruleSet, accountName, folder, 3)
		}
	}
}

func processOnce(log *logging.Logger, client *imap.Client, aiProv *ai.OpenAIProvider, executor *actions.Executor, ruleSet *rules.RuleSet, accountName, folder string, limit uint) {
	emails, err := client.ListEmails(folder, uint32(limit))
	if err != nil {
		log.LogError("list_emails", err, map[string]interface{}{"account": accountName})
		return
	}

	if len(emails) == 0 {
		log.Debug("no_emails", "account", accountName, "folder", folder)
		return
	}

	log.Info("processing_emails",
		"account", accountName,
		"count", len(emails),
		"folder", folder,
	)

	for _, email := range emails {
		// Read full email content
		fullEmail, err := client.ReadEmail(folder, email.UID)
		if err != nil {
			log.LogError("read_email", err, map[string]interface{}{
				"account": accountName,
				"uid":     email.UID,
			})
			continue
		}

		// Build prompt for AI
		prompt := ruleSet.BuildPromptForAI(fullEmail)

		// Get AI response
		aiResponse, err := aiProv.Process(context.Background(), prompt)
		if err != nil {
			log.LogError("ai_process", err, map[string]interface{}{
				"account": accountName,
				"subject": fullEmail.Subject,
			})
			continue
		}

		log.Info("ai_decision",
			"account", accountName,
			"subject", fullEmail.Subject,
			"action", aiResponse.Action,
			"confidence", fmt.Sprintf("%.0f%%", aiResponse.Confidence*100),
			"motivation", aiResponse.Motivation,
		)

		// Convert AI response to rules.Response for executor
		ruleResp := &rules.Response{
			Action:     aiResponse.Action,
			Motivation: aiResponse.Motivation,
			Confidence: aiResponse.Confidence,
			ActionArgs: aiResponse.ActionArgs,
		}

		// Execute action
		if err := executor.Execute(fullEmail, ruleResp); err != nil {
			log.LogError("execute_action", err, map[string]interface{}{
				"account": accountName,
				"action":  aiResponse.Action,
				"uid":     fullEmail.UID,
			})
		}
	}
}
