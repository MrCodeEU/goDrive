"use strict";

const state = {
  token: localStorage.getItem("godrive_token") || "",
  user: null,
  path: "/",
  entries: [],
  uploads: [],
  previewEntry: null,
  previewURL: "",
  selectedPaths: new Set(),
  thumbnailURLs: new Map(),
  layout: localStorage.getItem("godrive_layout") || "list",
  adminPoll: 0,
};

const el = {
  authView: document.querySelector('[data-view="auth"]'),
  fileView: document.querySelector('[data-view="files"]'),
  loginForm: document.getElementById("loginForm"),
  loginError: document.getElementById("loginError"),
  username: document.getElementById("username"),
  password: document.getElementById("password"),
  userLabel: document.getElementById("userLabel"),
  logoutButton: document.getElementById("logoutButton"),
  adminButton: document.getElementById("adminButton"),
  trashButton: document.getElementById("trashButton"),
  breadcrumbs: document.getElementById("breadcrumbs"),
  upButton: document.getElementById("upButton"),
  refreshButton: document.getElementById("refreshButton"),
  newFolderButton: document.getElementById("newFolderButton"),
  fileInput: document.getElementById("fileInput"),
  browser: document.getElementById("browser"),
  fileRows: document.getElementById("fileRows"),
  selectAllCheckbox: document.getElementById("selectAllCheckbox"),
  selectionToolbar: document.getElementById("selectionToolbar"),
  selectionSummary: document.getElementById("selectionSummary"),
  bulkRenameButton: document.getElementById("bulkRenameButton"),
  bulkDownloadButton: document.getElementById("bulkDownloadButton"),
  bulkMoveButton: document.getElementById("bulkMoveButton"),
  bulkDeleteButton: document.getElementById("bulkDeleteButton"),
  clearSelectionButton: document.getElementById("clearSelectionButton"),
  listLayoutButton: document.getElementById("listLayoutButton"),
  smallGridLayoutButton: document.getElementById("smallGridLayoutButton"),
  largeGridLayoutButton: document.getElementById("largeGridLayoutButton"),
  uploadQueue: document.getElementById("uploadQueue"),
  uploadSummary: document.getElementById("uploadSummary"),
  uploadItems: document.getElementById("uploadItems"),
  clearUploadsButton: document.getElementById("clearUploadsButton"),
  nameDialog: document.getElementById("nameDialog"),
  nameForm: document.getElementById("nameForm"),
  nameDialogTitle: document.getElementById("nameDialogTitle"),
  nameInput: document.getElementById("nameInput"),
  nameCancelButton: document.getElementById("nameCancelButton"),
  trashDialog: document.getElementById("trashDialog"),
  trashRows: document.getElementById("trashRows"),
  previewDialog: document.getElementById("previewDialog"),
  previewTitle: document.getElementById("previewTitle"),
  previewMeta: document.getElementById("previewMeta"),
  previewBody: document.getElementById("previewBody"),
  previewDownloadButton: document.getElementById("previewDownloadButton"),
  previewCloseButton: document.getElementById("previewCloseButton"),
  adminDialog: document.getElementById("adminDialog"),
  adminStats: document.getElementById("adminStats"),
  reindexButton: document.getElementById("reindexButton"),
  warmPreviewsButton: document.getElementById("warmPreviewsButton"),
  refreshAdminButton: document.getElementById("refreshAdminButton"),
  jobTitle: document.getElementById("jobTitle"),
  jobStatus: document.getElementById("jobStatus"),
  jobProgress: document.getElementById("jobProgress"),
  jobMessage: document.getElementById("jobMessage"),
  toast: document.getElementById("toast"),
};

