package email

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"github.com/webmail/backend/internal/auth"
	"github.com/webmail/backend/internal/models"
)

type Handler struct {
	svc    *Service
	logger *zap.Logger
}

func NewHandler(svc *Service, logger *zap.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// RegisterRoutes mendaftarkan semua route email (semua butuh auth middleware)
func (h *Handler) RegisterRoutes(router fiber.Router) {
	email := router.Group("/email")

	// Folder & stats
	email.Get("/folders", h.GetFolderStats)
	email.Get("/unread-count", h.GetUnreadCount)

	// Thread list
	email.Get("/threads", h.ListThreads)
	email.Get("/threads/:thread_id", h.GetThread)

	// Message operations
	email.Get("/messages/:folder/:uid", h.GetMessage)
	email.Get("/messages/:folder/:uid/attachments/:attachment_id", h.DownloadAttachment)
	email.Post("/messages/mark-read", h.MarkRead)
	email.Post("/messages/mark-starred", h.MarkStarred)
	email.Post("/messages/move", h.MoveMessage)
	email.Post("/messages/trash", h.MoveToTrash)

	// Compose & Send
	email.Post("/send", h.Send)
	email.Post("/drafts", h.SaveDraft)
	email.Put("/drafts/:draft_id", h.UpdateDraft)
	email.Get("/drafts", h.ListDrafts)
	email.Delete("/drafts/:draft_id", h.DeleteDraft)

	// Search
	email.Get("/search", h.Search)

	// SSE — Server-Sent Events untuk real-time updates
	email.Get("/events", h.SSEHandler)
}

// GetFolderStats godoc
// @Summary Dapatkan statistik semua folder (total, unread)
// @Tags email
// @Security BearerAuth
// @Router /email/folders [get]
func (h *Handler) GetFolderStats(c *fiber.Ctx) error {
	user := auth.GetUserFromCtx(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse("unauthorized: no user in context"))
	}

	defaultFolders := []string{"INBOX", "Sent", "Drafts", "Trash", "Junk", "Archive"}
	stats, err := h.svc.GetFolderStats(c.UserContext(), user, defaultFolders)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse(err.Error()))
	}

	return c.JSON(models.SuccessResponse(stats))
}

// GetUnreadCount godoc
// @Summary Dapatkan jumlah email belum dibaca
// @Tags email
// @Security BearerAuth
// @Router /email/unread-count [get]
func (h *Handler) GetUnreadCount(c *fiber.Ctx) error {
	user := auth.GetUserFromCtx(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse("unauthorized: no user in context"))
	}

	count, err := h.svc.GetUnreadCount(c.UserContext(), user)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse(err.Error()))
	}

	return c.JSON(models.SuccessResponse(fiber.Map{"unread_count": count}))
}

// ListThreads godoc
// @Summary Daftar thread email untuk folder tertentu
// @Tags email
// @Security BearerAuth
// @Param folder query string false "Folder (default: INBOX)"
// @Param page query int false "Halaman (default: 1)"
// @Param per_page query int false "Item per halaman (default: 50)"
// @Router /email/threads [get]
func (h *Handler) ListThreads(c *fiber.Ctx) error {
	user := auth.GetUserFromCtx(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse("unauthorized: no user in context"))
	}

	folder := c.Query("folder", "INBOX")
	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", "50"))

	result, err := h.svc.ListThreads(c.UserContext(), user, folder, page, perPage)
	if err != nil {
		h.logger.Error("Failed to list threads", zap.Error(err), zap.String("user", user.Email))
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse("failed to load emails"))
	}

	return c.JSON(models.SuccessResponse(result))
}

// GetThread godoc
// @Summary Dapatkan thread lengkap beserta semua messages
// @Tags email
// @Security BearerAuth
// @Param thread_id path string true "Thread ID"
// @Param folder query string false "Folder IMAP"
// @Router /email/threads/:thread_id [get]
func (h *Handler) GetThread(c *fiber.Ctx) error {
	user := auth.GetUserFromCtx(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse("unauthorized: no user in context"))
	}
	threadID := c.Params("thread_id")
	folder := c.Query("folder", "INBOX")

	thread, err := h.svc.GetThread(c.UserContext(), user, folder, threadID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse("thread not found"))
	}

	return c.JSON(models.SuccessResponse(thread))
}

