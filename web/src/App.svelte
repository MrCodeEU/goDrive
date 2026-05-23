<script lang="ts">
  import { onMount, tick } from "svelte";
  import Icon from "./lib/Icon.svelte";
  import CodeEditor from "./lib/CodeEditor.svelte";
  import ThreeDGridThumb from "./lib/ThreeDGridThumb.svelte";
  import {
    adminStats,
    authHeaders,
    bulkDelete,
    saveFileContent,
    bulkDownloadBlob,
    bulkMove,
    cancelAdminJob,
    clearPreviewCache,
    createAdminUser,
    currentAdminJob,
    currentToken,
    deleteTrash,
    downloadBlob,
    fetchTextPreview,
    fetchExif,
    fetchPublicConfig,
    type ExifData,
    joinPath,
    listAdminUsers,
    listFiles,
    listFileTree,
    listTrash,
    login,
    logout,
    me,
    mkdir,
    move,
    parentPath,
    rawFileURL,
    restoreTrash,
    resumeUploadTus,
    saveBlob,
    searchFiles,
    startPreviewWarmup,
    startReindex,
    setAdminUserPassword,
    thumbnailURL,
    trashThumbnailURL,
    updateAdminUser,
    uploadTus,
    createAPIKey,
    listAPIKeys,
    revokeAPIKey,
    type APIKey,
    type AdminJob,
    type AdminStats,
    type FileEntry,
    type TextPreview,
    type TrashItem,
    type User
  } from "./lib/api";

  const previewKinds = new Set(["image", "raw", "video", "pdf", "office", "text", "markdown", "3d"]);
  const thumbnailKinds = new Set(["image", "raw", "video", "pdf", "office"]);
  const uploadConcurrency = 3;
  const QUEUE_STORAGE_KEY = "godrive_upload_queue";

  let threeDViewerPromise: Promise<typeof import("./lib/ThreeDViewer.svelte")> | null = null;
  function loadThreeDViewer() {
    threeDViewerPromise ??= import("./lib/ThreeDViewer.svelte");
    return threeDViewerPromise;
  }

  type UploadQueueItem = {
    id: string;
    file: File | null;
    name: string;
    size: number;
    targetPath: string;
    progress: number;
    status: "queued" | "uploading" | "done" | "error" | "interrupted";
    error?: string;
    finalPath?: string;
    tusUrl?: string;
    attempts: number;
  };

  type TreeRow = FileEntry & { level: number };
  type SortOption = "name_asc" | "name_desc" | "modified_desc" | "modified_asc" | "size_desc" | "size_asc" | "type_asc";
  type FileTypeFilter = "all" | "folders" | "images" | "videos" | "documents" | "text" | "3d" | "other";
  type ActionDialog = {
    kind: "create" | "rename" | "move" | "delete";
    title: string;
    message?: string;
    value: string;
    entry?: FileEntry;
  };

  let user: User | null = null;
  let username = "";
  let password = "";
  let demoLogin: { username: string; password: string } | null = null;
  let demoMode = false;
  let loading = true;
  let busy = "";
  let error = "";

  let currentPath = "/";
  let entries: FileEntry[] = [];
  let folderTotal = 0;
  let folderOffset = 0;
  let folderCursor = "";
  let folderHasMore = false;
  let treeEntries: FileEntry[] = [];
  let openTree = new Set<string>(["/"]);
  let selectedIds: string[] = [];
  let anchorId = "";
  let viewMode: "grid" | "list" | "masonry" = (() => {
    try { return (localStorage.getItem('godrive_view') as "grid" | "list" | "masonry") || "grid"; } catch { return "grid"; }
  })();
  let gridSize: "s" | "m" | "l" = (() => {
    try { return (localStorage.getItem('godrive_grid_size') as "s" | "m" | "l") || "m"; } catch { return "m"; }
  })();
  function setGridSize(s: "s" | "m" | "l") {
    gridSize = s;
    try { localStorage.setItem('godrive_grid_size', s); } catch {}
  }
  let darkMode: boolean = (() => {
    try { return localStorage.getItem('godrive_theme') === 'dark'; } catch { return false; }
  })();
  $: { try { document.documentElement.classList.toggle('light', !darkMode); } catch {} }
  let sortOption: SortOption = "name_asc";
  let fileTypeFilter: FileTypeFilter = "all";

  let searchQuery = "";
  let searchResults: FileEntry[] = [];
  let searchOpen = false;
  let searchDialogInput: HTMLInputElement | null = null;
  let shortcutsOpen = false;
  let searchDebounce: ReturnType<typeof setTimeout> | null = null;
  let uploadInput: HTMLInputElement | null = null;
  let uploadQueue: UploadQueueItem[] = loadQueueFromStorage();
  let uploadQueueCollapsed = uploadQueue.length === 0;
  let uploadPreparing = false;
  let dragOver = false;
  let uploadsActive = false;

  let viewerFile: FileEntry | null = null;
  let viewerText: TextPreview | null = null;
  let viewerTextLoading = false;
  let viewerTextError = "";
  let viewerZoom = 1;
  let viewerOriginal = false;
  let editorMode = false;
  let editorContent = '';
  let editorDirty = false;
  let viewerSidebarOpen: boolean = (() => { try { return localStorage.getItem('godrive_viewer_sidebar') !== 'false'; } catch { return true; } })();
  let viewerExif: ExifData | null = null;
  let viewerExifLoading = false;
  function toggleViewerSidebar() {
    viewerSidebarOpen = !viewerSidebarOpen;
    try { localStorage.setItem('godrive_viewer_sidebar', String(viewerSidebarOpen)); } catch {}
  }
  let editorSaving = false;
  let editorRef: CodeEditor | null = null;

  let trashOpen = false;
  let trashItems: TrashItem[] = [];
  let trashSelectedIds: string[] = [];
  let trashAnchorId = "";
  let trashViewMode: "grid" | "list" = "list";
  let trashBusy = "";
  let actionDialog: ActionDialog | null = null;
  let adminOpen = false;
  let stats: AdminStats | null = null;
  let adminJob: AdminJob | null = null;
  let adminUsers: User[] = [];
  let adminBusy = "";
  let adminPoll: ReturnType<typeof window.setInterval> | null = null;
  let newUser = emptyNewUser();
  let passwordReset: Record<number, string> = {};
  let apiKeys: APIKey[] = [];
  let newKeyName = "";
  let newKeyUserID = 0;
  let newKeyToken = "";
  let apiKeyBusy = "";
  let mobileNavOpen = false;

  let fileEvents: AbortController | null = null;
  let liveRefreshTimer: ReturnType<typeof setTimeout> | null = null;

  type Toast = { id: number; message: string; type: 'success' | 'error' | 'info' };
  let toasts: Toast[] = [];
  let toastSeq = 0;

  type ContextMenu = { x: number; y: number; entry: FileEntry };
  let contextMenu: ContextMenu | null = null;

  let infoEntry: FileEntry | null = null;
  let infoExif: ExifData | null = null;
  let infoExifLoading = false;
  let sentinelEl: HTMLElement | null = null;
  let loadMoreObserver: IntersectionObserver | null = null;
  let dragTargetPath: string | null = null;

  function addToast(message: string, type: Toast['type'] = 'success') {
    const id = ++toastSeq;
    toasts = [...toasts, { id, message, type }];
    setTimeout(() => { toasts = toasts.filter(t => t.id !== id); }, 3000);
  }

  function openContextMenu(event: MouseEvent, entry: FileEntry) {
    event.preventDefault();
    contextMenu = { x: event.clientX, y: event.clientY, entry };
  }

  function closeContextMenu() { contextMenu = null; }

  async function copyPath(path: string) {
    try {
      await navigator.clipboard.writeText(path);
      addToast('Path copied to clipboard', 'info');
    } catch {
      addToast('Failed to copy path', 'error');
    }
  }

  async function openInfoPanel(entry: FileEntry | null) {
    infoEntry = entry;
    infoExif = null;
    if (!entry || entry.type !== 'file') return;
    const kind = entry.preview_kind || '';
    if (kind !== 'image' && kind !== 'raw') return;
    infoExifLoading = true;
    try {
      infoExif = await fetchExif(entry.path);
    } catch {
      // exiftool unavailable or not an image — ignore
    } finally {
      infoExifLoading = false;
    }
  }

  function toggleTheme() {
    darkMode = !darkMode;
    try { localStorage.setItem('godrive_theme', darkMode ? 'dark' : 'light'); } catch {}
  }

  function observeSentinel(node: HTMLElement) {
    loadMoreObserver?.observe(node);
    return { destroy() { loadMoreObserver?.unobserve(node); } };
  }

  $: visibleEntries = sortEntries(filterEntries(entries, fileTypeFilter), sortOption);
  $: visibleTree = treeRows(treeEntries, entries, currentPath, openTree);
  $: treeChildPaths = new Set(
    mergeFolderLists(
      ...treeEntries,
      ...currentPathFallbackEntries(currentPath),
      ...entries.filter(entry => entry.type === "dir" && parentPath(entry.path) === currentPath)
    ).map(entry => parentPath(entry.path))
  );
  $: trashSelectedItems = trashItems.filter(item => trashSelectedIds.includes(item.id));
  $: selectedEntries = selectedIds.map(id => entryByPath(id)).filter((entry): entry is FileEntry => !!entry);
  $: selectedSize = selectedEntries.reduce((sum, entry) => sum + (entry.type === "file" ? entry.size : 0), 0);
  $: uploadsActive = uploadQueue.some(item => item.status === "queued" || item.status === "uploading");
  $: saveQueueToStorage(uploadQueue);

  onMount(() => {
    document.documentElement.classList.toggle('light', !darkMode);
    const onPopState = () => {
      void loadPath(pathFromURL(), { push: false });
    };
    const onBeforeUnload = (event: BeforeUnloadEvent) => {
      if (!uploadsActive) return;
      event.preventDefault();
      event.returnValue = "";
    };
    const onKeyDown = (event: KeyboardEvent) => handleGlobalKeydown(event);
    const closeCtx = () => closeContextMenu();
    window.addEventListener("popstate", onPopState);
    window.addEventListener("beforeunload", onBeforeUnload);
    window.addEventListener("keydown", onKeyDown);
    window.addEventListener("click", closeCtx);

    loadMoreObserver = new IntersectionObserver(entries => {
      if (entries[0].isIntersecting && folderHasMore && !busy) {
        void loadMoreFolder();
      }
    }, { rootMargin: '200px' });

    void loadPublicConfig();
    void restoreSession();
    return () => {
      window.removeEventListener("popstate", onPopState);
      window.removeEventListener("beforeunload", onBeforeUnload);
      window.removeEventListener("keydown", onKeyDown);
      window.removeEventListener("click", closeCtx);
      stopFileEvents();
      stopAdminPolling();
      loadMoreObserver?.disconnect();
    };
  });

  async function restoreSession() {
    if (!currentToken()) {
      loading = false;
      return;
    }
    loading = true;
    try {
      user = await me();
      await Promise.all([loadTree(), loadPath(pathFromURL(), { push: false })]);
      startFileEvents();
    } catch (err) {
      error = messageFromError(err);
    } finally {
      loading = false;
    }
  }

  async function loadPublicConfig() {
    try {
      const config = await fetchPublicConfig();
      demoMode = config.demo_mode;
      if (config.demo_mode && config.demo_user && config.demo_password) {
        demoLogin = { username: config.demo_user, password: config.demo_password };
        if (!currentToken() && !user && !username && !password) {
          username = demoLogin.username;
          password = demoLogin.password;
        }
      }
    } catch {
      // Public config is optional; keep the login form usable if it cannot load.
    }
  }

  async function submitLogin() {
    loading = true;
    error = "";
    try {
      user = await login(username, password);
      password = "";
      await Promise.all([loadTree(), loadPath("/", { push: true })]);
      startFileEvents();
    } catch (err) {
      error = messageFromError(err);
    } finally {
      loading = false;
    }
  }

  async function submitLogout() {
    await logout();
    stopFileEvents();
    user = null;
    mobileNavOpen = false;
    if (demoLogin) {
      username = demoLogin.username;
      password = demoLogin.password;
    }
    entries = [];
    treeEntries = [];
    selectedIds = [];
  }

  async function loadPath(path: string, options: { push?: boolean } = {}) {
    const clean = normalizePath(path);
    busy = "Loading folder";
    try {
      const response = await listFiles(clean);
      currentPath = response.path || clean;
      entries = response.entries;
      mergeTreeEntries(response.entries.filter(entry => entry.type === "dir"));
      folderTotal = response.total;
      folderOffset = response.offset + response.entries.length;
      folderCursor = response.next_cursor || "";
      folderHasMore = response.has_more;
      selectedIds = [];
      anchorId = "";
      openAncestors(currentPath);
      if (options.push !== false) syncURLPath(currentPath);
    } catch (err) {
      error = messageFromError(err);
    } finally {
      busy = "";
    }
  }

  async function refreshCurrentFolder() {
    await Promise.all([loadTree(), loadPath(currentPath, { push: false })]);
  }

  async function loadMoreFolder() {
    if (!folderHasMore) return;
    const response = await listFiles(currentPath, folderOffset, 500, folderCursor);
    entries = [...entries, ...response.entries];
    folderOffset += response.entries.length;
    folderTotal = response.total;
    folderCursor = response.next_cursor || "";
    folderHasMore = response.has_more;
  }

  async function loadTree() {
    try {
      const [treeResponse, rootResponse] = await Promise.all([
        listFileTree(),
        listFiles("/", 0, 2000)
      ]);
      treeEntries = mergeFolderLists(...treeResponse.entries, ...rootResponse.entries.filter(entry => entry.type === "dir"));
      openAncestors(currentPath);
    } catch (err) {
      error = messageFromError(err);
    }
  }

  async function createFolder() {
    actionDialog = {
      kind: "create",
      title: "New folder",
      message: `Create a folder in ${currentPath}`,
      value: ""
    };
  }

  async function renameSelected(entry = selectedEntries[0]) {
    if (!entry) return;
    actionDialog = {
      kind: "rename",
      title: "Rename",
      message: entry.path,
      value: entry.name,
      entry
    };
  }

  async function moveSelected() {
    if (selectedIds.length === 0) return;
    actionDialog = {
      kind: "move",
      title: "Move items",
      message: `${selectedIds.length} selected item(s)`,
      value: currentPath
    };
  }

  async function deleteSelected() {
    if (selectedIds.length === 0) return;
    actionDialog = {
      kind: "delete",
      title: "Move to trash",
      message: `Move ${selectedIds.length} selected item(s) to trash?`,
      value: ""
    };
  }

  async function submitActionDialog() {
    const dialog = actionDialog;
    if (!dialog) return;
    const value = dialog.value.trim();
    if (dialog.kind !== "delete" && !value) return;

    actionDialog = null;
    if (dialog.kind === "create") {
      await runAction("Creating folder", async () => {
        await mkdir(joinPath(currentPath, value));
        await refreshCurrentFolder();
        addToast('Folder created');
      });
      return;
    }
    if (dialog.kind === "rename") {
      const entry = dialog.entry;
      if (!entry || value === entry.name) return;
      await runAction("Renaming", async () => {
        await move(entry.path, joinPath(parentPath(entry.path), value));
        await refreshCurrentFolder();
        addToast('Moved successfully');
      });
      return;
    }
    if (dialog.kind === "move") {
      await runAction("Moving items", async () => {
        const response = await bulkMove(selectedIds, normalizePath(value));
        assertBulkSuccess(response.results);
        await refreshCurrentFolder();
        addToast('Moved successfully');
      });
      return;
    }
    const deleteCount = selectedIds.length;
    await runAction("Moving to trash", async () => {
      const response = await bulkDelete(selectedIds);
      assertBulkSuccess(response.results);
      await refreshCurrentFolder();
      addToast(`${deleteCount} item(s) moved to trash`);
    });
  }

  async function downloadSelected() {
    if (selectedIds.length === 0) return;
    await runAction("Preparing download", async () => {
      if (selectedIds.length === 1) {
        const entry = entryByPath(selectedIds[0]);
        const blob = await downloadBlob(selectedIds[0]);
        saveBlob(blob, entry?.name || basename(selectedIds[0]));
        return;
      }
      const blob = await bulkDownloadBlob(selectedIds);
      saveBlob(blob, "godrive-selection.zip");
    });
  }

  async function uploadSelectedFiles(event: Event) {
    const input = event.currentTarget as HTMLInputElement;
    uploadPreparing = true;
    uploadQueueCollapsed = false;
    await tick();
    const files = Array.from(input.files || []);
    input.value = "";
    uploadPreparing = false;
    await uploadFiles(files);
  }

  async function uploadFiles(files: File[]) {
    if (files.length === 0) return;
    const queued = files.map(file => createUploadQueueItem(file, currentPath));
    uploadQueue = [...queued, ...uploadQueue];
    uploadQueueCollapsed = false;
    void uploadQueuedFiles(queued);
  }

  async function uploadQueuedFiles(items: UploadQueueItem[]) {
    let next = 0;
    const workers = Array.from({ length: Math.min(uploadConcurrency, items.length) }, async () => {
      while (next < items.length) {
        const item = items[next++];
        await uploadOne(item);
        void refreshCurrentFolder();
      }
    });
    await Promise.all(workers);
  }

  const uploadMaxAttempts = 3;

  async function uploadOne(item: UploadQueueItem) {
    if (!item.file) {
      setUploadItem(item.id, { status: "interrupted" });
      return;
    }
    const attempt = (item.attempts || 0) + 1;
    setUploadItem(item.id, { status: "uploading", progress: 0, error: "", attempts: attempt });
    try {
      const finalPath = await uploadTus(
        item.file,
        item.targetPath,
        progress => setUploadItem(item.id, { progress: progress.percent }),
        { onUploadCreated: url => setUploadItem(item.id, { tusUrl: url }) }
      );
      setUploadItem(item.id, { status: "done", progress: 100, finalPath: finalPath || joinPath(item.targetPath, item.name) });
    } catch (err) {
      if (attempt < uploadMaxAttempts) {
        await new Promise(r => setTimeout(r, 1000 * attempt));
        const current = uploadQueue.find(i => i.id === item.id);
        if (current) await uploadOne(current);
      } else {
        setUploadItem(item.id, { status: "error", error: messageFromError(err) });
      }
    }
  }

  async function retryUpload(item: UploadQueueItem) {
    if (!item.file) return;
    setUploadItem(item.id, { attempts: 0 });
    const current = uploadQueue.find(i => i.id === item.id) ?? item;
    if (item.tusUrl) {
      setUploadItem(item.id, { status: "uploading", progress: 0, error: "" });
      try {
        const finalPath = await resumeUploadTus(item.tusUrl, item.file, progress => setUploadItem(item.id, { progress: progress.percent }));
        setUploadItem(item.id, { status: "done", progress: 100, finalPath: finalPath || joinPath(item.targetPath, item.name) });
      } catch (err) {
        setUploadItem(item.id, { status: "error", error: messageFromError(err) });
      }
    } else {
      await uploadOne(current);
    }
    void refreshCurrentFolder();
  }

  function selectEntry(entry: FileEntry, event: MouseEvent) {
    const ids = visibleEntries.map(item => item.path);
    if (event.shiftKey && anchorId) {
      const start = ids.indexOf(anchorId);
      const end = ids.indexOf(entry.path);
      if (start >= 0 && end >= 0) {
        const [left, right] = start < end ? [start, end] : [end, start];
        selectedIds = ids.slice(left, right + 1);
        return;
      }
    }
    if (event.ctrlKey || event.metaKey) {
      selectedIds = selectedIds.includes(entry.path)
        ? selectedIds.filter(id => id !== entry.path)
        : [...selectedIds, entry.path];
      anchorId = entry.path;
      return;
    }
    selectedIds = [entry.path];
    anchorId = entry.path;
  }

  function openEntry(entry: FileEntry) {
    if (entry.type === "dir") {
      void loadPath(entry.path, { push: true });
      return;
    }
    if (entry.preview_kind && previewKinds.has(entry.preview_kind)) {
      openViewer(entry);
    } else {
      selectedIds = [entry.path];
      void downloadSelected();
    }
  }

  async function openViewer(entry: FileEntry) {
    selectedIds = [entry.path];
    anchorId = entry.path;
    viewerFile = entry;
    viewerText = null;
    viewerTextError = "";
    viewerExif = null;
    viewerZoom = 1;
    viewerOriginal = false;
    editorMode = false;
    editorContent = '';
    editorDirty = false;
    if (entry.preview_kind === "image" || entry.preview_kind === "raw") {
      viewerExifLoading = true;
      fetchExif(entry.path).then(d => { viewerExif = d; }).catch(() => {}).finally(() => { viewerExifLoading = false; });
    }
    if (entry.preview_kind === "text" || entry.preview_kind === "markdown") {
      viewerTextLoading = true;
      try {
        viewerText = await fetchTextPreview(entry.path);
      } catch (err) {
        viewerTextError = messageFromError(err);
      } finally {
        viewerTextLoading = false;
      }
    }
  }

  function closeViewer() {
    viewerFile = null;
    viewerText = null;
    viewerTextError = "";
    editorMode = false;
    editorDirty = false;
    viewerExif = null;
  }

  async function saveEditorContent() {
    if (!viewerFile || editorSaving) return;
    editorSaving = true;
    try {
      const content = editorRef?.getValue() ?? editorContent;
      await saveFileContent(viewerFile.path, content);
      editorDirty = false;
      if (viewerText) viewerText = { ...viewerText, content };
    } catch (err) {
      error = messageFromError(err);
    } finally {
      editorSaving = false;
    }
  }

  function showAdjacentImage(delta: number) {
    if (!viewerFile) return;
    const images = visibleEntries.filter(entry => entry.type === "file" && entry.preview_kind === "image");
    const index = images.findIndex(entry => entry.path === viewerFile?.path);
    if (index < 0 || images.length < 2) return;
    viewerFile = images[(index + delta + images.length) % images.length];
    selectedIds = [viewerFile.path];
    anchorId = viewerFile.path;
    viewerZoom = 1;
  }

  async function openTrash() {
    trashOpen = true;
    await refreshTrash();
  }

  async function refreshTrash() {
    trashBusy = "Loading trash";
    try {
      trashItems = (await listTrash()).items;
      trashSelectedIds = trashSelectedIds.filter(id => trashItems.some(item => item.id === id));
    } catch (err) {
      error = messageFromError(err);
    } finally {
      trashBusy = "";
    }
  }

  async function restoreTrashItem(item: TrashItem) {
    trashBusy = "Restoring";
    try {
      await restoreTrash(item.id);
      trashSelectedIds = trashSelectedIds.filter(id => id !== item.id);
      await Promise.all([refreshTrash(), refreshCurrentFolder()]);
      addToast('Restored');
    } catch (err) {
      error = messageFromError(err);
    } finally {
      trashBusy = "";
    }
  }

  async function deleteTrashItem(item: TrashItem) {
    trashBusy = "Deleting";
    try {
      await deleteTrash(item.id);
      trashSelectedIds = trashSelectedIds.filter(id => id !== item.id);
      await refreshTrash();
    } catch (err) {
      error = messageFromError(err);
    } finally {
      trashBusy = "";
    }
  }

  async function restoreSelectedTrash() {
    if (trashSelectedItems.length === 0) return;
    trashBusy = "Restoring";
    try {
      await Promise.all(trashSelectedItems.map(item => restoreTrash(item.id)));
      trashSelectedIds = [];
      await Promise.all([refreshTrash(), refreshCurrentFolder()]);
    } catch (err) {
      error = messageFromError(err);
    } finally {
      trashBusy = "";
    }
  }

  async function deleteSelectedTrash() {
    if (trashSelectedItems.length === 0) return;
    trashBusy = "Deleting";
    try {
      await Promise.all(trashSelectedItems.map(item => deleteTrash(item.id)));
      trashSelectedIds = [];
      await refreshTrash();
    } catch (err) {
      error = messageFromError(err);
    } finally {
      trashBusy = "";
    }
  }

  function selectTrashItem(item: TrashItem, event: MouseEvent) {
    const ids = trashItems.map(row => row.id);
    if (event.shiftKey && trashAnchorId) {
      const start = ids.indexOf(trashAnchorId);
      const end = ids.indexOf(item.id);
      if (start >= 0 && end >= 0) {
        const [left, right] = start < end ? [start, end] : [end, start];
        trashSelectedIds = ids.slice(left, right + 1);
        return;
      }
    }
    if (event.ctrlKey || event.metaKey) {
      trashSelectedIds = trashSelectedIds.includes(item.id)
        ? trashSelectedIds.filter(id => id !== item.id)
        : [...trashSelectedIds, item.id];
      trashAnchorId = item.id;
      return;
    }
    trashSelectedIds = [item.id];
    trashAnchorId = item.id;
  }

  async function openAdmin() {
    adminOpen = true;
    await Promise.all([refreshAdmin(), refreshAdminUsers(), refreshAPIKeys()]);
    startAdminPolling();
  }

  function closeAdmin() {
    adminOpen = false;
    stopAdminPolling();
  }

  async function refreshAdmin() {
    try {
      stats = await adminStats();
      adminJob = stats.current_job || null;
    } catch (err) {
      error = messageFromError(err);
    }
  }

  async function refreshAdminUsers() {
    adminBusy = "Loading users";
    try {
      adminUsers = (await listAdminUsers()).users;
    } catch (err) {
      error = messageFromError(err);
    } finally {
      adminBusy = "";
    }
  }

  async function pollAdminJob() {
    try {
      adminJob = (await currentAdminJob()).job;
      if (adminJob?.status !== "running") await refreshAdmin();
    } catch (err) {
      error = messageFromError(err);
    }
  }

  function startAdminPolling() {
    stopAdminPolling();
    adminPoll = window.setInterval(pollAdminJob, 1500);
  }

  function stopAdminPolling() {
    if (adminPoll) window.clearInterval(adminPoll);
    adminPoll = null;
  }

  async function runAdminJob(kind: "reindex" | "preview") {
    if (demoMode) {
      error = "Admin changes are disabled in demo mode";
      return;
    }
    if (adminJob?.status === "running") {
      error = "Another admin job is already running";
      return;
    }
    try {
      adminJob = (kind === "reindex" ? await startReindex() : await startPreviewWarmup()).job;
      startAdminPolling();
    } catch (err) {
      error = messageFromError(err);
    }
  }

  async function cancelRunningAdminJob() {
    if (demoMode) {
      error = "Admin changes are disabled in demo mode";
      return;
    }
    try {
      adminJob = (await cancelAdminJob()).job;
      startAdminPolling();
    } catch (err) {
      error = messageFromError(err);
    }
  }

  async function clearPreviewCacheFromAdmin() {
    if (demoMode) {
      error = "Admin changes are disabled in demo mode";
      return;
    }
    adminBusy = "Clearing preview cache";
    try {
      await clearPreviewCache();
      await refreshAdmin();
    } catch (err) {
      error = messageFromError(err);
    } finally {
      adminBusy = "";
    }
  }

  async function saveAdminUser(item: User) {
    if (demoMode) {
      error = "Admin changes are disabled in demo mode";
      return;
    }
    adminBusy = `Saving ${item.username}`;
    try {
      await updateAdminUser(item.id, {
        username: item.username,
        home_root: item.home_root,
        is_admin: item.is_admin,
        disabled: item.disabled
      });
      const nextPassword = passwordReset[item.id]?.trim();
      if (nextPassword) {
        await setAdminUserPassword(item.id, nextPassword);
        passwordReset = { ...passwordReset, [item.id]: "" };
      }
      await refreshAdminUsers();
      await refreshAdmin();
    } catch (err) {
      error = messageFromError(err);
    } finally {
      adminBusy = "";
    }
  }

  async function createUserFromAdmin() {
    if (demoMode) {
      error = "Admin changes are disabled in demo mode";
      return;
    }
    if (!newUser.username.trim() || !newUser.password) {
      error = "Username and password are required";
      return;
    }
    adminBusy = "Creating user";
    try {
      await createAdminUser({
        username: newUser.username.trim(),
        password: newUser.password,
        home_root: newUser.home_root.trim(),
        is_admin: newUser.is_admin,
        disabled: newUser.disabled
      });
      newUser = emptyNewUser();
      await refreshAdminUsers();
      await refreshAdmin();
    } catch (err) {
      error = messageFromError(err);
    } finally {
      adminBusy = "";
    }
  }

  async function refreshAPIKeys() {
    try {
      apiKeys = (await listAPIKeys()).api_keys ?? [];
    } catch (err) {
      error = messageFromError(err);
    }
  }

  async function createNewAPIKey() {
    if (demoMode) {
      error = "Admin changes are disabled in demo mode";
      return;
    }
    if (!newKeyName.trim() || !newKeyUserID) return;
    apiKeyBusy = "Creating…";
    try {
      const result = await createAPIKey(newKeyUserID, newKeyName.trim());
      newKeyToken = result.token;
      newKeyName = "";
      newKeyUserID = 0;
      await refreshAPIKeys();
    } catch (err) {
      error = messageFromError(err);
    } finally {
      apiKeyBusy = "";
    }
  }

  async function revokeKey(id: string) {
    if (demoMode) {
      error = "Admin changes are disabled in demo mode";
      return;
    }
    apiKeyBusy = "Revoking…";
    try {
      await revokeAPIKey(id);
      await refreshAPIKeys();
    } catch (err) {
      error = messageFromError(err);
    } finally {
      apiKeyBusy = "";
    }
  }

  function startFileEvents() {
    stopFileEvents();
    const url = eventSourceURL();
    if (!url) return;
    const controller = new AbortController();
    fileEvents = controller;
    void readFileEvents(url, controller);
  }

  async function readFileEvents(url: string, controller: AbortController) {
    try {
      const response = await fetch(url, {
        headers: authHeaders(),
        credentials: "include",
        signal: controller.signal
      });
      if (!response.ok || !response.body) return;
      const reader = response.body.pipeThrough(new TextDecoderStream()).getReader();
      let buffer = "";
      while (!controller.signal.aborted) {
        const { value, done } = await reader.read();
        if (done) break;
        buffer += value;
        const chunks = buffer.split("\n\n");
        buffer = chunks.pop() || "";
        for (const chunk of chunks) {
          const data = chunk.split("\n").find(line => line.startsWith("data: "))?.slice(6);
          if (data) scheduleRefreshForEvent(data);
        }
      }
    } catch {
      // Live refresh is opportunistic; manual refresh remains available.
    } finally {
      if (fileEvents === controller) fileEvents = null;
    }
  }

  function eventSourceURL() {
    const configured = (import.meta as ImportMeta & { env?: Record<string, string> }).env?.VITE_EVENTS_URL;
    if (configured) return configured;
    return "/api/events";
  }

  function stopFileEvents() {
    if (liveRefreshTimer) window.clearTimeout(liveRefreshTimer);
    liveRefreshTimer = null;
    fileEvents?.abort();
    fileEvents = null;
  }

  function scheduleRefreshForEvent(raw: string) {
    let event: { data?: Record<string, unknown> };
    try {
      event = JSON.parse(raw);
    } catch {
      return;
    }
    if (!eventAffectsCurrentFolder(event)) return;
    if (liveRefreshTimer) window.clearTimeout(liveRefreshTimer);
    liveRefreshTimer = window.setTimeout(() => {
      liveRefreshTimer = null;
      void refreshCurrentFolder();
    }, 250);
  }

  function eventAffectsCurrentFolder(event: { data?: Record<string, unknown> }) {
    const paths = [event.data?.path, event.data?.old_path].filter((value): value is string => typeof value === "string");
    return paths.some(raw => {
      const path = normalizePath(raw);
      return parentPath(path) === currentPath || path === currentPath || currentPath.startsWith(`${path}/`);
    });
  }

  function handleGlobalKeydown(event: KeyboardEvent) {
    if (!user || shouldIgnoreShortcut(event.target)) return;
    const key = event.key.toLowerCase();
    if ((event.ctrlKey || event.metaKey) && key === "a") {
      event.preventDefault();
      if (trashOpen) trashSelectedIds = trashItems.map(item => item.id);
      else selectedIds = visibleEntries.map(entry => entry.path);
    } else if (event.key === "Delete") {
      if (trashOpen) void deleteSelectedTrash();
      else void deleteSelected();
    } else if (event.key === "F2") {
      void renameSelected();
    } else if (event.key === "Enter" && selectedIds.length === 1) {
      const entry = entryByPath(selectedIds[0]);
      if (entry) openEntry(entry);
    } else if ((event.ctrlKey || event.metaKey) && event.shiftKey && key === "n") {
      event.preventDefault();
      void createFolder();
    } else if (event.key === 'i' && selectedIds.length === 1 && !viewerFile) {
      event.preventDefault();
      openInfoPanel(entryByPath(selectedIds[0]));
    } else if (event.key === '?' && !shouldIgnoreShortcut(event.target)) {
      event.preventDefault();
      shortcutsOpen = !shortcutsOpen;
    } else if (event.key === "Escape") {
      if (shortcutsOpen) shortcutsOpen = false;
      else if (infoEntry) infoEntry = null;
      else if (contextMenu) contextMenu = null;
      else if (actionDialog) actionDialog = null;
      else if (viewerFile) closeViewer();
      else if (trashOpen) {
        trashSelectedIds = [];
        trashOpen = false;
      } else selectedIds = [];
    } else if (event.key === "ArrowRight" || event.key === "ArrowLeft" || event.key === "ArrowDown" || event.key === "ArrowUp") {
      if (trashOpen) moveTrashSelection(event);
      else moveSelection(event);
    }
  }

  function moveSelection(event: KeyboardEvent) {
    if (visibleEntries.length === 0) return;
    event.preventDefault();
    const columns = (viewMode === "grid" || viewMode === "masonry") ? gridColumnEstimate() : 1;
    const delta = event.key === "ArrowRight" ? 1 : event.key === "ArrowLeft" ? -1 : event.key === "ArrowDown" ? columns : -columns;
    const current = selectedIds.at(-1) || anchorId;
    const currentIndex = Math.max(0, current ? visibleEntries.findIndex(entry => entry.path === current) : 0);
    const nextIndex = Math.max(0, Math.min(visibleEntries.length - 1, currentIndex + delta));
    const nextId = visibleEntries[nextIndex].path;
    if (event.shiftKey) {
      const startId = anchorId || selectedIds[0] || nextId;
      const start = Math.max(0, visibleEntries.findIndex(entry => entry.path === startId));
      const [left, right] = start < nextIndex ? [start, nextIndex] : [nextIndex, start];
      selectedIds = visibleEntries.slice(left, right + 1).map(entry => entry.path);
      anchorId = startId;
      return;
    }
    if (event.ctrlKey || event.metaKey) {
      selectedIds = selectedIds.includes(nextId) ? selectedIds : [...selectedIds, nextId];
    } else {
      selectedIds = [nextId];
    }
    anchorId = nextId;
  }

  function moveTrashSelection(event: KeyboardEvent) {
    if (trashItems.length === 0) return;
    event.preventDefault();
    const delta = event.key === "ArrowRight" || event.key === "ArrowDown" ? 1 : -1;
    const current = trashSelectedIds.at(-1) || trashAnchorId;
    const currentIndex = Math.max(0, current ? trashItems.findIndex(item => item.id === current) : 0);
    const nextIndex = Math.max(0, Math.min(trashItems.length - 1, currentIndex + delta));
    const nextId = trashItems[nextIndex].id;
    if (event.shiftKey) {
      const startId = trashAnchorId || trashSelectedIds[0] || nextId;
      const start = Math.max(0, trashItems.findIndex(item => item.id === startId));
      const [left, right] = start < nextIndex ? [start, nextIndex] : [nextIndex, start];
      trashSelectedIds = trashItems.slice(left, right + 1).map(item => item.id);
      trashAnchorId = startId;
      return;
    }
    if (event.ctrlKey || event.metaKey) {
      trashSelectedIds = trashSelectedIds.includes(nextId) ? trashSelectedIds : [...trashSelectedIds, nextId];
    } else {
      trashSelectedIds = [nextId];
    }
    trashAnchorId = nextId;
  }

  function gridColumnEstimate() {
    if (typeof window === "undefined") return 4;
    if (window.innerWidth < 700) return 2;
    if (window.innerWidth < 1100) return 3;
    if (window.innerWidth < 1500) return 4;
    return 5;
  }

  function onDragOver(event: DragEvent) {
    if (event.dataTransfer?.types.includes("Files")) {
      event.preventDefault();
      dragOver = true;
    }
  }

  function onDragLeave(event: DragEvent) {
    if (!(event.currentTarget as Element)?.contains(event.relatedTarget as Node)) {
      dragOver = false;
    }
  }

  async function onDrop(event: DragEvent) {
    event.preventDefault();
    dragOver = false;
    await uploadFiles(Array.from(event.dataTransfer?.files ?? []));
  }

  function setUploadItem(id: string, patch: Partial<UploadQueueItem>) {
    uploadQueue = uploadQueue.map(item => (item.id === id ? { ...item, ...patch } : item));
  }

  function clearCompletedUploads() {
    uploadQueue = uploadQueue.filter(item => item.status !== "done" && item.status !== "interrupted");
    if (uploadQueue.length === 0) {
      try { localStorage.removeItem(QUEUE_STORAGE_KEY); } catch {}
    }
  }

  function createUploadQueueItem(file: File, targetPath: string): UploadQueueItem {
    return {
      id: crypto.randomUUID(),
      file,
      name: file.name,
      size: file.size,
      targetPath,
      progress: 0,
      status: "queued",
      attempts: 0,
    };
  }

  function loadQueueFromStorage(): UploadQueueItem[] {
    try {
      const raw = localStorage.getItem(QUEUE_STORAGE_KEY);
      if (!raw) return [];
      return JSON.parse(raw).map((item: UploadQueueItem) => ({ ...item, file: null }));
    } catch {
      return [];
    }
  }

  function saveQueueToStorage(queue: UploadQueueItem[]) {
    const items = queue
      .filter(item => item.status === "done" || item.status === "error" || item.status === "interrupted")
      .map(({ file, ...item }) => item);
    try {
      localStorage.setItem(QUEUE_STORAGE_KEY, JSON.stringify(items));
    } catch {}
  }

  function filterEntries(items: FileEntry[], filter: FileTypeFilter) {
    return items.filter(entry => {
      if (filter === "all") return true;
      if (filter === "folders") return entry.type === "dir";
      if (entry.type === "dir") return false;
      const kind = entry.preview_kind || "";
      if (filter === "images") return kind === "image" || kind === "raw";
      if (filter === "videos") return kind === "video";
      if (filter === "documents") return kind === "pdf" || kind === "office";
      if (filter === "text") return kind === "text" || kind === "markdown";
      if (filter === "3d") return kind === "3d";
      return !["image", "raw", "video", "pdf", "office", "text", "markdown", "3d"].includes(kind);
    });
  }

  function sortEntries(items: FileEntry[], option: SortOption) {
    return [...items].sort((a, b) => {
      if (a.type !== b.type) return a.type === "dir" ? -1 : 1;
      let result = 0;
      if (option === "size_asc" || option === "size_desc") result = a.size - b.size;
      else if (option === "modified_asc" || option === "modified_desc") result = new Date(a.modified_at).getTime() - new Date(b.modified_at).getTime();
      else if (option === "type_asc") result = fileTypeLabel(a).localeCompare(fileTypeLabel(b), undefined, { sensitivity: "base" }) || a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: "base" });
      else result = a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: "base" });

      if (option === "name_desc" || option === "modified_desc" || option === "size_desc") result *= -1;
      return result;
    });
  }

  function setTableSort(key: "name" | "size" | "modified") {
    if (key === "name") sortOption = sortOption === "name_asc" ? "name_desc" : "name_asc";
    if (key === "size") sortOption = sortOption === "size_asc" ? "size_desc" : "size_asc";
    if (key === "modified") sortOption = sortOption === "modified_asc" ? "modified_desc" : "modified_asc";
  }

  function changeSort(event: Event) {
    sortOption = (event.currentTarget as HTMLSelectElement).value as SortOption;
  }

  function changeFileTypeFilter(event: Event) {
    fileTypeFilter = (event.currentTarget as HTMLSelectElement).value as FileTypeFilter;
    selectedIds = selectedIds.filter(id => filterEntries(entries, fileTypeFilter).some(entry => entry.path === id));
  }

  function setActionDialogValue(value: string) {
    if (!actionDialog) return;
    actionDialog = { ...actionDialog, value };
  }

  function entryByPath(path: string) {
    return entries.find(entry => entry.path === path) || treeEntries.find(entry => entry.path === path) || null;
  }

  function treeRows(tree: FileEntry[], currentEntries: FileEntry[], path: string, opened: Set<string>) {
    const byParent = new Map<string, FileEntry[]>();
    const folders = mergeFolderLists(
      ...tree,
      ...currentPathFallbackEntries(path),
      ...currentEntries.filter(entry => entry.type === "dir" && parentPath(entry.path) === path)
    );
    const seen = new Set<string>();
    for (const entry of folders) {
      if (entry.type !== "dir" || entry.path === "/" || seen.has(entry.path)) continue;
      seen.add(entry.path);
      const parent = parentPath(entry.path);
      const list = byParent.get(parent) || [];
      list.push(entry);
      byParent.set(parent, list);
    }
    for (const list of byParent.values()) {
      list.sort((a, b) => a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: "base" }));
    }
    const rows: TreeRow[] = [];
    const visit = (parent: string, level: number) => {
      if (!opened.has(parent)) return;
      for (const entry of byParent.get(parent) || []) {
        rows.push({ ...entry, level });
        visit(entry.path, level + 1);
      }
    };
    visit("/", 1);
    return rows;
  }

  function mergeTreeEntries(folders: FileEntry[]) {
    treeEntries = mergeFolderLists(...treeEntries, ...folders);
  }

  function mergeFolderLists(...folders: FileEntry[]) {
    const byPath = new Map<string, FileEntry>();
    for (const raw of folders) {
      if (raw.type !== "dir") continue;
      const path = normalizePath(raw.path || raw.name);
      if (path === "/") continue;
      byPath.set(path, {
        ...raw,
        name: raw.name || basename(path),
        path,
        type: "dir"
      });
    }
    return [...byPath.values()].sort((a, b) => a.path.localeCompare(b.path, undefined, { numeric: true, sensitivity: "base" }));
  }

  function currentPathFallbackEntries(pathValue: string) {
    const rows: FileEntry[] = [];
    const parts = pathValue.split("/").filter(Boolean);
    let path = "";
    for (const part of parts) {
      path = joinPath(path || "/", part);
      rows.push({
        name: part,
        path,
        type: "dir",
        size: 0,
        modified_at: new Date(0).toISOString()
      });
    }
    return rows;
  }

  function masonryLayout(node: HTMLElement) {
    const ROW_PX = 4;
    function layout() {
      const isMasonry = node.classList.contains('masonry');
      node.querySelectorAll<HTMLElement>('.file-card').forEach(card => {
        card.style.gridRowEnd = '';
        if (!isMasonry) return;
        const h = card.getBoundingClientRect().height;
        const span = Math.ceil((h + ROW_PX) / ROW_PX);
        card.style.gridRowEnd = `span ${span}`;
      });
    }
    const ro = new ResizeObserver(layout);
    ro.observe(node);
    node.addEventListener('load', layout, true); // image load (capture)
    layout();
    return {
      update() { layout(); },
      destroy() { ro.disconnect(); node.removeEventListener('load', layout, true); }
    };
  }

  function fileTypeLabel(entry: FileEntry) {
    if (entry.type === "dir") return "Folder";
    const kind = entry.preview_kind || "";
    if (kind === "image" || kind === "raw") return "Image";
    if (kind === "video") return "Video";
    if (kind === "pdf") return "PDF";
    if (kind === "office") return "Document";
    if (kind === "text" || kind === "markdown") return "Text";
    if (kind === "3d") return "3D";
    return (entry.name.split(".").pop() || "File").toUpperCase();
  }

  function fileIconName(entry: FileEntry) {
    if (entry.type === "dir") return "folder";
    if (entry.preview_kind === "image" || entry.preview_kind === "raw") return "image";
    return "file";
  }

  function trashKindLabel(item: TrashItem) {
    if (item.is_dir) return "Folder";
    return (item.original_name.split(".").pop() || "File").toUpperCase();
  }

  function trashKindIcon(item: TrashItem) {
    return item.is_dir ? "folder" : "file";
  }

  function trashHasPreview(item: TrashItem) {
    if (item.is_dir) return false;
    const ext = item.original_name.split(".").pop()?.toLowerCase() || "";
    return ["jpg", "jpeg", "png", "gif", "webp", "heic", "heif", "avif", "tif", "tiff", "bmp", "raw", "cr2", "nef", "arw", "dng", "pdf", "mp4", "mov", "mkv", "webm", "avi", "doc", "docx", "xls", "xlsx", "ppt", "pptx", "odt", "ods", "odp"].includes(ext);
  }

  function toggleTree(path: string) {
    const next = new Set(openTree);
    if (next.has(path)) next.delete(path);
    else next.add(path);
    openTree = next;
  }

  function openAncestors(path: string) {
    const next = new Set(openTree);
    next.add("/");
    let current = parentPath(path);
    while (current && current !== "/") {
      next.add(current);
      current = parentPath(current);
    }
    openTree = next;
  }

  async function submitSearch() {
    const query = searchQuery.trim();
    if (!query) {
      searchOpen = false;
      searchResults = [];
      return;
    }
    await runAction("Searching", async () => {
      searchResults = (await searchFiles(query, 80)).entries;
      searchOpen = true;
      await tick();
      searchDialogInput?.focus();
    });
  }

  function onSearchInput() {
    if (searchDebounce) clearTimeout(searchDebounce);
    if (!searchQuery.trim()) {
      searchOpen = false;
      searchResults = [];
      return;
    }
    searchDebounce = setTimeout(() => void submitSearch(), 300);
  }

  function breadcrumbs() {
    const parts = currentPath.split("/").filter(Boolean);
    const rows = [{ name: "My files", path: "/" }];
    let current = "";
    for (const part of parts) {
      current += `/${part}`;
      rows.push({ name: part, path: current });
    }
    return rows;
  }

  function pathFromURL() {
    const pathname = window.location.pathname;
    if (pathname === "/" || pathname === "/files") return "/";
    const prefix = "/files/";
    if (!pathname.startsWith(prefix)) return "/";
    return normalizePath(`/${pathname.slice(prefix.length).split("/").filter(Boolean).map(decodeURIComponent).join("/")}`);
  }

  function syncURLPath(path: string) {
    const clean = normalizePath(path);
    const route = clean === "/" ? "/files" : `/files/${clean.split("/").filter(Boolean).map(encodeURIComponent).join("/")}`;
    window.history.replaceState(null, "", route);
  }

  function normalizePath(path: string) {
    const parts = path.replaceAll("\\", "/").split("/").filter(Boolean);
    return parts.length ? `/${parts.join("/")}` : "/";
  }

  function basename(path: string) {
    return path.split("/").filter(Boolean).pop() || "download";
  }

  function formatBytes(value: number) {
    if (!Number.isFinite(value) || value < 0) return "0 B";
    if (value < 1024) return `${value} B`;
    const units = ["KB", "MB", "GB", "TB"];
    let size = value / 1024;
    for (const unit of units) {
      if (size < 1024) return `${size.toFixed(size >= 10 ? 0 : 1)} ${unit}`;
      size /= 1024;
    }
    return `${size.toFixed(1)} PB`;
  }

  function formatDate(value: string) {
    return new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
  }

  function previewURL(entry: FileEntry, size = 420) {
    return thumbnailKinds.has(entry.preview_kind || "") ? thumbnailURL(entry.path, size) : "";
  }

  function jobProgress(job: AdminJob) {
    const deleted = job.deleted ? `, ${job.deleted} deleted` : "";
    return job.total_known ? `${job.done}/${job.total}${deleted}` : `${job.done} indexed${deleted}`;
  }

  function progressValue(job: AdminJob) {
    if (!job.total_known || job.total === 0) return 0;
    return Math.min(100, Math.round((job.done / job.total) * 100));
  }

  function uploadStatusText(item: UploadQueueItem) {
    if (item.status === "uploading") return `${Math.round(item.progress)}%`;
    if (item.status === "queued") return "Waiting";
    if (item.status === "done") return "Done";
    if (item.status === "interrupted") return "Interrupted";
    return "Failed";
  }

  function uploadSummaryText(queue: UploadQueueItem[]) {
    const total = queue.length;
    const done = queue.filter(i => i.status === "done").length;
    const failed = queue.filter(i => i.status === "error").length;
    if (failed > 0) return `${done}/${total} · ${failed} failed`;
    if (done === total) return `${total} done`;
    return `${done}/${total}`;
  }

  function emptyNewUser() {
    return { username: "", password: "", home_root: "", is_admin: false, disabled: false };
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

  function assertBulkSuccess(results: Array<{ path: string; ok: boolean; error?: string }>) {
    const failed = results.filter(result => !result.ok);
    if (failed.length) throw new Error(`${failed.length} item(s) failed: ${failed[0].path} ${failed[0].error || ""}`.trim());
  }

  function messageFromError(err: unknown) {
    return err instanceof Error ? err.message : "Request failed";
  }

  function shouldIgnoreShortcut(target: EventTarget | null) {
    if (!(target instanceof HTMLElement)) return false;
    const tag = target.tagName.toLowerCase();
    return tag === "input" || tag === "textarea" || tag === "select" || target.isContentEditable;
  }
</script>

{#if loading}
  <main class="app app-centered"><div class="status-card">Loading</div></main>
{:else if !user}
  <main class="app app-centered">
    <form class="login-card" on:submit|preventDefault={submitLogin}>
      <div>
        <p class="eyebrow">goDrive</p>
        <h1>Sign in</h1>
      </div>
      {#if error}<div class="error" role="alert">{error}</div>{/if}
      <label><span>Username</span><input bind:value={username} autocomplete="username" /></label>
      <label><span>Password</span><input bind:value={password} type="password" autocomplete="current-password" /></label>
      <button type="submit">Sign in</button>
    </form>
  </main>
{:else}
  <main class="app" on:dragover={onDragOver} on:dragleave={onDragLeave} on:drop={onDrop}>
    {#if mobileNavOpen}<button class="mobile-scrim" type="button" aria-label="Close navigation" on:click={() => (mobileNavOpen = false)}></button>{/if}
    <aside class="sidebar" class:mobile-open={mobileNavOpen}>
      <div class="brand">
        <strong>goDrive</strong>
        <span>{user.username}</span>
      </div>
      <button class:active={currentPath === "/"} type="button" on:click={() => { mobileNavOpen = false; loadPath("/", { push: true }); }}><Icon name="folder" />My files</button>
      <div class="tree">
        {#if visibleTree.length === 0}
          <div class="tree-empty">No folders found</div>
        {:else}
          {#each visibleTree as row (row.path)}
          <div class="tree-row" role="listitem" class:active={currentPath === row.path} class:drag-target={dragTargetPath === row.path} style={`--level: ${row.level}`}
            on:dragover|preventDefault={(e) => { e.dataTransfer!.dropEffect = 'move'; dragTargetPath = row.path; }}
            on:dragleave={() => { dragTargetPath = null; }}
            on:drop|preventDefault={async (e) => {
              dragTargetPath = null;
              const raw = e.dataTransfer!.getData('godrive/paths');
              if (!raw) return;
              const paths = JSON.parse(raw) as string[];
              if (paths.includes(row.path)) return;
              await runAction('Moving', async () => {
                const res = await bulkMove(paths, row.path);
                assertBulkSuccess(res.results);
                await refreshCurrentFolder();
                addToast(`Moved to ${row.path}`, 'success');
              });
            }}
          >
            <button type="button" class="tree-toggle" on:click={() => toggleTree(row.path)} disabled={!treeChildPaths.has(row.path)}>
              {#if treeChildPaths.has(row.path)}<Icon name={openTree.has(row.path) ? "chevronDown" : "chevronRight"} />{/if}
            </button>
            <button type="button" class="tree-link"
              on:click={() => { if (treeChildPaths.has(row.path)) toggleTree(row.path); else { mobileNavOpen = false; loadPath(row.path, { push: true }); } }}
              on:dblclick={() => { mobileNavOpen = false; loadPath(row.path, { push: true }); }}
            ><Icon name="folder" />{row.name}</button>
          </div>
          {/each}
        {/if}
      </div>
      <div class="sidebar-trash">
        <button type="button" on:click={() => { mobileNavOpen = false; openTrash(); }}><Icon name="trash" />Trash</button>
      </div>
    </aside>

    <section class="workspace">
      <header class="topbar">
        <button class="mobile-menu-button" type="button" aria-label="Open navigation" on:click={() => (mobileNavOpen = true)}><Icon name="menu" /></button>
        <form class="search" on:submit|preventDefault={submitSearch}>
          <span class="search-icon"><Icon name="search" /></span>
          <input bind:value={searchQuery} placeholder="Search files…" on:input={onSearchInput} on:focus={() => { if (searchQuery.trim()) searchOpen = true; }} readonly={searchOpen} />
          {#if busy}<span class="search-spinner"></span>{/if}
        </form>
        <div class="topbar-actions">
          <div class="topbar-btn-group">
            <button type="button" title="Refresh" on:click={refreshCurrentFolder}><Icon name="refresh" /></button>
            <button type="button" title="Keyboard shortcuts" on:click={() => (shortcutsOpen = true)}><Icon name="keyboard" /></button>
            {#if user.is_admin}<button type="button" title="Admin" on:click={openAdmin}><Icon name="admin" /></button>{/if}
          </div>
          <button class="theme-toggle" type="button" title={darkMode ? 'Light mode' : 'Dark mode'} on:click={toggleTheme}>{darkMode ? '☀️' : '🌙'}</button>
          <button class="topbar-logout" type="button" title="Logout" on:click={submitLogout}><Icon name="logout" /></button>
        </div>
      </header>

      {#if error}<div class="error" role="alert"><span>{error}</span><button type="button" on:click={() => (error = "")}>×</button></div>{/if}
      {#if demoMode}
        <div class="demo-banner" role="status">
          <strong>Public demo</strong>
          <span>Data is visible to anyone using this instance, resets on restart, and write/admin actions are disabled.</span>
        </div>
      {/if}

      <div class="pathbar">
        <div class="crumbs">
          {#each breadcrumbs() as crumb, index}
            {#if index > 0}<span>/</span>{/if}
            <button type="button" on:click={() => loadPath(crumb.path, { push: true })}>{crumb.name}</button>
          {/each}
        </div>
        <div class="view-toggle view-controls">
          <button class:active={viewMode === "grid"} type="button" title="Grid view" on:click={() => { viewMode = "grid"; try { localStorage.setItem('godrive_view', 'grid'); } catch {} }}><Icon name="grid" /></button>
          <button class:active={viewMode === "masonry"} type="button" title="Masonry view" on:click={() => { viewMode = "masonry"; try { localStorage.setItem('godrive_view', 'masonry'); } catch {} }}>
            <svg viewBox="0 0 16 16" fill="currentColor" width="16" height="16"><rect x="1" y="1" width="6" height="4" rx="1"/><rect x="9" y="1" width="6" height="6" rx="1"/><rect x="1" y="7" width="6" height="8" rx="1"/><rect x="9" y="9" width="6" height="6" rx="1"/></svg>
          </button>
          <button class:active={viewMode === "list"} type="button" title="List view" on:click={() => { viewMode = "list"; try { localStorage.setItem('godrive_view', 'list'); } catch {} }}><Icon name="list" /></button>
          <span class="view-sep"></span>
          <button class:active={gridSize === "s"} type="button" title="Small" on:click={() => setGridSize("s")}>S</button>
          <button class:active={gridSize === "m"} type="button" title="Medium" on:click={() => setGridSize("m")}>M</button>
          <button class:active={gridSize === "l"} type="button" title="Large" on:click={() => setGridSize("l")}>L</button>
        </div>
      </div>

      {#if entries.length > 0}
        <div class="folder-stats">
          {entries.length}{folderHasMore ? '+' : ''} item{entries.length !== 1 ? 's' : ''}
          {#if selectedIds.length > 0} · {selectedIds.length} selected{/if}
        </div>
      {/if}

      <div class="commandbar">
        <button type="button" on:click={createFolder}><Icon name="plus" />New folder</button>
        <button type="button" on:click={() => uploadInput?.click()}><Icon name="upload" />Upload</button>
        <input bind:this={uploadInput} class="hidden-input" type="file" multiple on:change={uploadSelectedFiles} />
        <button type="button" disabled={selectedIds.length !== 1} on:click={() => renameSelected()}><Icon name="rename" />Rename</button>
        <button type="button" disabled={selectedIds.length === 0} on:click={moveSelected}><Icon name="move" />Move</button>
        <button type="button" disabled={selectedIds.length === 0} on:click={downloadSelected}><Icon name="download" />Download</button>
        <button type="button" disabled={selectedIds.length === 0} on:click={deleteSelected}><Icon name="trash" />Delete</button>
        <button type="button" disabled={selectedIds.length !== 1} on:click={() => copyPath(selectedIds[0])}><Icon name="copy" />Copy path</button>
        <button type="button" disabled={selectedIds.length !== 1} on:click={() => openInfoPanel(entryByPath(selectedIds[0]))}><Icon name="info" />Info</button>
        <div class="spacer"></div>
        <label class="control-field"><Icon name="sort" /><span>Sort</span><select bind:value={sortOption} on:change={changeSort}>
          <option value="name_asc">Name A-Z</option>
          <option value="name_desc">Name Z-A</option>
          <option value="modified_desc">Modified newest</option>
          <option value="modified_asc">Modified oldest</option>
          <option value="size_desc">Size largest</option>
          <option value="size_asc">Size smallest</option>
          <option value="type_asc">File type</option>
        </select></label>
        <label class="control-field"><Icon name="filter" /><span>Type</span><select bind:value={fileTypeFilter} on:change={changeFileTypeFilter}>
          <option value="all">All</option>
          <option value="folders">Folders</option>
          <option value="images">Images</option>
          <option value="videos">Videos</option>
          <option value="documents">Documents</option>
          <option value="text">Text</option>
          <option value="3d">3D</option>
          <option value="other">Other</option>
        </select></label>
      </div>

      {#if selectedIds.length > 0}
        <div class="selectionbar">
          <span>{selectedIds.length} selected · {formatBytes(selectedSize)}</span>
          <button type="button" on:click={() => (selectedIds = [])}>Clear</button>
        </div>
      {/if}

      <section class="file-area" class:list={viewMode === "list"} aria-label="File list"
        style="--grid-min:{gridSize==='s'?'160px':gridSize==='l'?'260px':'210px'};--list-thumb:{gridSize==='s'?'26px':gridSize==='l'?'52px':'36px'};--list-pad:{gridSize==='s'?'5px 10px':gridSize==='l'?'10px 14px':'7px 12px'};--list-fs:{gridSize==='s'?'12.5px':gridSize==='l'?'14.5px':'13.5px'}">
        {#if currentPath !== "/"}
          <button type="button" class="parent-link" on:click={() => loadPath(parentPath(currentPath), { push: true })}>
            ← Back to parent folder
          </button>
        {/if}

        {#if busy === "Loading folder" && entries.length === 0}
          <div class="grid">
            {#each Array(12) as _, i}
              <div class="file-card skeleton-card" style="animation-delay: {i * 40}ms"></div>
            {/each}
          </div>
        {/if}

        {#if visibleEntries.length === 0 && !busy}
          <div class="empty-state">
            <div class="empty-icon">📁</div>
            <p class="empty-title">This folder is empty</p>
            <p class="empty-sub">Drop files here or use the Upload button to add files.</p>
          </div>
        {:else}

        {#if viewMode === "grid" || viewMode === "masonry"}
          <div class="grid" class:masonry={viewMode === "masonry"} use:masonryLayout>
            {#each visibleEntries as entry (entry.path)}
              <button
                type="button"
                class="file-card"
                class:selected={selectedIds.includes(entry.path)}
                class:masonry-card={viewMode === "masonry"}
                draggable={true}
                on:click={(event) => selectEntry(entry, event)}
                on:dblclick={() => openEntry(entry)}
                on:contextmenu={(e) => openContextMenu(e, entry)}
                on:dragstart={(e) => {
                  const paths = selectedIds.includes(entry.path) ? selectedIds : [entry.path];
                  e.dataTransfer!.setData('godrive/paths', JSON.stringify(paths));
                  e.dataTransfer!.effectAllowed = 'move';
                }}
              >
                <div class="thumb" class:folder={entry.type === "dir"} class:masonry-thumb={viewMode === "masonry" && thumbnailKinds.has(entry.preview_kind || "")}>
                  {#if entry.type === "dir"}
                    <span class="folder-icon"></span>
                  {:else if thumbnailKinds.has(entry.preview_kind || "")}
                    <img src={previewURL(entry)} alt="" loading="lazy" />
                  {:else if (entry.preview_kind === "text" || entry.preview_kind === "markdown") && entry.snippet}
                    <span class="text-thumb"><span class="text-thumb-content">{entry.snippet}</span></span>
                  {:else if entry.preview_kind === "3d"}
                    <ThreeDGridThumb src={rawFileURL(entry.path)} name={entry.name} path={entry.path} />
                  {:else}
                    <span class="file-icon"><Icon name={fileIconName(entry)} />{(entry.preview_kind || entry.name.split(".").pop() || "file").slice(0, 4).toUpperCase()}</span>
                  {/if}
                </div>
                <div class="file-name">{entry.name}</div>
                <div class="file-meta">{fileTypeLabel(entry)}{entry.type === "file" ? ` · ${formatBytes(entry.size)}` : ""}</div>
              </button>
            {/each}
          </div>
        {:else}
          <table class="file-table">
            <thead>
              <tr>
                <th><button type="button" on:click={() => setTableSort("name")}>Name {sortOption==='name_asc'?'↑':sortOption==='name_desc'?'↓':''}</button></th>
                <th><button type="button" on:click={() => (sortOption = 'type_asc')}>Type</button></th>
                <th><button type="button" on:click={() => setTableSort("size")}>Size {sortOption==='size_asc'?'↑':sortOption==='size_desc'?'↓':''}</button></th>
                <th><button type="button" on:click={() => setTableSort("modified")}>Modified {sortOption==='modified_asc'?'↑':sortOption==='modified_desc'?'↓':''}</button></th>
              </tr>
            </thead>
            <tbody>
              {#each visibleEntries as entry (entry.path)}
                <tr class:selected={selectedIds.includes(entry.path)} on:click={(event) => selectEntry(entry, event)} on:dblclick={() => openEntry(entry)} on:contextmenu={(e) => openContextMenu(e, entry)}>
                  <td>
                    <span class="list-name">
                      <span class="list-thumb" class:folder={entry.type === "dir"}>
                        {#if entry.type === "dir"}
                          <span class="mini-folder"></span>
                        {:else if thumbnailKinds.has(entry.preview_kind || "")}
                          <img src={previewURL(entry, 128)} alt="" loading="lazy" />
                        {:else}
                          <Icon name={fileIconName(entry)} />
                        {/if}
                      </span>
                      <span>{entry.name}</span>
                    </span>
                  </td>
                  <td>{fileTypeLabel(entry)}</td>
                  <td>{entry.type === "dir" ? "Folder" : formatBytes(entry.size)}</td>
                  <td>{formatDate(entry.modified_at)}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        {/if}

        <div bind:this={sentinelEl} class="load-sentinel" use:observeSentinel></div>
        {/if}
      </section>
    </section>

    {#if dragOver}<div class="drop-overlay">Drop to upload to {currentPath}</div>{/if}

    {#if searchOpen}
      <div class="search-overlay" role="dialog" aria-modal="true" on:click={(e) => e.target === e.currentTarget && (searchOpen = false)} on:keydown={(e) => e.key === "Escape" && (searchOpen = false)} tabindex="-1">
        <div class="search-dialog">
          <form class="search-dialog-input" on:submit|preventDefault={submitSearch}>
            <Icon name="search" />
            <input bind:this={searchDialogInput} bind:value={searchQuery} placeholder="Search files…" on:input={onSearchInput} />
            {#if busy}<span class="search-spinner"></span>{/if}
            <button type="button" class="search-close" on:click={() => (searchOpen = false)}>Esc</button>
          </form>
          {#if searchResults.length > 0}
            <div class="search-filter-row">
              <span class="search-count">{searchResults.length} result{searchResults.length !== 1 ? 's' : ''}</span>
            </div>
            <div class="search-results">
              {#each searchResults as result (result.path)}
                <button type="button" class="search-result" on:click={() => { searchOpen = false; if (result.type === "dir") { loadPath(result.path, { push: true }); } else { loadPath(parentPath(result.path), { push: true }); setTimeout(() => openEntry(result), 400); } }}>
                  <div class="search-result-thumb" class:dir={result.type === "dir"}>
                    {#if thumbnailKinds.has(result.preview_kind || "")}
                      <img src={thumbnailURL(result.path, 128)} alt="" loading="lazy" />
                    {:else if result.type === "dir"}
                      <span class="mini-folder"></span>
                    {:else}
                      <Icon name={fileIconName(result)} />
                    {/if}
                  </div>
                  <div class="search-result-body">
                    <span class="search-result-name">{result.name}</span>
                    <span class="search-result-path">{result.path}</span>
                    {#if result.snippet}<span class="search-result-snippet">{@html result.snippet}</span>{/if}
                  </div>
                  <span class="search-result-kind">{result.type === "dir" ? "Folder" : fileTypeLabel(result)}</span>
                </button>
              {/each}
            </div>
          {:else if searchQuery.length > 0 && !busy}
            <div class="search-empty">No results for <strong>{searchQuery}</strong></div>
          {:else}
            <div class="search-empty">Type to search across all your files</div>
          {/if}
        </div>
      </div>
    {/if}

    {#if viewerFile}
      <div class="viewer" class:viewer-sidebar-open={viewerSidebarOpen} role="dialog" aria-modal="true" aria-label={viewerFile.name} tabindex="-1"
        on:keydown={(e) => {
          if (e.key === "Escape") closeViewer();
          else if (e.key === "ArrowLeft" && viewerFile?.preview_kind === "image") showAdjacentImage(-1);
          else if (e.key === "ArrowRight" && viewerFile?.preview_kind === "image") showAdjacentImage(1);
        }}
        on:wheel|passive={(e) => {
          if (viewerFile?.preview_kind === "image" || viewerFile?.preview_kind === "raw") {
            viewerZoom = Math.min(5, Math.max(0.25, viewerZoom - e.deltaY * 0.001));
          }
        }}
      >
        <!-- Header -->
        <header class="viewer-header">
          <button class="viewer-close" type="button" title="Close (Esc)" on:click={closeViewer}>
            <Icon name="chevronLeft" /><span>Back</span>
          </button>
          <div class="viewer-title">
            <strong title={viewerFile.name}>{viewerFile.name}</strong>
            <span>{fileTypeLabel(viewerFile)} · {formatBytes(viewerFile.size)}</span>
          </div>
          <div class="viewer-toolbar">
            {#if viewerFile.preview_kind === "image" || viewerFile.preview_kind === "raw"}
              <div class="viewer-stepper">
                <button type="button" title="Previous (←)" on:click={() => showAdjacentImage(-1)}><Icon name="chevronLeft" /></button>
                <button type="button" title="Next (→)" on:click={() => showAdjacentImage(1)}><Icon name="chevronRight" /></button>
              </div>
              <div class="viewer-stepper">
                <button type="button" title="Zoom out" on:click={() => (viewerZoom = Math.max(0.25, viewerZoom - 0.25))}>−</button>
                <button type="button" title="Reset zoom" on:click={() => (viewerZoom = 1)}>{Math.round(viewerZoom * 100)}%</button>
                <button type="button" title="Zoom in" on:click={() => (viewerZoom = Math.min(5, viewerZoom + 0.25))}>+</button>
              </div>
              <button type="button" class:active={viewerOriginal} title="Toggle original" on:click={() => (viewerOriginal = !viewerOriginal)}>
                {viewerOriginal ? "Preview" : "Original"}
              </button>
            {/if}
            {#if viewerFile.preview_kind === "text" || viewerFile.preview_kind === "markdown"}
              <button type="button" on:click={() => (editorMode = !editorMode)}>{editorMode ? "Preview" : "Edit"}</button>
            {/if}
            <button type="button" title="Download" on:click={downloadSelected}><Icon name="download" /></button>
            <button type="button" class:active={viewerSidebarOpen} title="Info panel" on:click={toggleViewerSidebar}>
              <Icon name="info" />
            </button>
          </div>
        </header>

        <!-- Body: content + sidebar -->
        <div class="viewer-body">
          <!-- Content area -->
          <div class="viewer-content">
            {#if viewerFile.preview_kind === "image" || viewerFile.preview_kind === "raw" || viewerFile.preview_kind === "office"}
              <div class="viewer-stage">
                <img style={`transform: scale(${viewerZoom})`} src={viewerFile.preview_kind === "image" && viewerOriginal ? rawFileURL(viewerFile.path) : thumbnailURL(viewerFile.path, 2048)} alt={viewerFile.name} />
              </div>
              {#if viewerFile.preview_kind === "image" || viewerFile.preview_kind === "raw"}
                <button class="viewer-nav viewer-nav-prev" type="button" title="Previous (←)" on:click={() => showAdjacentImage(-1)}><Icon name="chevronLeft" /></button>
                <button class="viewer-nav viewer-nav-next" type="button" title="Next (→)" on:click={() => showAdjacentImage(1)}><Icon name="chevronRight" /></button>
              {/if}
            {:else if viewerFile.preview_kind === "video"}
              <video controls src={rawFileURL(viewerFile.path)}><track kind="captions" /></video>
            {:else if viewerFile.preview_kind === "pdf"}
              <iframe src={rawFileURL(viewerFile.path)} title={viewerFile.name}></iframe>
            {:else if viewerFile.preview_kind === "text" || viewerFile.preview_kind === "markdown"}
              {#if editorMode}
                <div class="code-editor-wrap">
                  <div class="code-editor-toolbar">
                    <span>{viewerFile.path}</span>
                    <button class="cancel-btn" on:click={() => { editorMode = false; editorDirty = false; }}>Preview</button>
                    <button class="save-btn" disabled={!editorDirty || editorSaving} on:click={saveEditorContent}>{editorSaving ? "Saving…" : "Save"}</button>
                  </div>
                  <CodeEditor bind:this={editorRef} content={viewerText?.content ?? ''} filename={viewerFile.name} onChange={(v) => { editorContent = v; editorDirty = true; }} />
                </div>
              {:else}
                <div class="text-preview">
                  {#if viewerTextLoading}
                    <p>Loading...</p>
                  {:else if viewerTextError}
                    <p class="viewer-error">Failed to load preview: {viewerTextError}</p>
                  {:else if viewerText}
                    <div class="code-editor-toolbar">
                      <span>{viewerFile.name}</span>
                      <button class="cancel-btn" on:click={() => { editorMode = true; editorContent = viewerText?.content ?? ''; }}>Edit</button>
                    </div>
                    <pre>{viewerText.content}</pre>
                  {:else}
                    <p>Preview unavailable.</p>
                  {/if}
                </div>
              {/if}
            {:else if viewerFile.preview_kind === "3d"}
              {#await loadThreeDViewer()}
                <div class="viewer-loading">Loading 3D viewer…</div>
              {:then module}
                <svelte:component this={module.default} src={rawFileURL(viewerFile.path)} name={viewerFile.name} />
              {:catch}
                <div class="viewer-loading">3D viewer unavailable.</div>
              {/await}
            {/if}
          </div>

          <!-- Collapsible info sidebar -->
          {#if viewerSidebarOpen}
            <aside class="viewer-sidebar" on:wheel|stopPropagation>
              <div class="viewer-sidebar-section">
                <h3>File</h3>
                <div class="vsb-row"><span>Name</span><strong title={viewerFile.name}>{viewerFile.name}</strong></div>
                <div class="vsb-row"><span>Path</span><strong class="vsb-mono" title={viewerFile.path}>{viewerFile.path}</strong></div>
                <div class="vsb-row"><span>Type</span><strong>{fileTypeLabel(viewerFile)}</strong></div>
                <div class="vsb-row"><span>Size</span><strong>{formatBytes(viewerFile.size)}</strong></div>
                <div class="vsb-row"><span>Modified</span><strong>{formatDate(viewerFile.modified_at)}</strong></div>
              </div>
              {#if viewerExifLoading}
                <div class="viewer-sidebar-section"><p class="vsb-loading">Loading EXIF…</p></div>
              {:else if viewerExif}
                <div class="viewer-sidebar-section">
                  <h3>EXIF</h3>
                  {#if viewerExif.has_gps}
                    <div class="vsb-row vsb-gps">
                      <span>GPS</span>
                      <strong class="vsb-mono">{viewerExif.gps_lat?.toFixed(5)}, {viewerExif.gps_lon?.toFixed(5)}</strong>
                    </div>
                  {/if}
                  {#each Object.entries(viewerExif.fields) as [key, value]}
                    {#if key !== 'GPSLatitude' && key !== 'GPSLongitude' && key !== 'GPSPosition' && key !== 'FileName' && key !== 'FileSize' && key !== 'FileModifyDate' && key !== 'FileType' && key !== 'FileTypeExtension' && key !== 'MIMEType'}
                      <div class="vsb-row">
                        <span class="vsb-key">{key.replace(/([A-Z])/g, ' $1').trim()}</span>
                        <strong class="vsb-val vsb-mono">{typeof value === 'object' ? JSON.stringify(value) : String(value)}</strong>
                      </div>
                    {/if}
                  {/each}
                </div>
              {/if}
            </aside>
          {/if}
        </div>
      </div>
    {/if}

    {#if actionDialog}
      <div class="modal-backdrop" role="presentation" tabindex="-1" on:click={(event) => event.target === event.currentTarget && (actionDialog = null)} on:keydown={(event) => event.key === "Escape" && (actionDialog = null)}>
        <form class="modal-panel action-panel" on:submit|preventDefault={submitActionDialog}>
          <header><h2>{actionDialog.title}</h2><button type="button" on:click={() => (actionDialog = null)}>×</button></header>
          {#if actionDialog.message}<p>{actionDialog.message}</p>{/if}
          {#if actionDialog.kind !== "delete"}
            <label>
              <span>{actionDialog.kind === "move" ? "Destination folder" : "Name"}</span>
              <input bind:value={actionDialog.value} />
            </label>
            {#if actionDialog.kind === "move"}
              <div class="folder-picks">
                <button type="button" on:click={() => setActionDialogValue("/")}>/</button>
                {#each visibleTree as row (row.path)}
                  <button type="button" style={`--level: ${row.level}`} on:click={() => setActionDialogValue(row.path)}>{row.path}</button>
                {/each}
              </div>
            {/if}
          {/if}
          <footer>
            <button type="button" on:click={() => (actionDialog = null)}>Cancel</button>
            <button type="submit">{actionDialog.kind === "delete" ? "Move to trash" : "Apply"}</button>
          </footer>
        </form>
      </div>
    {/if}

    {#if trashOpen}
      <div class="modal-backdrop" role="presentation" tabindex="-1" on:click={(event) => event.target === event.currentTarget && (trashOpen = false)} on:keydown={(event) => event.key === "Escape" && (trashOpen = false)}>
        <section class="modal-panel trash-panel">
          <header>
            <div><h2>Trash</h2><span>{trashItems.length} item(s)</span></div>
            <button type="button" on:click={() => (trashOpen = false)}>×</button>
          </header>
          {#if trashBusy}<p>{trashBusy}</p>{/if}
          <div class="trash-toolbar">
            <div class="view-toggle">
              <button class:active={trashViewMode === "grid"} type="button" title="Grid view" on:click={() => (trashViewMode = "grid")}><Icon name="grid" /></button>
              <button class:active={trashViewMode === "list"} type="button" title="List view" on:click={() => (trashViewMode = "list")}><Icon name="list" /></button>
            </div>
            <span>{trashSelectedIds.length} selected</span>
            <button type="button" disabled={trashSelectedIds.length === 0} on:click={restoreSelectedTrash}>Restore selected</button>
            <button type="button" disabled={trashSelectedIds.length === 0} on:click={deleteSelectedTrash}>Delete selected</button>
          </div>
          <div class:trash-grid={trashViewMode === "grid"} class:trash-list={trashViewMode === "list"}>
            {#each trashItems as item (item.id)}
              <div class="trash-item" class:selected={trashSelectedIds.includes(item.id)} role="button" tabindex="0" on:click={(event) => selectTrashItem(item, event)} on:keydown={(event) => event.key === "Enter" && (trashSelectedIds = [item.id], trashAnchorId = item.id)}>
                <span class="list-thumb trash-thumb" class:folder={item.is_dir}>
                  {#if trashHasPreview(item)}
                    <img src={trashThumbnailURL(item.id, 160)} alt="" loading="lazy" />
                  {:else}
                    <Icon name={trashKindIcon(item)} />
                  {/if}
                </span>
                <span><strong>{item.original_name}</strong><small>{item.original_path}</small></span>
                <span>{trashKindLabel(item)}</span>
                <span>{item.is_dir ? "Folder" : formatBytes(item.size)}</span>
                <span>{formatDate(item.deleted_at)}</span>
                <span class="trash-actions">
                  <button type="button" on:click|stopPropagation={() => restoreTrashItem(item)}>Restore</button>
                  <button type="button" on:click|stopPropagation={() => deleteTrashItem(item)}>Delete</button>
                </span>
              </div>
            {/each}
          </div>
        </section>
      </div>
    {/if}

    {#if adminOpen}
      <div class="modal-backdrop" role="presentation" tabindex="-1" on:click={(event) => event.target === event.currentTarget && closeAdmin()} on:keydown={(event) => event.key === "Escape" && closeAdmin()}>
        <section class="modal-panel admin-panel">
          <header><h2>Admin</h2><button type="button" on:click={closeAdmin}>×</button></header>
          {#if stats}
            {#if demoMode}
              <p class="api-keys-hint">Demo admin mode is read-only. Stats, users, jobs, and API keys are visible, but admin actions are disabled.</p>
            {/if}
            <div class="stats-grid">
              <article><span>Users</span><strong>{stats.users.total}</strong><small>{stats.users.disabled} disabled</small></article>
              <article><span>Indexed</span><strong>{stats.index.indexed_files}</strong><small>{stats.index.indexed_directories} folders</small></article>
              <article><span>Previews</span><strong>{stats.index.preview_candidates}</strong><small>{formatBytes(stats.preview_cache.bytes)} cached</small></article>
              <article><span>Watcher</span><strong>{stats.watcher.enabled ? "On" : "Off"}</strong><small>{stats.watcher.watched_paths} paths</small></article>
            </div>
          {/if}
          <div class="commandbar">
            <button type="button" disabled={demoMode || adminJob?.status === "running"} on:click={() => runAdminJob("reindex")}>Full reindex</button>
            <button type="button" disabled={demoMode || adminJob?.status === "running"} on:click={() => runAdminJob("preview")}>Warm previews</button>
            <button type="button" disabled={demoMode || !adminJob?.cancelable || adminJob?.status !== "running"} on:click={cancelRunningAdminJob}>Cancel job</button>
            <button type="button" disabled={demoMode} on:click={clearPreviewCacheFromAdmin}>Clear preview cache</button>
          </div>
          {#if adminJob}
            <section class="job-panel">
              <strong>{adminJob.type}</strong>
              <span>{adminJob.status} · {jobProgress(adminJob)} · {adminJob.failed} failed</span>
              {#if adminJob.status === "running" && adminJob.total_known}<progress value={progressValue(adminJob)} max="100"></progress>{:else if adminJob.status === "running"}<progress></progress>{/if}
              <p>{adminJob.message}</p>
            </section>
          {/if}
          <section>
            <h3>Users</h3>
            {#if adminBusy}<p>{adminBusy}</p>{/if}
            <div class="user-list">
              {#each adminUsers as item (item.id)}
                <article>
                  <label class="admin-field"><span>Username</span><input bind:value={item.username} disabled={demoMode} /></label>
                  <label class="admin-field"><span>Home root</span><input bind:value={item.home_root} disabled={demoMode} /></label>
                  <label><input type="checkbox" bind:checked={item.is_admin} disabled={demoMode} /> Admin</label>
                  <label><input type="checkbox" bind:checked={item.disabled} disabled={demoMode} /> Disabled</label>
                  <label class="admin-field"><span>New password</span><input type="password" placeholder="Leave unchanged" value={passwordReset[item.id] || ""} disabled={demoMode} on:input={(event) => (passwordReset = { ...passwordReset, [item.id]: event.currentTarget.value })} /></label>
                  <button type="button" disabled={demoMode} on:click={() => saveAdminUser(item)}>Save</button>
                </article>
              {/each}
              <article class="new-user">
                <label class="admin-field"><span>Username</span><input placeholder="Username" bind:value={newUser.username} disabled={demoMode} /></label>
                <label class="admin-field"><span>Home root</span><input placeholder="Home root" bind:value={newUser.home_root} disabled={demoMode} /></label>
                <label class="admin-field"><span>Password</span><input type="password" placeholder="Password" bind:value={newUser.password} disabled={demoMode} /></label>
                <label><input type="checkbox" bind:checked={newUser.is_admin} disabled={demoMode} /> Admin</label>
                <label><input type="checkbox" bind:checked={newUser.disabled} disabled={demoMode} /> Disabled</label>
                <button type="button" disabled={demoMode} on:click={createUserFromAdmin}>Create</button>
              </article>
            </div>
          </section>
          <section class="api-keys-section">
            <h3>API Keys</h3>
            <p class="api-keys-hint">API keys allow external apps to authenticate via <code>Authorization: Bearer &lt;token&gt;</code>. Tokens are shown only once at creation.</p>
            {#if apiKeyBusy}<p>{apiKeyBusy}</p>{/if}
            {#if newKeyToken}
              <div class="api-key-token-reveal">
                <strong>Copy this token — it will not be shown again:</strong>
                <code class="api-key-token">{newKeyToken}</code>
                <button type="button" on:click={() => { navigator.clipboard.writeText(newKeyToken); }}>Copy</button>
                <button type="button" on:click={() => (newKeyToken = "")}>Dismiss</button>
              </div>
            {/if}
            <div class="api-key-list">
              {#each apiKeys as key (key.id)}
                <article class="api-key-row" class:revoked={!!key.revoked_at}>
                  <div class="api-key-info">
                    <strong>{key.name}</strong>
                    <span class="api-key-meta">{key.username} · created {formatDate(key.created_at)}{key.last_used_at ? ` · last used ${formatDate(key.last_used_at)}` : " · never used"}</span>
                    {#if key.revoked_at}<span class="api-key-revoked">Revoked {formatDate(key.revoked_at)}</span>{/if}
                  </div>
                  {#if !key.revoked_at}
                    <button type="button" class="danger" disabled={demoMode} on:click={() => revokeKey(key.id)}>Revoke</button>
                  {/if}
                </article>
              {:else}
                <p class="api-keys-empty">No API keys yet.</p>
              {/each}
            </div>
            <article class="new-api-key">
              <select bind:value={newKeyUserID} disabled={demoMode}>
                <option value={0} disabled>Select user…</option>
                {#each adminUsers as u (u.id)}
                  <option value={u.id}>{u.username}</option>
                {/each}
              </select>
              <input placeholder="Key name (e.g. My Organizer App)" bind:value={newKeyName} disabled={demoMode} />
              <button type="button" on:click={createNewAPIKey} disabled={demoMode || !newKeyName.trim() || !newKeyUserID}>Create</button>
            </article>
          </section>
        </section>
      </div>
    {/if}

    {#if uploadPreparing || uploadQueue.length > 0}
      <aside class="upload-queue">
        <header>
          <strong>Uploads</strong>
          {#if uploadQueue.length > 0}
            <span class="upload-summary">{uploadSummaryText(uploadQueue)}</span>
          {/if}
          <button type="button" on:click={() => (uploadQueueCollapsed = !uploadQueueCollapsed)}>{uploadQueueCollapsed ? "Show" : "Hide"}</button>
        </header>
        {#if !uploadQueueCollapsed}
          {#if uploadPreparing}
            <p class="upload-preparing">Preparing files…</p>
          {/if}
          <div class="upload-list">
            {#each uploadQueue as item (item.id)}
              <article class:error={item.status === "error"}>
                <div><strong>{item.name}</strong><span>{uploadStatusText(item)}</span></div>
                {#if item.status !== "interrupted"}<progress value={item.progress} max="100"></progress>{/if}
                {#if item.error}<p>{item.error} <button type="button" on:click={() => retryUpload(item)}>Retry</button></p>{/if}
              </article>
            {/each}
          </div>
          {#if uploadQueue.length > 0}
            <button type="button" on:click={clearCompletedUploads}>Clear completed</button>
          {/if}
        {/if}
      </aside>
    {/if}

    {#if shortcutsOpen}
      <div class="modal-backdrop" role="presentation" tabindex="-1"
        on:click={(event) => event.target === event.currentTarget && (shortcutsOpen = false)}
        on:keydown={(event) => event.key === 'Escape' && (shortcutsOpen = false)}>
        <section class="modal-panel shortcuts-panel">
          <header><h2>Keyboard Shortcuts</h2><button type="button" on:click={() => (shortcutsOpen = false)}>×</button></header>
          <div class="shortcuts-list">
            {#each [
              ['?', 'Show this help'],
              ['i', 'File info (1 selected)'],
              ['Ctrl+A', 'Select all'],
              ['Delete', 'Move to trash'],
              ['F2', 'Rename selected'],
              ['Enter', 'Open selected'],
              ['Escape', 'Clear selection / close'],
              ['Ctrl+Shift+N', 'New folder'],
              ['Arrow keys', 'Navigate files'],
              ['Shift+Arrow', 'Extend selection'],
              ['Ctrl+Arrow', 'Add to selection'],
            ] as [key, desc]}
              <div class="shortcut-row">
                <kbd>{key}</kbd>
                <span>{desc}</span>
              </div>
            {/each}
          </div>
        </section>
      </div>
    {/if}

    {#if infoEntry}
      <div class="modal-backdrop" role="presentation" tabindex="-1"
        on:click={(e) => e.target === e.currentTarget && (infoEntry = null)}
        on:keydown={(e) => e.key === 'Escape' && (infoEntry = null)}>
        <section class="modal-panel info-panel">
          <header>
            <h2>File info</h2>
            <button type="button" on:click={() => (infoEntry = null)}>×</button>
          </header>
          <div class="info-rows">
            <div class="info-row"><span>Name</span><strong>{infoEntry.name}</strong></div>
            <div class="info-row">
              <span>Path</span>
              <strong class="info-path">{infoEntry.path}
                <button type="button" class="info-copy" on:click={() => copyPath(infoEntry!.path)} title="Copy path">⎘</button>
              </strong>
            </div>
            <div class="info-row"><span>Type</span><strong>{infoEntry.type === 'dir' ? 'Folder' : fileTypeLabel(infoEntry)}</strong></div>
            {#if infoEntry.type === 'file'}
              <div class="info-row"><span>Size</span><strong>{formatBytes(infoEntry.size)}</strong></div>
            {/if}
            <div class="info-row"><span>Modified</span><strong>{formatDate(infoEntry.modified_at)}</strong></div>
            {#if infoEntry.mime_type}
              <div class="info-row"><span>MIME</span><strong class="info-mono">{infoEntry.mime_type}</strong></div>
            {/if}
            {#if infoEntry.preview_kind}
              <div class="info-row"><span>Preview</span><strong>{infoEntry.preview_kind}</strong></div>
            {/if}
            {#if infoExifLoading}
              <div class="info-exif-loading">Loading EXIF…</div>
            {:else if infoExif}
              <div class="info-exif-section">
                <h3>EXIF Metadata</h3>
                {#if infoExif.has_gps}
                  <div class="info-row info-gps">
                    <span>GPS</span>
                    <strong class="info-mono">{infoExif.gps_lat?.toFixed(6)}, {infoExif.gps_lon?.toFixed(6)}</strong>
                  </div>
                {/if}
                {#each Object.entries(infoExif.fields) as [key, value]}
                  {#if key !== 'GPSLatitude' && key !== 'GPSLongitude' && key !== 'GPSPosition'}
                    <div class="info-row">
                      <span class="info-exif-key">{key.replace(/([A-Z])/g, ' $1').trim()}</span>
                      <strong class="info-mono info-exif-val">{typeof value === 'object' ? JSON.stringify(value) : String(value)}</strong>
                    </div>
                  {/if}
                {/each}
              </div>
            {/if}
          </div>
        </section>
      </div>
    {/if}

    {#if contextMenu}
      <div class="context-menu" style="left:{contextMenu.x}px;top:{contextMenu.y}px" role="menu">
        <button type="button" on:click|stopPropagation={() => { openEntry(contextMenu!.entry); closeContextMenu(); }}>
          Open
        </button>
        <button type="button" on:click|stopPropagation={() => { selectedIds=[contextMenu!.entry.path]; void downloadSelected(); closeContextMenu(); }}>
          Download
        </button>
        <button type="button" on:click|stopPropagation={() => { copyPath(contextMenu!.entry.path); closeContextMenu(); }}>
          Copy path
        </button>
        <div class="context-divider"></div>
        <button type="button" on:click|stopPropagation={() => { selectedIds=[contextMenu!.entry.path]; void renameSelected(contextMenu!.entry); closeContextMenu(); }}>
          Rename
        </button>
        <button type="button" on:click|stopPropagation={() => { selectedIds=[contextMenu!.entry.path]; void moveSelected(); closeContextMenu(); }}>
          Move to…
        </button>
        <div class="context-divider"></div>
        <button type="button" class="context-danger" on:click|stopPropagation={() => { selectedIds=[contextMenu!.entry.path]; void deleteSelected(); closeContextMenu(); }}>
          Move to trash
        </button>
      </div>
    {/if}

    <div class="toast-container" aria-live="polite">
      {#each toasts as toast (toast.id)}
        <div class="toast toast-{toast.type}" role="alert">
          {toast.message}
          <button type="button" on:click={() => { toasts = toasts.filter(t => t.id !== toast.id); }}>×</button>
        </div>
      {/each}
    </div>
  </main>
{/if}