el.loginForm.addEventListener("submit", onLogin);
el.logoutButton.addEventListener("click", onLogout);
el.refreshButton.addEventListener("click", () => loadFiles(state.path));
el.newFolderButton.addEventListener("click", onNewFolder);
el.upButton.addEventListener("click", () => loadFiles(parentPath(state.path)));
el.fileInput.addEventListener("change", onUploadSelected);
el.adminButton.addEventListener("click", openAdmin);
el.trashButton.addEventListener("click", openTrash);
el.clearUploadsButton.addEventListener("click", clearFinishedUploads);
el.selectAllCheckbox.addEventListener("change", toggleSelectAll);
el.bulkRenameButton.addEventListener("click", renameSelected);
el.bulkDownloadButton.addEventListener("click", downloadSelected);
el.bulkMoveButton.addEventListener("click", moveSelected);
el.bulkDeleteButton.addEventListener("click", deleteSelected);
el.clearSelectionButton.addEventListener("click", clearSelection);
el.listLayoutButton.addEventListener("click", () => setLayout("list"));
el.smallGridLayoutButton.addEventListener("click", () => setLayout("grid-small"));
el.largeGridLayoutButton.addEventListener("click", () => setLayout("grid-large"));
el.previewCloseButton.addEventListener("click", () => el.previewDialog.close());
el.previewDownloadButton.addEventListener("click", () => {
  if (state.previewEntry) {
    downloadEntry(state.previewEntry);
  }
});
el.previewDialog.addEventListener("close", cleanupPreview);
el.reindexButton.addEventListener("click", startReindex);
el.warmPreviewsButton.addEventListener("click", startPreviewWarmup);
el.refreshAdminButton.addEventListener("click", refreshAdmin);
el.adminDialog.addEventListener("close", stopAdminPolling);

bootstrap();

async function bootstrap() {
  if (!state.token) {
    showAuth();
    return;
  }

  try {
    const data = await api("/api/me");
    state.user = data.user;
    showFiles();
    await loadFiles("/");
  } catch {
    localStorage.removeItem("godrive_token");
    state.token = "";
    showAuth();
  }
}

async function onLogin(event) {
  event.preventDefault();
  el.loginError.textContent = "";

  try {
    const data = await api("/api/auth/login", {
      method: "POST",
      body: {
        username: el.username.value.trim(),
        password: el.password.value,
      },
      auth: false,
    });
    state.token = data.token;
    state.user = data.user;
    localStorage.setItem("godrive_token", state.token);
    el.password.value = "";
    showFiles();
    await loadFiles("/");
  } catch (err) {
    el.loginError.textContent = err.message || "Sign in failed";
  }
}

async function onLogout() {
  try {
    await api("/api/auth/logout", { method: "POST" });
  } catch {
    // The local session should still be discarded if the server is unreachable.
  }
  localStorage.removeItem("godrive_token");
  state.token = "";
  state.user = null;
  state.path = "/";
  state.entries = [];
  showAuth();
}

async function loadFiles(path) {
  const data = await api(`/api/files/list?path=${encodeURIComponent(path || "/")}`);
  state.path = data.path || path || "/";
  state.entries = data.entries || [];
  state.selectedPaths.clear();
  renderPath();
  renderRows();
  renderSelection();
}

async function onNewFolder() {
  const name = await askName("New folder", "");
  if (!name) {
    return;
  }

  try {
    await api("/api/files/mkdir", {
      method: "POST",
      body: { path: joinPath(state.path, name) },
    });
    await loadFiles(state.path);
    showToast("Folder created");
  } catch (err) {
    showToast(err.message || "Could not create folder");
  }
}

async function renameEntry(entry) {
  const name = await askName("Rename", entry.name);
  if (!name || name === entry.name) {
    return;
  }

  try {
    await api("/api/files/move", {
      method: "POST",
      body: {
        from: entry.path,
        to: joinPath(parentPath(entry.path), name),
      },
    });
    await loadFiles(state.path);
    showToast("Renamed");
  } catch (err) {
    showToast(err.message || "Rename failed");
  }
}

async function deleteEntry(entry) {
  if (!window.confirm(`Move "${entry.name}" to trash?`)) {
    return;
  }

  try {
    await api(`/api/files?path=${encodeURIComponent(entry.path)}`, { method: "DELETE" });
    await loadFiles(state.path);
    showToast("Moved to trash");
  } catch (err) {
    showToast(err.message || "Delete failed");
  }
}

async function renameSelected() {
  const entries = selectedEntries();
  if (entries.length !== 1) {
    return;
  }
  await renameEntry(entries[0]);
}

async function deleteSelected() {
  const paths = selectedPaths();
  if (paths.length === 0) {
    return;
  }
  if (!window.confirm(`Move ${paths.length} selected item${paths.length === 1 ? "" : "s"} to trash?`)) {
    return;
  }

  try {
    const data = await api("/api/files/bulk/delete", {
      method: "POST",
      body: { paths },
    });
    const failed = failedResults(data.results);
    state.selectedPaths.clear();
    await loadFiles(state.path);
    showToast(failed.length === 0 ? "Moved to trash" : `${failed.length} item${failed.length === 1 ? "" : "s"} failed`);
  } catch (err) {
    showToast(err.message || "Delete failed");
  }
}