// GetMessage godoc
// @Summary Dapatkan satu email lengkap dengan body
// @Tags email
// @Security BearerAuth
// @Param folder path string true "Folder IMAP"
// @Param uid path int true "IMAP UID"
// @Router /email/messages/:folder/:uid [get]
func (h *Handler) GetMessage(c *fiber.Ctx) error {
	user := auth.GetUserFromCtx(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse("unauthorized: no user in context"))
	}
	folder := c.Params("folder")
	uid64, err := strconv.ParseUint(c.Params("uid"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse("invalid uid"))
	}

	msg, err := h.svc.GetMessage(c.UserContext(), user, folder, uint32(uid64))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse("message not found"))
	}

	return c.JSON(models.SuccessResponse(msg))
}

// DownloadAttachment godoc
// @Summary Download attachment dari email
// @Tags email
// @Security BearerAuth
// @Param folder path string true "Folder IMAP"
// @Param uid path int true "IMAP UID"
// @Param attachment_id path string true "Attachment ID"
// @Router /email/messages/:folder/:uid/attachments/:attachment_id [get]
func (h *Handler) DownloadAttachment(c *fiber.Ctx) error {
	user := auth.GetUserFromCtx(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse("unauthorized: no user in context"))
	}

	folder := c.Params("folder")
	uidStr := c.Params("uid")
	attachmentID := c.Params("attachment_id")

	fmt.Printf("[DEBUG] DownloadAttachment: user=%s, folder=%q, uid=%q, attachmentID=%q\n", user.Email, folder, uidStr, attachmentID)

	uid64, err := strconv.ParseUint(uidStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse("invalid uid"))
	}

	if attachmentID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse("attachment_id is required"))
	}

	data, filename, mimeType, err := h.svc.GetAttachment(c.UserContext(), user, folder, uint32(uid64), attachmentID)
	if err != nil {
		fmt.Printf("[DEBUG] DownloadAttachment ERROR: %v\n", err)
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse("attachment not found"))
	}

	fmt.Printf("[DEBUG] DownloadAttachment SUCCESS: filename=%q, mimeType=%q, dataLen=%d\n", filename, mimeType, len(data))

	c.Set("Content-Type", mimeType)
	c.Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	c.Set("Content-Length", strconv.FormatInt(int64(len(data)), 10))

	return c.Send(data)
}

// MarkRead godoc
// @Summary Tandai email sebagai sudah/belum dibaca
// @Tags email
// @Security BearerAuth
// @Router /email/messages/mark-read [post]
func (h *Handler) MarkRead(c *fiber.Ctx) error {
	user := auth.GetUserFromCtx(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse("unauthorized: no user in context"))
	}

	var req struct {
		Folder string   `json:"folder"`
		UIDs   []uint32 `json:"uids"`
		Read   bool     `json:"read"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse("invalid request"))
	}

	// Debug: log request details
	h.logger.Debug("MarkRead request",
		zap.String("user", user.Email),
		zap.String("folder", req.Folder),
		zap.Uint32s("uids", req.UIDs),
		zap.Bool("read", req.Read))

	// Filter out invalid UIDs (UID 0 is invalid in IMAP, valid UIDs start from 1)
	validUIDs := make([]uint32, 0, len(req.UIDs))
	for _, uid := range req.UIDs {
		if uid > 0 {
			validUIDs = append(validUIDs, uid)
		} else {
			h.logger.Warn("Skipping invalid UID", zap.Uint32("uid", uid))
		}
	}

	if len(validUIDs) == 0 {
		return c.JSON(models.SuccessResponse(fiber.Map{"updated": 0}))
	}

	if err := h.svc.MarkRead(c.UserContext(), user, req.Folder, validUIDs, req.Read); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse(err.Error()))
	}

	return c.JSON(models.SuccessResponse(fiber.Map{"updated": len(validUIDs)}))
}

// MarkStarred godoc
// @Summary Toggle starred pada email
// @Tags email
// @Security BearerAuth
// @Router /email/messages/mark-starred [post]
func (h *Handler) MarkStarred(c *fiber.Ctx) error {
	user := auth.GetUserFromCtx(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse("unauthorized: no user in context"))
	}

	var req struct {
		Folder  string   `json:"folder"`
		UIDs    []uint32 `json:"uids"`
		Starred bool     `json:"starred"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse("invalid request"))
	}

	// Filter out invalid UIDs (UID 0 is invalid in IMAP, valid UIDs start from 1)
	validUIDs := make([]uint32, 0, len(req.UIDs))
	for _, uid := range req.UIDs {
		if uid > 0 {
			validUIDs = append(validUIDs, uid)
		}
	}

	if len(validUIDs) == 0 {
		return c.JSON(models.SuccessResponse(fiber.Map{"updated": 0}))
	}

	if err := h.svc.MarkStarred(c.UserContext(), user, req.Folder, validUIDs, req.Starred); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse(err.Error()))
	}

	return c.JSON(models.SuccessResponse(fiber.Map{"updated": len(validUIDs)}))
}

