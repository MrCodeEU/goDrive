// Generated from docs/openapi.yaml by scripts/generate-openapi-types.rb.
// Do not edit manually. Run `make api-types` after changing the API contract.

export type Error = {
  error: string;
};

export type PublicConfig = {
  demo_mode: boolean;
  demo_user?: string;
  demo_password?: string;
};

export type LoginRequest = {
  username: string;
  password: string;
};

export type LoginResponse = {
  token: string;
  csrf_token?: string;
  session_id?: string;
  expires_at?: string;
  user: User;
};

export type User = {
  id: number;
  username: string;
  is_admin: boolean;
  disabled: boolean;
  home_root: string;
};

export type FileEntry = {
  name: string;
  path: string;
  type: "file" | "dir";
  size: number;
  modified_at: string;
  mime_type?: string;
  preview_kind?: "image" | "raw" | "video" | "pdf" | "text" | "markdown" | "office" | "3d";
  snippet?: string;
};

export type ListResponse = {
  path: string;
  entries: FileEntry[];
  total: number;
  offset: number;
  limit: number;
  has_more: boolean;
  next_cursor?: string;
  source?: "index";
};

export type SearchResponse = {
  query: string;
  entries: FileEntry[];
};

export type TextPreview = {
  path: string;
  name: string;
  size: number;
  truncated: boolean;
  max_bytes: number;
  content: string;
  mime_type: string;
  modified_at: string;
};

export type ExifData = {
  fields: Record<string, unknown>;
  gps_lat?: number;
  gps_lon?: number;
  has_gps: boolean;
};

export type TrashItem = {
  id: string;
  user_id: number;
  original_path: string;
  original_name: string;
  is_dir: boolean;
  size: number;
  deleted_at: string;
};

export type BulkResult = {
  path: string;
  ok: boolean;
  error?: string;
  entry?: FileEntry;
};

export type CreateUserRequest = {
  username: string;
  password: string;
  home_root: string;
  is_admin?: boolean;
  disabled?: boolean;
};

export type UpdateUserRequest = {
  username?: string;
  home_root?: string;
  is_admin?: boolean;
  disabled?: boolean;
};

export type AdminJob = {
  id: string;
  type: string;
  status: "running" | "done" | "failed" | "canceled";
  started_at: string;
  finished_at?: string;
  total: number;
  total_known: boolean;
  done: number;
  failed: number;
  deleted?: number;
  user?: string;
  scope?: string;
  cancelable?: boolean;
  message: string;
};

export type AdminJobResponseNullable = {
  job: AdminJob | null;
};

export type PreviewToolStatus = {
  name: string;
  purpose: string;
  available: boolean;
  path?: string;
  error?: string;
};

export type AdminStats = {
  users: {
  total: number;
  disabled: number;
};
  index: {
  indexed_files: number;
  indexed_directories: number;
  indexed_bytes: number;
  preview_candidates: number;
};
  trash: {
  items: number;
  bytes: number;
};
  preview_cache: {
  files: number;
  bytes: number;
};
  preview: {
  workers: number;
  sizes: number[];
  tools: PreviewToolStatus[];
};
  watcher: {
  enabled: boolean;
  roots: number;
  watched_paths: number;
  events: number;
  pending: number;
  errors: number;
  last_error?: string;
  last_error_at?: string;
  last_event_at?: string;
  last_index_at?: string;
  needs_rescan: boolean;
};
  reconciliation: {
  enabled: boolean;
  interval_seconds: number;
  interval: string;
};
  current_job?: AdminJob | null;
};

export type APIKey = {
  id: string;
  user_id: number;
  username?: string;
  name: string;
  created_at: string;
  last_used_at?: string;
  revoked_at?: string;
};

export type Webhook = {
  id: string;
  url: string;
  events: string[];
  description: string;
  created_at: string;
  updated_at: string;
};

export type CreateWebhookRequest = {
  url: string;
  events?: string[];
  description?: string;
};
