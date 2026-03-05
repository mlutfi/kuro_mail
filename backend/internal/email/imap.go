package email

import (
	"crypto/tls"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"go.uber.org/zap"

	"github.com/webmail/backend/internal/config"
	"github.com/webmail/backend/internal/models"
)

// ─── IMAP CONNECTION POOL ────────────────────────────────────────────────────

// IMAPPool manages a per-user pool of reusable IMAP connections.
type IMAPPool struct {
	cfg    *config.IMAPConfig
	pools  map[string]*userPool
	mu     sync.RWMutex
	logger *zap.Logger
}

type userPool struct {
	conns    chan *IMAPConn
	email    string
	password string
}

// IMAPConn wraps a single authenticated IMAP client.
type IMAPConn struct {
	client    *imapclient.Client
	userEmail string
	createdAt time.Time
	lastUsed  time.Time
}

func NewIMAPPool(cfg *config.IMAPConfig, logger *zap.Logger) *IMAPPool {
	return &IMAPPool{
		cfg:    cfg,
		pools:  make(map[string]*userPool),
		logger: logger,
	}
}

// GetConn returns a healthy connection from the pool, or creates a new one.
func (p *IMAPPool) GetConn(userEmail, password string) (*IMAPConn, error) {
	p.mu.RLock()
	pool, exists := p.pools[userEmail]
	p.mu.RUnlock()

	if exists {
		select {
		case conn := <-pool.conns:
			if conn.isAlive() {
				conn.lastUsed = time.Now()
				return conn, nil
			}
			conn.Close() // connection is dead, fall through to create a new one
		default:
			// pool is empty, fall through to create a new one
		}
	}

	return p.createConn(userEmail, password)
}

// Authenticate verifies IMAP credentials without keeping the connection open.
func (p *IMAPPool) Authenticate(userEmail, password string) error {
	conn, err := p.GetConn(userEmail, password)
	if err != nil {
		return err
	}
	p.ReturnConn(conn)
	return nil
}

// ReturnConn puts a connection back into the pool, closing it if the pool is
// full or the connection is too old.
func (p *IMAPPool) ReturnConn(conn *IMAPConn) {
	if conn == nil {
		return
	}

	p.mu.RLock()
	pool, exists := p.pools[conn.userEmail]
	p.mu.RUnlock()

	if !exists || time.Since(conn.createdAt) > 30*time.Minute {
		conn.Close()
		return
	}

	select {
	case pool.conns <- conn:
		// returned successfully
	default:
		conn.Close() // pool is full
	}
}

func (p *IMAPPool) createConn(userEmail, password string) (*IMAPConn, error) {
	addr := fmt.Sprintf("%s:%d", p.cfg.Host, p.cfg.Port)

	var (
		c   *imapclient.Client
		err error
	)

	if p.cfg.UseTLS {
		tlsCfg := &tls.Config{
			ServerName: p.cfg.Host,
			MinVersion: tls.VersionTLS12,
		}
		c, err = imapclient.DialTLS(addr, &imapclient.Options{TLSConfig: tlsCfg})
	} else {
		c, err = imapclient.DialStartTLS(addr, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("IMAP connect: %w", err)
	}

	if err := c.Login(userEmail, password).Wait(); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("IMAP login: %w", err)
	}

	conn := &IMAPConn{
		client:    c,
		userEmail: userEmail,
		createdAt: time.Now(),
		lastUsed:  time.Now(),
	}

	// Ensure a pool entry exists for this user.
	p.mu.Lock()
	if _, exists := p.pools[userEmail]; !exists {
		p.pools[userEmail] = &userPool{
			conns:    make(chan *IMAPConn, p.cfg.PoolSize),
			email:    userEmail,
			password: password,
		}
	}
	p.mu.Unlock()

	return conn, nil
}

// isAlive sends a NOOP to verify the connection is still healthy.
func (c *IMAPConn) isAlive() bool {
	return c.client.Noop().Wait() == nil
}

