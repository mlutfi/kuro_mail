package email

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"strings"

	"github.com/webmail/backend/internal/models"
)

// attachmentData is a package-level map to store attachment binary content
// This is used to store attachment data for download after the message is parsed
var attachmentData = make(map[string][]byte)

// GetAttachmentData returns the binary content of an attachment by ID
func GetAttachmentData(id string) ([]byte, bool) {
	data, ok := attachmentData[id]
	return data, ok
}

// ClearAttachmentData removes attachment data from memory (call after sending to client)
func ClearAttachmentData(id string) {
	delete(attachmentData, id)
}

// GetAttachmentDataKeys returns all keys in the attachmentData map
func GetAttachmentDataKeys() []string {
	keys := make([]string, 0, len(attachmentData))
	for k := range attachmentData {
		keys = append(keys, k)
	}
	return keys
}

// htmlToPreview strips HTML tags and returns a plain-text preview of at most
// maxLen characters.
func htmlToPreview(html string, maxLen int) string {
	var buf bytes.Buffer
	inTag := false
	for _, r := range html {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			buf.WriteRune(r)
		}
	}
	result := strings.Join(strings.Fields(buf.String()), " ")
	if len(result) > maxLen {
		return result[:maxLen] + "..."
	}
	return result
}

// parseMIMEMessage parses a raw RFC822 message and populates the email's
// body fields (HTML, plain text, preview) and attachments.
// It handles multipart/alternative, multipart/mixed, base64, and
// quoted-printable transfer encodings.
func parseMIMEMessage(raw []byte, email *models.EmailMessage) error {
	fmt.Printf("[DEBUG] parseMIMEMessage: Starting parse, raw body len=%d, email.IMAPUID=%d\n", len(raw), email.IMAPUID)

	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("read message: %w", err)
	}

	// Fill in any header fields not already set by the IMAP envelope.
	if email.InReplyTo == "" {
		email.InReplyTo = strings.TrimSpace(msg.Header.Get("In-Reply-To"))
	}
	if len(email.References) == 0 {
		if refs := msg.Header.Get("References"); refs != "" {
			email.References = parseReferencesHeader(refs)
		}
	}
	if email.MessageID == "" {
		email.MessageID = strings.TrimSpace(msg.Header.Get("Message-Id"))
	}

	// Parse the body tree.
	parsePart(textproto.MIMEHeader(msg.Header), msg.Body, email)

	fmt.Printf("[DEBUG] parseMIMEMessage: Finished, found %d attachments\n", len(email.Attachments))

	// Build preview from the parsed body.
	if email.BodyPreview == "" {
		if email.BodyHTML != "" {
			email.BodyPreview = htmlToPreview(email.BodyHTML, 200)
		} else if email.BodyText != "" {
			if len(email.BodyText) > 200 {
				email.BodyPreview = email.BodyText[:200] + "..."
			} else {
				email.BodyPreview = email.BodyText
			}
		}
	}

	return nil
}

