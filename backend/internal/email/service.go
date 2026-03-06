package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"math"
	"net/smtp"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"go.uber.org/zap"

	"github.com/webmail/backend/internal/auth"
	"github.com/webmail/backend/internal/cache"
	"github.com/webmail/backend/internal/config"
	"github.com/webmail/backend/internal/models"
)

// emailDateLayout is the RFC 2822 date format used in outgoing email headers.
const emailDateLayout = "Mon, 02 Jan 2006 15:04:05 -0700"

// Service handles all email business logic.
type Service struct {
	imapPool     *IMAPPool
	threadEngine *ThreadEngine
	cache        *cache.Cache
	cfg          *config.Config
	logger       *zap.Logger
}

func NewService(pool *IMAPPool, c *cache.Cache, cfg *config.Config, logger *zap.Logger) *Service {
	return &Service{
		imapPool:     pool,
		threadEngine: NewThreadEngine(),
		cache:        c,
		cfg:          cfg,
		logger:       logger,
	}
}

// ─── FOLDER ──────────────────────────────────────────────────────────────────

func (s *Service) GetFolderStats(ctx context.Context, user *models.User, folders []string) ([]*models.FolderStats, error) {
	var cached []*models.FolderStats
	if err := s.cache.GetFolderStats(ctx, user.ID.String(), &cached); err == nil {
		return cached, nil
	}

	conn, err := s.getConn(ctx, user)
	if err != nil {
		return nil, err
	}
	defer s.imapPool.ReturnConn(conn)

	var stats []*models.FolderStats
	for _, folder := range folders {
		stat, err := conn.GetFolderStats(folder)
		if err != nil {
			s.logger.Warn("Failed to get folder stats", zap.String("folder", folder), zap.Error(err))
			stat = &models.FolderStats{Folder: folder}
		}
		stats = append(stats, stat)
	}

	_ = s.cache.SetFolderStats(ctx, user.ID.String(), stats)
	return stats, nil
}

// ─── THREAD LIST ─────────────────────────────────────────────────────────────

func (s *Service) ListThreads(ctx context.Context, user *models.User, folder string, page, perPage int) (*models.PaginatedThreads, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 50
	}

	cacheKey := fmt.Sprintf("%s:%d:%d", folder, page, perPage)

	var cached models.PaginatedThreads
	if err := s.cache.GetInboxCache(ctx, user.ID.String(), cacheKey, 0, &cached); err == nil {
		return &cached, nil
	}

	conn, err := s.getConn(ctx, user)
	if err != nil {
		return nil, err
	}
	defer s.imapPool.ReturnConn(conn)

	// Over-fetch to allow for threading (N messages ≈ N/3 threads).
	fetchCount := page * perPage * 3
	if fetchCount > 1500 {
		fetchCount = 1500
	}

	var messages []*models.EmailMessage
	var totalMessages int

	if strings.EqualFold(folder, "Starred") {
		// Search for flagged messages in INBOX
		uids, err := conn.SearchMessages("INBOX", &imap.SearchCriteria{
			Flag: []imap.Flag{imap.FlagFlagged},
		})
		if err != nil {
			return nil, fmt.Errorf("search starred: %w", err)
		}

		totalMessages = len(uids)

		// Reverse to get newest first
		for i, j := 0, len(uids)-1; i < j; i, j = i+1, j-1 {
			uids[i], uids[j] = uids[j], uids[i]
		}

		start := (page - 1) * fetchCount
		end := start + fetchCount
		if start >= len(uids) {
			uids = []uint32{}
		} else {
			if end > len(uids) {
				end = len(uids)
			}
			uids = uids[start:end]
		}

		messages, err = conn.FetchMessageListByUIDs("INBOX", uids)
		if err != nil {
			return nil, fmt.Errorf("fetch starred messages: %w", err)
		}
	} else {
		var err error
		messages, totalMessages, err = conn.FetchMessageList(folder, 1, fetchCount)
		if err != nil {
			return nil, fmt.Errorf("fetch messages: %w", err)
		}
	}

	threads := s.threadEngine.BuildThreads(messages)
	s.threadEngine.SortThreadsForFolder(threads, folder)

	var items []*models.ThreadListItem
	for _, t := range threads {
		items = append(items, s.threadEngine.ToListItem(t))
	}

	// Paginate the thread list.
	startIdx := (page - 1) * perPage
	endIdx := startIdx + perPage
	if startIdx >= len(items) {
		items = []*models.ThreadListItem{}
	} else {
		if endIdx > len(items) {
			endIdx = len(items)
		}
		items = items[startIdx:endIdx]
	}

	totalPages := int(math.Ceil(float64(totalMessages) / float64(perPage)))
	result := &models.PaginatedThreads{
		Data:       items,
		Total:      int64(totalMessages),
		Page:       page,
		PerPage:    perPage,
		TotalPages: totalPages,
		HasNext:    page < totalPages,
	}

	_ = s.cache.SetInboxCache(ctx, user.ID.String(), cacheKey, 0, result)
	return result, nil
}