// Close logs out and closes the underlying IMAP connection.
func (c *IMAPConn) Close() {
	if c.client != nil {
		_ = c.client.Logout().Wait()
	}
}

// ─── IMAP OPERATIONS ─────────────────────────────────────────────────────────

// FetchFolderList lists all mailboxes with their message/unseen counts.
func (c *IMAPConn) FetchFolderList() ([]imap.ListData, error) {
	cmd := c.client.List("", "*", &imap.ListOptions{
		ReturnStatus: &imap.StatusOptions{
			NumMessages: true,
			NumUnseen:   true,
			UIDNext:     true,
		},
	})

	var folders []imap.ListData
	for {
		data := cmd.Next()
		if data == nil {
			break
		}
		folders = append(folders, *data)
	}
	return folders, cmd.Close()
}

// FetchMessageList retrieves lightweight message headers for a paginated inbox view.
// Returns the messages (newest first) and the total message count.
func (c *IMAPConn) FetchMessageList(folder string, page, perPage int) ([]*models.EmailMessage, int, error) {
	selectData, err := c.client.Select(folder, &imap.SelectOptions{ReadOnly: true}).Wait()
	if err != nil {
		return nil, 0, fmt.Errorf("select folder %q: %w", folder, err)
	}

	fmt.Printf("[DEBUG] FetchMessageList: folder=%s, total messages=%d\n", folder, selectData.NumMessages)

	total := int(selectData.NumMessages)
	if total == 0 {
		return []*models.EmailMessage{}, 0, nil
	}

	// Calculate sequence range for this page (messages are 1-indexed, newest = highest seq).
	start := total - (page-1)*perPage
	end := start - perPage + 1
	if end < 1 {
		end = 1
	}
	if start < 1 {
		return []*models.EmailMessage{}, total, nil
	}

	var seqSet imap.SeqSet
	seqSet.AddRange(uint32(end), uint32(start))

	fmt.Printf("[DEBUG] FetchMessageList: fetching seq range %d:%d\n", end, start)

	fetchCmd := c.client.Fetch(seqSet, &imap.FetchOptions{
		Flags:         true,
		Envelope:      true,
		BodyStructure: &imap.FetchItemBodyStructure{Extended: false},
		RFC822Size:    true,
		InternalDate:  true,
		UID:           true, // Always request UID
		BodySection: []*imap.FetchItemBodySection{
			{Specifier: imap.PartSpecifierHeader}, // headers only — fast
		},
	})

	var messages []*models.EmailMessage
	for {
		msg := fetchCmd.Next()
		if msg == nil {
			break
		}
		if email := parseEnvelopeMessage(msg, folder); email != nil {
			messages = append(messages, email)
		}
	}

	if err := fetchCmd.Close(); err != nil {
		return nil, 0, err
	}

	reverseMessages(messages)
	return messages, total, nil
}

// FetchMessageBody fetches the complete RFC822 body of a single message and
// parses it with the full MIME parser (handles multipart, attachments, encoding).
func (c *IMAPConn) FetchMessageBody(folder string, uid uint32) (*models.EmailMessage, error) {
	// Use a writable SELECT so mark-as-read works on this connection.
	if _, err := c.client.Select(folder, nil).Wait(); err != nil {
		return nil, fmt.Errorf("select folder %q: %w", folder, err)
	}

	uidSet := imap.UIDSetNum(imap.UID(uid))
	fetchCmd := c.client.Fetch(imap.UIDSet(uidSet), &imap.FetchOptions{
		Flags:        true,
		Envelope:     true,
		InternalDate: true,
		RFC822Size:   true,
		BodySection: []*imap.FetchItemBodySection{
			// Fetch the full raw RFC822 message for proper MIME parsing.
			{Specifier: imap.PartSpecifierNone},
		},
	})

	msg := fetchCmd.Next()
	if msg == nil {
		return nil, fmt.Errorf("message uid=%d not found in %q", uid, folder)
	}

	email, rawBody := parseEnvelopeWithRaw(msg, folder)
	_ = fetchCmd.Close()

	if email == nil {
		return nil, fmt.Errorf("failed to parse message uid=%d", uid)
	}

	// Use the full MIME parser on the raw RFC822 bytes.
	if len(rawBody) > 0 {
		_ = parseMIMEMessage(rawBody, email) // non-fatal: partial parse is OK
	}

	return email, nil
}

