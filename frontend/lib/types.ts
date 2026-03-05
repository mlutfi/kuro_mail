// ─── API Response ────────────────────────────────────────────────────────────

export interface APIResponse<T = unknown> {
  success: boolean;
  message?: string;
  data?: T;
  error?: string;
}

// ─── User ────────────────────────────────────────────────────────────────────

export interface UserProfile {
  id: string;
  email: string;
  display_name: string;
  avatar_url?: string;
  totp_enabled: boolean;
  timezone: string;
  theme: string;
  signature: string;
}

// ─── Auth ────────────────────────────────────────────────────────────────────

export interface LoginRequest {
  email: string;
  password: string;
  device_id: string;
  device_name: string;
  remember_me?: boolean;
}

export interface LoginResponse {
  requires_two_fa: boolean;
  temp_token?: string;
  access_token?: string;
  refresh_token?: string;
  user?: UserProfile;
}

export interface TwoFAVerifyRequest {
  temp_token: string;
  code: string;
  trust_device?: boolean;
}

export interface TwoFASetupResponse {
  secret: string;
  qr_code_url: string;
  qr_code_data: string; // base64 PNG
}

export interface TwoFAEnableRequest {
  code: string;
}

export interface RefreshTokenRequest {
  refresh_token: string;
}

export interface TokenPair {
  access_token: string;
  refresh_token: string;
  expires_in: number;
}

// ─── Session ─────────────────────────────────────────────────────────────────

export interface Session {
  id: string;
  user_id: string;
  device_id?: string;
  device_name?: string;
  device_type?: string;
  ip_address?: string;
  is_trusted: boolean;
  is_revoked: boolean;
  created_at: string;
  last_active_at: string;
  expires_at: string;
}

// ─── Email ───────────────────────────────────────────────────────────────────

export interface EmailAddress {
  name: string;
  email: string;
}

export interface Attachment {
  id: string;
  filename: string;
  size: number;
  mime_type: string;
  content_id?: string;
}

export interface EmailMessage {
  id: string;
  imap_uid: number;
  imap_folder: string;
  message_id: string;
  thread_id: string;
  in_reply_to?: string;
  references?: string[];
  subject: string;
  from: EmailAddress;
  to: EmailAddress[];
  cc?: EmailAddress[];
  bcc?: EmailAddress[];
  reply_to?: EmailAddress;
  body_html?: string;
  body_text?: string;
  body_preview: string;
  attachments?: Attachment[];
  has_attachments: boolean;
  is_read: boolean;
  is_starred: boolean;
  is_draft: boolean;
  label_ids?: string[];
  sent_at?: string;
  received_at: string;
}

// ─── Thread ──────────────────────────────────────────────────────────────────

export interface Thread {
  thread_id: string;
  subject: string;
  messages: EmailMessage[];
  message_count: number;
  unread_count: number;
  participants: EmailAddress[];
  has_attachments: boolean;
  is_starred: boolean;
  label_ids?: string[];
  latest_date: string;
}

export interface ThreadListItem {
  thread_id: string;
  subject: string;
  message_count: number;
  unread_count: number;
  participants: EmailAddress[];
  body_preview: string;
  has_attachments: boolean;
  is_starred: boolean;
  label_ids?: string[];
  latest_date: string;
  message_uids: number[]; // IMAP UIDs of all messages in thread
}

export interface PaginatedThreads {
  data: ThreadListItem[];
  total: number;
  page: number;
  per_page: number;
  total_pages: number;
  has_next: boolean;
}

// ─── Compose ─────────────────────────────────────────────────────────────────

export interface ComposeAttachment {
  filename: string;
  mime_type: string;
  base64: string;
}

export interface ComposeRequest {
  to: EmailAddress[];
  cc?: EmailAddress[];
  bcc?: EmailAddress[];
  subject: string;
  body_html?: string;
  body_text?: string;
  in_reply_to?: string;
  references?: string;
  attachments?: ComposeAttachment[];
  attachment_ids?: string[];
  send_at?: string;
}

// ─── Draft ───────────────────────────────────────────────────────────────────

export interface Draft {
  id: string;
  thread_id?: string;
  to: EmailAddress[];
  cc?: EmailAddress[];
  bcc?: EmailAddress[];
  subject: string;
  body_html: string;
  body_text: string;
  attachments?: Attachment[];
  send_at?: string;
  last_saved_at: string;
  created_at: string;
  updated_at: string;
}

// ─── Folder & Label ──────────────────────────────────────────────────────────

export interface FolderStats {
  folder: string;
  total_count: number;
  unread_count: number;
}

export interface Label {
  id: string;
  user_id: string;
  name: string;
  color: string;
  icon?: string;
  is_system: boolean;
  imap_folder?: string;
  position: number;
  is_hidden: boolean;
  created_at: string;
}

// ─── Contact ─────────────────────────────────────────────────────────────────

export interface Contact {
  id: string;
  email: string;
  display_name: string;
  company?: string;
  avatar_url?: string;
  email_count: number;
  last_emailed_at?: string;
}

// ─── Search ──────────────────────────────────────────────────────────────────

export interface SearchRequest {
  q: string;
  folder?: string;
  from?: string;
  has_attach?: boolean;
  date_from?: string;
  date_to?: string;
  unread?: boolean;
  page?: number;
  per_page?: number;
}

// ─── WebSocket ───────────────────────────────────────────────────────────────

export type WSEventType =
  | "new_email"
  | "email_read"
  | "email_deleted"
  | "email_moved"
  | "unread_count"
  | "connected"
  | "ping"
  | "pong"
  | "inbox_update";

export interface WSEvent<T = unknown> {
  type: WSEventType;
  payload?: T;
  ts: string;
}

export interface WSNewEmailPayload {
  folder: string;
  unread_count: number;
  from: string;
  subject: string;
  preview: string;
}

// ─── Mark / Move Requests ────────────────────────────────────────────────────

export interface MarkReadRequest {
  folder: string;
  uids: number[];
  read: boolean;
}

export interface MarkStarredRequest {
  folder: string;
  uids: number[];
  starred: boolean;
}

export interface MoveRequest {
  src_folder: string;
  dst_folder: string;
  uids: number[];
}

export interface TrashRequest {
  folder: string;
  uids: number[];
}