// parsePart recursively walks a MIME tree.
// For multipart/* it descends into each child part.
// For leaf parts it decodes the transfer encoding and stores the content.
func parsePart(h textproto.MIMEHeader, body io.Reader, email *models.EmailMessage) {
	ct := h.Get("Content-Type")
	if ct == "" {
		ct = "text/plain; charset=utf-8"
	}

	mediaType, params, err := mime.ParseMediaType(ct)
	if err != nil {
		// Fallback: treat as plain text
		data, _ := io.ReadAll(body)
		if email.BodyText == "" {
			email.BodyText = string(data)
		}
		return
	}

	// ── Multipart: recurse into each sub-part ────────────────────────────
	if strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			return
		}
		mr := multipart.NewReader(body, boundary)
		for {
			part, err := mr.NextPart()
			if err != nil {
				break // io.EOF or parse error
			}
			parsePart(part.Header, part, email)
		}
		return
	}

	// ── Leaf part: decode content-transfer-encoding ──────────────────────
	decoded := decodeCTE(h.Get("Content-Transfer-Encoding"), body)

	data, err := io.ReadAll(decoded)
	if err != nil {
		return
	}

	// ── Attachment detection ─────────────────────────────────────────────
	if isAttachment(h, params) {
		filename := resolveFilename(h, params, len(email.Attachments))
		attID := fmt.Sprintf("att-%d-%d", email.IMAPUID, len(email.Attachments))
		fmt.Printf("[DEBUG] parsePart: Found attachment: ID=%q, Filename=%q, MimeType=%q, Size=%d\n", attID, filename, mediaType, len(data))
		email.Attachments = append(email.Attachments, models.Attachment{
			ID:       attID,
			Filename: filename,
			MimeType: mediaType,
			Size:     int64(len(data)),
		})
		// Store the binary content for later retrieval
		attachmentData[attID] = data
		fmt.Printf("[DEBUG] parsePart: Stored attachment data, key=%q, data len=%d\n", attID, len(data))
		email.HasAttachments = true
		return
	}

	// ── Inline text content ──────────────────────────────────────────────
	switch mediaType {
	case "text/html":
		if email.BodyHTML == "" {
			email.BodyHTML = string(data)
		}
	case "text/plain":
		if email.BodyText == "" {
			email.BodyText = string(data)
		}
	}
}

// decodeCTE wraps body with the appropriate decoder for the given
// Content-Transfer-Encoding value.
func decodeCTE(cte string, body io.Reader) io.Reader {
	switch strings.ToLower(strings.TrimSpace(cte)) {
	case "base64":
		// Strip all whitespace (MIME base64 has 76-char CRLF-delimited lines).
		raw, _ := io.ReadAll(body)
		cleaned := bytes.Map(func(r rune) rune {
			if r == ' ' || r == '\t' || r == '\r' || r == '\n' {
				return -1
			}
			return r
		}, raw)
		// Try standard encoding first, then raw (no-padding) as fallback.
		decoded, err := base64.StdEncoding.DecodeString(string(cleaned))
		if err != nil {
			decoded, _ = base64.RawStdEncoding.DecodeString(string(cleaned))
		}
		return bytes.NewReader(decoded)

	case "quoted-printable":
		return quotedprintable.NewReader(body)

	default:
		// 7bit / 8bit / binary — pass through as-is.
		return body
	}
}

// isAttachment returns true when the part should be treated as an attachment
// rather than inline body content.
func isAttachment(h textproto.MIMEHeader, ctParams map[string]string) bool {
	cd := h.Get("Content-Disposition")
	fmt.Printf("[DEBUG] isAttachment: Content-Disposition=%q\n", cd)
	if cd == "" {
		return false
	}
	disposition, dispParams, err := mime.ParseMediaType(cd)
	if err != nil {
		return false
	}
	if strings.EqualFold(disposition, "attachment") {
		fmt.Printf("[DEBUG] isAttachment: disposition=attachment, returning true\n")
		return true
	}
	// Some clients send inline parts with a filename — treat those as attachments too.
	if strings.EqualFold(disposition, "inline") {
		if dispParams["filename"] != "" || ctParams["name"] != "" {
			fmt.Printf("[DEBUG] isAttachment: disposition=inline with filename, returning true\n")
			return true
		}
	}
	return false
}

// resolveFilename picks the best filename for an attachment part.
func resolveFilename(h textproto.MIMEHeader, ctParams map[string]string, idx int) string {
	cd := h.Get("Content-Disposition")
	if cd != "" {
		if _, dispParams, err := mime.ParseMediaType(cd); err == nil {
			if name := dispParams["filename"]; name != "" {
				return name
			}
		}
	}
	if name := ctParams["name"]; name != "" {
		return name
	}
	return fmt.Sprintf("attachment-%d", idx+1)
}