// ─── GET THREAD ───────────────────────────────────────────────────────────────

func (s *Service) GetThread(ctx context.Context, user *models.User, folder, threadID string) (*models.Thread, error) {
	var cached models.Thread
	if err := s.cache.GetThreadCache(ctx, user.ID.String(), threadID, &cached); err == nil {
		// Debug: check cached thread's UIDs
		for i, m := range cached.Messages {
			s.logger.Debug("cached message",
				zap.Int("index", i),
				zap.Uint32("imap_uid", m.IMAPUID),
				zap.String("id", m.ID))
		}
		return &cached, nil
	}

	conn, err := s.getConn(ctx, user)
	if err != nil {
		return nil, err
	}
	defer s.imapPool.ReturnConn(conn)

	messages, _, err := conn.FetchMessageList(folder, 1, 200)
	if err != nil {
		return nil, err
	}

	s.logger.Debug("Fetched messages for thread",
		zap.Int("count", len(messages)),
		zap.String("threadID", threadID))
	for i, m := range messages {
		s.logger.Debug("fetched message",
			zap.Int("index", i),
			zap.Uint32("imap_uid", m.IMAPUID),
			zap.String("id", m.ID),
			zap.String("subject", m.Subject))
	}

	threads := s.threadEngine.BuildThreads(messages)
	for _, t := range threads {
		if t.ThreadID == threadID {
			// Debug: check built thread's messages
			for i, m := range t.Messages {
				s.logger.Debug("thread message",
					zap.Int("index", i),
					zap.Uint32("imap_uid", m.IMAPUID),
					zap.String("id", m.ID))
			}
			_ = s.cache.SetThreadCache(ctx, user.ID.String(), threadID, t)
			return t, nil
		}
	}

	return nil, fmt.Errorf("thread %s not found", threadID)
}

// ─── GET MESSAGE ──────────────────────────────────────────────────────────────

func (s *Service) GetMessage(ctx context.Context, user *models.User, folder string, uid uint32) (*models.EmailMessage, error) {
	conn, err := s.getConn(ctx, user)
	if err != nil {
		return nil, err
	}
	defer s.imapPool.ReturnConn(conn)

	msg, err := conn.FetchMessageBody(folder, uid)
	if err != nil {
		return nil, err
	}

	// Mark as read asynchronously.
	// FIX: extract password from the request context BEFORE spawning the goroutine
	// so the background context can open a new IMAP connection.
	imapPassword := auth.IMAPPasswordFromCtx(ctx)
	go s.markReadAsync(user, folder, uid, imapPassword)

	return msg, nil
}

// markReadAsync marks a single message as read in a background goroutine.
// It uses the supplied password directly instead of reading from context,
// which avoids the "empty password" bug when using context.Background().
func (s *Service) markReadAsync(user *models.User, folder string, uid uint32, imapPassword string) {
	ctx := context.Background()

	conn, err := s.imapPool.GetConn(user.Email, imapPassword)
	if err != nil {
		s.logger.Warn("markReadAsync: failed to get IMAP conn",
			zap.String("user", user.Email), zap.Error(err))
		return
	}
	defer s.imapPool.ReturnConn(conn)

	if err := conn.MarkRead(folder, []uint32{uid}, true); err != nil {
		s.logger.Warn("markReadAsync: MarkRead failed",
			zap.String("user", user.Email), zap.Uint32("uid", uid), zap.Error(err))
		return
	}

	_ = s.cache.IncrUnreadCount(ctx, user.ID.String(), -1)
	_ = s.cache.InvalidateFolderStats(ctx, user.ID.String())
	_ = s.cache.InvalidateInboxCache(ctx, user.ID.String(), folder)
	_ = s.cache.InvalidateAllThreadCaches(ctx, user.ID.String())
}