async function moveSelected() {
  const paths = selectedPaths();
  if (paths.length === 0) {
    return;
  }
  const targetDir = await askName("Move selected to folder", state.path);
  if (!targetDir) {
    return;
  }

  try {
    const data = await api("/api/files/bulk/move", {
      method: "POST",
      body: { paths, target_dir: targetDir },
    });
    const failed = failedResults(data.results);
    state.selectedPaths.clear();
    await loadFiles(state.path);
    showToast(failed.length === 0 ? "Moved" : `${failed.length} item${failed.length === 1 ? "" : "s"} failed`);
  } catch (err) {
    showToast(err.message || "Move failed");
  }
}

async function downloadSelected() {
  const paths = selectedPaths();
  if (paths.length === 0) {
    return;
  }

  try {
    const response = await fetch("/api/files/bulk/download", {
      method: "POST",
      headers: {
        ...authHeaders(),
        Accept: "application/zip",
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ paths }),
    });
    if (!response.ok) {
      throw new Error(await errorMessage(response));
    }
    const blob = await response.blob();
    saveBlob(blob, filenameFromDisposition(response.headers.get("Content-Disposition")) || "godrive-selection.zip");
  } catch (err) {
    showToast(err.message || "Download failed");
  }
}

async function downloadEntry(entry) {
  try {
    const response = await fetch(`/api/files/download?path=${encodeURIComponent(entry.path)}`, {
      headers: authHeaders(),
    });
    if (!response.ok) {
      throw new Error(await errorMessage(response));
    }
    saveBlob(await response.blob(), entry.name);
  } catch (err) {
    showToast(err.message || "Download failed");
  }
}

async function previewEntry(entry) {
  if (!canPreview(entry)) {
    await downloadEntry(entry);
    return;
  }

  cleanupPreview();
  state.previewEntry = entry;
  el.previewTitle.textContent = entry.name;
  el.previewMeta.textContent = previewMeta(entry);
  el.previewBody.replaceChildren(loadingPreview());
  el.previewDialog.showModal();

  try {
    switch (entry.preview_kind) {
      case "image":
        await previewBlob(entry, "image");
        break;
      case "video":
        await previewBlob(entry, "video");
        break;
      case "pdf":
        await previewBlob(entry, "pdf");
        break;
      case "text":
      case "markdown":
        await previewText(entry);
        break;
      case "3d":
        renderUnsupportedPreview("3D preview is not implemented yet.");
        break;
      default:
        renderUnsupportedPreview("Preview is not available for this file type.");
        break;
    }
  } catch (err) {
    renderUnsupportedPreview(err.message || "Preview failed.");
  }
}

async function previewBlob(entry, kind) {
  const response = await fetch(`/api/files/download?path=${encodeURIComponent(entry.path)}`, {
    headers: authHeaders(),
  });
  if (!response.ok) {
    throw new Error(await errorMessage(response));
  }

  const blob = await response.blob();
  state.previewURL = URL.createObjectURL(blob);

  let node;
  if (kind === "image") {
    node = document.createElement("img");
    node.alt = entry.name;
    node.addEventListener("error", () => {
      renderUnsupportedPreview("This image format is not supported by this browser yet.");
    });
  } else if (kind === "video") {
    node = document.createElement("video");
    node.controls = true;
    node.playsInline = true;
  } else {
    node = document.createElement("iframe");
    node.title = entry.name;
  }
  node.src = state.previewURL;
  el.previewBody.replaceChildren(node);
}

async function previewText(entry) {
  const data = await api(`/api/files/text?path=${encodeURIComponent(entry.path)}`);
  const pre = document.createElement("pre");
  pre.className = "text-preview";
  pre.textContent = data.truncated
    ? `${data.content}\n\n[Preview truncated at ${formatBytes(data.max_bytes)}]`
    : data.content;
  el.previewBody.replaceChildren(pre);
}

function renderUnsupportedPreview(message) {
  const box = document.createElement("div");
  box.className = "unsupported-preview";
  const title = document.createElement("h2");
  title.textContent = "No preview";
  const text = document.createElement("p");
  text.className = "muted";
  text.textContent = message;
  box.append(title, text);
  el.previewBody.replaceChildren(box);
}