// MarkRead sets or clears the \Seen flag on the given UIDs.
func (c *IMAPConn) MarkRead(folder string, uids []uint32, read bool) error {
	if _, err := c.client.Select(folder, nil).Wait(); err != nil {
		return fmt.Errorf("select folder %q: %w", folder, err)
	}

	fmt.Printf("[DEBUG] MarkRead: folder=%s, uids=%v, read=%v\n", folder, uids, read)

	op := imap.StoreFlagsAdd
	if !read {
		op = imap.StoreFlagsDel
		fmt.Printf("[DEBUG] MarkRead: Operation = REMOVE \\Seen flag (mark as UNREAD)\n")
	} else {
		fmt.Printf("[DEBUG] MarkRead: Operation = ADD \\Seen flag (mark as READ)\n")
	}

	uidSet := imap.UIDSet(uidsToSet(uids))
	fmt.Printf("[DEBUG] MarkRead: UID set = %v\n", uidSet)

	err := c.client.Store(uidSet, &imap.StoreFlags{
		Op:     op,
		Silent: true,
		Flags:  []imap.Flag{imap.FlagSeen},
	}, nil).Close()

	if err != nil {
		fmt.Printf("[DEBUG] MarkRead ERROR: %v\n", err)
		return err
	}

	fmt.Printf("[DEBUG] MarkRead: SUCCESS\n")
	return nil
}

// MarkStarred sets or clears the \Flagged flag on the given UIDs.
func (c *IMAPConn) MarkStarred(folder string, uids []uint32, starred bool) error {
	if _, err := c.client.Select(folder, nil).Wait(); err != nil {
		return fmt.Errorf("select folder %q: %w", folder, err)
	}

	op := imap.StoreFlagsAdd
	if !starred {
		op = imap.StoreFlagsDel
	}

	return c.client.Store(imap.UIDSet(uidsToSet(uids)), &imap.StoreFlags{
		Op:     op,
		Silent: true,
		Flags:  []imap.Flag{imap.FlagFlagged},
	}, nil).Close()
}

// MoveMessage moves messages to another folder using the IMAP MOVE extension.
// If the destination folder doesn't exist, it will be created.
func (c *IMAPConn) MoveMessage(srcFolder, dstFolder string, uids []uint32) error {
	// Try to create the destination folder - ignore error since it might already exist
	c.client.Create(dstFolder, nil)

	if _, err := c.client.Select(srcFolder, nil).Wait(); err != nil {
		return fmt.Errorf("select folder %q: %w", srcFolder, err)
	}
	_, err := c.client.Move(imap.UIDSet(uidsToSet(uids)), dstFolder).Wait()
	return err
}