// ─── SEND EMAIL ───────────────────────────────────────────────────────────────

func (s *Service) SendEmail(ctx context.Context, user *models.User, userPassword string, req *models.ComposeRequest) error {
	smtpHost := s.cfg.SMTP.Host
	if user.SMTPHost != nil {
		smtpHost = *user.SMTPHost
	}
	smtpPort := s.cfg.SMTP.Port
	if user.SMTPPort != 0 {
		smtpPort = user.SMTPPort
	}

	addr := fmt.Sprintf("%s:%d", smtpHost, smtpPort)
	msgBytes := buildMIMEMessage(user, req)

	var recipients []string
	for _, a := range req.To {
		recipients = append(recipients, a.Email)
	}
	for _, a := range req.Cc {
		recipients = append(recipients, a.Email)
	}
	for _, a := range req.Bcc {
		recipients = append(recipients, a.Email)
	}

	var sendErr error
	if s.cfg.SMTP.UseTLS {
		sendErr = sendMailTLS(addr, smtpHost, user.Email, userPassword, user.Email, recipients, msgBytes)
	} else {
		auth := smtp.PlainAuth("", user.Email, userPassword, smtpHost)
		sendErr = smtp.SendMail(addr, auth, user.Email, recipients, msgBytes)
	}
	if sendErr != nil {
		return fmt.Errorf("SMTP send: %w", sendErr)
	}

	go func() {
		bgCtx := context.Background()
		_ = s.cache.InvalidateInboxCache(bgCtx, user.ID.String(), "Sent")
		_ = s.cache.InvalidateFolderStats(bgCtx, user.ID.String())
		s.updateContactStats(bgCtx, user, req.To)
	}()

	return nil
}

// SaveDraft constructs an email and appends it to the IMAP Drafts folder.
func (s *Service) SaveDraft(ctx context.Context, user *models.User, userPassword string, req *models.ComposeRequest) error {
	msgBytes := buildMIMEMessage(user, req)

	conn, err := s.imapPool.GetConn(user.Email, userPassword)
	if err != nil {
		return fmt.Errorf("imap GetConn: %w", err)
	}
	defer s.imapPool.ReturnConn(conn)

	// Append to "Drafts"
	flags := []imap.Flag{imap.FlagDraft, imap.FlagSeen}
	if err := conn.AppendMessage("Drafts", flags, time.Now(), msgBytes); err != nil {
		return fmt.Errorf("append to Drafts: %w", err)
	}

	// Invalidate caches
	go func() {
		bgCtx := context.Background()
		_ = s.cache.InvalidateInboxCache(bgCtx, user.ID.String(), "Drafts")
		_ = s.cache.InvalidateFolderStats(bgCtx, user.ID.String())
	}()

	return nil
}

