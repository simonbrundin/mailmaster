package mcp

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"mailagent/actions"
	"mailagent/ai"
	"mailagent/config"
	"mailagent/imap"
	"mailagent/rules"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server wraps the MCP server with mail agent dependencies
type Server struct {
	cfg        *config.Config
	aiProvider *ai.OpenAIProvider
	ruleSet    *rules.RuleSet
	executor   *actions.Executor
	imapClient *imap.Client
	account    *config.MailAccount
	httpServer *http.Server

	// sessions keeps one IMAP session per account, so that switching accounts
	// in the UI does not require a new login every time.
	sessions *sessionStore

	// connMu serialises switching the active account and connection.
	connMu sync.Mutex

	// logins holds the pending device code logins per account.
	loginsMu sync.Mutex
	logins   map[string]*pendingLogin

	// analyses holds the AI proposals for the emails the user has analysed,
	// so that an approval can be executed with the exact proposed arguments.
	analysesMu sync.Mutex
	analyses   map[string]map[uint32]analysisItem
}

// NewServer creates a new MCP server
func NewServer(cfg *config.Config, aiProv *ai.OpenAIProvider, ruleSet *rules.RuleSet) *Server {
	return &Server{
		cfg:        cfg,
		aiProvider: aiProv,
		ruleSet:    ruleSet,
		sessions:   newSessionStore(),
		logins:     make(map[string]*pendingLogin),
		analyses:   make(map[string]map[uint32]analysisItem),
	}
}

// analysisKey identifies the analyses for one account and folder.
func analysisKey(account, folder string) string {
	return account + "|" + folder
}

// storeAnalyses saves the analyses for an account and folder.
func (s *Server) storeAnalyses(account, folder string, items map[uint32]analysisItem) {
	s.analysesMu.Lock()
	defer s.analysesMu.Unlock()
	s.analyses[analysisKey(account, folder)] = items
}

// analysisFor returns a previously stored analysis.
func (s *Server) analysisFor(account, folder string, uid uint32) (analysisItem, bool) {
	s.analysesMu.Lock()
	defer s.analysesMu.Unlock()
	item, ok := s.analyses[analysisKey(account, folder)][uid]
	return item, ok
}

// Start begins the MCP server on stdio
func (s *Server) Start(ctx context.Context) error {
	server := s.newMCPServer()
	return server.Run(ctx, &mcp.StdioTransport{})
}

// newMCPServer creates a new MCP server with tools configured
func (s *Server) newMCPServer() *mcp.Server {
	server := mcp.NewServer(
		&mcp.Implementation{Name: "mailagent", Version: "1.0.0"},
		&mcp.ServerOptions{Instructions: "MailAgent - Email processing with AI"},
	)
	s.addTools(server)
	return server
}

// StartHTTP starts the MCP server with HTTP endpoints
func (s *Server) StartHTTP(ctx context.Context, addr string) error {
	server := s.newMCPServer()

	// Create HTTP handler for MCP protocol
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, nil)

	// Create mux to handle both MCP protocol and our custom HTTP endpoints
	mux := http.NewServeMux()
	mux.Handle("/mcp", handler)
	mux.Handle("/api/mcp", handler)

	// Add custom HTTP endpoints (UI API + MCP endpoints)
	httpHandler := s.HTTPHandler()
	mux.HandleFunc("/", httpHandler.ServeHTTP)

	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	log.Printf("MCP HTTP server starting on %s", addr)
	return s.httpServer.ListenAndServe()
}

// Shutdown stops the HTTP server
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// --- Connection Management ---

// ensureConnection connects to the default account.
func (s *Server) ensureConnection() (*accountSession, error) {
	return s.ensureConnectionFor("")
}

// ensureConnectionFor makes sure an IMAP connection exists for the given
// account name. An empty name means the first configured account. The
// connection is cached per account, so switching back and forth is cheap.
func (s *Server) ensureConnectionFor(accountName string) (*accountSession, error) {
	s.connMu.Lock()
	defer s.connMu.Unlock()

	account, err := s.accountByName(accountName)
	if err != nil {
		return nil, err
	}

	// Reuse the session from an earlier visit to this account, unless its
	// connection died.
	if session, ok := s.sessions.get(account.Name); ok && !session.broken {
		return session, nil
	}
	s.sessions.remove(account.Name)

	credential, err := credentialFor(account)
	if err != nil {
		return nil, fmt.Errorf("failed to get credentials: %w", err)
	}

	client, err := imap.NewClient(account)
	if err != nil {
		return nil, fmt.Errorf("failed to create IMAP client: %w", err)
	}
	if err := client.Connect(credential); err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	session := &accountSession{
		account:    account,
		imapClient: client,
		credential: credential,
	}
	s.sessions.put(account.Name, session)

	return session, nil
}

// activeSession returns the session used when no account is specified.
func (s *Server) activeSession() *accountSession {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	if session, ok := s.sessions.get(s.defaultAccountName()); ok {
		return session
	}
	return nil
}