function loadingPreview() {
  const box = document.createElement("div");
  box.className = "unsupported-preview";
  box.textContent = "Loading preview...";
  return box;
}

function cleanupPreview() {
  if (state.previewURL) {
    URL.revokeObjectURL(state.previewURL);
  }
  state.previewURL = "";
  state.previewEntry = null;
  el.previewBody.replaceChildren();
}

async function onUploadSelected(event) {
  const files = Array.from(event.target.files || []);
  event.target.value = "";
  if (files.length === 0) {
    return;
  }

  for (const file of files) {
    await uploadFile(file);
  }
  await loadFiles(state.path);
}

async function uploadFile(file) {
  const item = {
    id: crypto.randomUUID ? crypto.randomUUID() : `${Date.now()}-${Math.random()}`,
    name: file.name,
    status: "queued",
  };
  state.uploads.push(item);
  renderUploads();

  try {
    item.status = "creating";
    renderUploads();

    const createResponse = await fetch(`/api/tus?path=${encodeURIComponent(state.path)}`, {
      method: "POST",
      headers: {
        ...authHeaders(),
        "Tus-Resumable": "1.0.0",
        "Upload-Length": String(file.size),
        "Upload-Metadata": `filename ${base64Utf8(file.name)}`,
      },
    });
    if (!createResponse.ok) {
      throw new Error(await errorMessage(createResponse));
    }

    const location = createResponse.headers.get("Location");
    if (!location) {
      throw new Error("Upload endpoint did not return a Location header");
    }

    if (file.size > 0) {
      item.status = "uploading";
      renderUploads();

      const patchResponse = await fetch(location, {
        method: "PATCH",
        headers: {
          ...authHeaders(),
          "Tus-Resumable": "1.0.0",
          "Content-Type": "application/offset+octet-stream",
          "Upload-Offset": "0",
        },
        body: file,
      });
      if (!patchResponse.ok) {
        throw new Error(await errorMessage(patchResponse));
      }
    }

    item.status = "done";
    renderUploads();
  } catch (err) {
    item.status = "failed";
    item.error = err.message || "Upload failed";
    renderUploads();
  }
}

async function openTrash() {
  try {
    const data = await api("/api/trash");
    renderTrash(data.items || []);
    el.trashDialog.showModal();
  } catch (err) {
    showToast(err.message || "Could not load trash");
  }
}

async function restoreTrash(id) {
  try {
    await api(`/api/trash/${encodeURIComponent(id)}/restore`, { method: "POST" });
    await openTrash();
    await loadFiles(state.path);
    showToast("Restored");
  } catch (err) {
    showToast(err.message || "Restore failed");
  }
}

async function deleteTrash(id) {
  if (!window.confirm("Permanently delete this item?")) {
    return;
  }

  try {
    await api(`/api/trash/${encodeURIComponent(id)}`, { method: "DELETE" });
    await openTrash();
    showToast("Deleted");
  } catch (err) {
    showToast(err.message || "Delete failed");
  }
}

async function openAdmin() {
  el.adminDialog.showModal();
  await refreshAdmin();
  startAdminPolling();
}

async function refreshAdmin() {
  try {
    const stats = await api("/api/admin/stats");
    renderAdminStats(stats);
    renderJob(stats.current_job);
  } catch (err) {
    showToast(err.message || "Could not load admin stats");
  }
}

async function startReindex() {
  try {
    const data = await api("/api/admin/jobs/reindex", { method: "POST" });
    renderJob(data.job);
    startAdminPolling();
    showToast("Reindex started");
  } catch (err) {
    showToast(err.message || "Could not start reindex");
  }
}

async function startPreviewWarmup() {
  try {
    const data = await api("/api/admin/jobs/preview-warmup", { method: "POST" });
    renderJob(data.job);
    startAdminPolling();
    showToast("Thumbnail generation started");
  } catch (err) {
    showToast(err.message || "Could not start thumbnail generation");
  }
}

function startAdminPolling() {
  stopAdminPolling();
  state.adminPoll = window.setInterval(async () => {
    if (!el.adminDialog.open) {
      stopAdminPolling();
      return;
    }
    try {
      const data = await api("/api/admin/jobs/current");
      renderJob(data.job);
      if (!data.job || data.job.status !== "running") {
        await refreshAdmin();
        stopAdminPolling();
      }
    } catch {
      stopAdminPolling();
    }
  }, 1500);
}