// MoveMessage godoc
// @Summary Pindahkan email ke folder lain
// @Tags email
// @Security BearerAuth
// @Router /email/messages/move [post]
func (h *Handler) MoveMessage(c *fiber.Ctx) error {
	user := auth.GetUserFromCtx(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse("unauthorized: no user in context"))
	}

	var req struct {
		SrcFolder string   `json:"src_folder"`
		DstFolder string   `json:"dst_folder"`
		UIDs      []uint32 `json:"uids"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse("invalid request"))
	}

	// Filter out invalid UIDs (UID 0 is invalid in IMAP, valid UIDs start from 1)
	validUIDs := make([]uint32, 0, len(req.UIDs))
	for _, uid := range req.UIDs {
		if uid > 0 {
			validUIDs = append(validUIDs, uid)
		}
	}

	if len(validUIDs) == 0 {
		return c.JSON(models.SuccessResponse(fiber.Map{"moved": 0, "to": req.DstFolder}))
	}

	if err := h.svc.MoveMessage(c.UserContext(), user, req.SrcFolder, req.DstFolder, validUIDs); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse(err.Error()))
	}

	return c.JSON(models.SuccessResponse(fiber.Map{"moved": len(validUIDs), "to": req.DstFolder}))
}

// MoveToTrash godoc
// @Summary Hapus email (pindah ke Trash)
// @Tags email
// @Security BearerAuth
// @Router /email/messages/trash [post]
func (h *Handler) MoveToTrash(c *fiber.Ctx) error {
	user := auth.GetUserFromCtx(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse("unauthorized: no user in context"))
	}

	var req struct {
		Folder string   `json:"folder"`
		UIDs   []uint32 `json:"uids"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse("invalid request"))
	}

	// Filter out invalid UIDs (UID 0 is invalid in IMAP, valid UIDs start from 1)
	validUIDs := make([]uint32, 0, len(req.UIDs))
	for _, uid := range req.UIDs {
		if uid > 0 {
			validUIDs = append(validUIDs, uid)
		}
	}

	if len(validUIDs) == 0 {
		return c.JSON(models.SuccessResponse(fiber.Map{"trashed": 0}))
	}

	if err := h.svc.MoveToTrash(c.UserContext(), user, req.Folder, validUIDs); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse(err.Error()))
	}

	return c.JSON(models.SuccessResponse(fiber.Map{"trashed": len(validUIDs)}))
}

// Send godoc
// @Summary Kirim email baru
// @Tags email
// @Security BearerAuth
// @Router /email/send [post]
func (h *Handler) Send(c *fiber.Ctx) error {
	user := auth.GetUserFromCtx(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse("unauthorized: no user in context"))
	}

	var req models.ComposeRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse("invalid request body"))
	}

	if len(req.To) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse("at least one recipient required"))
	}
	if req.Subject == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse("subject is required"))
	}

	// Ambil password dari context yang diset oleh auth middleware
	userPassword := auth.GetIMAPPasswordFromCtx(c)

	if req.SendAt != nil && req.SendAt.After(time.Now()) {
		// Scheduled send — simpan ke drafts dengan send_at
		// TODO: implement schedule queue
		return c.JSON(models.SuccessResponse(fiber.Map{
			"message": "email scheduled",
			"send_at": req.SendAt,
		}))
	}

	if err := h.svc.SendEmail(c.UserContext(), user, userPassword, &req); err != nil {
		h.logger.Error("Send email failed", zap.Error(err), zap.String("user", user.Email))
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse("failed to send email"))
	}

	return c.JSON(models.SuccessResponse(fiber.Map{"message": "email sent successfully"}))
}

