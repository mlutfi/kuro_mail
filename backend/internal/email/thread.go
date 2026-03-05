package email

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/webmail/backend/internal/models"
)

var reSubjectPrefix = regexp.MustCompile(`(?i)^(re|fwd|fw|aw|wg|sv|vs):\s*`)

// ThreadEngine membangun thread dari daftar email
type ThreadEngine struct{}

func NewThreadEngine() *ThreadEngine {
	return &ThreadEngine{}
}

// BuildThreads mengelompokkan email menjadi thread-thread
func (t *ThreadEngine) BuildThreads(messages []*models.EmailMessage) []*models.Thread {
	// Map dari Message-ID → email
	byMessageID := make(map[string]*models.EmailMessage)
	for _, m := range messages {
		if m.MessageID != "" {
			byMessageID[m.MessageID] = m
		}
	}

	// Assign thread IDs
	threadAssignment := make(map[string]string) // messageID → threadID

	for _, m := range messages {
		if m.ThreadID != "" {
			// JMAP sudah assign thread ID — gunakan itu
			threadAssignment[m.MessageID] = m.ThreadID
			continue
		}
		threadAssignment[m.MessageID] = t.computeThreadID(m, byMessageID, threadAssignment)
	}

	// Group messages by thread ID
	threadMap := make(map[string][]*models.EmailMessage)
	for _, m := range messages {
		tid := threadAssignment[m.MessageID]
		if tid == "" {
			tid = t.computeThreadID(m, byMessageID, threadAssignment)
		}
		m.ThreadID = tid
		threadMap[tid] = append(threadMap[tid], m)
	}

	// Build Thread objects
	var threads []*models.Thread
	for threadID, msgs := range threadMap {
		thread := t.buildThread(threadID, msgs)
		threads = append(threads, thread)
	}

	// Sort: thread dengan email terbaru di atas
	sort.Slice(threads, func(i, j int) bool {
		return threads[i].LatestDate.After(threads[j].LatestDate)
	})

	return threads
}

// computeThreadID menentukan thread ID berdasarkan hierarki In-Reply-To
func (t *ThreadEngine) computeThreadID(
	m *models.EmailMessage,
	byMessageID map[string]*models.EmailMessage,
	assigned map[string]string,
) string {
	// Kalau ada In-Reply-To dan email parent ada di daftar kita
	if m.InReplyTo != "" {
		if parent, ok := byMessageID[m.InReplyTo]; ok {
			// Gunakan thread ID parent (recursion via assignment map)
			if parentTID, ok := assigned[parent.MessageID]; ok && parentTID != "" {
				return parentTID
			}
			// Parent belum di-assign — compute dulu
			parentTID := t.computeThreadID(parent, byMessageID, assigned)
			assigned[parent.MessageID] = parentTID
			return parentTID
		}
	}

	// Cek References header (array dari Message-ID)
	// Gunakan root Message-ID sebagai thread anchor
	if len(m.References) > 0 {
		// References[0] adalah root email
		rootMID := m.References[0]
		if rootEmail, ok := byMessageID[rootMID]; ok {
			// Hash dari root Message-ID sebagai thread ID
			return hashMessageID(rootEmail.MessageID)
		}
		// Root tidak ada di daftar tapi kita tahu ID-nya
		return hashMessageID(rootMID)
	}

	// Tidak ada parent — ini root thread baru
	// Normalize subject untuk group email dengan subject sama
	normalizedSubject := normalizeSubject(m.Subject)
	if normalizedSubject != "" && m.MessageID != "" {
		return hashMessageID(m.MessageID)
	}

	// Fallback: gunakan message ID sendiri
	if m.MessageID != "" {
		return hashMessageID(m.MessageID)
	}

	// Last resort: IMAP UID
	return fmt.Sprintf("uid-%d", m.IMAPUID)
}

