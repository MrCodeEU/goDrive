<script lang="ts">
  import { onMount } from "svelte";
  import {
    Filemanager,
    Willow,
    getMenuOptions,
    type FilePreview,
    type IApi,
    type IExtraInfo,
    type IFileMenuOption,
    type IParsedEntity,
    type TMode,
    type TContextMenuType
  } from "@svar-ui/svelte-filemanager";
  import {
    adminStats,
    bulkDelete,
    bulkMove,
    currentAdminJob,
    currentToken,
    deleteTrash,
    downloadBlob,
    joinPath,
    listTrash,
    listFiles,
    login,
    logout,
    me,
    mkdir,
    move,
    parentPath,
    restoreTrash,
    saveBlob,
    startPreviewWarmup,
    startReindex,
    thumbnailURL,
    uploadTus,
    type AdminJob,
    type AdminStats,
    type TrashItem,
    type User
  } from "./lib/api";
  import { toSvarFiles, type SvarFile } from "./lib/svar";

  const previewKinds = new Set(["image", "video", "pdf"]);
  const unsupportedMenuItems = new Set(["add-file", "copy", "paste"]);
  const viewModeKey = "godrive_view_mode";

  type UploadQueueItem = {
    id: string;
    file: File;
    name: string;
    size: number;
    targetPath: string;
    progress: number;
    status: "queued" | "uploading" | "done" | "error";
    error?: string;
    finalPath?: string;
  };

  let user: User | null = null;
  let username = "admin";
  let password = "";
  let loading = true;
  let error = "";
  let busy = "";
  let initialData: SvarFile[] = [];
  let fileApi: IApi | null = null;
  let managerKey = 0;
  let trashOpen = false;
  let trashItems: TrashItem[] = [];
  let trashBusy = "";
  let adminOpen = false;
  let stats: AdminStats | null = null;
  let adminJob: AdminJob | null = null;
  let adminPoll: ReturnType<typeof window.setInterval> | null = null;
  let viewerFile: IParsedEntity | null = null;
  let uploadInput: HTMLInputElement | null = null;
  let uploadQueue: UploadQueueItem[] = [];
  let uploadQueueCollapsed = false;
  let viewMode: TMode = loadViewMode();

  onMount(() => {
    const onPopState = () => {
      if (fileApi) {
        void navigateToURLPath(fileApi);
      }
    };
    window.addEventListener("popstate", onPopState);
    void (async () => {
      if (!currentToken()) {
        loading = false;
        return;
      }
      await restoreSession();
    })();
    return () => {
      window.removeEventListener("popstate", onPopState);
      stopAdminPolling();
    };
  });

  async function restoreSession() {
    loading = true;
    error = "";
    try {
      user = await me();
      await loadInitialFiles();
    } catch (err) {
      error = messageFromError(err);
    } finally {
      loading = false;
    }
  }

  async function submitLogin() {
    loading = true;
    error = "";
    try {
      user = await login(username, password);
      password = "";
      await loadInitialFiles();
    } catch (err) {
      error = messageFromError(err);
    } finally {
      loading = false;
    }
  }

  async function submitLogout() {
    await logout();
    user = null;
    initialData = [];
    fileApi = null;
    managerKey += 1;
  }

  async function loadInitialFiles() {
    initialData = await loadFolder("/");
    managerKey += 1;
  }

  async function loadFolder(path: string) {
    const response = await listFiles(path || "/");
    return toSvarFiles(response.entries);
  }

  function initFilemanager(api: IApi) {
    fileApi = api;

    api.on("set-path", event => {
      syncURLPath(event.id);
    });

    api.on("set-mode", event => {
      viewMode = event.mode;
      localStorage.setItem(viewModeKey, event.mode);
    });

    api.intercept("request-data", async event => {
      await runAction("Loading folder", async () => {
        await refreshFolder(api, event.id);
      });
      return false;
    });

    api.intercept("create-file", async event => {
      await runAction("Saving changes", async () => {
        if (event.file.file) {
          await uploadFiles([event.file.file], event.parent, api);
        } else if (event.file.type === "folder") {
          await mkdir(joinPath(event.parent, event.file.name));
          await refreshFolder(api, event.parent);
        } else {
          throw new Error("Creating empty files is not supported yet");
        }
      });
      return false;
    });

    api.intercept("rename-file", async event => {
      await runAction("Renaming item", async () => {
        const destination = joinPath(parentPath(event.id), event.name);
        await move(event.id, destination);
        await refreshFolder(api, parentPath(event.id));
      });
      return false;
    });

    api.intercept("delete-files", async event => {
      await runAction("Moving to trash", async () => {
        const parents = uniqueParents(event.ids);
        const response = await bulkDelete(event.ids);
        assertBulkSuccess(response.results);
        await Promise.all(parents.map(parent => refreshFolder(api, parent)));
        await api.exec("select-file", {});
      });
      return false;
    });

    api.intercept("move-files", async event => {
      await runAction("Moving items", async () => {
        const parents = uniqueParents(event.ids);
        const response = await bulkMove(event.ids, event.target);
        assertBulkSuccess(response.results);
        await Promise.all([...new Set([...parents, event.target])].map(parent => refreshFolder(api, parent)));
        await api.exec("select-file", {});
      });
      return false;
    });

    api.intercept("copy-files", () => {
      error = "Copy is not wired to the backend yet.";
      return false;
    });

    api.intercept("download-file", async event => {
      await runAction("Preparing download", async () => {
        const file = api.getFile(event.id);
        const blob = await downloadBlob(event.id);
        saveBlob(blob, file?.name || basename(event.id));
      });
      return false;
    });

    api.intercept("open-file", async event => {
      const file = api.getFile(event.id);
      if (file?.previewKind === "image") {
        openViewer(file);
      } else if (file?.previewKind && previewKinds.has(file.previewKind)) {
        await api.exec("show-preview", { mode: true });
      } else {
        await api.exec("download-file", { id: event.id });
      }
      return false;
    });

    void navigateToURLPath(api);
  }

  async function refreshFolder(api: IApi, path: string) {
    const data = await loadFolder(path);
    await api.exec("provide-data", { id: path || "/", data });
  }

  async function runAction(label: string, action: () => Promise<void>) {
    busy = label;
    error = "";
    try {
      await action();
    } catch (err) {
      error = messageFromError(err);
    } finally {
      busy = "";
    }
  }

  async function uploadFiles(files: File[], targetPath = currentFolderPath(), api = fileApi) {
    if (!api || files.length === 0) {
      return;
    }
    const queued = files.map(file => createUploadQueueItem(file, targetPath));
    uploadQueue = [...queued, ...uploadQueue].slice(0, 100);
    uploadQueueCollapsed = false;
    for (let index = 0; index < files.length; index++) {
      await uploadOne(files[index], queued[index], targetPath);
    }
    await refreshFolder(api, targetPath);
  }

  async function uploadOne(file: File, item: UploadQueueItem, targetPath: string) {
    setUploadItem(item.id, { status: "uploading", progress: 0, error: "" });
    busy = `Uploading ${file.name}`;
    try {
      const finalPath = await uploadTus(file, targetPath, progress => setUploadItem(item.id, { progress: progress.percent }));
      setUploadItem(item.id, { status: "done", progress: 100, finalPath: finalPath || joinPath(targetPath, file.name) });
    } catch (err) {
      setUploadItem(item.id, { status: "error", error: messageFromError(err) });
    }
  }

  async function uploadSelectedFiles(event: Event) {
    const input = event.currentTarget as HTMLInputElement;
    const files = Array.from(input.files || []);
    input.value = "";
    await runAction("Uploading files", async () => {
      await uploadFiles(files);
    });
  }

  function currentFolderPath() {
    if (!fileApi) {
      return "/";
    }
    const state = fileApi.getState();
    const panel = state.panels?.[state.activePanel ?? 0];
    return panel?.path || "/";
  }

  function createUploadQueueItem(file: File, targetPath: string): UploadQueueItem {
    return {
      id: crypto.randomUUID(),
      file,
      name: file.name,
      size: file.size,
      targetPath,
      progress: 0,
      status: "queued"
    };
  }

  function setUploadItem(id: string, patch: Partial<UploadQueueItem>) {
    uploadQueue = uploadQueue.map(item => (item.id === id ? { ...item, ...patch } : item));
  }

  function clearCompletedUploads() {
    uploadQueue = uploadQueue.filter(item => item.status !== "done");
  }

  async function retryUpload(item: UploadQueueItem) {
    uploadQueueCollapsed = false;
    await uploadOne(item.file, item, item.targetPath);
    await refreshCurrentFolder();
  }

  function uploadSummary() {
    let active = 0, failed = 0;
    for (const item of uploadQueue) {
      if (item.status === "uploading" || item.status === "queued") active++;
      else if (item.status === "error") failed++;
    }
    const total = uploadQueue.length;
    if (active > 0) return `${active}/${total} active`;
    if (failed > 0) return `${failed}/${total} failed`;
    return `${total} completed`;
  }

  async function navigateToURLPath(api: IApi) {
    const path = pathFromURL();
    if (path === "/") {
      return;
    }
    await loadPathAncestors(api, path);
    await api.exec("set-path", { id: path });
  }

  async function loadPathAncestors(api: IApi, path: string) {
    const parts = path.split("/").filter(Boolean);
    let current = "/";
    for (const part of parts.slice(0, -1)) {
      const next = joinPath(current, part);
      await refreshFolder(api, current);
      current = next;
    }
    if (current !== "/" || parts.length > 1) {
      await refreshFolder(api, current);
    }
  }

  function syncURLPath(path: string) {
    const clean = normalizeURLPath(path);
    const url = new URL(window.location.href);
    url.pathname = pathToRoute(clean);
    url.searchParams.delete("path");
    window.history.replaceState(null, "", url);
  }

  function pathFromURL() {
    const url = new URL(window.location.href);
    const queryPath = url.searchParams.get("path");
    if (queryPath) {
      return normalizeURLPath(queryPath);
    }
    return routeToPath(url.pathname);
  }

  function pathToRoute(path: string) {
    const clean = normalizeURLPath(path);
    if (clean === "/") {
      return "/files";
    }
    return `/files/${clean.split("/").filter(Boolean).map(encodeURIComponent).join("/")}`;
  }

  function routeToPath(pathname: string) {
    if (pathname === "/" || pathname === "/files") {
      return "/";
    }
    const prefix = "/files/";
    if (!pathname.startsWith(prefix)) {
      return "/";
    }
    const parts = pathname.slice(prefix.length).split("/").filter(Boolean).map(part => {
      try {
        return decodeURIComponent(part);
      } catch {
        return part;
      }
    });
    return normalizeURLPath(`/${parts.join("/")}`);
  }

  function normalizeURLPath(path: string) {
    const parts = path.split("/").filter(Boolean);
    return parts.length ? `/${parts.join("/")}` : "/";
  }

  function loadViewMode(): TMode {
    const stored = localStorage.getItem(viewModeKey);
    return stored === "cards" || stored === "table" || stored === "panels" || stored === "search" ? stored : "cards";
  }

  function menuOptions(mode: TContextMenuType, item?: IParsedEntity) {
    return getMenuOptions(mode)
      .filter(option => option.id && !unsupportedMenuItems.has(String(option.id)))
      .map(option => {
        if (option.id === "add-folder") {
          return { ...option, text: "New folder" };
        }
        return option;
      }) as IFileMenuOption[];
  }

  function previewTemplate(file: FilePreview, width: number, height: number) {
    if (file.type !== "file" || !previewKinds.has(file.previewKind || "")) {
      return null;
    }
    const requested = Math.max(width, height, 240);
    const size = requested > 420 ? 1024 : 420;
    return thumbnailURL(file.id, size);
  }

  function iconTemplate(file: IParsedEntity, size: "big" | "small") {
    const large = size === "big";
    if (file.type === "folder") {
      return svgIcon(folderIcon(large));
    }
    if (file.previewKind === "image") {
      return svgIcon(fileIcon("#0b6f68", "IMG", large));
    }
    if (file.previewKind === "video") {
      return svgIcon(fileIcon("#6b4bd8", "VID", large));
    }
    if (file.previewKind === "pdf") {
      return svgIcon(fileIcon("#b73232", "PDF", large));
    }
    if (file.previewKind === "text") {
      return svgIcon(fileIcon("#50606b", "TXT", large));
    }
    return svgIcon(fileIcon("#50606b", (file.ext || "FILE").slice(0, 4).toUpperCase(), large));
  }

  function extraInfo(file: IParsedEntity): IExtraInfo {
    const info: Record<string, string> = {
      Path: file.id
    };
    if (file.mimeType) {
      info.MIME = file.mimeType;
    }
    if (file.previewKind) {
      info.Preview = file.previewKind;
    }
    return info as IExtraInfo;
  }

  function uniqueParents(paths: string[]) {
    return [...new Set(paths.map(parentPath))];
  }

  function assertBulkSuccess(results: Array<{ path: string; ok: boolean; error?: string }>) {
    const failed = results.filter(result => !result.ok);
    if (failed.length) {
      throw new Error(`${failed.length} item(s) failed: ${failed[0].path} ${failed[0].error || ""}`.trim());
    }
  }

  function basename(path: string) {
    return path.split("/").filter(Boolean).pop() || "download";
  }

  function openViewer(file: IParsedEntity) {
    viewerFile = file;
  }

  function closeViewer() {
    viewerFile = null;
  }

  async function openTrash() {
    trashOpen = true;
    await refreshTrash();
  }

  async function refreshTrash() {
    trashBusy = "Loading trash";
    error = "";
    try {
      const response = await listTrash();
      trashItems = response.items;
    } catch (err) {
      error = messageFromError(err);
    } finally {
      trashBusy = "";
    }
  }

  async function restoreTrashItem(item: TrashItem) {
    trashBusy = "Restoring";
    error = "";
    try {
      await restoreTrash(item.id);
      await Promise.all([refreshTrash(), refreshCurrentFolder()]);
    } catch (err) {
      error = messageFromError(err);
    } finally {
      trashBusy = "";
    }
  }

  async function deleteTrashItem(item: TrashItem) {
    trashBusy = "Deleting";
    error = "";
    try {
      await deleteTrash(item.id);
      await refreshTrash();
    } catch (err) {
      error = messageFromError(err);
    } finally {
      trashBusy = "";
    }
  }

  async function refreshCurrentFolder() {
    if (!fileApi) {
      return;
    }
    await refreshFolder(fileApi, currentFolderPath());
  }

  async function openAdmin() {
    adminOpen = true;
    await refreshAdmin();
    startAdminPolling();
  }

  function closeAdmin() {
    adminOpen = false;
    stopAdminPolling();
  }

  async function refreshAdmin() {
    error = "";
    try {
      stats = await adminStats();
      adminJob = stats.current_job || null;
    } catch (err) {
      error = messageFromError(err);
    }
  }

  async function refreshAdminJob() {
    try {
      const response = await currentAdminJob();
      const newJob = response.job;
      if (newJob?.status !== adminJob?.status || newJob?.done !== adminJob?.done) {
        adminJob = newJob;
      }
      if (adminJob?.status !== "running") {
        stats = await adminStats();
      }
    } catch (err) {
      error = messageFromError(err);
    }
  }

  function startAdminPolling() {
    stopAdminPolling();
    adminPoll = window.setInterval(refreshAdminJob, 1500);
  }

  function stopAdminPolling() {
    if (adminPoll) {
      window.clearInterval(adminPoll);
      adminPoll = null;
    }
  }

  async function runAdminJob(kind: "reindex" | "preview") {
    error = "";
    try {
      const response = kind === "reindex" ? await startReindex() : await startPreviewWarmup();
      adminJob = response.job;
      startAdminPolling();
    } catch (err) {
      error = messageFromError(err);
    }
  }

  function jobProgress(job: AdminJob) {
    if (job.total_known) {
      return `${job.done}/${job.total}`;
    }
    return `${job.done} indexed`;
  }

  function progressValue(job: AdminJob) {
    if (!job.total_known || job.total === 0) {
      return 0;
    }
    return Math.min(100, Math.round((job.done / job.total) * 100));
  }

  function formatDate(value: string) {
    return new Intl.DateTimeFormat(undefined, {
      dateStyle: "medium",
      timeStyle: "short"
    }).format(new Date(value));
  }

  function formatBytes(size: number) {
    if (!Number.isFinite(size) || size < 0) {
      return "0 B";
    }
    if (size < 1024) {
      return `${size} B`;
    }
    const units = ["KB", "MB", "GB", "TB"];
    let value = size / 1024;
    for (const unit of units) {
      if (value < 1024) {
        return `${value.toFixed(value >= 10 ? 0 : 1)} ${unit}`;
      }
      value /= 1024;
    }
    return `${value.toFixed(1)} PB`;
  }

  function closeOnBackdrop(close: () => void) {
    return (event: MouseEvent) => {
      if (event.target === event.currentTarget) close();
    };
  }

  function closeOnEscape(event: KeyboardEvent, close: () => void) {
    if (event.key === "Escape") {
      close();
    }
  }

  function downloadViewer() {
    if (fileApi && viewerFile) {
      fileApi.exec("download-file", { id: viewerFile.id });
    }
  }

  function svgIcon(svg: string) {
    return `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`;
  }

  function folderIcon(large: boolean) {
    const stroke = large ? 0 : 2;
    return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 96 96"><path fill="#d6ebe8" stroke="#bfd8d4" stroke-width="${stroke}" d="M8 24h30l8 10h42v42a8 8 0 0 1-8 8H16a8 8 0 0 1-8-8z"/><path fill="#0b6f68" d="M8 30a8 8 0 0 1 8-8h22l8 10h34a8 8 0 0 1 8 8v8H8z"/></svg>`;
  }

  function fileIcon(color: string, label: string, large: boolean) {
    const safeLabel = (label || "FILE").replace(/[^A-Z0-9]/g, "").slice(0, 4) || "FILE";
    const fontSize = label.length > 3 ? 13 : 15;
    const stroke = large ? 3 : 5;
    return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 96 96"><path fill="#f8fafb" stroke="#cfd8dc" stroke-width="${stroke}" d="M24 8h32l16 16v64H24z"/><path fill="#e8eef2" d="M56 8v18h18z"/><rect x="18" y="48" width="60" height="24" rx="5" fill="${color}"/><text x="48" y="64" text-anchor="middle" font-family="Arial,sans-serif" font-size="${fontSize}" font-weight="700" fill="#fff">${safeLabel}</text></svg>`;
  }

  function messageFromError(err: unknown) {
    return err instanceof Error ? err.message : "Request failed";
  }
