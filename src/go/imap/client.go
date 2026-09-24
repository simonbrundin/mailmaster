package imap

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	imapclient "github.com/emersion/go-imap/client"

	"mailagent/config"
	"mailagent/rules"
	"mailagent/sasl"
)

// Client wraps the IMAP connection
type Client struct {
	cfg  *config.MailAccount
	conn *imapclient.Client
}

// NewClient creates a new IMAP client
func NewClient(cfg *config.MailAccount) (*Client, error) {
	return &Client{
		cfg: cfg,
	}, nil
}

// GetAccount returns the mail account configuration
func (c *Client) GetAccount() *config.MailAccount {
	return c.cfg
}

// Connect establishes the IMAP connection
func (c *Client) Connect(password string) error {
	var err error
	addr := fmt.Sprintf("%s:%d", c.cfg.Server, c.cfg.Port)

	// Connect with TLS
	c.conn, err = imapclient.DialTLS(addr, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to %s with TLS: %w", addr, err)
	}

	if c.cfg.UseOAuth2 {
		if err := c.authenticateXOAuth2(password); err != nil {
			c.conn.Close()
			return fmt.Errorf("OAuth2 authentication failed: %w", err)
		}
		return nil
	}

	// one.com/Dovecot uses regular LOGIN authentication.
	if err := c.conn.Login(c.cfg.Username, password); err != nil {
		c.conn.Close()
		return fmt.Errorf("login failed: %w", err)
	}
	return nil
}

// authenticateXOAuth2 authenticates using the raw XOAUTH2 SASL mechanism.
// The access token belongs to Username; SharedMailbox is the mailbox being
// accessed and is therefore used as the XOAUTH2 user identity.
func (c *Client) authenticateXOAuth2(accessToken string) error {
	authUser := c.cfg.Username
	if c.cfg.SharedMailbox != "" {
		authUser = c.cfg.SharedMailbox
	}
	return c.conn.Authenticate(sasl.NewClient(authUser, accessToken))
}

// Close closes the IMAP connection
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// ListFolders returns all available folders
func (c *Client) ListFolders() ([]string, error) {
	mailboxes := make(chan *imap.MailboxInfo)
	done := make(chan error, 1)

	go func() {
		done <- c.conn.List("", "*", mailboxes)
	}()

	var folders []string
	for m := range mailboxes {
		folders = append(folders, m.Name)
	}

	if err := <-done; err != nil {
		return nil, err
	}

	return folders, nil
}

// CountEmails returns the total number of emails and unread count in a folder
func (c *Client) CountEmails(folder string) (total int64, unread int64, err error) {
	actualFolder := c.getFolderName(folder)

	// Select the folder to get status
	_, err = c.conn.Select(actualFolder, false)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to select folder: %w", err)
	}

	mbox := c.conn.Mailbox()
	return int64(mbox.Messages), int64(mbox.Unseen), nil
}

// getFolderName returns the IMAP folder name. A shared mailbox is selected
// through the XOAUTH2 identity, not by prefixing the folder name.
// On one.com, all folders live under INBOX.* namespace.
func (c *Client) getFolderName(folder string) string {
	// one.com uses INBOX.* namespace - if folder doesn't start with INBOX,
	// assume it's a top-level folder and prefix it
	if !strings.HasPrefix(folder, "INBOX") && c.cfg.Server == "imap.one.com" {
		return "INBOX." + folder
	}
	return folder
}

// ListEmails retrieves emails from a folder
func (c *Client) ListEmails(folder string, limit uint32) ([]*rules.EmailContext, error) {
	// Get the actual folder name (handles shared mailboxes)
	actualFolder := c.getFolderName(folder)

	// Select the folder
	_, err := c.conn.Select(actualFolder, false)
	if err != nil {
		return nil, fmt.Errorf("failed to select folder %s: %w", actualFolder, err)
	}

	mbox := c.conn.Mailbox()
	if mbox.Messages == 0 {
		return []*rules.EmailContext{}, nil
	}

	// Get sequence numbers for emails
	from := uint32(1)
	to := mbox.Messages
	if limit > 0 && to > limit {
		from = to - limit + 1
	}

	seqset := new(imap.SeqSet)
	seqset.AddRange(from, to)

	// Fetch email headers + UID
	items := []imap.FetchItem{
		imap.FetchEnvelope,
		imap.FetchBodyStructure,
		imap.FetchFlags,
		imap.FetchUid,
	}

	messages := make(chan *imap.Message)
	done := make(chan error, 1)

	go func() {
		done <- c.conn.Fetch(seqset, items, messages)
	}()

	var emails []*rules.EmailContext

	for msg := range messages {
		email := c.messageToEmailContext(msg)
		emails = append(emails, email)
	}

	if err := <-done; err != nil {
		return nil, fmt.Errorf("failed to fetch messages: %w", err)
	}

	return emails, nil
}

