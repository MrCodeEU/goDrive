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
  preview_kind?: string;
  snippet?: string;
};

export type LoginResponse = {
  token: string;
  user: User;
};

export type ListResponse = {
  path: string;
  entries: FileEntry[];
  total: number;
  offset: number;
  limit: number;
  has_more: boolean;
  next_cursor?: string;
};

export type SearchResponse = {
  query: string;
  entries: FileEntry[];
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

export type AdminJob = {
  id: string;
  type: string;
  status: string;
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
  };
  watcher: {
    enabled: boolean;
    roots: number;
    watched_paths: number;
  };
  reconciliation: {
    enabled: boolean;
    interval_seconds: number;
    interval: string;
  };
  current_job?: AdminJob | null;
};

export type UploadProgress = {
  loaded: number;
  total: number;
  percent: number;
};

export type UploadTransport = {
  fetch?: typeof fetch;
  xhrFactory?: () => XMLHttpRequest;
  onUploadCreated?: (url: string) => void;
};

const TOKEN_KEY = "godrive_token";

let token = readToken();

export function currentToken() {
  return token;
}

export function setToken(value: string) {
  token = value;
  if (value) {
    storage()?.setItem(TOKEN_KEY, value);
  } else {
    storage()?.removeItem(TOKEN_KEY);
  }
}

type APIOptions = Omit<RequestInit, "body"> & {
  body?: BodyInit | Record<string, unknown> | null;
};

export async function api<T>(path: string, options: APIOptions = {}): Promise<T> {
  const headers = new Headers(options.headers);
  headers.set("Accept", "application/json");
  if (token) {
    headers.set("Authorization", `Bearer ${token}`);
  }

  let body = options.body;
  if (body && typeof body === "object" && !(body instanceof Blob) && !(body instanceof FormData) && !(body instanceof URLSearchParams)) {
    headers.set("Content-Type", "application/json");
    body = JSON.stringify(body);
  }

  const response = await fetch(path, {
    ...options,
    headers,
    body: body as BodyInit | undefined
  });

  if (!response.ok) {
    throw new Error(await errorMessage(response));
  }
  if (response.status === 204) {
    return {} as T;
  }
  return response.json() as Promise<T>;
}

export async function login(username: string, password: string) {
  const response = await api<LoginResponse>("/api/auth/login", {
    method: "POST",
    body: { username, password }
  });
  setToken(response.token);
  return response.user;
}

export async function logout() {
  try {
    await api("/api/auth/logout", { method: "POST" });
  } finally {
    setToken("");
  }
}

export async function me() {
  const response = await api<{ user: User }>("/api/me");
  return response.user;
}

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

export async function fetchTextPreview(path: string) {
  return api<TextPreview>(`/api/files/text?path=${encodeURIComponent(path)}`);
}

export type ExifData = {
  fields: Record<string, unknown>;
  gps_lat?: number;
  gps_lon?: number;
  has_gps: boolean;
};

export async function fetchExif(path: string) {
  return api<ExifData>(`/api/files/exif?path=${encodeURIComponent(path)}`);
}

export async function listFiles(path: string, offset = 0, limit = 500, cursor = "") {
  const params = new URLSearchParams({ path: path || "/", limit: String(limit) });
  if (cursor) {
    params.set("cursor", cursor);
  } else {
    params.set("offset", String(offset));
  }
  return api<ListResponse>(`/api/files/list?${params.toString()}`);
}

export async function listFileTree() {
  return api<{ entries: FileEntry[] }>("/api/files/tree");
}

export async function searchFiles(query: string, limit = 50) {
  const params = new URLSearchParams({
    q: query,
    limit: String(limit)
  });
  return api<SearchResponse>(`/api/files/search?${params.toString()}`);
}

export async function mkdir(path: string) {
  return api<{ entry: FileEntry }>("/api/files/mkdir", {
    method: "POST",
    body: { path }
  });
}

export async function move(from: string, to: string) {
  return api<{ entry: FileEntry }>("/api/files/move", {
    method: "POST",
    body: { from, to }
  });
}

export async function bulkDelete(paths: string[]) {
  return api<{ results: Array<{ path: string; ok: boolean; error?: string }> }>("/api/files/bulk/delete", {
    method: "POST",
    body: { paths }
  });
}

export async function bulkMove(paths: string[], targetDir: string) {
  return api<{ results: Array<{ path: string; ok: boolean; error?: string }> }>("/api/files/bulk/move", {
    method: "POST",
    body: { paths, target_dir: targetDir }
  });
}