</script>

{#if loading}
  <main class="app app-centered">
    <div class="status-card">Loading</div>
  </main>
{:else if !user}
  <main class="app app-centered">
    <form class="login-card" on:submit|preventDefault={submitLogin}>
      <div>
        <p class="eyebrow">goDrive</p>
        <h1>Sign in</h1>
      </div>
      {#if error}
        <div class="error" role="alert">{error}</div>
      {/if}
      <label>
        <span>Username</span>
        <input bind:value={username} autocomplete="username" />
      </label>
      <label>
        <span>Password</span>
        <input bind:value={password} autocomplete="current-password" type="password" />
      </label>
      <button type="submit">Sign in</button>
    </form>
  </main>
{:else}
  <main class="app">
    <header class="topbar">
      <div>
        <p class="eyebrow">goDrive</p>
        <h1>Files</h1>
      </div>
      <div class="topbar-actions">
        {#if busy}
          <span class="busy">{busy}</span>
        {/if}
        <span class="user-pill">{user.username}</span>
        {#if user.is_admin}
          <button class="secondary" type="button" on:click={openAdmin}>Admin</button>
        {/if}
        <button class="secondary" type="button" on:click={openTrash}>Trash</button>
        <button class="secondary" type="button" on:click={() => uploadInput?.click()}>Upload</button>
        <input
          class="hidden-input"
          bind:this={uploadInput}
          type="file"
          multiple
          on:change={uploadSelectedFiles}
        />
        <button class="secondary" type="button" on:click={submitLogout}>Sign out</button>
      </div>
    </header>

    {#if error}
      <div class="banner" role="alert">
        <span>{error}</span>
        <button type="button" aria-label="Dismiss error" on:click={() => (error = "")}>×</button>
      </div>
    {/if}

    <section class="manager-shell">
      {#key managerKey}
        <Willow>
          <Filemanager
            data={initialData}
            mode={viewMode}
            preview={false}
            {extraInfo}
            icons={iconTemplate}
            init={initFilemanager}
            {menuOptions}
            previews={previewTemplate}
          />
        </Willow>
      {/key}
    </section>

    {#if trashOpen}
      <div class="modal-backdrop" role="presentation" on:click={closeOnBackdrop(() => (trashOpen = false))}>
        <div
          class="modal-panel"
          role="dialog"
          aria-modal="true"
          aria-label="Trash"
          tabindex="-1"
          on:keydown={event => closeOnEscape(event, () => (trashOpen = false))}
        >
          <header class="modal-head">
            <div>
              <p class="eyebrow">Trash</p>
              <h2>Deleted items</h2>
            </div>
            <button type="button" aria-label="Close trash" on:click={() => (trashOpen = false)}>×</button>
          </header>
          {#if trashBusy}
            <p class="muted">{trashBusy}</p>
          {/if}
          <div class="trash-list">
            {#if trashItems.length === 0}
              <p class="muted">Trash is empty.</p>
            {:else}
              {#each trashItems as item}
                <article class="trash-row">
                  <div>
                    <strong>{item.original_name}</strong>
                    <span>{item.original_path}</span>
                    <span>{formatBytes(item.size)} · {formatDate(item.deleted_at)}</span>
                  </div>
                  <div class="row-actions">
                    <button type="button" on:click={() => restoreTrashItem(item)}>Restore</button>
                    <button class="danger" type="button" on:click={() => deleteTrashItem(item)}>Delete</button>
                  </div>
                </article>
              {/each}
            {/if}
          </div>
        </div>
      </div>
    {/if}

    {#if adminOpen}
      <div class="modal-backdrop" role="presentation" on:click={closeOnBackdrop(closeAdmin)}>
        <div
          class="modal-panel wide"
          role="dialog"
          aria-modal="true"
          aria-label="Admin"
          tabindex="-1"
          on:keydown={event => closeOnEscape(event, closeAdmin)}
        >
          <header class="modal-head">
            <div>
              <p class="eyebrow">Admin</p>
              <h2>Management</h2>
            </div>
            <button type="button" aria-label="Close admin" on:click={closeAdmin}>×</button>
          </header>
          {#if stats}
            <div class="stats-grid">
              <article><span>Users</span><strong>{stats.users.total}</strong><small>{stats.users.disabled} disabled</small></article>
              <article><span>Indexed</span><strong>{stats.index.indexed_files}</strong><small>{stats.index.indexed_directories} folders</small></article>
              <article><span>Previews</span><strong>{stats.index.preview_candidates}</strong><small>{formatBytes(stats.preview_cache.bytes)} cached</small></article>
              <article><span>Trash</span><strong>{stats.trash.items}</strong><small>{formatBytes(stats.trash.bytes)}</small></article>
            </div>
          {/if}
          <div class="admin-actions">
            <button type="button" on:click={() => runAdminJob("reindex")}>Full reindex</button>
            <button type="button" on:click={() => runAdminJob("preview")}>Warm previews</button>
            <button type="button" on:click={refreshAdmin}>Refresh stats</button>
          </div>
          {#if adminJob}
            <section class="job-panel">
              <div>
                <strong>{adminJob.type}</strong>
                <span>{adminJob.status} · {jobProgress(adminJob)} · {adminJob.failed} failed</span>
              </div>
              {#if adminJob.total_known}
                <progress value={progressValue(adminJob)} max="100"></progress>
              {:else}
                <progress></progress>
              {/if}
              <p>{adminJob.message}</p>
            </section>
          {:else}
            <p class="muted">No admin job has run yet.</p>
          {/if}
        </div>
      </div>
    {/if}

    {#if viewerFile}
      <div
        class="viewer"
        role="dialog"
        aria-modal="true"
        aria-label={viewerFile.name}
        tabindex="-1"
        on:click={closeOnBackdrop(closeViewer)}
        on:keydown={event => closeOnEscape(event, closeViewer)}
      >
        <header>
          <strong>{viewerFile.name}</strong>
          <div>
            <button type="button" on:click={downloadViewer}>Download</button>
            <button type="button" on:click={closeViewer}>Close</button>
          </div>
        </header>
        <img src={thumbnailURL(viewerFile.id, 1024)} alt={viewerFile.name} />
      </div>
    {/if}

    {#if uploadQueue.length > 0}
      <aside class="upload-queue" aria-label="Upload queue">
        <header>
          <div>
            <strong>Uploads</strong>
            <span>{uploadSummary()}</span>
          </div>
          <div>
            <button type="button" title="Clear completed uploads" on:click={clearCompletedUploads}>Clear</button>
            <button type="button" title="Toggle upload queue" on:click={() => (uploadQueueCollapsed = !uploadQueueCollapsed)}>
              {uploadQueueCollapsed ? "▲" : "▼"}
            </button>
          </div>
        </header>
        {#if !uploadQueueCollapsed}
          <div class="upload-list">
            {#each uploadQueue as item (item.id)}
              <article class:failed={item.status === "error"} class:complete={item.status === "done"}>
                <div class="upload-row-head">
                  <strong>{item.name}</strong>
                  <span>{item.status}</span>
                </div>
                <div class="upload-meta">
                  <span>{formatBytes(item.size)}</span>
                  <span>{item.targetPath}</span>
                </div>
                <progress value={item.progress} max="100"></progress>
                {#if item.error}
                  <div class="upload-error">
                    <span>{item.error}</span>
                    <button type="button" on:click={() => retryUpload(item)}>Retry</button>
                  </div>
                {/if}
              </article>
            {/each}
          </div>
        {/if}
      </aside>
    {/if}
  </main>
{/if}
