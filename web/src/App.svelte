<script lang="ts">
  import { onMount } from "svelte";
  import Icon from "./lib/Icon.svelte";
  import {
    adminStats,
    authHeaders,
    bulkDelete,
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
  let username = "admin";
  let password = "";
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
  let viewMode: "grid" | "list" = "grid";
  let sortOption: SortOption = "name_asc";
  let fileTypeFilter: FileTypeFilter = "all";

  let searchQuery = "";
  let searchResults: FileEntry[] = [];
  let searchOpen = false;
  let uploadInput: HTMLInputElement | null = null;
  let uploadQueue: UploadQueueItem[] = loadQueueFromStorage();
  let uploadQueueCollapsed = uploadQueue.length === 0;
  let dragOver = false;
  let uploadsActive = false;

  let viewerFile: FileEntry | null = null;
  let viewerText: TextPreview | null = null;
  let viewerTextLoading = false;
  let viewerZoom = 1;
  let viewerOriginal = false;

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

  let fileEvents: AbortController | null = null;
  let liveRefreshTimer: ReturnType<typeof setTimeout> | null = null;

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
    const onPopState = () => {
      void loadPath(pathFromURL(), { push: false });
    };
    const onBeforeUnload = (event: BeforeUnloadEvent) => {
      if (!uploadsActive) return;
      event.preventDefault();
      event.returnValue = "";
    };
    const onKeyDown = (event: KeyboardEvent) => handleGlobalKeydown(event);
    window.addEventListener("popstate", onPopState);
    window.addEventListener("beforeunload", onBeforeUnload);
    window.addEventListener("keydown", onKeyDown);
    void restoreSession();
    return () => {
      window.removeEventListener("popstate", onPopState);
      window.removeEventListener("beforeunload", onBeforeUnload);
      window.removeEventListener("keydown", onKeyDown);
      stopFileEvents();
      stopAdminPolling();
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
      });
      return;
    }
    if (dialog.kind === "rename") {
      const entry = dialog.entry;
      if (!entry || value === entry.name) return;
      await runAction("Renaming", async () => {
        await move(entry.path, joinPath(parentPath(entry.path), value));
        await refreshCurrentFolder();
      });
      return;
    }
    if (dialog.kind === "move") {
      await runAction("Moving items", async () => {
        const response = await bulkMove(selectedIds, normalizePath(value));
        assertBulkSuccess(response.results);
        await refreshCurrentFolder();
      });
      return;
    }
    await runAction("Moving to trash", async () => {
      const response = await bulkDelete(selectedIds);
      assertBulkSuccess(response.results);
      await refreshCurrentFolder();
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
    const files = Array.from(input.files || []);
    input.value = "";
    await uploadFiles(files);
  }

  async function uploadFiles(files: File[]) {
    if (files.length === 0) return;
    const queued = files.map(file => createUploadQueueItem(file, currentPath));
    uploadQueue = [...queued, ...uploadQueue].slice(0, 100);
    uploadQueueCollapsed = false;
    await uploadQueuedFiles(queued);
    await refreshCurrentFolder();
  }

  async function uploadQueuedFiles(items: UploadQueueItem[]) {
    let next = 0;
    const workers = Array.from({ length: Math.min(uploadConcurrency, items.length) }, async () => {
      while (next < items.length) {
        const item = items[next++];
        await uploadOne(item);
      }
    });
    await Promise.all(workers);
  }

  async function uploadOne(item: UploadQueueItem) {
    if (!item.file) {
      setUploadItem(item.id, { status: "interrupted" });
      return;
    }
    setUploadItem(item.id, { status: "uploading", progress: 0, error: "" });
    try {
      const finalPath = await uploadTus(
        item.file,
        item.targetPath,
        progress => setUploadItem(item.id, { progress: progress.percent }),
        { onUploadCreated: url => setUploadItem(item.id, { tusUrl: url }) }
      );
      setUploadItem(item.id, { status: "done", progress: 100, finalPath: finalPath || joinPath(item.targetPath, item.name) });
    } catch (err) {
      setUploadItem(item.id, { status: "error", error: messageFromError(err) });
    }
  }

  async function retryUpload(item: UploadQueueItem) {
    if (!item.file) return;
    if (item.tusUrl) {
      setUploadItem(item.id, { status: "uploading", progress: 0, error: "" });
      try {
        const finalPath = await resumeUploadTus(item.tusUrl, item.file, progress => setUploadItem(item.id, { progress: progress.percent }));
        setUploadItem(item.id, { status: "done", progress: 100, finalPath: finalPath || joinPath(item.targetPath, item.name) });
      } catch (err) {
        setUploadItem(item.id, { status: "error", error: messageFromError(err) });
      }
    } else {
      await uploadOne(item);
    }
    await refreshCurrentFolder();
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
    viewerZoom = 1;
    viewerOriginal = false;
    if (entry.preview_kind === "text" || entry.preview_kind === "markdown") {
      viewerTextLoading = true;
      try {
        viewerText = await fetchTextPreview(entry.path);
      } catch (err) {
        error = messageFromError(err);
      } finally {
        viewerTextLoading = false;
      }
    }
  }

  function closeViewer() {
    viewerFile = null;
    viewerText = null;
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
    await Promise.all([refreshAdmin(), refreshAdminUsers()]);
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
    try {
      adminJob = (await cancelAdminJob()).job;
      startAdminPolling();
    } catch (err) {
      error = messageFromError(err);
    }
  }

  async function clearPreviewCacheFromAdmin() {
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
    } else if (event.key === "Escape") {
      if (actionDialog) actionDialog = null;
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
    const columns = viewMode === "grid" ? gridColumnEstimate() : 1;
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
      status: "queued"
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
    });
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
    <aside class="sidebar">
      <div class="brand">
        <strong>goDrive</strong>
        <span>{user.username}</span>
      </div>
      <button class:active={currentPath === "/"} type="button" on:click={() => loadPath("/", { push: true })}><Icon name="folder" />My files</button>
      <button type="button" on:click={openTrash}><Icon name="trash" />Trash</button>
      <div class="tree">
        {#if visibleTree.length === 0}
          <div class="tree-empty">No folders found</div>
        {:else}
          {#each visibleTree as row (row.path)}
          <div class="tree-row" class:active={currentPath === row.path} style={`--level: ${row.level}`}>
            <button type="button" class="tree-toggle" on:click={() => toggleTree(row.path)} disabled={!treeChildPaths.has(row.path)}>
              {#if treeChildPaths.has(row.path)}<Icon name={openTree.has(row.path) ? "chevronDown" : "chevronRight"} />{/if}
            </button>
            <button type="button" class="tree-link" on:click={() => loadPath(row.path, { push: true })}><Icon name="folder" />{row.name}</button>
          </div>
          {/each}
        {/if}
      </div>
    </aside>

    <section class="workspace">
      <header class="topbar">
        <form class="search" on:submit|preventDefault={submitSearch}>
          <input bind:value={searchQuery} placeholder="Search files" />
          <button type="submit"><Icon name="search" />Search</button>
        </form>
        <div class="topbar-actions">
          {#if busy}<span class="busy">{busy}</span>{/if}
          <button type="button" on:click={refreshCurrentFolder}><Icon name="refresh" />Refresh</button>
          {#if user.is_admin}<button type="button" on:click={openAdmin}><Icon name="admin" />Admin</button>{/if}
          <button type="button" on:click={openTrash}><Icon name="trash" />Trash</button>
          <button type="button" on:click={submitLogout}><Icon name="logout" />Logout</button>
        </div>
      </header>

      {#if error}<div class="error" role="alert"><span>{error}</span><button type="button" on:click={() => (error = "")}>×</button></div>{/if}

      <div class="pathbar">
        <div class="crumbs">
          {#each breadcrumbs() as crumb, index}
            {#if index > 0}<span>/</span>{/if}
            <button type="button" on:click={() => loadPath(crumb.path, { push: true })}>{crumb.name}</button>
          {/each}
        </div>
        <div class="view-toggle">
          <button class:active={viewMode === "grid"} type="button" title="Grid view" aria-label="Grid view" on:click={() => (viewMode = "grid")}><Icon name="grid" /></button>
          <button class:active={viewMode === "list"} type="button" title="List view" aria-label="List view" on:click={() => (viewMode = "list")}><Icon name="list" /></button>
        </div>
      </div>

      <div class="commandbar">
        <button type="button" on:click={createFolder}><Icon name="plus" />New folder</button>
        <button type="button" on:click={() => uploadInput?.click()}><Icon name="upload" />Upload</button>
        <input bind:this={uploadInput} class="hidden-input" type="file" multiple on:change={uploadSelectedFiles} />
        <button type="button" disabled={selectedIds.length !== 1} on:click={() => renameSelected()}><Icon name="rename" />Rename</button>
        <button type="button" disabled={selectedIds.length === 0} on:click={moveSelected}><Icon name="move" />Move</button>
        <button type="button" disabled={selectedIds.length === 0} on:click={downloadSelected}><Icon name="download" />Download</button>
        <button type="button" disabled={selectedIds.length === 0} on:click={deleteSelected}><Icon name="trash" />Delete</button>
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

      <section class="file-area" class:list={viewMode === "list"} aria-label="File list">
        {#if currentPath !== "/"}
          <button type="button" class="parent-link" on:click={() => loadPath(parentPath(currentPath), { push: true })}>
            ← Back to parent folder
          </button>
        {/if}

        {#if viewMode === "grid"}
          <div class="grid">
            {#each visibleEntries as entry (entry.path)}
              <button
                type="button"
                class="file-card"
                class:selected={selectedIds.includes(entry.path)}
                on:click={(event) => selectEntry(entry, event)}
                on:dblclick={() => openEntry(entry)}
              >
                <div class="thumb" class:folder={entry.type === "dir"}>
                  {#if entry.type === "dir"}
                    <span class="folder-icon"></span>
                  {:else if thumbnailKinds.has(entry.preview_kind || "")}
                    <img src={previewURL(entry)} alt="" loading="lazy" />
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
                <th><button type="button" on:click={() => setTableSort("name")}>Name</button></th>
                <th><button type="button" on:click={() => (sortOption = "type_asc")}>Type</button></th>
                <th><button type="button" on:click={() => setTableSort("size")}>Size</button></th>
                <th><button type="button" on:click={() => setTableSort("modified")}>Modified</button></th>
              </tr>
            </thead>
            <tbody>
              {#each visibleEntries as entry (entry.path)}
                <tr class:selected={selectedIds.includes(entry.path)} on:click={(event) => selectEntry(entry, event)} on:dblclick={() => openEntry(entry)}>
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

        {#if folderHasMore}
          <button type="button" class="load-more" on:click={loadMoreFolder}>Load more ({folderOffset}/{folderTotal})</button>
        {/if}
      </section>
    </section>

    {#if dragOver}<div class="drop-overlay">Drop to upload to {currentPath}</div>{/if}

    {#if searchOpen}
      <div class="modal-backdrop" role="presentation" tabindex="-1" on:click={(event) => event.target === event.currentTarget && (searchOpen = false)} on:keydown={(event) => event.key === "Escape" && (searchOpen = false)}>
        <section class="modal-panel">
          <header><h2>Search results</h2><button type="button" on:click={() => (searchOpen = false)}>×</button></header>
          <div class="result-list">
            {#each searchResults as result (result.path)}
              <button type="button" on:click={() => { searchOpen = false; result.type === "dir" ? loadPath(result.path, { push: true }) : loadPath(parentPath(result.path), { push: true }); }}>
                <strong>{result.name}</strong><span>{result.path}</span>
              </button>
            {/each}
          </div>
        </section>
      </div>
    {/if}

    {#if viewerFile}
      <div class="viewer" role="dialog" aria-modal="true" aria-label={viewerFile.name} tabindex="-1" on:keydown={(event) => event.key === "Escape" && closeViewer()}>
        <header>
          <div class="viewer-title"><strong>{viewerFile.name}</strong><span>{fileTypeLabel(viewerFile)} · {formatBytes(viewerFile.size)} · {formatDate(viewerFile.modified_at)}</span></div>
          <div class="viewer-actions">
            {#if viewerFile.preview_kind === "image"}
              <div class="viewer-stepper">
              <button type="button" title="Previous image" on:click={() => showAdjacentImage(-1)}><Icon name="chevronLeft" /></button>
              <button type="button" title="Next image" on:click={() => showAdjacentImage(1)}><Icon name="chevronRight" /></button>
              </div>
              <div class="viewer-stepper">
              <button type="button" on:click={() => (viewerZoom = Math.max(0.25, viewerZoom - 0.25))}>−</button>
              <button type="button" on:click={() => (viewerZoom = 1)}>{Math.round(viewerZoom * 100)}%</button>
              <button type="button" on:click={() => (viewerZoom = Math.min(5, viewerZoom + 0.25))}>+</button>
              </div>
              <button type="button" class:active={viewerOriginal} on:click={() => (viewerOriginal = !viewerOriginal)}>{viewerOriginal ? "Preview" : "Original"}</button>
            {/if}
            <button type="button" on:click={downloadSelected}><Icon name="download" />Download</button>
            <button type="button" on:click={closeViewer}>Close</button>
          </div>
        </header>
        {#if viewerFile.preview_kind === "image" || viewerFile.preview_kind === "raw" || viewerFile.preview_kind === "office"}
          <div class="viewer-stage">
            <img style={`transform: scale(${viewerZoom})`} src={viewerFile.preview_kind === "image" && viewerOriginal ? rawFileURL(viewerFile.path) : thumbnailURL(viewerFile.path, 2048)} alt={viewerFile.name} />
          </div>
        {:else if viewerFile.preview_kind === "video"}
          <video controls src={rawFileURL(viewerFile.path)}><track kind="captions" /></video>
        {:else if viewerFile.preview_kind === "pdf"}
          <iframe src={rawFileURL(viewerFile.path)} title={viewerFile.name}></iframe>
        {:else if viewerFile.preview_kind === "text" || viewerFile.preview_kind === "markdown"}
          <div class="text-preview">
            {#if viewerTextLoading}<p>Loading...</p>{:else if viewerText}<pre>{viewerText.content}</pre>{:else}<p>Preview unavailable.</p>{/if}
          </div>
        {:else if viewerFile.preview_kind === "3d"}
          {#await loadThreeDViewer()}
            <p>Loading 3D viewer...</p>
          {:then module}
            <svelte:component this={module.default} src={rawFileURL(viewerFile.path)} name={viewerFile.name} />
          {:catch}
            <p>3D viewer unavailable.</p>
          {/await}
        {/if}
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
            <div class="stats-grid">
              <article><span>Users</span><strong>{stats.users.total}</strong><small>{stats.users.disabled} disabled</small></article>
              <article><span>Indexed</span><strong>{stats.index.indexed_files}</strong><small>{stats.index.indexed_directories} folders</small></article>
              <article><span>Previews</span><strong>{stats.index.preview_candidates}</strong><small>{formatBytes(stats.preview_cache.bytes)} cached</small></article>
              <article><span>Watcher</span><strong>{stats.watcher.enabled ? "On" : "Off"}</strong><small>{stats.watcher.watched_paths} paths</small></article>
            </div>
          {/if}
          <div class="commandbar">
            <button type="button" disabled={adminJob?.status === "running"} on:click={() => runAdminJob("reindex")}>Full reindex</button>
            <button type="button" disabled={adminJob?.status === "running"} on:click={() => runAdminJob("preview")}>Warm previews</button>
            <button type="button" disabled={!adminJob?.cancelable || adminJob?.status !== "running"} on:click={cancelRunningAdminJob}>Cancel job</button>
            <button type="button" on:click={clearPreviewCacheFromAdmin}>Clear preview cache</button>
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
                  <input bind:value={item.username} />
                  <input bind:value={item.home_root} />
                  <label><input type="checkbox" bind:checked={item.is_admin} /> Admin</label>
                  <label><input type="checkbox" bind:checked={item.disabled} /> Disabled</label>
                  <input type="password" placeholder="New password" value={passwordReset[item.id] || ""} on:input={(event) => (passwordReset = { ...passwordReset, [item.id]: event.currentTarget.value })} />
                  <button type="button" on:click={() => saveAdminUser(item)}>Save</button>
                </article>
              {/each}
              <article class="new-user">
                <input placeholder="Username" bind:value={newUser.username} />
                <input placeholder="Home root" bind:value={newUser.home_root} />
                <input type="password" placeholder="Password" bind:value={newUser.password} />
                <label><input type="checkbox" bind:checked={newUser.is_admin} /> Admin</label>
                <label><input type="checkbox" bind:checked={newUser.disabled} /> Disabled</label>
                <button type="button" on:click={createUserFromAdmin}>Create</button>
              </article>
            </div>
          </section>
        </section>
      </div>
    {/if}

    {#if uploadQueue.length > 0}
      <aside class="upload-queue">
        <header><strong>Uploads</strong><button type="button" on:click={() => (uploadQueueCollapsed = !uploadQueueCollapsed)}>{uploadQueueCollapsed ? "Show" : "Hide"}</button></header>
        {#if !uploadQueueCollapsed}
          {#each uploadQueue as item (item.id)}
            <article class:error={item.status === "error"}>
              <div><strong>{item.name}</strong><span>{uploadStatusText(item)}</span></div>
              {#if item.status !== "interrupted"}<progress value={item.progress} max="100"></progress>{/if}
              {#if item.error}<p>{item.error} <button type="button" on:click={() => retryUpload(item)}>Retry</button></p>{/if}
            </article>
          {/each}
          <button type="button" on:click={clearCompletedUploads}>Clear completed</button>
        {/if}
      </aside>
    {/if}
  </main>
{/if}
