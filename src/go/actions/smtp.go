package actions

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"time"

	"mailagent/sasl"
)

// smtpConfig holds the resolved SMTP settings for the current account.
type smtpConfig struct {
	server    string
	port      int
	username  string
	password  string
	fromEmail string
	useOAuth2 bool
}

// buildAuth creates the appropriate SMTP authentication mechanism.
func buildAuth(cfg smtpConfig) smtp.Auth {
	if cfg.username == "" || cfg.password == "" {
		return nil
	}
	if cfg.useOAuth2 {
		return saslXOAuth2(cfg.username, cfg.password)
	}
	return smtp.PlainAuth("", cfg.username, cfg.password, cfg.server)
}

// formatMessage formats an email message with headers.
func formatMessage(from, to, subject, body string) string {
	return fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		from, to, subject, body,
	)
}

// smtpClient handles SMTP email sending with STARTTLS.
type smtpClient struct {
	cfg smtpConfig
}

// newSMTPClient creates a new SMTP client with the given configuration.
func newSMTPClient(cfg smtpConfig) *smtpClient {
	return &smtpClient{cfg: cfg}
}

// send delivers an email via SMTP with STARTTLS (port 587), using a custom DNS
// resolver when the system resolver fails to reach the SMTP server.
func (c *smtpClient) send(to string, msg []byte) error {
	conn, err := c.dial()
	if err != nil {
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, c.cfg.server)
	if err != nil {
		return fmt.Errorf("SMTP client: %w", err)
	}
	defer client.Close()

	if err := c.startTLS(client); err != nil {
		return err
	}

	if err := c.authenticate(client); err != nil {
		return err
	}

	if err := c.sendMail(client, to); err != nil {
		return err
	}

	return c.sendData(client, to, msg)
}

// dial establishes a TCP connection to the SMTP server, falling back to
// direct IP connection if DNS resolution fails.
func (c *smtpClient) dial() (net.Conn, error) {
	addr := fmt.Sprintf("%s:%d", c.cfg.server, c.cfg.port)

	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err == nil {
		return conn, nil
	}

	// System DNS failed — resolve via public DNS (8.8.8.8)
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 5 * time.Second}
			return d.DialContext(ctx, "tcp", "8.8.8.8:53")
		},
	}

	ips, err := resolver.LookupIP(context.Background(), "ip", c.cfg.server)
	if err != nil || len(ips) == 0 {
		return nil, fmt.Errorf("DNS lookup failed for %s: %w", c.cfg.server, err)
	}

	conn, err = net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ips[0].String(), c.cfg.port), 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("SMTP dial %s: %w", ips[0].String(), err)
	}
	return conn, nil
}

// startTLS upgrades the connection to TLS via STARTTLS.
func (c *smtpClient) startTLS(client *smtp.Client) error {
	tlsConfig := &tls.Config{ServerName: c.cfg.server}
	if err := client.StartTLS(tlsConfig); err != nil {
		return fmt.Errorf("STARTTLS failed: %w", err)
	}
	return nil
}

// authenticate performs SMTP authentication.
func (c *smtpClient) authenticate(client *smtp.Client) error {
	auth := buildAuth(c.cfg)
	if auth == nil {
		return nil
	}
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("SMTP auth: %w", err)
	}
	return nil
}

// sendMail sets the sender and recipient.
func (c *smtpClient) sendMail(client *smtp.Client, to string) error {
	if err := client.Mail(c.cfg.fromEmail); err != nil {
		return fmt.Errorf("SMTP MAIL: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("SMTP RCPT: %w", err)
	}
	return nil
}

// sendData sends the message body.
func (c *smtpClient) sendData(client *smtp.Client, to string, msg []byte) error {
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("SMTP write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("SMTP close: %w", err)
	}
	return client.Quit()
}

// saslXOAuth2 creates an XOAUTH2 authenticator for SMTP.
func saslXOAuth2(username, accessToken string) smtp.Auth {
	return sasl.NewXOAuth2(username, accessToken)
}

var _ = sasl.NewXOAuth2 // Ensure sasl package is used