// sendMailTLS sends email over implicit TLS (port 465).
func sendMailTLS(addr, host, username, password, from string, to []string, msg []byte) error {
	tlsCfg := &tls.Config{
		ServerName: host,
		MinVersion: tls.VersionTLS12,
	}

	conn, err := tls.Dial("tcp", addr, tlsCfg)
	if err != nil {
		return fmt.Errorf("TLS dial: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := client.Auth(smtp.PlainAuth("", username, password, host)); err != nil {
		return err
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err := client.Rcpt(rcpt); err != nil {
			return err
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	return w.Close()
}

// buildMIMEMessage assembles a raw RFC 5322 / MIME email.
// It sets In-Reply-To and References headers for threading so that
// mail clients can group the reply/forward with the original thread.
func buildMIMEMessage(user *models.User, req *models.ComposeRequest) []byte {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("From: %s <%s>\r\n", user.DisplayName, user.Email))
	sb.WriteString(fmt.Sprintf("To: %s\r\n", formatAddresses(req.To)))
	if len(req.Cc) > 0 {
		sb.WriteString(fmt.Sprintf("Cc: %s\r\n", formatAddresses(req.Cc)))
	}
	sb.WriteString(fmt.Sprintf("Subject: %s\r\n", req.Subject))
	sb.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().UTC().Format(emailDateLayout)))
	sb.WriteString(fmt.Sprintf("Message-ID: <%d.%s@%s>\r\n",
		time.Now().UnixNano(), "kuromail", extractDomain(user.Email)))
	sb.WriteString("MIME-Version: 1.0\r\n")

	// Threading headers — essential for reply/forward to be grouped correctly.
	if req.InReplyTo != "" {
		sb.WriteString(fmt.Sprintf("In-Reply-To: %s\r\n", req.InReplyTo))
	}
	if req.References != "" {
		sb.WriteString(fmt.Sprintf("References: %s\r\n", req.References))
	}

	// Body
	if len(req.Attachments) > 0 {
		mixedBoundary := fmt.Sprintf("mixed_%d", time.Now().UnixNano())
		sb.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=\"%s\"\r\n\r\n", mixedBoundary))

		// Start of mixed body
		sb.WriteString(fmt.Sprintf("--%s\r\n", mixedBoundary))

		if req.BodyHTML != "" && req.BodyText != "" {
			altBoundary := fmt.Sprintf("alt_%d", time.Now().UnixNano())
			sb.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n", altBoundary))
			sb.WriteString(fmt.Sprintf("--%s\r\n", altBoundary))
			sb.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
			sb.WriteString(req.BodyText)
			sb.WriteString("\r\n")
			sb.WriteString(fmt.Sprintf("--%s\r\n", altBoundary))
			sb.WriteString("Content-Type: text/html; charset=utf-8\r\n\r\n")
			sb.WriteString(req.BodyHTML)
			sb.WriteString("\r\n")
			sb.WriteString(fmt.Sprintf("--%s--\r\n", altBoundary))
		} else if req.BodyHTML != "" {
			sb.WriteString("Content-Type: text/html; charset=utf-8\r\n\r\n")
			sb.WriteString(req.BodyHTML)
			sb.WriteString("\r\n")
		} else {
			sb.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
			sb.WriteString(req.BodyText)
			sb.WriteString("\r\n")
		}

		// Attachments
		for _, att := range req.Attachments {
			sb.WriteString(fmt.Sprintf("--%s\r\n", mixedBoundary))
			sb.WriteString(fmt.Sprintf("Content-Type: %s; name=\"%s\"\r\n", att.MimeType, att.Filename))
			sb.WriteString("Content-Disposition: attachment; filename=\"" + att.Filename + "\"\r\n")
			sb.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")

			// Try to strip base64 prefix if it exists (e.g., data:image/png;base64,...)
			base64Data := att.Base64
			if idx := strings.Index(base64Data, ","); idx != -1 {
				base64Data = base64Data[idx+1:]
			}

			// Format base64 string to 76 characters per line (RFC 2045)
			sb.WriteString(formatBase64(base64Data))
			sb.WriteString("\r\n")
		}

		sb.WriteString(fmt.Sprintf("--%s--\r\n", mixedBoundary))
	} else {
		if req.BodyHTML != "" && req.BodyText != "" {
			boundary := fmt.Sprintf("alt_%d", time.Now().UnixNano())
			sb.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n", boundary))
			sb.WriteString(fmt.Sprintf("--%s\r\n", boundary))
			sb.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
			sb.WriteString(req.BodyText)
			sb.WriteString("\r\n")
			sb.WriteString(fmt.Sprintf("--%s\r\n", boundary))
			sb.WriteString("Content-Type: text/html; charset=utf-8\r\n\r\n")
			sb.WriteString(req.BodyHTML)
			sb.WriteString("\r\n")
			sb.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
		} else if req.BodyHTML != "" {
			sb.WriteString("Content-Type: text/html; charset=utf-8\r\n\r\n")
			sb.WriteString(req.BodyHTML)
		} else {
			sb.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
			sb.WriteString(req.BodyText)
		}
	}

	return []byte(sb.String())
}