// FetchAttachment retrieves the binary content of an attachment
func (c *IMAPConn) FetchAttachment(folder string, uid uint32, attachmentID string) ([]byte, string, string, error) {
	fmt.Printf("[DEBUG] FetchAttachment: folder=%q, uid=%d, attachmentID=%q\n", folder, uid, attachmentID)

	if _, err := c.client.Select(folder, nil).Wait(); err != nil {
		return nil, "", "", fmt.Errorf("select folder %q: %w", folder, err)
	}

	uidSet := imap.UIDSetNum(imap.UID(uid))
	fetchCmd := c.client.Fetch(imap.UIDSet(uidSet), &imap.FetchOptions{
		Flags:        true,
		Envelope:     true,
		InternalDate: true,
		RFC822Size:   true,
		BodySection: []*imap.FetchItemBodySection{
			{Specifier: imap.PartSpecifierNone},
		},
	})

	msg := fetchCmd.Next()
	if msg == nil {
		return nil, "", "", fmt.Errorf("message uid=%d not found in %q", uid, folder)
	}

	// Parse the message to find the attachment and populate attachmentData
	emailMsg, rawBody := parseEnvelopeWithRaw(msg, folder)
	_ = fetchCmd.Close()
	if emailMsg == nil {
		return nil, "", "", fmt.Errorf("failed to parse message")
	}

	fmt.Printf("[DEBUG] FetchAttachment: emailMsg.IMAPUID=%d (requested uid=%d), rawBody len=%d\n", emailMsg.IMAPUID, uid, len(rawBody))

	// Ensure the emailMsg has the correct UID set (from the request) before parsing MIME
	// This is critical because parseMIMEMessage uses email.IMAPUID to generate attachment IDs
	if emailMsg.IMAPUID == 0 {
		fmt.Printf("[DEBUG] FetchAttachment: IMAPUID was 0, setting to requested uid=%d\n", uid)
		emailMsg.IMAPUID = uid
		emailMsg.ID = fmt.Sprintf("%d", uid)
	}

	if len(rawBody) > 0 {
		_ = parseMIMEMessage(rawBody, emailMsg)
	}

	fmt.Printf("[DEBUG] FetchAttachment: After MIME parse - Attachments count=%d\n", len(emailMsg.Attachments))
	for i, att := range emailMsg.Attachments {
		fmt.Printf("[DEBUG] FetchAttachment: Attachment[%d]: ID=%q, Filename=%q, MimeType=%q\n", i, att.ID, att.Filename, att.MimeType)
	}

	fmt.Printf("[DEBUG] FetchAttachment: Looking for attachmentID=%q\n", attachmentID)

	// Find the attachment by ID and return the data directly from attachmentData map
	fmt.Printf("[DEBUG] FetchAttachment: Looking for attachmentID=%q in map (keys: %v)\n", attachmentID, GetAttachmentDataKeys())

	for _, att := range emailMsg.Attachments {
		fmt.Printf("[DEBUG] FetchAttachment: Comparing requestID=%q with foundID=%q\n", attachmentID, att.ID)
		if att.ID == attachmentID {
			// Get the stored binary content from the map
			data, ok := attachmentData[attachmentID]
			fmt.Printf("[DEBUG] FetchAttachment: Found attachment ID=%q in attachments list, data found=%v, data len=%d\n", attachmentID, ok, len(data))
			if !ok || len(data) == 0 {
				// Try to find any matching key in the map
				fmt.Printf("[DEBUG] FetchAttachment: Data not found for %q, checking all keys...\n", attachmentID)
				for k, v := range attachmentData {
					fmt.Printf("[DEBUG] FetchAttachment: Map key=%q, len=%d\n", k, len(v))
				}
				return nil, att.Filename, att.MimeType, fmt.Errorf("attachment content not found: %s", attachmentID)
			}
			return data, att.Filename, att.MimeType, nil
		}
	}

	// Attachment ID not found - check if it's a UID mismatch issue
	// The attachmentID in the request might have a different UID than what's in the message
	fmt.Printf("[DEBUG] FetchAttachment: Attachment not found by ID. Checking if UID mismatch...\n")
	fmt.Printf("[DEBUG] FetchAttachment: Current message UID=%d, looking for attachments with UID in ID\n", uid)

	return nil, "", "", fmt.Errorf("attachment not found: %s", attachmentID)
}

// DeleteMessage permanently deletes messages or moves them to Trash.
func (c *IMAPConn) DeleteMessage(folder string, uids []uint32, permanent bool) error {
	if permanent {
		if _, err := c.client.Select(folder, nil).Wait(); err != nil {
			return fmt.Errorf("select folder %q: %w", folder, err)
		}
		_ = c.client.Store(imap.UIDSet(uidsToSet(uids)), &imap.StoreFlags{
			Op:     imap.StoreFlagsAdd,
			Silent: true,
			Flags:  []imap.Flag{imap.FlagDeleted},
		}, nil).Close()
		_, err := c.client.Expunge().Collect()
		return err
	}
	return c.MoveMessage(folder, "Trash", uids)
}