// defaultAccountName returns the name of the first configured account.
func (s *Server) defaultAccountName() string {
	if s.cfg == nil || len(s.cfg.Mail) == 0 {
		return ""
	}
	return s.cfg.Mail[0].Name
}

// resolveFolder returns the folder to use. When no folder is given, the
// folder configured for the account is used.
func (s *Server) resolveFolder(account *config.MailAccount, folder string) string {
	if folder != "" {
		return folder
	}
	if account != nil && account.Folder != "" {
		return account.Folder
	}
	return "INBOX"
}

// --- Tool Definitions ---

func (s *Server) addTools(server *mcp.Server) {
	// List emails
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_emails",
		Description: "List emails in a folder. Returns email subjects, senders, and UIDs.",
	}, s.toolListEmails)

	// Read email
	mcp.AddTool(server, &mcp.Tool{
		Name:        "read_email",
		Description: "Read the full content of an email including body.",
	}, s.toolReadEmail)

	// Archive email
	mcp.AddTool(server, &mcp.Tool{
		Name:        "archive_email",
		Description: "Archive an email.",
	}, s.toolArchiveEmail)

	// Reply to email
	mcp.AddTool(server, &mcp.Tool{
		Name:        "reply_to_email",
		Description: "Reply to an email.",
	}, s.toolReplyToEmail)

	// Forward email
	mcp.AddTool(server, &mcp.Tool{
		Name:        "forward_email",
		Description: "Forward an email to another email address. Requires 'to' parameter.",
	}, s.toolForwardEmail)

	// Move email
	mcp.AddTool(server, &mcp.Tool{
		Name:        "move_email",
		Description: "Move an email to a different folder.",
	}, s.toolMoveEmail)

	// Process emails with AI
	mcp.AddTool(server, &mcp.Tool{
		Name:        "process_emails_with_ai",
		Description: "Process emails using AI to decide and execute actions.",
	}, s.toolProcessEmailsWithAI)
}

// --- Tool Handlers ---

type ListEmailsParams struct {
	Folder string `json:"folder"`
	Limit  int    `json:"limit"`
}

func (s *Server) toolListEmails(ctx context.Context, req *mcp.CallToolRequest, args ListEmailsParams) (*mcp.CallToolResult, any, error) {
	session, err := s.ensureConnection()
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + err.Error()}},
			IsError: true,
		}, nil, nil
	}

	folder := s.resolveFolder(session.account, args.Folder)
	limit := args.Limit
	if limit == 0 {
		limit = 10
	}

	emails, err := session.imapClient.ListEmails(folder, uint32(limit))
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + err.Error()}},
			IsError: true,
		}, nil, nil
	}

	var result string
	for _, email := range emails {
		result += fmt.Sprintf("- #%d: %s (from: %s)\n", email.UID, email.Subject, email.From)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Found %d emails in %s:\n%s", len(emails), folder, result)}},
	}, nil, nil
}

type ReadEmailParams struct {
	Folder string `json:"folder"`
	UID    int    `json:"uid"`
}

func (s *Server) toolReadEmail(ctx context.Context, req *mcp.CallToolRequest, args ReadEmailParams) (*mcp.CallToolResult, any, error) {
	session, err := s.ensureConnection()
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + err.Error()}},
			IsError: true,
		}, nil, nil
	}

	if args.UID == 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: uid is required"}},
			IsError: true,
		}, nil, nil
	}

	folder := s.resolveFolder(session.account, args.Folder)

	email, err := session.imapClient.ReadEmail(folder, uint32(args.UID))
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + err.Error()}},
			IsError: true,
		}, nil, nil
	}

	text := fmt.Sprintf("From: %s\nTo: %s\nDate: %s\nSubject: %s\n\n%s",
		email.From, email.To, email.Date.Format(time.RFC3339), email.Subject, email.Body)

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}, nil, nil
}

type ArchiveEmailParams struct {
	Folder string `json:"folder"`
	UID    int    `json:"uid"`
}

func (s *Server) toolArchiveEmail(ctx context.Context, req *mcp.CallToolRequest, args ArchiveEmailParams) (*mcp.CallToolResult, any, error) {
	session, err := s.ensureConnection()
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + err.Error()}},
			IsError: true,
		}, nil, nil
	}

	if args.UID == 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: uid is required"}},
			IsError: true,
		}, nil, nil
	}

	folder := s.resolveFolder(session.account, args.Folder)

	if err := session.imapClient.ArchiveEmail(folder, uint32(args.UID)); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + err.Error()}},
			IsError: true,
		}, nil, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Email %d archived", args.UID)}},
	}, nil, nil
}

type ReplyToEmailParams struct {
	Folder  string `json:"folder"`
	UID     int    `json:"uid"`
	Message string `json:"message"`
}