func formatBase64(data string) string {
	var sb strings.Builder
	for i := 0; i < len(data); i += 76 {
		end := i + 76
		if end > len(data) {
			end = len(data)
		}
		sb.WriteString(data[i:end])
		sb.WriteString("\r\n")
	}
	return sb.String()
}

// ─── EMAIL ACTIONS ────────────────────────────────────────────────────────────

func (s *Service) MarkRead(ctx context.Context, user *models.User, folder string, uids []uint32, read bool) error {
	s.logger.Debug("MarkRead service",
		zap.String("user", user.Email),
		zap.String("folder", folder),
		zap.Uint32s("uids", uids),
		zap.Bool("read", read))

	conn, err := s.getConn(ctx, user)
	if err != nil {
		return err
	}
	defer s.imapPool.ReturnConn(conn)

	if err := conn.MarkRead(folder, uids, read); err != nil {
		return err
	}

	// Debug: Check Redis before cache update
	oldUnread, _ := s.cache.GetUnreadCount(ctx, user.ID.String())
	s.logger.Debug("Redis unread count before update", zap.Int("unread", oldUnread))

	// Only update unread count for INBOX folder
	folderUpper := strings.ToUpper(folder)
	if folderUpper == "INBOX" {
		delta := len(uids)
		if read {
			delta = -delta
		}
		_ = s.cache.IncrUnreadCount(ctx, user.ID.String(), delta)
	}
	_ = s.cache.InvalidateFolderStats(ctx, user.ID.String())
	_ = s.cache.InvalidateInboxCache(ctx, user.ID.String(), folder)
	_ = s.cache.InvalidateAllThreadCaches(ctx, user.ID.String())

	// Publish inbox_update event for real-time UI refresh
	_ = s.cache.PublishNotification(ctx, user.ID.String(), &cache.NotifyEvent{
		Type:    "inbox_update",
		Payload: map[string]string{"folder": folder},
	})

	// Debug: Check Redis after cache update
	newUnread, _ := s.cache.GetUnreadCount(ctx, user.ID.String())
	s.logger.Debug("Redis unread count after update",
		zap.Int("unread", newUnread),
		zap.Int("delta_applied", len(uids)))

	return nil
}

func (s *Service) MarkStarred(ctx context.Context, user *models.User, folder string, uids []uint32, starred bool) error {
	conn, err := s.getConn(ctx, user)
	if err != nil {
		return err
	}
	defer s.imapPool.ReturnConn(conn)

	targetFolder := folder
	if strings.EqualFold(folder, "Starred") {
		targetFolder = "INBOX"
	}

	if err := conn.MarkStarred(targetFolder, uids, starred); err != nil {
		return err
	}
	_ = s.cache.InvalidateInboxCache(ctx, user.ID.String(), folder)
	_ = s.cache.InvalidateInboxCache(ctx, user.ID.String(), "Starred")
	return nil
}

func (s *Service) MoveToTrash(ctx context.Context, user *models.User, folder string, uids []uint32) error {
	conn, err := s.getConn(ctx, user)
	if err != nil {
		return err
	}
	defer s.imapPool.ReturnConn(conn)

	// Jika folder sudah Trash, maka hapus permanen (permanent=true)
	permanent := strings.EqualFold(folder, "Trash")
	if err := conn.DeleteMessage(folder, uids, permanent); err != nil {
		return err
	}
	_ = s.cache.InvalidateInboxCache(ctx, user.ID.String(), folder)
	if !permanent {
		_ = s.cache.InvalidateInboxCache(ctx, user.ID.String(), "Trash")
	}
	_ = s.cache.InvalidateFolderStats(ctx, user.ID.String())
	return nil
}

func (s *Service) MoveMessage(ctx context.Context, user *models.User, srcFolder, dstFolder string, uids []uint32) error {
	conn, err := s.getConn(ctx, user)
	if err != nil {
		return err
	}
	defer s.imapPool.ReturnConn(conn)

	if err := conn.MoveMessage(srcFolder, dstFolder, uids); err != nil {
		return err
	}
	_ = s.cache.InvalidateInboxCache(ctx, user.ID.String(), srcFolder)
	_ = s.cache.InvalidateInboxCache(ctx, user.ID.String(), dstFolder)
	_ = s.cache.InvalidateFolderStats(ctx, user.ID.String())
	return nil
}

