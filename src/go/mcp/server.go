package mcp

import (
	"context"
	"fmt"
	"log"
	"net/http"
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
	cfg         *config.Config
	aiProvider  *ai.OpenAIProvider
	ruleSet     *rules.RuleSet
	executor    *actions.Executor
	imapClient  *imap.Client
	account     *config.MailAccount
	httpServer  *http.Server
}

// NewServer creates a new MCP server
func NewServer(cfg *config.Config, aiProv *ai.OpenAIProvider, ruleSet *rules.RuleSet) *Server {
	return &Server{
		cfg:        cfg,
		aiProvider: aiProv,
		ruleSet:    ruleSet,
	}
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

func (s *Server) ensureConnection() error {
	if s.imapClient != nil {
		return nil
	}

	if len(s.cfg.Mail) == 0 {
		return fmt.Errorf("no mail accounts configured")
	}

	s.account = &s.cfg.Mail[0]

	password, err := s.account.GetPassword()
	if err != nil {
		return fmt.Errorf("failed to get credentials: %w", err)
	}

	s.imapClient, err = imap.NewClient(s.account)
	if err != nil {
		return fmt.Errorf("failed to create IMAP client: %w", err)
	}
	if err := s.imapClient.Connect(password); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	s.executor = actions.NewExecutor(s.imapClient, &s.cfg.Fallback)
	s.executor.SetSMTPCredentials(password)

	return nil
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
	if err := s.ensureConnection(); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + err.Error()}},
			IsError: true,
		}, nil, nil
	}

	folder := args.Folder
	if folder == "" {
		folder = "INBOX"
	}
	limit := args.Limit
	if limit == 0 {
		limit = 10
	}

	emails, err := s.imapClient.ListEmails(folder, uint32(limit))
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
	if err := s.ensureConnection(); err != nil {
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

	folder := args.Folder
	if folder == "" {
		folder = "INBOX"
	}

	email, err := s.imapClient.ReadEmail(folder, uint32(args.UID))
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
	if err := s.ensureConnection(); err != nil {
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

	folder := args.Folder
	if folder == "" {
		folder = "INBOX"
	}

	if err := s.imapClient.ArchiveEmail(folder, uint32(args.UID)); err != nil {
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
	if err := s.ensureConnection(); err != nil {
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

	folder := args.Folder
	if folder == "" {
		folder = "INBOX"
	}

	email, err := s.imapClient.ReadEmail(folder, uint32(args.UID))
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

	if err := s.executor.Execute(email, ruleResp); err != nil {
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
	if err := s.ensureConnection(); err != nil {
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

	folder := args.Folder
	if folder == "" {
		folder = "INBOX"
	}

	email, err := s.imapClient.ReadEmail(folder, uint32(args.UID))
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

	if err := s.executor.Execute(email, ruleResp); err != nil {
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
	if err := s.ensureConnection(); err != nil {
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

	folder := args.Folder
	if folder == "" {
		folder = "INBOX"
	}

	if err := s.imapClient.MoveEmail(folder, args.Destination, uint32(args.UID)); err != nil {
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
	if err := s.ensureConnection(); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + err.Error()}},
			IsError: true,
		}, nil, nil
	}

	folder := args.Folder
	if folder == "" {
		folder = "INBOX"
	}
	limit := args.Limit
	if limit == 0 {
		limit = 10
	}

	emails, err := s.imapClient.ListEmails(folder, uint32(limit))
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + err.Error()}},
			IsError: true,
		}, nil, nil
	}

	var results string
	for _, email := range emails {
		fullEmail, err := s.imapClient.ReadEmail(folder, email.UID)
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

		if err := s.executor.Execute(fullEmail, ruleResp); err != nil {
			results += fmt.Sprintf("⚠️ #%d (%s): %s - %v\n", email.UID, email.Subject, aiResp.Action, err)
		} else {
			results += fmt.Sprintf("✅ #%d (%s): %s\n", email.UID, email.Subject, aiResp.Action)
		}
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: results}},
	}, nil, nil
}
