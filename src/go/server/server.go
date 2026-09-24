package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"mailagent/actions"
	"mailagent/ai"
	"mailagent/config"
	"mailagent/imap"
	"mailagent/rules"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Server wraps the HTTP server with mail agent dependencies
type Server struct {
	cfg         *config.Config
	aiProvider  *ai.OpenAIProvider
	ruleSet     *rules.RuleSet
	httpServer  *http.Server
	connections map[string]*imap.Client // Active IMAP connections per account
	executors   map[string]*actions.Executor
}

// New creates a new API server
func New(cfg *config.Config, aiProvider *ai.OpenAIProvider, ruleSet *rules.RuleSet) *Server {
	return &Server{
		cfg:         cfg,
		aiProvider:  aiProvider,
		ruleSet:     ruleSet,
		connections: make(map[string]*imap.Client),
		executors:   make(map[string]*actions.Executor),
	}
}

// Handler returns the HTTP handler with all routes configured
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Timeout(30 * time.Second))

	// CORS
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	})

	// Health check
	r.Get("/health", s.handleHealth)

	// API v1 routes
	r.Route("/api/v1", func(r chi.Router) {
		// Accounts
		r.Get("/accounts", s.handleListAccounts)
		r.Route("/accounts/{accountName}", func(r chi.Router) {
			r.Use(s.withAccount)

			// Folders
			r.Get("/folders", s.handleListFolders)
			r.Get("/folders/{folder}/count", s.handleCountEmails)
			r.Get("/folders/{folder}/emails", s.handleListEmails)
			r.Get("/folders/{folder}/emails/{uid}", s.handleGetEmail)

			// Process emails with AI
			r.Post("/process", s.handleProcessEmails)

			// Execute actions on email
			r.Post("/emails/{uid}/action", s.handleExecuteAction)

			// Move email
			r.Post("/emails/{uid}/move", s.handleMoveEmail)
		})

		// Rules
		r.Get("/rules", s.handleGetRules)
		r.Put("/rules", s.handleUpdateRules)
	})

	return r
}

// withAccount is a middleware that loads the account into the request context
func (s *Server) withAccount(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accountName := chi.URLParam(r, "accountName")
		acc := s.getAccount(accountName)
		if acc == nil {
			errorResponse(w, http.StatusNotFound, "Account not found")
			return
		}
		ctx := context.WithValue(r.Context(), "account", acc)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Start begins the HTTP server
func (s *Server) Start(addr string) error {
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      s.Handler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully stops the server
func (s *Server) Shutdown(ctx context.Context) error {
	// Close all IMAP connections
	for name, conn := range s.connections {
		conn.Close()
		delete(s.connections, name)
	}
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// getAccount retrieves account config by name
func (s *Server) getAccount(name string) *config.MailAccount {
	for i := range s.cfg.Mail {
		if s.cfg.Mail[i].Name == name {
			return &s.cfg.Mail[i]
		}
	}
	return nil
}

// getConnection returns or creates an IMAP connection for an account
func (s *Server) getConnection(acc *config.MailAccount) (*imap.Client, error) {
	// Check cache
	if conn, ok := s.connections[acc.Name]; ok {
		return conn, nil
	}

	// Get credentials
	password, err := s.getAccountCredential(acc)
	if err != nil {
		return nil, fmt.Errorf("failed to get credentials: %w", err)
	}

	// Create new IMAP client
	client, err := imap.NewClient(acc)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	// Connect with timeout
	connectCh := make(chan error, 1)
	go func() {
		connectCh <- client.Connect(password)
	}()

	select {
	case <-time.After(10 * time.Second):
		client.Close()
		return nil, fmt.Errorf("connection timeout after 10s")
	case err := <-connectCh:
		if err != nil {
			return nil, fmt.Errorf("failed to connect: %w", err)
		}
	}

	s.connections[acc.Name] = client

	// Create executor with SMTP credentials
	executor := actions.NewExecutor(client, &s.cfg.Fallback)
	executor.SetSMTPCredentials(password)
	s.executors[acc.Name] = executor

	return client, nil
}

// getAccountCredential returns password or OAuth2 token for an account
func (s *Server) getAccountCredential(acc *config.MailAccount) (string, error) {
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

// closeConnection closes and removes an IMAP connection
func (s *Server) closeConnection(accountName string) {
	if conn, ok := s.connections[accountName]; ok {
		conn.Close()
		delete(s.connections, accountName)
		delete(s.executors, accountName)
	}
}

// JSON response helpers
func jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

func errorResponse(w http.ResponseWriter, status int, message string) {
	jsonResponse(w, status, map[string]string{"error": message})
}