// GetAttachment retrieves the binary data of an attachment
func (s *Service) GetAttachment(ctx context.Context, user *models.User, folder string, uid uint32, attachmentID string) ([]byte, string, string, error) {
	conn, err := s.getConn(ctx, user)
	if err != nil {
		return nil, "", "", err
	}
	defer s.imapPool.ReturnConn(conn)

	return conn.FetchAttachment(folder, uid, attachmentID)
}

// ─── SEARCH ───────────────────────────────────────────────────────────────────

func (s *Service) Search(ctx context.Context, user *models.User, req *models.SearchRequest) ([]*models.ThreadListItem, error) {
	folder := req.Folder
	if folder == "" {
		folder = "INBOX"
	}

	conn, err := s.getConn(ctx, user)
	if err != nil {
		return nil, err
	}
	defer s.imapPool.ReturnConn(conn)

	criteria := buildSearchCriteria(req)

	uids, err := conn.SearchMessages(folder, criteria)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	if len(uids) == 0 {
		return []*models.ThreadListItem{}, nil
	}

	var messages []*models.EmailMessage
	for _, uid := range uids {
		msg, err := conn.FetchMessageBody(folder, uid)
		if err != nil {
			continue
		}
		messages = append(messages, msg)
	}

	threads := s.threadEngine.BuildThreads(messages)
	var items []*models.ThreadListItem
	for _, t := range threads {
		items = append(items, s.threadEngine.ToListItem(t))
	}
	return items, nil
}

func buildSearchCriteria(req *models.SearchRequest) *imap.SearchCriteria {
	c := &imap.SearchCriteria{}
	if req.Query != "" {
		c.Text = []string{req.Query}
	}
	if req.From != "" {
		c.Header = append(c.Header, imap.SearchCriteriaHeaderField{Key: "From", Value: req.From})
	}
	if req.IsUnread != nil && *req.IsUnread {
		c.NotFlag = []imap.Flag{imap.FlagSeen}
	}
	if req.DateFrom != nil {
		c.Since = *req.DateFrom
	}
	if req.DateTo != nil {
		c.Before = *req.DateTo
	}
	return c
}

// ─── REAL-TIME: IMAP IDLE ─────────────────────────────────────────────────────

// WatchInbox runs an IMAP IDLE loop and publishes notifications to Redis Pub/Sub.
// Should be called as a goroutine per user.
func (s *Service) WatchInbox(ctx context.Context, user *models.User, userPassword string) {
	backoff := 5 * time.Second
	maxBackoff := 2 * time.Minute

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		conn, err := s.imapPool.GetConn(user.Email, userPassword)
		if err != nil {
			s.logger.Error("IDLE: failed to get IMAP conn",
				zap.String("user", user.Email), zap.Error(err))
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				backoff = min(backoff*2, maxBackoff)
				continue
			}
		}
		backoff = 5 * time.Second

		statsBefore, _ := conn.GetFolderStats("INBOX")

		if _, err := conn.client.Select("INBOX", &imap.SelectOptions{ReadOnly: true}).Wait(); err != nil {
			s.imapPool.ReturnConn(conn)
			time.Sleep(5 * time.Second)
			continue
		}

		idleCmd, err := conn.client.Idle()
		if err != nil {
			s.imapPool.ReturnConn(conn)
			time.Sleep(5 * time.Second)
			continue
		}

		// RFC 2177 recommends re-issuing IDLE before 29 minutes.
		idleTimer := time.NewTimer(28 * time.Minute)
		done := make(chan struct{})
		go func() {
			defer close(done)
			_ = idleCmd.Wait()
		}()

		select {
		case <-ctx.Done():
			_ = idleCmd.Close()
			s.imapPool.ReturnConn(conn)
			idleTimer.Stop()
			return
		case <-idleTimer.C:
			_ = idleCmd.Close()
		case <-done:
			// Server sent an unsolicited response (new mail, expunge, etc.)
		}
		idleTimer.Stop()

		statsAfter, err := conn.GetFolderStats("INBOX")
		s.imapPool.ReturnConn(conn)

		if err == nil && statsBefore != nil && statsAfter.TotalCount > statsBefore.TotalCount {
			newCount := statsAfter.TotalCount - statsBefore.TotalCount
			s.logger.Info("New email detected via IDLE",
				zap.String("user", user.Email), zap.Int("new_count", newCount))

			_ = s.cache.InvalidateInboxCache(ctx, user.ID.String(), "INBOX")
			_ = s.cache.InvalidateFolderStats(ctx, user.ID.String())
			_ = s.cache.SetUnreadCount(ctx, user.ID.String(), statsAfter.UnreadCount)
			_ = s.cache.PublishNotification(ctx, user.ID.String(), &cache.NotifyEvent{
				Type: "new_email",
				Payload: map[string]interface{}{
					"folder":       "INBOX",
					"new_count":    newCount,
					"unread_count": statsAfter.UnreadCount,
				},
			})
		} else {
			_ = s.cache.InvalidateInboxCache(ctx, user.ID.String(), "INBOX")
			_ = s.cache.PublishNotification(ctx, user.ID.String(), &cache.NotifyEvent{
				Type:    "inbox_update",
				Payload: map[string]string{"folder": "INBOX"},
			})
		}
	}
}