// ReadEmail retrieves full email content
func (c *Client) ReadEmail(folder string, uid uint32) (*rules.EmailContext, error) {
	// Get the actual folder name (handles shared mailboxes)
	actualFolder := c.getFolderName(folder)

	// Select folder
	_, err := c.conn.Select(actualFolder, false)
	if err != nil {
		return nil, fmt.Errorf("failed to select folder: %w", err)
	}

	// Search for the UID using UidSearch
	uidSet := new(imap.SeqSet)
	uidSet.AddNum(uid)
	seqNums, err := c.conn.UidSearch(&imap.SearchCriteria{
		Uid: uidSet,
	})
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	if len(seqNums) == 0 {
		return nil, fmt.Errorf("email with UID %d not found", uid)
	}

	// Fetch the message using UID
	seqSet := new(imap.SeqSet)
	seqSet.AddNum(seqNums[0])

	items := []imap.FetchItem{
		imap.FetchEnvelope,
		imap.FetchBodyStructure,
		imap.FetchFlags,
		imap.FetchBody,
	}

	messages := make(chan *imap.Message)
	done := make(chan error, 1)

	go func() {
		done <- c.conn.UidFetch(seqSet, items, messages)
	}()

	msg := <-messages
	if err := <-done; err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}

	email := c.messageToEmailContext(msg)
	email.UID = uid
	email.Folder = folder

	return email, nil
}

// ArchiveEmail moves email to Archive folder
func (c *Client) ArchiveEmail(folder string, uid uint32) error {
	// Use INBOX.Archive for one.com (IMAP namespace), plain Archive for others
	archiveFolder := "Archive"
	if c.cfg.Server == "imap.one.com" {
		archiveFolder = "INBOX.Archive"
	}
	return c.MoveEmail(folder, archiveFolder, uid)
}

// MoveEmail moves an email to another folder. Creates destination if needed.
func (c *Client) MoveEmail(fromFolder, toFolder string, uid uint32) error {
	actualFromFolder := c.getFolderName(fromFolder)
	actualToFolder := c.getFolderName(toFolder)

	// Try to create destination folder if it doesn't exist
	if err := c.ensureFolderExists(actualToFolder); err != nil {
		// Log but continue
	}

	// Select source folder
	_, err := c.conn.Select(actualFromFolder, false)
	if err != nil {
		return fmt.Errorf("select source folder: %w", err)
	}

	// Build UID seqset and try UidMove directly
	uidSet := new(imap.SeqSet)
	uidSet.AddNum(uid)

	if err := c.conn.UidMove(uidSet, actualToFolder); err != nil {
		// Try UidCopy + Delete
		if copyErr := c.conn.UidCopy(uidSet, actualToFolder); copyErr != nil {
			return fmt.Errorf("move/copy email UID %d: %w (copy err: %v)", uid, err, copyErr)
		}
		// Mark deleted and expunge
		if storeErr := c.conn.UidStore(uidSet, imap.AddFlags, []interface{}{imap.DeletedFlag}, nil); storeErr != nil {
			return fmt.Errorf("mark deleted: %w", storeErr)
		}
		done := make(chan uint32)
		if expungeErr := c.conn.Expunge(done); expungeErr != nil {
			return fmt.Errorf("expunge: %w", expungeErr)
		}
	}

	return nil
}

// ensureFolderExists creates a folder if it doesn't already exist.
// Silently succeeds if the folder already exists.
func (c *Client) ensureFolderExists(folder string) error {
	// Just try to create it - IMAP servers return success or "already exists" error
	if err := c.conn.Create(folder); err != nil {
		// "ALREADYEXISTS" or similar means the folder is already there - that's fine
		return nil
	}
	return nil
}

// MarkAsRead marks an email as read
func (c *Client) MarkAsRead(folder string, uid uint32) error {
	actualFolder := c.getFolderName(folder)
	_, err := c.conn.Select(actualFolder, false)
	if err != nil {
		return err
	}

	uidSet := new(imap.SeqSet)
	uidSet.AddNum(uid)
	seqNums, err := c.conn.UidSearch(&imap.SearchCriteria{
		Uid: uidSet,
	})
	if err != nil {
		return err
	}

	seqSet := new(imap.SeqSet)
	seqSet.AddNum(seqNums[0])

	return c.conn.Store(seqSet, imap.AddFlags, []interface{}{imap.SeenFlag}, nil)
}

// MarkAsUnread marks an email as unread
func (c *Client) MarkAsUnread(folder string, uid uint32) error {
	actualFolder := c.getFolderName(folder)
	_, err := c.conn.Select(actualFolder, false)
	if err != nil {
		return err
	}

	uidSet := new(imap.SeqSet)
	uidSet.AddNum(uid)
	seqNums, err := c.conn.UidSearch(&imap.SearchCriteria{
		Uid: uidSet,
	})
	if err != nil {
		return err
	}

	seqSet := new(imap.SeqSet)
	seqSet.AddNum(seqNums[0])

	return c.conn.Store(seqSet, imap.RemoveFlags, []interface{}{imap.SeenFlag}, nil)
}

