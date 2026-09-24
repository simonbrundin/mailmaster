package logging

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Logger wraps structured logging
type Logger struct {
	*slog.Logger
	file  *os.File
	mu    sync.Mutex
}

// New creates a new logger
func New(level string, logFile string) (*Logger, error) {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	var handler slog.Handler
	var file *os.File

	if logFile != "" {
		// Ensure directory exists
		dir := filepath.Dir(logFile)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %w", err)
		}

		var err error
		file, err = os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file: %w", err)
		}

		// Use JSON format for file
		handler = slog.NewJSONHandler(file, opts)
	}

	// Use text format for stdout
	stdHandler := slog.NewTextHandler(os.Stdout, opts)

	// Combine handlers
	if handler != nil {
		handler = slog.NewMultiHandler(handler, stdHandler)
	} else {
		handler = stdHandler
	}

	return &Logger{
		Logger: slog.New(handler),
		file:   file,
	}, nil
}

// Close closes the log file
func (l *Logger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// LogEmailAction logs an email processing action
func (l *Logger) LogEmailAction(emailSubject, emailFrom, action, motivation string, confidence float64) {
	l.Info("email_action",
		"subject", emailSubject,
		"from", emailFrom,
		"action", action,
		"motivation", motivation,
		"confidence", fmt.Sprintf("%.0f%%", confidence*100),
	)
}

// LogError logs an error with context
func (l *Logger) LogError(operation string, err error, context map[string]interface{}) {
	args := []interface{}{"operation", operation, "error", err.Error()}
	for k, v := range context {
		args = append(args, k, v)
	}
	l.Error("operation_failed", args...)
}

// LogConnection logs IMAP connection events
func (l *Logger) LogConnection(server, status string, keyvals ...interface{}) {
	args := []interface{}{"server", server, "status", status}
	args = append(args, keyvals...)
	l.Info("imap_connection", args...)
}

// LogRuleMatch logs when a rule matches
func (l *Logger) LogRuleMatch(ruleName string, emailSubject string) {
	l.Debug("rule_matched",
		"rule", ruleName,
		"subject", emailSubject,
	)
}

// LogStartup logs application startup
func (l *Logger) LogStartup(folder string, ruleFile string) {
	l.Info("mailagent_started",
		"folder", folder,
		"rules", ruleFile,
		"version", "1.0.0",
		"time", time.Now().Format(time.RFC3339),
	)
}

// LogShutdown logs application shutdown
func (l *Logger) LogShutdown() {
	l.Info("mailagent_shutdown",
		"time", time.Now().Format(time.RFC3339),
	)
}

// LogFallback logs when fallback email was sent
func (l *Logger) LogFallback(emailSubject, emailFrom, toEmail string) {
	l.Warn("fallback_email_sent",
		"original_subject", emailSubject,
		"original_from", emailFrom,
		"notified_email", toEmail,
	)
}
