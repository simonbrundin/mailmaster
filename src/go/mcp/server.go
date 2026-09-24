package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"

	"mailagent/actions"
	"mailagent/ai"
	"mailagent/config"
	"mailagent/imap"
	"mailagent/rules"
)

// Server implements the MCP protocol over stdio
type Server struct {
	cfg        *config.Config
	aiProvider *ai.OpenAIProvider
	ruleSet    *rules.RuleSet
	executor   *actions.Executor
	imapClient *imap.Client
	account    *config.MailAccount
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
	dec := json.NewDecoder(os.Stdin)
	enc := json.NewEncoder(os.Stdout)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			var req MCPRequest
			if err := dec.Decode(&req); err != nil {
				if err == io.EOF {
					return nil
				}
				log.Printf("Decode error: %v", err)
				continue
			}

			resp := s.handleRequest(&req)
			if err := enc.Encode(resp); err != nil {
				log.Printf("Encode error: %v", err)
			}
		}
	}
}

func (s *Server) handleRequest(req *MCPRequest) *MCPResponse {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolCall(req)
	case "notifications/initialized":
		return &MCPResponse{JSONRPC: "2.0"} // Ack
	default:
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    -32601,
				Message: fmt.Sprintf("Method not found: %s", req.Method),
			},
		}
	}
}

func (s *Server) handleInitialize(req *MCPRequest) *MCPResponse {
	result := InitializeResult{
		ProtocolVersion: "2024-11-05",
		ServerInfo: struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}{
			Name:    "mailagent-mcp",
			Version: "1.0.0",
		},
	}

	return &MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
}

func (s *Server) handleToolsList(req *MCPRequest) *MCPResponse {
	return &MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: ToolsListResult{
			Tools: s.getTools(),
		},
	}
}

func (s *Server) handleToolCall(req *MCPRequest) *MCPResponse {
	var params ToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    -32602,
				Message: fmt.Sprintf("Invalid params: %v", err),
			},
		}
	}

	args := params.Arguments
	if args == nil {
		args = make(map[string]interface{})
	}

	var result ContentBlock
	var err error

	switch params.Name {
	case "list_emails":
		result, err = s.toolListEmails(args)
	case "read_email":
		result, err = s.toolReadEmail(args)
	case "archive_email":
		result, err = s.toolArchiveEmail(args)
	case "reply_to_email":
		result, err = s.toolReplyToEmail(args)
	case "forward_email":
		result, err = s.toolForwardEmail(args)
	case "move_email":
		result, err = s.toolMoveEmail(args)
	case "process_emails_with_ai":
		result, err = s.toolProcessEmailsWithAI(args)
	default:
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    -32602,
				Message: fmt.Sprintf("Unknown tool: %s", params.Name),
			},
		}
	}

	if err != nil {
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: ToolResult{
				Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Error: %v", err)}},
				IsError: true,
			},
		}
	}

	return &MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: ToolResult{
			Content: []ContentBlock{result},
		},
	}
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