// WaitForNewEmails waits for new emails using IDLE
func (c *Client) WaitForNewEmails(ctx context.Context, folder string) error {
	actualFolder := c.getFolderName(folder)
	_, err := c.conn.Select(actualFolder, false)
	if err != nil {
		return err
	}

	// Create status channel for updates
	status := make(chan imapclient.Update, 10)
	c.conn.Updates = status

	// Start IDLE in a goroutine
	stop := make(chan struct{})
	idleErr := make(chan error, 1)
	go func() {
		idleErr <- c.conn.Idle(stop, nil)
	}()

	for {
		select {
		case <-ctx.Done():
			close(stop)
			return ctx.Err()
		case update := <-status:
			if _, ok := update.(*imapclient.MailboxUpdate); ok {
				// New emails available
				close(stop)
				return nil
			}
		case err := <-idleErr:
			if err != nil {
				return err
			}
			// Restart idle
			stop = make(chan struct{})
			go func() {
				idleErr <- c.conn.Idle(stop, nil)
			}()
		case <-time.After(2 * time.Minute):
			// Send NOOP to keep connection alive
			c.conn.Noop()
		}
	}
}

// Watch continuously monitors a folder for changes
func (c *Client) Watch(ctx context.Context, folder string, callback func([]*rules.EmailContext)) error {
	// Initial fetch
	emails, err := c.ListEmails(folder, 50)
	if err != nil {
		return err
	}
	callback(emails)

	// Create status channel for updates
	status := make(chan imapclient.Update, 10)
	c.conn.Updates = status

	// Start IDLE
	stop := make(chan struct{})
	idleErr := make(chan error, 1)
	go func() {
		idleErr <- c.conn.Idle(stop, nil)
	}()

	for {
		select {
		case <-ctx.Done():
			close(stop)
			return nil
		case update := <-status:
			if _, ok := update.(*imapclient.MailboxUpdate); ok {
				// New emails available - check for new emails
				emails, err := c.ListEmails(folder, 10)
				if err != nil {
					continue
				}
				if len(emails) > 0 {
					callback(emails)
				}
			}
		case err := <-idleErr:
			if err != nil {
				return err
			}
			// Restart idle
			stop = make(chan struct{})
			go func() {
				idleErr <- c.conn.Idle(stop, nil)
			}()
		case <-time.After(2 * time.Minute):
			// NOOP to keep alive
			c.conn.Noop()
		}
	}
}

func (c *Client) messageToEmailContext(msg *imap.Message) *rules.EmailContext {
	email := &rules.EmailContext{
		UID: uint32(msg.Uid),
	}

	if msg.Envelope != nil {
		email.Date = msg.Envelope.Date
		email.Subject = decodeHeader(msg.Envelope.Subject)
		email.From = extractAddress(msg.Envelope.From)
		email.To = extractAddress(msg.Envelope.To)
	}

	if msg.BodyStructure != nil {
		email.HasAttachment = hasAttachments(msg.BodyStructure)
	}

	// Extract body text
	email.Body = extractBodyText(msg)

	return email
}

// extractBodyText extracts the plain text body from an IMAP message
func extractBodyText(msg *imap.Message) string {
	// Try to get the text part using BodySectionName with TEXT specifier
	textSpec := &imap.BodySectionName{
		BodyPartName: imap.BodyPartName{
			Specifier: imap.TextSpecifier,
		},
	}

	if literal, ok := msg.Body[textSpec]; ok {
		buf := make([]byte, literal.Len())
		literal.Read(buf)
		return string(buf)
	}

	// Try to find any text body section by iterating
	for spec, literal := range msg.Body {
		if spec != nil && spec.Specifier == imap.TextSpecifier {
			buf := make([]byte, literal.Len())
			literal.Read(buf)
			return string(buf)
		}
	}

	return ""
}

func hasAttachments(bs *imap.BodyStructure) bool {
	if bs == nil {
		return false
	}

	disposition := strings.ToLower(bs.Disposition)
	if disposition == "attachment" {
		return true
	}

	if bs.MIMEType == "MULTIPART" && bs.Parts != nil {
		for _, part := range bs.Parts {
			if hasAttachments(part) {
				return true
			}
		}
	}

	return false
}

func extractAddress(addrs []*imap.Address) string {
	if len(addrs) == 0 {
		return ""
	}
	addr := addrs[0]
	if addr.HostName != "" {
		return addr.PersonalName + " <" + addr.MailboxName + "@" + addr.HostName + ">"
	}
	return addr.MailboxName
}

func decodeHeader(s string) string {
	// Simple decoder - for production use go-message package
	return s
}