export async function bulkDownloadBlob(paths: string[]) {
  const response = await fetch("/api/files/bulk/download", {
    method: "POST",
    headers: new Headers({
      ...authHeaderObject(),
      "Accept": "application/zip",
      "Content-Type": "application/json"
    }),
    body: JSON.stringify({ paths })
  });
  if (!response.ok) {
    throw new Error(await errorMessage(response));
  }
  return response.blob();
}

export function downloadURL(path: string) {
  return `/api/files/download?path=${encodeURIComponent(path)}`;
}

export function thumbnailURL(path: string, size: number) {
  return `/api/files/thumbnail?path=${encodeURIComponent(path)}&size=${size}`;
}

export function trashThumbnailURL(id: string, size: number) {
  return `/api/trash/${encodeURIComponent(id)}/thumbnail?size=${size}`;
}

export function rawFileURL(path: string) {
  return `/api/files/raw?path=${encodeURIComponent(path)}`;
}

export async function downloadBlob(path: string) {
  const response = await fetch(downloadURL(path), {
    headers: authHeaders()
  });
  if (!response.ok) {
    throw new Error(await errorMessage(response));
  }
  return response.blob();
}

export async function listTrash() {
  return api<{ items: TrashItem[] }>("/api/trash");
}

export async function restoreTrash(id: string) {
  return api<{ entry: FileEntry }>(`/api/trash/${encodeURIComponent(id)}/restore`, {
    method: "POST"
  });
}

export async function deleteTrash(id: string) {
  return api<{ status: string }>(`/api/trash/${encodeURIComponent(id)}`, {
    method: "DELETE"
  });
}

export async function adminStats() {
  return api<AdminStats>("/api/admin/stats");
}

export async function listAdminUsers() {
  return api<{ users: User[] }>("/api/admin/users");
}

export async function createAdminUser(input: {
  username: string;
  password: string;
  home_root: string;
  is_admin: boolean;
  disabled: boolean;
}) {
  return api<{ user: User }>("/api/admin/users", {
    method: "POST",
    body: input
  });
}

export async function updateAdminUser(id: number, input: Partial<Pick<User, "username" | "home_root" | "is_admin" | "disabled">>) {
  return api<{ user: User }>(`/api/admin/users/${id}`, {
    method: "PATCH",
    body: input
  });
}

export async function setAdminUserPassword(id: number, password: string) {
  return api<{ status: string }>(`/api/admin/users/${id}/password`, {
    method: "POST",
    body: { password }
  });
}

export async function currentAdminJob() {
  return api<{ job: AdminJob | null }>("/api/admin/jobs/current");
}

export async function startReindex(input?: { username?: string; path?: string }) {
  return api<{ job: AdminJob }>("/api/admin/jobs/reindex", {
    method: "POST",
    body: input || null
  });
}

export async function startPreviewWarmup() {
  return api<{ job: AdminJob }>("/api/admin/jobs/preview-warmup", {
    method: "POST"
  });
}

export async function cancelAdminJob() {
  return api<{ job: AdminJob }>("/api/admin/jobs/cancel", {
    method: "POST"
  });
}

export async function clearPreviewCache() {
  return api<{ status: string }>("/api/admin/preview-cache", {
    method: "DELETE"
  });
}

export interface APIKey {
  id: string;
  user_id: number;
  username: string;
  name: string;
  created_at: string;
  last_used_at?: string;
  revoked_at?: string;
}

export async function listAPIKeys() {
  return api<{ api_keys: APIKey[] }>("/api/admin/api-keys");
}

export async function createAPIKey(user_id: number, name: string) {
  return api<{ api_key: APIKey; token: string }>("/api/admin/api-keys", {
    method: "POST",
    body: JSON.stringify({ user_id, name }),
  });
}

export async function revokeAPIKey(id: string) {
  return api<{ status: string }>(`/api/admin/api-keys/${id}`, {
    method: "DELETE",
  });
}

export async function saveFileContent(path: string, content: string) {
  return api<{ path: string; modified_at: string }>(
    `/api/files/content?path=${encodeURIComponent(path)}`,
    { method: 'PATCH', body: content, headers: { 'Content-Type': 'text/plain; charset=utf-8' } }
  );
}