// buildThread membangun objek Thread dari daftar message
func (t *ThreadEngine) buildThread(threadID string, messages []*models.EmailMessage) *models.Thread {
	// Sort messages by date ascending (oldest first dalam thread)
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].ReceivedAt.Before(messages[j].ReceivedAt)
	})

	thread := &models.Thread{
		ThreadID: threadID,
		Messages: messages,
	}

	// Collect stats dari messages
	participantMap := make(map[string]models.EmailAddress)
	labelMap := make(map[string]bool)
	var latestDate time.Time

	for _, m := range messages {
		thread.MessageCount++

		if !m.IsRead {
			thread.UnreadCount++
		}
		if m.IsStarred {
			thread.IsStarred = true
		}
		if m.HasAttachments {
			thread.HasAttachments = true
		}

		// Track participants
		key := m.From.Email
		if key != "" {
			if prevThread, ok := participantMap[key]; ok {
				_ = prevThread
			} else {
				participantMap[key] = m.From
			}
		}
		for _, to := range m.To {
			if _, exists := participantMap[to.Email]; !exists {
				participantMap[to.Email] = to
			}
		}

		// Labels
		for _, lid := range m.LabelIDs {
			labelMap[lid] = true
		}

		if m.ReceivedAt.After(latestDate) {
			latestDate = m.ReceivedAt
		}
	}

	// Subject dari email pertama (root), strip Re:/Fwd:
	if len(messages) > 0 {
		thread.Subject = normalizeSubject(messages[0].Subject)
	}

	// Participants sebagai slice (ordered: sender pertama, kemudian recipients)
	for _, p := range participantMap {
		thread.Participants = append(thread.Participants, p)
	}

	// Labels
	for lid := range labelMap {
		thread.LabelIDs = append(thread.LabelIDs, lid)
	}

	thread.LatestDate = latestDate

	return thread
}

// ToListItem mengkonversi Thread menjadi ThreadListItem yang ringan
func (t *ThreadEngine) ToListItem(thread *models.Thread) *models.ThreadListItem {
	preview := ""
	var uids []uint32
	if len(thread.Messages) > 0 {
		last := thread.Messages[len(thread.Messages)-1]
		preview = last.BodyPreview
		if len(preview) > 150 {
			preview = preview[:150] + "..."
		}
		for _, m := range thread.Messages {
			uids = append(uids, m.IMAPUID)
		}
	}

	return &models.ThreadListItem{
		ThreadID:       thread.ThreadID,
		Subject:        thread.Subject,
		MessageCount:   thread.MessageCount,
		UnreadCount:    thread.UnreadCount,
		Participants:   thread.Participants,
		BodyPreview:    preview,
		HasAttachments: thread.HasAttachments,
		IsStarred:      thread.IsStarred,
		LabelIDs:       thread.LabelIDs,
		LatestDate:     thread.LatestDate,
		MessageUIDs:    uids,
	}
}

// SortThreadsForFolder menyortir thread sesuai folder
// Inbox: terbaru di atas; Sent: terbaru di atas; Drafts: terbaru di atas
func (t *ThreadEngine) SortThreadsForFolder(threads []*models.Thread, folder string) {
	sort.Slice(threads, func(i, j int) bool {
		return threads[i].LatestDate.After(threads[j].LatestDate)
	})
}

// ─── HELPERS ─────────────────────────────────────────────────────────────────

// normalizeSubject menghapus prefix Re:, Fwd:, dll untuk perbandingan subject
func normalizeSubject(subject string) string {
	prev := ""
	result := strings.TrimSpace(subject)
	for result != prev {
		prev = result
		result = strings.TrimSpace(reSubjectPrefix.ReplaceAllString(result, ""))
	}
	return result
}

// hashMessageID membuat hash pendek dari Message-ID untuk dijadikan thread ID
func hashMessageID(messageID string) string {
	h := sha256.Sum256([]byte(messageID))
	return fmt.Sprintf("%x", h[:8]) // 16 karakter hex
}

// BuildThreadID adalah helper untuk menentukan thread ID satu email
// Digunakan saat menyimpan email baru ke database
func BuildThreadID(messageID, inReplyTo string, references []string) string {
	// Kalau ada references, gunakan root (element pertama) sebagai anchor
	if len(references) > 0 {
		return hashMessageID(references[0])
	}
	// Kalau ada In-Reply-To, hash itu sebagai thread ID
	if inReplyTo != "" {
		return hashMessageID(inReplyTo)
	}
	// Ini root email — hash messageID sendiri
	if messageID != "" {
		return hashMessageID(messageID)
	}
	return hashMessageID(fmt.Sprintf("%d", time.Now().UnixNano()))
}

// ParseReferences mem-parsing header References menjadi slice Message-ID
func ParseReferences(refsHeader string) []string {
	if refsHeader == "" {
		return nil
	}

	var refs []string
	// References bisa dipisah spasi atau newline
	parts := strings.FieldsFunc(refsHeader, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\r' || r == '\n'
	})

	for _, part := range parts {
		part = strings.Trim(part, "<>")
		if part != "" {
			refs = append(refs, "<"+part+">")
		}
	}
	return refs
}