// SearchMessages runs an IMAP UID SEARCH with the given criteria.
func (c *IMAPConn) SearchMessages(folder string, criteria *imap.SearchCriteria) ([]uint32, error) {
	if _, err := c.client.Select(folder, &imap.SelectOptions{ReadOnly: true}).Wait(); err != nil {
		return nil, fmt.Errorf("select folder %q: %w", folder, err)
	}

	data, err := c.client.UIDSearch(criteria, nil).Wait()
	if err != nil {
		return nil, err
	}

	var uids []uint32
	for _, uid := range data.AllUIDs() {
		uids = append(uids, uint32(uid))
	}
	return uids, nil
}

// GetFolderStats returns total and unread message counts for a mailbox.
func (c *IMAPConn) GetFolderStats(folder string) (*models.FolderStats, error) {
	data, err := c.client.Status(folder, &imap.StatusOptions{
		NumMessages: true,
		NumUnseen:   true,
	}).Wait()
	if err != nil {
		return nil, err
	}

	stats := &models.FolderStats{Folder: folder}
	if data.NumMessages != nil {
		stats.TotalCount = int(*data.NumMessages)
	}
	if data.NumUnseen != nil {
		stats.UnreadCount = int(*data.NumUnseen)
	}
	return stats, nil
}

// AppendMessage appends a new RFC822 message to the specified folder.
func (c *IMAPConn) AppendMessage(folder string, flags []imap.Flag, date time.Time, msg []byte) error {
	opts := &imap.AppendOptions{
		Flags: flags,
	}
	if !date.IsZero() {
		opts.Time = date
	}

	appendCmd := c.client.Append(folder, int64(len(msg)), opts)
	if _, err := appendCmd.Write(msg); err != nil {
		_ = appendCmd.Close()
		return fmt.Errorf("append write: %w", err)
	}
	if err := appendCmd.Close(); err != nil {
		return fmt.Errorf("append close: %w", err)
	}
	if _, err := appendCmd.Wait(); err != nil {
		return fmt.Errorf("append wait: %w", err)
	}
	return nil
}

// ─── IMAP PARSE HELPERS ──────────────────────────────────────────────────────

// parseEnvelopeMessage parses flags, envelope, date and header section from a
// fetch response. Used for the list view (no full body needed).
func parseEnvelopeMessage(msg *imapclient.FetchMessageData, folder string) *models.EmailMessage {
	if msg == nil {
		return nil
	}

	email := &models.EmailMessage{
		IMAPFolder: folder,
		ReceivedAt: time.Now(),
	}

	for {
		item := msg.Next()
		if item == nil {
			break
		}

		switch v := item.(type) {
		case imapclient.FetchItemDataUID:
			email.IMAPUID = uint32(v.UID)
			email.ID = fmt.Sprintf("%d", email.IMAPUID)
			fmt.Printf("[DEBUG] parseEnvelopeMessage: UID = %d\n", v.UID)

		case imapclient.FetchItemDataFlags:
			for _, flag := range v.Flags {
				switch flag {
				case imap.FlagSeen:
					email.IsRead = true
				case imap.FlagFlagged:
					email.IsStarred = true
				case imap.FlagDraft:
					email.IsDraft = true
				}
			}

		case imapclient.FetchItemDataEnvelope:
			fillFromEnvelope(email, v.Envelope)

		case imapclient.FetchItemDataInternalDate:
			email.ReceivedAt = v.Time

		case imapclient.FetchItemDataBodySection:
			// Header-only section: extract In-Reply-To and References for threading.
			headerBytes, err := io.ReadAll(v.Literal)
			if err == nil {
				headerStr := string(headerBytes)
				email.InReplyTo = extractHeader(headerStr, "In-Reply-To")
				if refs := extractHeader(headerStr, "References"); refs != "" {
					email.References = parseReferencesHeader(refs)
				}
			}
		}
	}

	if email.ID == "" {
		email.ID = fmt.Sprintf("%d", email.IMAPUID)
	}
	// Debug: print final UID
	fmt.Printf("[DEBUG] parseEnvelopeMessage: final IMAPUID = %d, ID = %s\n", email.IMAPUID, email.ID)
	return email
}