function stopAdminPolling() {
  if (state.adminPoll) {
    window.clearInterval(state.adminPoll);
    state.adminPoll = 0;
  }
}

function renderAdminStats(stats) {
  const cards = [
    ["Users", `${stats.users?.total ?? 0}`, `${stats.users?.disabled ?? 0} disabled`],
    ["Indexed files", `${stats.index?.indexed_files ?? 0}`, formatBytes(stats.index?.indexed_bytes ?? 0)],
    ["Indexed folders", `${stats.index?.indexed_directories ?? 0}`, "from last reindex"],
    ["Preview candidates", `${stats.index?.preview_candidates ?? 0}`, "image, video, PDF"],
    ["Preview cache", `${stats.preview_cache?.files ?? 0}`, formatBytes(stats.preview_cache?.bytes ?? 0)],
    ["Trash", `${stats.trash?.items ?? 0}`, formatBytes(stats.trash?.bytes ?? 0)],
  ];

  el.adminStats.replaceChildren();
  for (const [label, value, detail] of cards) {
    const card = document.createElement("div");
    card.className = "stat-card";
    const title = document.createElement("span");
    title.className = "muted";
    title.textContent = label;
    const number = document.createElement("strong");
    number.textContent = value;
    const sub = document.createElement("span");
    sub.className = "muted";
    sub.textContent = detail;
    card.append(title, number, sub);
    el.adminStats.append(card);
  }
}

function renderJob(job) {
  const running = job && job.status === "running";
  el.reindexButton.disabled = running;
  el.warmPreviewsButton.disabled = running;

  if (!job) {
    el.jobTitle.textContent = "No active job";
    el.jobStatus.textContent = "";
    el.jobProgress.max = 1;
    el.jobProgress.value = 0;
    el.jobMessage.textContent = "Run a reindex after external filesystem changes, then warm thumbnails for grid browsing.";
    return;
  }

  el.jobTitle.textContent = humanJobType(job.type);
  el.jobStatus.textContent = job.status;
  if (job.total_known && job.total > 0) {
    el.jobProgress.max = job.total;
    el.jobProgress.value = Math.min(job.done || 0, job.total);
  } else if (job.status === "running") {
    el.jobProgress.removeAttribute("value");
    el.jobProgress.max = 1;
  } else {
    el.jobProgress.max = Math.max(job.done || 1, 1);
    el.jobProgress.value = job.done || 0;
  }

  const parts = [];
  if (job.total_known) {
    parts.push(job.total > 0 ? `${job.done || 0}/${job.total}` : "no items");
  } else {
    parts.push(`${job.done || 0} indexed`);
  }
  if (job.failed) {
    parts.push(`${job.failed} failed`);
  }
  if (job.message) {
    parts.push(job.message);
  }
  el.jobMessage.textContent = parts.join(" · ");
}

function humanJobType(type) {
  switch (type) {
    case "reindex":
      return "Full reindex";
    case "preview_warmup":
      return "Thumbnail generation";
    default:
      return type || "Job";
  }
}

function showAuth() {
  el.authView.classList.remove("hidden");
  el.fileView.classList.add("hidden");
  el.adminButton.classList.add("hidden");
  el.username.focus();
}

function showFiles() {
  el.authView.classList.add("hidden");
  el.fileView.classList.remove("hidden");
  el.userLabel.textContent = state.user ? state.user.username : "";
  el.adminButton.classList.toggle("hidden", !state.user?.is_admin);
}

function renderPath() {
  el.upButton.disabled = state.path === "/";
  el.breadcrumbs.replaceChildren();

  const root = document.createElement("button");
  root.type = "button";
  root.textContent = "Home";
  root.addEventListener("click", () => loadFiles("/"));
  el.breadcrumbs.append(root);

  const parts = state.path.split("/").filter(Boolean);
  let current = "";
  for (const part of parts) {
    current = `${current}/${part}`;
    const target = current;
    const separator = document.createElement("span");
    separator.textContent = "/";
    el.breadcrumbs.append(separator);

    const button = document.createElement("button");
    button.type = "button";
    button.textContent = part;
    button.addEventListener("click", () => loadFiles(target));
    el.breadcrumbs.append(button);
  }
}