func (s *Server) toolReplyToEmail(ctx context.Context, req *mcp.CallToolRequest, args ReplyToEmailParams) (*mcp.CallToolResult, any, error) {
	session, err := s.ensureConnection()
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + err.Error()}},
			IsError: true,
		}, nil, nil
	}

	if args.UID == 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: uid is required"}},
			IsError: true,
		}, nil, nil
	}

	folder := s.resolveFolder(session.account, args.Folder)

	email, err := session.imapClient.ReadEmail(folder, uint32(args.UID))
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + err.Error()}},
			IsError: true,
		}, nil, nil
	}

	ruleResp := &rules.Response{
		Action:     "reply",
		Motivation: "",
		ActionArgs: map[string]interface{}{"reply": args.Message},
	}

	if err := session.executor.Execute(email, ruleResp); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + err.Error()}},
			IsError: true,
		}, nil, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Replied to email %d", args.UID)}},
	}, nil, nil
}

type ForwardEmailParams struct {
	Folder string `json:"folder"`
	UID    int    `json:"uid"`
	To     string `json:"to"`
	Note   string `json:"note"`
}

func (s *Server) toolForwardEmail(ctx context.Context, req *mcp.CallToolRequest, args ForwardEmailParams) (*mcp.CallToolResult, any, error) {
	session, err := s.ensureConnection()
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + err.Error()}},
			IsError: true,
		}, nil, nil
	}

	if args.UID == 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: uid is required"}},
			IsError: true,
		}, nil, nil
	}
	if args.To == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: 'to' address is required"}},
			IsError: true,
		}, nil, nil
	}

	folder := s.resolveFolder(session.account, args.Folder)

	email, err := session.imapClient.ReadEmail(folder, uint32(args.UID))
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + err.Error()}},
			IsError: true,
		}, nil, nil
	}

	note := args.Note
	if note == "" {
		note = "Forwarded email"
	}

	ruleResp := &rules.Response{
		Action:     "forward",
		Motivation: "",
		ActionArgs: map[string]interface{}{"to": args.To, "reply": note},
	}

	if err := session.executor.Execute(email, ruleResp); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + err.Error()}},
			IsError: true,
		}, nil, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Email %d forwarded to %s", args.UID, args.To)}},
	}, nil, nil
}

type MoveEmailParams struct {
	Folder      string `json:"folder"`
	UID         int    `json:"uid"`
	Destination string `json:"destination"`
}

func (s *Server) toolMoveEmail(ctx context.Context, req *mcp.CallToolRequest, args MoveEmailParams) (*mcp.CallToolResult, any, error) {
	session, err := s.ensureConnection()
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + err.Error()}},
			IsError: true,
		}, nil, nil
	}

	if args.UID == 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: uid is required"}},
			IsError: true,
		}, nil, nil
	}
	if args.Destination == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: 'destination' is required"}},
			IsError: true,
		}, nil, nil
	}

	folder := s.resolveFolder(session.account, args.Folder)

	if err := session.imapClient.MoveEmail(folder, args.Destination, uint32(args.UID)); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + err.Error()}},
			IsError: true,
		}, nil, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Email %d moved to %s", args.UID, args.Destination)}},
	}, nil, nil
}

type ProcessEmailsParams struct {
	Folder string `json:"folder"`
	Limit  int    `json:"limit"`
}

func (s *Server) toolProcessEmailsWithAI(ctx context.Context, req *mcp.CallToolRequest, args ProcessEmailsParams) (*mcp.CallToolResult, any, error) {
	session, err := s.ensureConnection()
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + err.Error()}},
			IsError: true,
		}, nil, nil
	}

	folder := s.resolveFolder(session.account, args.Folder)
	limit := args.Limit
	if limit == 0 {
		limit = 10
	}

	emails, err := session.imapClient.ListEmails(folder, uint32(limit))
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + err.Error()}},
			IsError: true,
		}, nil, nil
	}

	var results string
	for _, email := range emails {
		fullEmail, err := session.imapClient.ReadEmail(folder, email.UID)
		if err != nil {
			results += fmt.Sprintf("❌ #%d: Error reading - %v\n", email.UID, err)
			continue
		}

		aiResp, err := s.aiProvider.Process(ctx, s.ruleSet.BuildPromptForAI(fullEmail))
		if err != nil {
			results += fmt.Sprintf("❌ #%d (%s): AI error - %v\n", email.UID, email.Subject, err)
			continue
		}

		ruleResp := &rules.Response{
			Action:     aiResp.Action,
			Motivation: aiResp.Motivation,
			ActionArgs: aiResp.ActionArgs,
		}

		if err := session.executor.Execute(fullEmail, ruleResp); err != nil {
			results += fmt.Sprintf("⚠️ #%d (%s): %s - %v\n", email.UID, email.Subject, aiResp.Action, err)
		} else {
			results += fmt.Sprintf("✅ #%d (%s): %s\n", email.UID, email.Subject, aiResp.Action)
		}
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: results}},
	}, nil, nil
}