export async function uploadTus(file: File, targetPath: string, onProgress?: (progress: UploadProgress) => void, transport: UploadTransport = {}) {
  const doFetch = transport.fetch || fetch;
  const createResponse = await doFetch(`/api/tus?path=${encodeURIComponent(targetPath)}`, {
    method: "POST",
    headers: {
      ...authHeaderObject(),
      "Tus-Resumable": "1.0.0",
      "Upload-Length": String(file.size),
      "Upload-Metadata": `filename ${base64Utf8(file.name)}`
    }
  });
  if (!createResponse.ok) {
    throw new Error(await errorMessage(createResponse));
  }

  const location = createResponse.headers.get("Location");
  if (!location) {
    throw new Error("Upload endpoint did not return Location");
  }
  transport.onUploadCreated?.(location);

  if (file.size === 0) {
    onProgress?.({ loaded: 0, total: 0, percent: 100 });
    return "";
  }

  return uploadTusPatch(location, file, 0, onProgress, transport.xhrFactory);
}

export async function resumeUploadTus(tusUrl: string, file: File, onProgress?: (progress: UploadProgress) => void, transport: UploadTransport = {}) {
  const doFetch = transport.fetch || fetch;
  const headResponse = await doFetch(tusUrl, {
    method: "HEAD",
    headers: {
      ...authHeaderObject(),
      "Tus-Resumable": "1.0.0"
    }
  });

  if (!headResponse.ok) {
    throw new Error("upload_gone");
  }

  const startOffset = parseInt(headResponse.headers.get("Upload-Offset") || "0", 10);
  if (startOffset >= file.size) {
    onProgress?.({ loaded: file.size, total: file.size, percent: 100 });
    return headResponse.headers.get("Upload-Final-Path") || "";
  }

  return uploadTusPatch(tusUrl, file, startOffset, onProgress, transport.xhrFactory);
}

export function saveBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  document.body.append(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(url);
}

export function joinPath(parent: string, name: string) {
  const cleanParent = parent && parent !== "/" ? parent.replace(/\/+$/, "") : "";
  return `${cleanParent}/${name}`;
}

export function parentPath(path: string) {
  if (!path || path === "/") {
    return "/";
  }
  const parts = path.split("/").filter(Boolean);
  parts.pop();
  return parts.length === 0 ? "/" : `/${parts.join("/")}`;
}

export function authHeaders() {
  return new Headers(authHeaderObject());
}

function authHeaderObject(): Record<string, string> {
  return token ? { Authorization: `Bearer ${token}` } : {};
}

function readToken() {
  return storage()?.getItem(TOKEN_KEY) || "";
}

function storage() {
  return typeof localStorage === "undefined" ? null : localStorage;
}

async function errorMessage(response: Response) {
  try {
    const data = await response.json();
    return data.error || response.statusText || "Request failed";
  } catch {
    return response.statusText || "Request failed";
  }
}

function uploadTusPatch(location: string, file: File, startOffset: number, onProgress?: (progress: UploadProgress) => void, xhrFactory: () => XMLHttpRequest = () => new XMLHttpRequest()) {
  return new Promise<string>((resolve, reject) => {
    const xhr = xhrFactory();
    xhr.open("PATCH", location);
    for (const [key, value] of Object.entries(authHeaderObject())) {
      xhr.setRequestHeader(key, value);
    }
    xhr.setRequestHeader("Tus-Resumable", "1.0.0");
    xhr.setRequestHeader("Content-Type", "application/offset+octet-stream");
    xhr.setRequestHeader("Upload-Offset", String(startOffset));

    xhr.upload.onprogress = event => {
      const chunkTotal = event.lengthComputable ? event.total : file.size - startOffset;
      const loaded = event.loaded + startOffset;
      const total = chunkTotal + startOffset;
      const percent = total > 0 ? Math.min(99, Math.round((loaded / total) * 100)) : 0;
      onProgress?.({ loaded, total, percent });
    };

    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        onProgress?.({ loaded: file.size, total: file.size, percent: 100 });
        resolve(xhr.getResponseHeader("Upload-Final-Path") || "");
        return;
      }
      reject(new Error(xhrErrorMessage(xhr)));
    };
    xhr.onerror = () => reject(new Error("Upload failed"));
    xhr.onabort = () => reject(new Error("Upload cancelled"));
    xhr.send(file.slice(startOffset));
  });
}

function xhrErrorMessage(xhr: XMLHttpRequest) {
  try {
    const data = JSON.parse(xhr.responseText);
    return data.error || xhr.statusText || "Request failed";
  } catch {
    return xhr.statusText || "Request failed";
  }
}

function base64Utf8(value: string) {
  const bytes = new TextEncoder().encode(value);
  let binary = "";
  for (const byte of bytes) {
    binary += String.fromCharCode(byte);
  }
  return btoa(binary);
}