function renderRows() {
  cleanupThumbnails();
  applyLayout();
  el.fileRows.replaceChildren();

  if (state.entries.length === 0) {
    const empty = document.createElement("div");
    empty.className = "empty-state";
    empty.textContent = "This folder is empty.";
    el.fileRows.append(empty);
    return;
  }

  for (const entry of state.entries) {
    const row = document.createElement("div");
    row.className = "file-row";
    row.role = "listitem";
    row.dataset.path = entry.path;

    const selectCell = document.createElement("label");
    selectCell.className = "select-cell";
    const checkbox = document.createElement("input");
    checkbox.type = "checkbox";
    checkbox.checked = state.selectedPaths.has(entry.path);
    checkbox.setAttribute("aria-label", `Select ${entry.name}`);
    checkbox.addEventListener("change", () => {
      setSelected(entry.path, checkbox.checked);
    });
    selectCell.append(checkbox);
    row.append(selectCell);

    const main = document.createElement("div");
    main.className = "file-main";

    const visual = fileVisual(entry);

    const name = document.createElement("button");
    name.type = "button";
    name.className = "file-name";
    name.textContent = entry.name;
    name.title = entry.name;
    name.addEventListener("click", () => {
      if (entry.type === "dir") {
        loadFiles(entry.path);
      } else if (canPreview(entry)) {
        previewEntry(entry);
      } else {
        downloadEntry(entry);
      }
    });

    main.append(visual, name);
    row.append(main);

    const size = document.createElement("span");
    size.className = "muted";
    size.textContent = entry.type === "dir" ? "" : formatBytes(entry.size);
    row.append(size);

    const modified = document.createElement("span");
    modified.className = "muted";
    modified.textContent = formatDate(entry.modified_at);
    row.append(modified);

    const actions = document.createElement("div");
    actions.className = "row-actions";

    if (entry.type !== "dir") {
      if (canPreview(entry)) {
        actions.append(actionButton("Preview", () => previewEntry(entry)));
      }
      actions.append(actionButton("Download", () => downloadEntry(entry)));
    }
    actions.append(actionButton("Rename", () => renameEntry(entry)));
    actions.append(actionButton("Delete", () => deleteEntry(entry), "danger"));
    row.append(actions);

    el.fileRows.append(row);
  }
  renderSelection();
}

function fileVisual(entry) {
  if (state.layout !== "list" && canThumbnail(entry)) {
    const frame = document.createElement("span");
    frame.className = "thumb-frame";
    frame.textContent = iconText(entry);

    const img = document.createElement("img");
    img.alt = "";
    img.loading = "lazy";
    frame.append(img);
    loadThumbnail(entry, img, frame);
    return frame;
  }

  const icon = document.createElement("span");
  icon.className = "file-icon";
  icon.textContent = iconText(entry);
  return icon;
}

async function loadThumbnail(entry, img, frame) {
  const size = state.layout === "grid-large" ? 420 : 240;
  try {
    const response = await fetch(`/api/files/thumbnail?path=${encodeURIComponent(entry.path)}&size=${size}`, {
      headers: authHeaders(),
    });
    if (!response.ok) {
      throw new Error("thumbnail unavailable");
    }
    const blob = await response.blob();
    const url = URL.createObjectURL(blob);
    if (!document.body.contains(img)) {
      URL.revokeObjectURL(url);
      return;
    }
    state.thumbnailURLs.set(entry.path, url);
    img.src = url;
    frame.classList.add("has-thumb");
  } catch {
    frame.classList.add("thumb-missing");
  }
}

function cleanupThumbnails() {
  for (const url of state.thumbnailURLs.values()) {
    URL.revokeObjectURL(url);
  }
  state.thumbnailURLs.clear();
}

function canThumbnail(entry) {
  return ["image", "video", "pdf"].includes(entry.preview_kind);
}

function setSelected(path, selected) {
  if (selected) {
    state.selectedPaths.add(path);
  } else {
    state.selectedPaths.delete(path);
  }
  renderSelection();
}

function toggleSelectAll() {
  const checked = el.selectAllCheckbox.checked;
  for (const entry of state.entries) {
    state.selectedPaths[checked ? "add" : "delete"](entry.path);
  }
  renderRows();
}

function clearSelection() {
  state.selectedPaths.clear();
  renderRows();
}

