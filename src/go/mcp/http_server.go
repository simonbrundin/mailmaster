package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// HTTPHandler creates an HTTP handler for the MCP server
func (s *Server) HTTPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("[MCP HTTP] %s %s\n", r.Method, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		// MCP protocol endpoints
		if path == "/mcp" || path == "/api/mcp" || path == "/tools/list" {
			s.httpMCPToolList(w, r)
			return
		}

		// API v1 endpoints for UI
		switch {
		// List emails: GET /api/v1/accounts/{account}/folders/{folder}/emails
		case r.Method == "GET" && strings.Contains(path, "/accounts/") && strings.Contains(path, "/folders/") && strings.HasSuffix(path, "/emails"):
			s.httpListEmails(w, r, path)

		// Process (placeholder - AI handles this): POST /api/v1/accounts/{account}/process
		case r.Method == "POST" && strings.Contains(path, "/accounts/") && strings.HasSuffix(path, "/process"):
			s.httpProcessPlaceholder(w, r)

		// Rules: GET/PUT /api/v1/rules
		case r.Method == "GET" && (path == "/api/v1/rules" || path == "/rules"):
			s.httpGetRules(w, r)
		case r.Method == "PUT" && (path == "/api/v1/rules" || path == "/rules"):
			s.httpUpdateRules(w, r)

		// Health
		case path == "/health":
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

		default:
			fmt.Printf("[MCP HTTP] No handler for %s %s\n", r.Method, path)
			http.Error(w, `{"error": "Not found"}`, http.StatusNotFound)
		}
	})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// --- MCP Tool List ---

func (s *Server) httpMCPToolList(w http.ResponseWriter, r *http.Request) {
	tools := []map[string]interface{}{
		{"name": "list_emails", "description": "List emails in a folder"},
		{"name": "read_email", "description": "Read full email content"},
		{"name": "forward_email", "description": "Forward email to another address"},
		{"name": "reply_to_email", "description": "Reply to an email"},
		{"name": "archive_email", "description": "Archive an email"},
		{"name": "move_email", "description": "Move email to another folder"},
	}
	writeJSON(w, 200, map[string]interface{}{"tools": tools})
}

// --- HTTP API Handlers ---

func (s *Server) httpListEmails(w http.ResponseWriter, r *http.Request, path string) {
	// Parse: /api/v1/accounts/{account}/folders/{folder}/emails
	folder := "INBOX"
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if p == "folders" && i+1 < len(parts) {
			folder = parts[i+1]
			break
		}
	}

	// Override from query param
	if f := r.URL.Query().Get("folder"); f != "" {
		folder = f
	}

	if err := s.ensureConnection(); err != nil {
		writeError(w, 500, err.Error())
		return
	}

	limit := uint32(50)
	if l := r.URL.Query().Get("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}

	emails, err := s.imapClient.ListEmails(folder, limit)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}

	result := make([]map[string]interface{}, 0, len(emails))
	for _, e := range emails {
		result = append(result, map[string]interface{}{
			"uid":             e.UID,
			"subject":         e.Subject,
			"from":            e.From,
			"date":            e.Date,
			"folder":          e.Folder,
			"has_attachment": e.HasAttachment,
		})
	}

	writeJSON(w, 200, map[string]interface{}{"emails": result})
}

func (s *Server) httpProcessPlaceholder(w http.ResponseWriter, r *http.Request) {
	// AI handles processing via MCP tools - this is just for compatibility
	body, _ := io.ReadAll(r.Body)
	fmt.Printf("[MCP HTTP] Process called with body: %s\n", string(body))
	
	writeJSON(w, 200, map[string]interface{}{
		"message": "Use MCP tools (forward_email, reply_to_email, archive_email) to process emails",
		"instructions": "Call MCP tools directly with email UID and action parameters",
	})
}

func (s *Server) httpGetRules(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]string{
		"content": "# MailMaster Rules\n\nDefine email processing rules here.",
	})
}

func (s *Server) httpUpdateRules(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	fmt.Printf("[MCP HTTP] Rules updated: %d chars\n", len(req.Content))
	writeJSON(w, 200, map[string]string{"message": "Rules updated"})
}