// ─── SSE ─────────────────────────────────────────────────────────────────────

// SubscribeEvents subscribes to Redis Pub/Sub for a user — used by SSE endpoint.
func (s *Service) SubscribeEvents(ctx context.Context, userID string) (<-chan string, error) {
	pubsub := s.cache.SubscribeNotifications(ctx, userID)
	ch := make(chan string, 10)

	go func() {
		defer close(ch)
		defer pubsub.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-pubsub.Channel():
				if !ok {
					return
				}
				select {
				case ch <- msg.Payload:
				default:
					// Channel full — drop the oldest-pending event.
				}
			}
		}
	}()

	return ch, nil
}

// ─── MISC ─────────────────────────────────────────────────────────────────────

func (s *Service) GetUnreadCount(ctx context.Context, user *models.User) (int, error) {
	if count, err := s.cache.GetUnreadCount(ctx, user.ID.String()); err == nil && count >= 0 {
		return count, nil
	}

	conn, err := s.getConn(ctx, user)
	if err != nil {
		return 0, err
	}
	defer s.imapPool.ReturnConn(conn)

	stats, err := conn.GetFolderStats("INBOX")
	if err != nil {
		return 0, err
	}

	_ = s.cache.SetUnreadCount(ctx, user.ID.String(), stats.UnreadCount)
	return stats.UnreadCount, nil
}

// getConn retrieves an IMAP connection using the password stored in the
// request context by the auth middleware.
func (s *Service) getConn(ctx context.Context, user *models.User) (*IMAPConn, error) {
	password := auth.IMAPPasswordFromCtx(ctx)
	return s.imapPool.GetConn(user.Email, password)
}

// GetConnWithPassword opens an IMAP connection with an explicit password.
// Used for operations that need a password outside of an HTTP request context
// (e.g. IDLE watcher).
func (s *Service) GetConnWithPassword(user *models.User, password string) (*IMAPConn, error) {
	return s.imapPool.GetConn(user.Email, password)
}

// ─── FORMATTING HELPERS ───────────────────────────────────────────────────────

func formatAddresses(addrs []models.EmailAddress) string {
	parts := make([]string, 0, len(addrs))
	for _, a := range addrs {
		if a.Name != "" {
			parts = append(parts, fmt.Sprintf("%s <%s>", a.Name, a.Email))
		} else {
			parts = append(parts, a.Email)
		}
	}
	return strings.Join(parts, ", ")
}

func extractDomain(email string) string {
	if parts := strings.SplitN(email, "@", 2); len(parts) == 2 {
		return parts[1]
	}
	return "localhost"
}

func (s *Service) updateContactStats(ctx context.Context, user *models.User, addresses []models.EmailAddress) {
	for _, addr := range addresses {
		s.logger.Debug("Updating contact stats", zap.String("email", addr.Email))
		// TODO: upsert into contacts table
		_ = ctx
		_ = user
	}
}