function renderSelection() {
  const count = state.selectedPaths.size;
  el.selectionToolbar.classList.toggle("hidden", count === 0);
  el.selectionSummary.textContent = `${count} selected`;
  el.bulkRenameButton.disabled = count !== 1;

  const selectableCount = state.entries.length;
  el.selectAllCheckbox.checked = selectableCount > 0 && count === selectableCount;
  el.selectAllCheckbox.indeterminate = count > 0 && count < selectableCount;

  for (const checkbox of el.fileRows.querySelectorAll(".select-cell input")) {
    const row = checkbox.closest(".file-row");
    const checked = row ? state.selectedPaths.has(row.dataset.path) : false;
    checkbox.checked = checked;
    if (row) {
      row.classList.toggle("is-selected", checked);
    }
  }
}

function selectedPaths() {
  return Array.from(state.selectedPaths);
}

function selectedEntries() {
  const selected = state.selectedPaths;
  return state.entries.filter((entry) => selected.has(entry.path));
}

function applyLayout() {
  el.browser.classList.toggle("layout-list", state.layout === "list");
  el.browser.classList.toggle("layout-grid-small", state.layout === "grid-small");
  el.browser.classList.toggle("layout-grid-large", state.layout === "grid-large");
  el.listLayoutButton.setAttribute("aria-pressed", String(state.layout === "list"));
  el.smallGridLayoutButton.setAttribute("aria-pressed", String(state.layout === "grid-small"));
  el.largeGridLayoutButton.setAttribute("aria-pressed", String(state.layout === "grid-large"));
}

function setLayout(layout) {
  state.layout = layout;
  localStorage.setItem("godrive_layout", layout);
  renderRows();
}

function renderUploads() {
  el.uploadQueue.classList.toggle("hidden", state.uploads.length === 0);
  el.uploadItems.replaceChildren();
  el.uploadSummary.textContent = uploadSummaryText();

  for (const upload of state.uploads) {
    const row = document.createElement("div");
    row.className = "upload-item";

    const name = document.createElement("div");
    name.className = "upload-name";
    name.textContent = upload.name;
    name.title = upload.error || upload.name;

    const status = document.createElement("div");
    status.className = `upload-status status-${upload.status}`;
    status.textContent = upload.status;

    row.append(name, status);
    el.uploadItems.append(row);
  }
}

function uploadSummaryText() {
  const total = state.uploads.length;
  if (total === 0) {
    return "Uploads";
  }
  const active = state.uploads.filter((upload) => upload.status === "queued" || upload.status === "creating" || upload.status === "uploading").length;
  const failed = state.uploads.filter((upload) => upload.status === "failed").length;
  if (failed > 0) {
    return `Uploads (${failed} failed)`;
  }
  if (active > 0) {
    return `Uploads (${active} active)`;
  }
  return `Uploads (${total} done)`;
}

function renderTrash(items) {
  el.trashRows.replaceChildren();

  if (items.length === 0) {
    const empty = document.createElement("div");
    empty.className = "empty-state";
    empty.textContent = "Trash is empty.";
    el.trashRows.append(empty);
    return;
  }

  for (const item of items) {
    const row = document.createElement("div");
    row.className = "trash-row";

    const path = document.createElement("div");
    path.className = "trash-path";
    path.textContent = item.original_path;
    path.title = item.original_path;

    const actions = document.createElement("div");
    actions.className = "row-actions";
    actions.append(actionButton("Restore", () => restoreTrash(item.id)));
    actions.append(actionButton("Delete", () => deleteTrash(item.id), "danger"));

    row.append(path, actions);
    el.trashRows.append(row);
  }
}

function actionButton(label, onClick, className = "") {
  const button = document.createElement("button");
  button.type = "button";
  button.textContent = label;
  if (className) {
    button.className = className;
  }
  button.addEventListener("click", onClick);
  return button;
}

function failedResults(results) {
  return (results || []).filter((result) => !result.ok);
}

function saveBlob(blob, filename) {
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  document.body.append(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(url);
}

function filenameFromDisposition(disposition) {
  if (!disposition) {
    return "";
  }
  const match = disposition.match(/filename="([^"]+)"/i) || disposition.match(/filename=([^;]+)/i);
  return match ? match[1].trim() : "";
}

function clearFinishedUploads() {
  state.uploads = state.uploads.filter((upload) => upload.status !== "done");
  renderUploads();
}