// Search godoc
// @Summary Search email
// @Tags email
// @Security BearerAuth
// @Param q query string true "Query string"
// @Param folder query string false "Folder (default: INBOX)"
// @Router /email/search [get]
func (h *Handler) Search(c *fiber.Ctx) error {
	user := auth.GetUserFromCtx(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse("unauthorized: no user in context"))
	}

	req := &models.SearchRequest{
		Query:   c.Query("q"),
		Folder:  c.Query("folder", "INBOX"),
		From:    c.Query("from"),
		Page:    1,
		PerPage: 50,
	}

	if req.Query == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse("search query is required"))
	}

	results, err := h.svc.Search(c.UserContext(), user, req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse(err.Error()))
	}

	return c.JSON(models.SuccessResponse(results))
}

// SSEHandler godoc
// @Summary Server-Sent Events untuk real-time inbox updates
// @Tags email
// @Security BearerAuth
// @Router /email/events [get]
func (h *Handler) SSEHandler(c *fiber.Ctx) error {
	user := auth.GetUserFromCtx(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse("unauthorized: no user in context"))
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no") // Disable Nginx buffering

	events, err := h.svc.SubscribeEvents(c.UserContext(), user.ID.String())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse("failed to subscribe"))
	}

	// Kirim initial event
	_, _ = fmt.Fprintf(c.Response().BodyWriter(), "data: {\"type\":\"connected\",\"user_id\":\"%s\"}\n\n", user.ID)

	// Stream events ke client
	for event := range events {
		_, _ = fmt.Fprintf(c.Response().BodyWriter(), "data: %s\n\n", event)
	}

	return nil
}

// SaveDraft godoc
// @Summary Simpan draft email
// @Tags email
// @Security BearerAuth
// @Router /email/drafts [post]
func (h *Handler) SaveDraft(c *fiber.Ctx) error {
	user := auth.GetUserFromCtx(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse("unauthorized: no user in context"))
	}

	var req models.ComposeRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse("invalid request body"))
	}

	userPassword := auth.GetIMAPPasswordFromCtx(c)

	if err := h.svc.SaveDraft(c.UserContext(), user, userPassword, &req); err != nil {
		h.logger.Error("Save draft failed", zap.Error(err), zap.String("user", user.Email))
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse("failed to save draft"))
	}

	return c.JSON(models.SuccessResponse(fiber.Map{"message": "draft saved"}))
}

// UpdateDraft godoc
// @Summary Update draft yang ada (auto-save)
// @Tags email
// @Security BearerAuth
// @Router /email/drafts/:draft_id [put]
func (h *Handler) UpdateDraft(c *fiber.Ctx) error {
	// TODO: Implement draft update dengan Redis debounce
	return c.JSON(models.SuccessResponse(fiber.Map{"message": "draft updated"}))
}

// ListDrafts godoc
// @Summary List semua draft
// @Tags email
// @Security BearerAuth
// @Router /email/drafts [get]
func (h *Handler) ListDrafts(c *fiber.Ctx) error {
	user := auth.GetUserFromCtx(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse("unauthorized: no user in context"))
	}
	// Fetch dari IMAP Drafts folder
	result, err := h.svc.ListThreads(c.UserContext(), user, "Drafts", 1, 50)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse(err.Error()))
	}
	return c.JSON(models.SuccessResponse(result))
}

// DeleteDraft godoc
// @Summary Hapus draft
// @Tags email
// @Security BearerAuth
// @Router /email/drafts/:draft_id [delete]
func (h *Handler) DeleteDraft(c *fiber.Ctx) error {
	// TODO: Implement delete draft dari IMAP Drafts + PostgreSQL
	return c.JSON(models.SuccessResponse(fiber.Map{"message": "draft deleted"}))
}