// parseEnvelopeWithRaw parses envelope/flags and also returns the raw RFC822
// bytes from the body section for the MIME parser.
func parseEnvelopeWithRaw(msg *imapclient.FetchMessageData, folder string) (*models.EmailMessage, []byte) {
	if msg == nil {
		return nil, nil
	}

	email := &models.EmailMessage{
		IMAPFolder: folder,
		ReceivedAt: time.Now(),
	}

	var rawBody []byte

	for {
		item := msg.Next()
		if item == nil {
			break
		}

		switch v := item.(type) {
		case imapclient.FetchItemDataUID:
			email.IMAPUID = uint32(v.UID)
			email.ID = fmt.Sprintf("%d", email.IMAPUID)

		case imapclient.FetchItemDataFlags:
			for _, flag := range v.Flags {
				switch flag {
				case imap.FlagSeen:
					email.IsRead = true
				case imap.FlagFlagged:
					email.IsStarred = true
				case imap.FlagDraft:
					email.IsDraft = true
				}
			}

		case imapclient.FetchItemDataEnvelope:
			fillFromEnvelope(email, v.Envelope)

		case imapclient.FetchItemDataInternalDate:
			email.ReceivedAt = v.Time

		case imapclient.FetchItemDataBodySection:
			// Full RFC822 body — read all bytes for the MIME parser.
			rawBody, _ = io.ReadAll(v.Literal)
		}
	}

	if email.ID == "" {
		email.ID = fmt.Sprintf("%d", email.IMAPUID)
	}
	return email, rawBody
}

// fillFromEnvelope populates address/subject/date fields from an IMAP envelope.
func fillFromEnvelope(email *models.EmailMessage, env *imap.Envelope) {
	if env == nil {
		return
	}

	email.Subject = env.Subject
	email.MessageID = env.MessageID

	if len(env.From) > 0 {
		a := env.From[0]
		email.From = models.EmailAddress{
			Name:  a.Name,
			Email: a.Mailbox + "@" + a.Host,
		}
	}
	for _, a := range env.To {
		email.To = append(email.To, models.EmailAddress{
			Name:  a.Name,
			Email: a.Mailbox + "@" + a.Host,
		})
	}
	for _, a := range env.Cc {
		email.Cc = append(email.Cc, models.EmailAddress{
			Name:  a.Name,
			Email: a.Mailbox + "@" + a.Host,
		})
	}
	if !env.Date.IsZero() {
		email.SentAt = &env.Date
		email.ReceivedAt = env.Date
	}
}

// ─── SMALL UTILITIES ─────────────────────────────────────────────────────────

func reverseMessages(msgs []*models.EmailMessage) {
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
}

func uidsToSet(uids []uint32) imap.UIDSet {
	var set imap.UIDSet
	for _, uid := range uids {
		set.AddNum(imap.UID(uid))
	}
	return set
}

// extractHeader returns the value of the first occurrence of a named header
// in a raw header block, handling RFC 2822 header folding.
func extractHeader(headers, name string) string {
	nameLower := strings.ToLower(name) + ":"
	lines := strings.Split(headers, "\r\n")
	var result strings.Builder
	capturing := false

	for _, line := range lines {
		if strings.HasPrefix(strings.ToLower(line), nameLower) {
			capturing = true
			result.WriteString(strings.TrimSpace(line[len(nameLower):]))
		} else if capturing && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) {
			result.WriteString(" ")
			result.WriteString(strings.TrimSpace(line))
		} else if capturing {
			break
		}
	}
	return result.String()
}

// parseReferencesHeader splits a References header into individual Message-IDs.
func parseReferencesHeader(refsHeader string) []string {
	var refs []string
	for _, part := range strings.FieldsFunc(refsHeader, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\r' || r == '\n'
	}) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !strings.HasPrefix(part, "<") {
			part = "<" + part + ">"
		}
		refs = append(refs, part)
	}
	return refs
}