function canPreview(entry) {
  return ["image", "video", "pdf", "text", "markdown", "3d"].includes(entry.preview_kind);
}

function previewMeta(entry) {
  const parts = [];
  if (entry.preview_kind) {
    parts.push(entry.preview_kind.toUpperCase());
  }
  if (entry.type !== "dir") {
    parts.push(formatBytes(entry.size));
  }
  const date = formatDate(entry.modified_at);
  if (date) {
    parts.push(date);
  }
  return parts.join(" · ");
}

function askName(title, initial) {
  if (!el.nameDialog.showModal) {
    const value = window.prompt(title, initial || "");
    return Promise.resolve(value ? value.trim() : "");
  }

  el.nameDialogTitle.textContent = title;
  el.nameInput.value = initial || "";

  return new Promise((resolve) => {
    const cleanup = () => {
      el.nameForm.removeEventListener("submit", onSubmit);
      el.nameCancelButton.removeEventListener("click", onCancel);
      el.nameDialog.removeEventListener("cancel", onCancel);
    };
    const onSubmit = (event) => {
      event.preventDefault();
      const value = el.nameInput.value.trim();
      cleanup();
      el.nameDialog.close();
      resolve(value);
    };
    const onCancel = () => {
      cleanup();
      el.nameDialog.close();
      resolve("");
    };

    el.nameForm.addEventListener("submit", onSubmit);
    el.nameCancelButton.addEventListener("click", onCancel);
    el.nameDialog.addEventListener("cancel", onCancel);
    el.nameDialog.showModal();
    el.nameInput.focus();
    el.nameInput.select();
  });
}

async function api(path, options = {}) {
  const headers = {
    Accept: "application/json",
    ...(options.headers || {}),
  };
  if (options.body !== undefined) {
    headers["Content-Type"] = "application/json";
  }
  if (options.auth !== false) {
    Object.assign(headers, authHeaders());
  }

  const response = await fetch(path, {
    method: options.method || "GET",
    headers,
    body: options.body === undefined ? undefined : JSON.stringify(options.body),
  });

  if (!response.ok) {
    throw new Error(await errorMessage(response));
  }
  if (response.status === 204) {
    return {};
  }
  return response.json();
}

function authHeaders() {
  if (!state.token) {
    return {};
  }
  return { Authorization: `Bearer ${state.token}` };
}

async function errorMessage(response) {
  try {
    const data = await response.json();
    return data.error || response.statusText || "Request failed";
  } catch {
    return response.statusText || "Request failed";
  }
}

function joinPath(parent, name) {
  const cleanParent = parent && parent !== "/" ? parent.replace(/\/+$/, "") : "";
  return `${cleanParent}/${name}`;
}

function parentPath(path) {
  if (!path || path === "/") {
    return "/";
  }
  const parts = path.split("/").filter(Boolean);
  parts.pop();
  return parts.length === 0 ? "/" : `/${parts.join("/")}`;
}

function formatBytes(size) {
  if (!Number.isFinite(size) || size < 0) {
    return "";
  }
  if (size < 1024) {
    return `${size} B`;
  }
  const units = ["KB", "MB", "GB", "TB"];
  let value = size / 1024;
  for (const unit of units) {
    if (value < 1024 || unit === "TB") {
      return `${value.toFixed(value < 10 ? 1 : 0)} ${unit}`;
    }
    value /= 1024;
  }
  return `${size} B`;
}

function formatDate(value) {
  if (!value) {
    return "";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "";
  }
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

function iconText(entry) {
  if (entry.type === "dir") {
    return "DIR";
  }
  switch (entry.preview_kind) {
    case "image":
      return "IMG";
    case "video":
      return "VID";
    case "pdf":
      return "PDF";
    case "text":
    case "markdown":
      return "TXT";
    case "3d":
      return "3D";
    default:
      return "FILE";
  }
}

function base64Utf8(value) {
  const bytes = new TextEncoder().encode(value);
  let binary = "";
  for (const byte of bytes) {
    binary += String.fromCharCode(byte);
  }
  return btoa(binary);
}

let toastTimer = 0;

function showToast(message) {
  window.clearTimeout(toastTimer);
  el.toast.textContent = message;
  el.toast.classList.remove("hidden");
  toastTimer = window.setTimeout(() => {
    el.toast.classList.add("hidden");
  }, 2600);
}
