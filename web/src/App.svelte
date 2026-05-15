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
    bulkDownloadBlob,
    bulkMove,
    cancelAdminJob,
    clearPreviewCache,
    createAdminUser,
    currentAdminJob,
    currentToken,
    deleteTrash,
    downloadBlob,
    joinPath,
    listTrash,
    listAdminUsers,
    fetchTextPreview,
    listFiles,
    listFileTree,
    resumeUploadTus,
    type TextPreview,
    login,
    logout,
    me,
    mkdir,
    move,
    parentPath,
    rawFileURL,
    restoreTrash,
    saveBlob,
    searchFiles,
    setAdminUserPassword,
    startPreviewWarmup,
    startReindex,
    thumbnailURL,
    updateAdminUser,
    uploadTus,
    type AdminJob,
    type AdminStats,
    type FileEntry,
    type TrashItem,
    type User
  } from "./lib/api";
  import { toSvarFiles, type SvarFile } from "./lib/svar";

  const previewKinds = new Set(["image", "raw", "video", "pdf", "office", "text", "markdown", "3d"]);
  const thumbnailKinds = new Set(["image", "raw", "video", "pdf", "office"]);
  const thumbnailViewerKinds = new Set(["image", "raw", "office"]);
  const unsupportedMenuItems = new Set(["add-file", "copy", "paste"]);
  const viewModeKey = "godrive_view_mode";
  const uploadConcurrency = 3;
  let threeDViewerPromise: Promise<typeof import("./lib/ThreeDViewer.svelte")> | null = null;

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

  type PersistedQueueItem = {
    id: string;
    name: string;
    size: number;
    targetPath: string;
    status: "done" | "interrupted";
    finalPath?: string;
  };

  const QUEUE_STORAGE_KEY = "godrive_upload_queue";

  function saveQueueToStorage(queue: UploadQueueItem[]) {
    const items: PersistedQueueItem[] = queue
      .filter(item => item.status === "done" || item.status === "error" || item.status === "interrupted")
      .map(item => ({
        id: item.id,
        name: item.name,
        size: item.size,
        targetPath: item.targetPath,
        status: item.status === "done" ? "done" : "interrupted",
        finalPath: item.finalPath
      }));
    try {
      localStorage.setItem(QUEUE_STORAGE_KEY, JSON.stringify(items));
    } catch {
      // localStorage unavailable
    }
  }

  function loadQueueFromStorage(): UploadQueueItem[] {
    try {
      const raw = localStorage.getItem(QUEUE_STORAGE_KEY);
      if (!raw) return [];
      const items: PersistedQueueItem[] = JSON.parse(raw);
      return items.map(item => ({
        id: item.id,
        file: null,
        name: item.name,
        size: item.size,
        targetPath: item.targetPath,
        progress: item.status === "done" ? 100 : 0,
        status: item.status,
        finalPath: item.finalPath
      }));
    } catch {
      return [];
    }
  }

  function loadThreeDViewer() {
    threeDViewerPromise ??= import("./lib/ThreeDViewer.svelte");
    return threeDViewerPromise;
  }

  let user: User | null = null;
  let username = "admin";
  let password = "";
  let loading = true;
  let error = "";
  let busy = "";
  let initialData: SvarFile[] = [];
  let fileApi: IApi | null = null;
  let managerKey = 0;
  let currentPath = "/";
  let trashOpen = false;
  let trashItems: TrashItem[] = [];
  let trashBusy = "";
  let adminOpen = false;
  let stats: AdminStats | null = null;
  let adminJob: AdminJob | null = null;
  let adminPoll: ReturnType<typeof window.setInterval> | null = null;
  let adminUsers: User[] = [];
  let adminBusy = "";
  let newUser = emptyNewUser();
  let passwordReset: Record<number, string> = {};
  let viewerFile: IParsedEntity | null = null;
  let uploadInput: HTMLInputElement | null = null;
  let uploadQueue: UploadQueueItem[] = loadQueueFromStorage();
  let uploadQueueCollapsed = uploadQueue.length === 0;
  let dragOver = false;
  let viewMode: TMode = loadViewMode();
  let folderHasMore = false;
  let folderTotal = 0;
  let folderOffset = 0;
  let folderCursor = "";
  let folderAccumulated: SvarFile[] = [];
  let treeData: SvarFile[] = [];
  let selectedIds: string[] = [];
  let searchQuery = "";
  let searchOpen = false;
  let searchBusy = false;
  let searchResults: FileEntry[] = [];
  let viewerZoom = 1;
  let viewerOriginal = false;
  let viewerStage: HTMLDivElement | null = null;
  let viewerDragging = false;
  let viewerDragStartX = 0;
  let viewerDragStartY = 0;
  let viewerDragScrollLeft = 0;
  let viewerDragScrollTop = 0;
  let viewerText: TextPreview | null = null;
  let viewerTextLoading = false;
  let fileEvents: EventSource | null = null;
  let liveRefreshTimer: ReturnType<typeof setTimeout> | null = null;
  $: viewerImages = currentImageFiles();
  $: viewerIndex = viewerFile ? viewerImages.findIndex(file => file.id === viewerFile?.id) : -1;
  $: uploadsActive = uploadQueue.some(item => item.status === "queued" || item.status === "uploading");
  $: saveQueueToStorage(uploadQueue);

  onMount(() => {
    const onPopState = () => {
      if (fileApi) {
        void navigateToURLPath(fileApi);
      }
    };
    const onBeforeUnload = (event: BeforeUnloadEvent) => {
      if (!uploadsActive) {
        return;
      }
      event.preventDefault();
      event.returnValue = "";
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "a") {
        selectAllVisible(event);
      }
    };
    const updateSelectionSoon = () => {
      setTimeout(() => updateSelection(), 0);
    };
    window.addEventListener("popstate", onPopState);
    window.addEventListener("beforeunload", onBeforeUnload);
    window.addEventListener("keydown", onKeyDown);
    window.addEventListener("pointerup", updateSelectionSoon);
    window.addEventListener("keyup", updateSelectionSoon);
    void (async () => {
      if (!currentToken()) {
        loading = false;
        return;
      }
      await restoreSession();
    })();
    return () => {
      window.removeEventListener("popstate", onPopState);
      window.removeEventListener("beforeunload", onBeforeUnload);
      window.removeEventListener("keydown", onKeyDown);
      window.removeEventListener("pointerup", updateSelectionSoon);
      window.removeEventListener("keyup", updateSelectionSoon);
      stopFileEvents();
      stopAdminPolling();
    };
  });

  async function restoreSession() {
    loading = true;
    error = "";
    try {
      user = await me();
      await loadInitialFiles();
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
      await loadInitialFiles();
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
    initialData = [];
    fileApi = null;
    managerKey += 1;
  }

  async function loadInitialFiles() {
    const [rootData, tree] = await Promise.all([loadFolder("/"), loadTree()]);
    treeData = tree;
    initialData = mergeSvarFiles(rootData, treeData);
    managerKey += 1;
  }

  async function loadTree() {
    const response = await listFileTree();
    return toSvarFiles(response.entries).map(file => ({ ...file, lazy: false }));
  }

  function mergeSvarFiles(primary: SvarFile[], secondary: SvarFile[]) {
    const seen = new Set(primary.map(file => file.id));
    const merged = [...primary];
    for (const file of secondary) {
      if (!seen.has(file.id)) {
        merged.push(file);
        seen.add(file.id);
      }
    }
    return merged;
  }

  function descendantsForTree(parent: string) {
    const cleanParent = normalizeURLPath(parent);
    return treeData.filter(file => file.id !== cleanParent && file.id.startsWith(`${cleanParent === "/" ? "" : cleanParent}/`));
  }

  async function loadFolder(path: string, updatePagination = true) {
    const response = await listFiles(path || "/");
    const data = toSvarFiles(response.entries);
    if (updatePagination) {
      folderHasMore = response.has_more;
      folderTotal = response.total;
      folderOffset = response.offset + response.entries.length;
      folderCursor = response.next_cursor || "";
      folderAccumulated = data;
    }
    return data;
  }

  async function loadMoreFolder() {
    if (!folderHasMore || !fileApi) return;
    const path = currentFolderPath();
    const response = await listFiles(path, folderOffset, 500, folderCursor);
    folderHasMore = response.has_more;
    folderCursor = response.next_cursor || "";
    folderOffset += response.entries.length;
    folderTotal = response.total;
    const newItems = toSvarFiles(response.entries);
    folderAccumulated = [...folderAccumulated, ...newItems];
    await fileApi.exec("provide-data", { id: path || "/", data: folderAccumulated });
  }

  function initFilemanager(api: IApi) {
    fileApi = api;
    updateSelection(api);

    api.on("set-path", event => {
      currentPath = normalizeURLPath(event.id || "/");
      syncURLPath(event.id);
      setTimeout(() => updateSelection(api), 0);
    });

    api.on("select-file", () => {
      setTimeout(() => updateSelection(api), 0);
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
      if (event.file.type === "folder") {
        await runAction("Saving changes", async () => {
          const parent = event.parent || currentFolderPath();
          await mkdir(joinPath(parent, event.file.name));
          await refreshFolder(api, parent);
        });
        return false;
      }
      await runAction("Saving changes", async () => {
        if (event.file.file) {
          await uploadFiles([event.file.file], event.parent, api);
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
        updateSelection(api);
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
        updateSelection(api);
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
        updateSelection(api);
      });
      return false;
    });

    api.intercept("copy-files", () => {
      error = "Copy is not wired to the backend yet.";
      return false;
    });

    api.intercept("filter-files", async event => {
      const query = String(event.text || "").trim();
      searchQuery = query;
      if (!query) {
        searchOpen = false;
        searchResults = [];
        return false;
      }
      await submitSearch();
      return false;
    });

    api.intercept("download-file", async event => {
      await runAction("Preparing download", async () => {
        const ids = event.ids?.length ? event.ids : [event.id];
        if (ids.length > 1) {
          const blob = await bulkDownloadBlob(ids);
          saveBlob(blob, "godrive-selection.zip");
          return;
        }
        const file = api.getFile(ids[0]);
        const blob = await downloadBlob(ids[0]);
        saveBlob(blob, file?.name || basename(ids[0]));
      });
      return false;
    });

    api.intercept("open-file", async event => {
      const file = api.getFile(event.id);
      if (file?.previewKind && previewKinds.has(file.previewKind)) {
        openViewer(file);
      } else {
        await api.exec("download-file", { id: event.id });
      }
      return false;
    });

    void navigateToURLPath(api);
  }

  async function refreshFolder(api: IApi, path: string) {
    const cleanPath = normalizeURLPath(path || "/");
    const isCurrent = normalizeURLPath(currentFolderPath()) === cleanPath;
    if (isCurrent) {
      folderOffset = 0;
      folderCursor = "";
      folderHasMore = false;
      folderAccumulated = [];
    }
    const [data, tree] = await Promise.all([loadFolder(cleanPath, isCurrent), loadTree()]);
    treeData = tree;
    if (cleanPath === "/") {
      initialData = mergeSvarFiles(data, treeData);
    }
    await api.exec("provide-data", { id: "/", data: mergeSvarFiles(cleanPath === "/" ? data : await loadFolder("/", false), treeData) });
    if (cleanPath !== "/") {
      await api.exec("provide-data", { id: cleanPath, data: mergeSvarFiles(data, descendantsForTree(cleanPath)) });
    }
  }

  function updateSelection(api = fileApi) {
    if (!api) {
      selectedIds = [];
      return;
    }
    const state = api.getState();
    const panel = state.panels?.[state.activePanel ?? 0];
    selectedIds = [...(panel?.selected || [])].filter(id => id && id !== "/wx-filemanager-parent-link");
  }

  async function batchDownloadSelected() {
    if (!fileApi || selectedIds.length === 0) return;
    await fileApi.exec("download-file", { id: selectedIds[0], ids: selectedIds });
  }

  async function batchDeleteSelected() {
    if (!fileApi || selectedIds.length === 0) return;
    await fileApi.exec("delete-files", { ids: selectedIds });
  }

  async function batchMoveSelected() {
    if (!fileApi || selectedIds.length === 0) return;
    const target = window.prompt("Move selected items to folder", currentFolderPath());
    if (!target) return;
    await fileApi.exec("move-files", { ids: selectedIds, target: normalizeURLPath(target) });
  }

  async function clearSelection() {
    await fileApi?.exec("select-file", {});
    selectedIds = [];
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
    await uploadQueuedFiles(queued, targetPath);
    await refreshFolder(api, targetPath);
  }

  async function uploadQueuedFiles(items: UploadQueueItem[], targetPath: string) {
    let next = 0;
    const workers = Array.from({ length: Math.min(uploadConcurrency, items.length) }, async () => {
      while (next < items.length) {
        const item = items[next++];
        await uploadOne(item.file, item, targetPath);
      }
    });
    await Promise.all(workers);
  }

  async function uploadOne(file: File | null, item: UploadQueueItem, targetPath: string) {
    if (!file) {
      setUploadItem(item.id, { status: "interrupted" });
      return;
    }
    setUploadItem(item.id, { status: "uploading", progress: 0, error: "" });
    busy = `Uploading ${file.name}`;
    try {
      const finalPath = await uploadTus(
        file,
        targetPath,
        progress => setUploadItem(item.id, { progress: progress.percent }),
        { onUploadCreated: url => setUploadItem(item.id, { tusUrl: url }) }
      );
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
    return panel?.path || currentPath || "/";
  }

  async function navigateToParentFolder() {
    if (!fileApi || currentPath === "/") return;
    const parent = parentPath(currentPath);
    const selected = currentPath;
    await loadPathAncestors(fileApi, parent);
    await fileApi.exec("set-path", { id: parent, selected: [selected] });
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
    uploadQueue = uploadQueue.filter(item => item.status !== "done" && item.status !== "interrupted");
    if (uploadQueue.length === 0) {
      try { localStorage.removeItem(QUEUE_STORAGE_KEY); } catch {}
    }
  }

  async function retryUpload(item: UploadQueueItem) {
    if (!item.file) return;
    uploadQueueCollapsed = false;
    if (item.tusUrl) {
      setUploadItem(item.id, { status: "uploading", progress: 0, error: "" });
      busy = `Uploading ${item.name}`;
      try {
        const finalPath = await resumeUploadTus(
          item.tusUrl,
          item.file,
          progress => setUploadItem(item.id, { progress: progress.percent })
        );
        setUploadItem(item.id, { status: "done", progress: 100, finalPath: finalPath || joinPath(item.targetPath, item.name) });
      } catch (err) {
        const msg = messageFromError(err);
        if (msg === "upload_gone") {
          setUploadItem(item.id, { tusUrl: undefined });
          await uploadOne(item.file, item, item.targetPath);
        } else {
          setUploadItem(item.id, { status: "error", error: msg });
        }
      } finally {
        busy = "";
      }
    } else {
      await uploadOne(item.file, item, item.targetPath);
    }
    await refreshCurrentFolder();
  }

  function uploadSummary() {
    let active = 0, failed = 0, interrupted = 0;
    for (const item of uploadQueue) {
      if (item.status === "uploading" || item.status === "queued") active++;
      else if (item.status === "error") failed++;
      else if (item.status === "interrupted") interrupted++;
    }
    const total = uploadQueue.length;
    if (active > 0) return `${active}/${total} active`;
    if (failed > 0) return `${failed}/${total} failed`;
    if (interrupted > 0) return `${interrupted} interrupted`;
    return `${total} completed`;
  }

  function uploadQueueNotice() {
    if (uploadsActive) {
      return "Keep this tab open until uploads finish. Browser file handles cannot be recovered after reload.";
    }
    if (uploadQueue.some(item => item.status === "error")) {
      return "Failed uploads can be retried while this tab remains open.";
    }
    if (uploadQueue.some(item => item.status === "interrupted")) {
      return "Interrupted uploads lost their file handles. Re-upload them using the Upload button.";
    }
    return "Upload history. Use Clear to dismiss.";
  }

  function uploadStatusText(item: UploadQueueItem) {
    switch (item.status) {
      case "uploading": return `${Math.round(item.progress)}%`;
      case "queued": return "Waiting";
      case "done": return "Done";
      case "interrupted": return "Interrupted";
      default: return "Failed";
    }
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

  function startFileEvents() {
    stopFileEvents();
    fileEvents = new EventSource("/api/events", { withCredentials: true });
    const refresh = (event: MessageEvent) => scheduleRefreshForEvent(event.data);
    for (const eventName of ["file.created", "file.moved", "file.deleted", "file.restored", "file.external_changed", "file.external_deleted", "upload.complete"]) {
      fileEvents.addEventListener(eventName, refresh);
    }
    fileEvents.onerror = () => {
      // EventSource reconnects automatically; keep local UI quiet unless requests fail.
    };
  }

  function stopFileEvents() {
    if (liveRefreshTimer) {
      clearTimeout(liveRefreshTimer);
      liveRefreshTimer = null;
    }
    fileEvents?.close();
    fileEvents = null;
  }

  function scheduleRefreshForEvent(raw: string) {
    if (!fileApi || !raw) {
      return;
    }
    let event: { event?: string; data?: Record<string, unknown> };
    try {
      event = JSON.parse(raw);
    } catch {
      return;
    }
    if (!liveEventAffectsCurrentFolder(event)) {
      return;
    }
    if (liveRefreshTimer) {
      clearTimeout(liveRefreshTimer);
    }
    liveRefreshTimer = setTimeout(() => {
      liveRefreshTimer = null;
      void refreshCurrentFolder();
    }, 250);
  }

  function liveEventAffectsCurrentFolder(event: { event?: string; data?: Record<string, unknown> }) {
    const current = normalizeURLPath(currentFolderPath());
    const paths = [event.data?.path, event.data?.old_path].filter((value): value is string => typeof value === "string" && value !== "");
    for (const rawPath of paths) {
      const changed = normalizeURLPath(rawPath);
      const parent = parentPath(changed);
      if (current === parent || current === changed || current.startsWith(`${changed}/`)) {
        return true;
      }
    }
    return false;
  }

  function loadViewMode(): TMode {
    const stored = localStorage.getItem(viewModeKey);
    return stored === "cards" || stored === "table" || stored === "panels" || stored === "search" ? stored : "cards";
  }

  function menuOptions(mode: TContextMenuType, item?: IParsedEntity) {
    const options = getMenuOptions(mode);
    if (mode === "multiselect" && !options.some(option => option.id === "download")) {
      options.unshift({ icon: "wxi-download", text: "Download", hotkey: "Ctrl+D", id: "download" });
    }
    return options
      .filter(option => option.id && !unsupportedMenuItems.has(String(option.id)))
      .map(option => {
        if (option.id === "add-folder") {
          return { ...option, text: "New folder" };
        }
        return option;
      }) as IFileMenuOption[];
  }

  function selectAllVisible(event: KeyboardEvent) {
    if (!fileApi || shouldIgnoreSelectAll(event.target)) {
      return;
    }
    const state = fileApi.getState();
    const panelIndex = state.activePanel ?? 0;
    const panel = state.panels?.[panelIndex];
    const ids = (panel?._files || [])
      .filter(file => file.id !== "/wx-filemanager-parent-link")
      .map(file => file.id);
    if (ids.length === 0) {
      return;
    }
    event.preventDefault();
    void (async () => {
      await fileApi?.exec("select-file", { panel: panelIndex });
      for (const id of ids) {
        await fileApi?.exec("select-file", { id, panel: panelIndex, toggle: true });
      }
    })();
  }

  function shouldIgnoreSelectAll(target: EventTarget | null) {
    if (!(target instanceof HTMLElement)) {
      return false;
    }
    const tag = target.tagName.toLowerCase();
    return tag === "input" || tag === "textarea" || tag === "select" || target.isContentEditable;
  }

  function previewTemplate(file: FilePreview, width: number, height: number) {
    if (file.type !== "file" || !thumbnailKinds.has(file.previewKind || "")) {
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
    if (file.previewKind === "raw") {
      return svgIcon(fileIcon("#7a4f1d", "RAW", large));
    }
    if (file.previewKind === "video") {
      return svgIcon(fileIcon("#6b4bd8", "VID", large));
    }
    if (file.previewKind === "pdf") {
      return svgIcon(fileIcon("#b73232", "PDF", large));
    }
    if (file.previewKind === "office") {
      return svgIcon(fileIcon("#2457a6", "DOC", large));
    }
    if (file.previewKind === "3d") {
      return svgIcon(fileIcon("#31784f", "3D", large));
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

  async function openViewer(file: IParsedEntity) {
    viewerFile = file;
    viewerText = null;
    resetViewerImage();
    if (file.previewKind === "text" || file.previewKind === "markdown") {
      viewerTextLoading = true;
      try {
        viewerText = await fetchTextPreview(file.id);
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
    viewerDragging = false;
  }

  function currentImageFiles() {
    if (!fileApi) {
      return [] as IParsedEntity[];
    }
    const state = fileApi.getState();
    const panel = state.panels?.[state.activePanel ?? 0];
    return (panel?._files || []).filter((file: IParsedEntity) => file.type === "file" && file.previewKind === "image");
  }

  function showAdjacentImage(direction: -1 | 1) {
    if (!viewerFile || viewerImages.length < 2 || viewerIndex < 0) {
      return;
    }
    const next = (viewerIndex + direction + viewerImages.length) % viewerImages.length;
    viewerFile = viewerImages[next];
    resetViewerImage();
  }

  function resetViewerImage() {
    viewerZoom = 1;
    viewerOriginal = false;
    viewerDragging = false;
    if (viewerStage) {
      viewerStage.scrollLeft = 0;
      viewerStage.scrollTop = 0;
    }
  }

  function zoomViewer(delta: number) {
    const nextZoom = Math.min(5, Math.max(0.25, Math.round((viewerZoom + delta) * 100) / 100));
    if (nextZoom === viewerZoom) {
      return;
    }
    const stage = viewerStage;
    const xRatio = stage && stage.scrollWidth > stage.clientWidth ? (stage.scrollLeft + stage.clientWidth / 2) / stage.scrollWidth : 0.5;
    const yRatio = stage && stage.scrollHeight > stage.clientHeight ? (stage.scrollTop + stage.clientHeight / 2) / stage.scrollHeight : 0.5;
    viewerZoom = nextZoom;
    requestAnimationFrame(() => {
      if (!stage) {
        return;
      }
      stage.scrollLeft = stage.scrollWidth * xRatio - stage.clientWidth / 2;
      stage.scrollTop = stage.scrollHeight * yRatio - stage.clientHeight / 2;
    });
  }

  function viewerImageURL(file: IParsedEntity) {
    return viewerOriginal ? rawFileURL(file.id) : thumbnailURL(file.id, 2048);
  }

  function viewerCanvasSize() {
    return `${Math.round(viewerZoom * 100)}%`;
  }

  function viewerDate(file: IParsedEntity) {
    const date = file.date instanceof Date ? file.date : new Date(file.date || Date.now());
    return formatDate(date.toISOString());
  }

  function viewerPointerDown(event: PointerEvent) {
    if (!viewerStage || viewerZoom <= 1 || event.button !== 0) {
      return;
    }
    viewerDragging = true;
    viewerDragStartX = event.clientX;
    viewerDragStartY = event.clientY;
    viewerDragScrollLeft = viewerStage.scrollLeft;
    viewerDragScrollTop = viewerStage.scrollTop;
    viewerStage.setPointerCapture(event.pointerId);
  }

  function viewerPointerMove(event: PointerEvent) {
    if (!viewerStage || !viewerDragging) {
      return;
    }
    viewerStage.scrollLeft = viewerDragScrollLeft - (event.clientX - viewerDragStartX);
    viewerStage.scrollTop = viewerDragScrollTop - (event.clientY - viewerDragStartY);
  }

  function stopViewerDrag(event: PointerEvent) {
    if (viewerStage?.hasPointerCapture(event.pointerId)) {
      viewerStage.releasePointerCapture(event.pointerId);
    }
    viewerDragging = false;
  }

  function viewerWheel(event: WheelEvent) {
    zoomViewer(event.deltaY < 0 ? 0.25 : -0.25);
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

  async function submitSearch() {
    const query = searchQuery.trim();
    if (!query) {
      searchOpen = false;
      searchResults = [];
      return;
    }
    searchBusy = true;
    error = "";
    try {
      const response = await searchFiles(query, 80);
      searchResults = response.entries;
      searchOpen = true;
    } catch (err) {
      error = messageFromError(err);
    } finally {
      searchBusy = false;
    }
  }

  async function openSearchResult(entry: FileEntry) {
    if (!fileApi) {
      return;
    }
    const targetPath = entry.type === "dir" ? entry.path : parentPath(entry.path);
    await runAction("Opening result", async () => {
      await loadPathAncestors(fileApi as IApi, targetPath);
      await (fileApi as IApi).exec("set-path", { id: targetPath });
      if (entry.type === "file") {
        await (fileApi as IApi).exec("select-file", { id: entry.path });
      }
      searchOpen = false;
    });
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
    error = "";
    try {
      stats = await adminStats();
      adminJob = stats.current_job || null;
    } catch (err) {
      error = messageFromError(err);
    }
  }

  async function refreshAdminUsers() {
    adminBusy = "Loading users";
    error = "";
    try {
      const response = await listAdminUsers();
      adminUsers = response.users;
    } catch (err) {
      error = messageFromError(err);
    } finally {
      adminBusy = "";
    }
  }

  async function refreshAdminJob() {
    try {
      const response = await currentAdminJob();
      const newJob = response.job;
      if (
        newJob?.status !== adminJob?.status ||
        newJob?.done !== adminJob?.done ||
        newJob?.failed !== adminJob?.failed ||
        newJob?.deleted !== adminJob?.deleted ||
        newJob?.message !== adminJob?.message
      ) {
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
    if (adminJob?.status === "running") {
      error = "Another admin job is already running";
      return;
    }
    error = "";
    try {
      const response = kind === "reindex" ? await startReindex() : await startPreviewWarmup();
      adminJob = response.job;
      startAdminPolling();
    } catch (err) {
      error = messageFromError(err);
    }
  }

  async function cancelRunningAdminJob() {
    error = "";
    try {
      const response = await cancelAdminJob();
      adminJob = response.job;
      startAdminPolling();
    } catch (err) {
      error = messageFromError(err);
    }
  }

  async function clearPreviewCacheFromAdmin() {
    adminBusy = "Clearing preview cache";
    error = "";
    try {
      await clearPreviewCache();
      await refreshAdmin();
    } catch (err) {
      error = messageFromError(err);
    } finally {
      adminBusy = "";
    }
  }

  async function createUserFromAdmin() {
    adminBusy = "Creating user";
    error = "";
    try {
      await createAdminUser(newUser);
      newUser = emptyNewUser();
      await Promise.all([refreshAdminUsers(), refreshAdmin()]);
    } catch (err) {
      error = messageFromError(err);
    } finally {
      adminBusy = "";
    }
  }

  async function saveAdminUser(target: User) {
    adminBusy = "Saving user";
    error = "";
    try {
      const response = await updateAdminUser(target.id, {
        username: target.username,
        home_root: target.home_root,
        is_admin: target.is_admin,
        disabled: target.disabled
      });
      adminUsers = adminUsers.map(item => (item.id === target.id ? response.user : item));
      if (user?.id === target.id) {
        user = response.user;
      }
      await refreshAdmin();
    } catch (err) {
      error = messageFromError(err);
    } finally {
      adminBusy = "";
    }
  }

  async function resetAdminPassword(target: User) {
    const password = (passwordReset[target.id] || "").trim();
    if (!password) {
      error = "Password is required";
      return;
    }
    adminBusy = "Resetting password";
    error = "";
    try {
      await setAdminUserPassword(target.id, password);
      passwordReset = { ...passwordReset, [target.id]: "" };
    } catch (err) {
      error = messageFromError(err);
    } finally {
      adminBusy = "";
    }
  }

  function emptyNewUser() {
    return {
      username: "",
      password: "",
      home_root: "",
      is_admin: false,
      disabled: false
    };
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
    const droppedFiles = Array.from(event.dataTransfer?.files ?? []);
    if (droppedFiles.length > 0) {
      await uploadFiles(droppedFiles);
    }
  }

  function jobProgress(job: AdminJob) {
    const deleted = job.deleted ? ` · ${job.deleted} deleted` : "";
    if (job.total_known) {
      return `${job.done}/${job.total}${deleted}`;
    }
    return `${job.done} indexed${deleted}`;
  }

  function jobScope(job: AdminJob) {
    if (!job.user && !job.scope) {
      return "";
    }
    return `${job.user || "all users"}${job.scope ? `:${job.scope}` : ""}`;
  }

  function progressValue(job: AdminJob) {
    if (!job.total_known || job.total === 0) {
      return 0;
    }
    return Math.min(100, Math.round((job.done / job.total) * 100));
  }

  function jobRunning() {
    return adminJob?.status === "running";
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

  function viewerKeydown(event: KeyboardEvent) {
    if (event.key === "Escape") {
      closeViewer();
    } else if (event.key === "ArrowLeft") {
      showAdjacentImage(-1);
    } else if (event.key === "ArrowRight") {
      showAdjacentImage(1);
    } else if (event.key === "+" || event.key === "=") {
      zoomViewer(0.25);
    } else if (event.key === "-") {
      zoomViewer(-0.25);
    } else if (event.key === "0") {
      viewerZoom = 1;
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
  <main class="app"
    on:dragover={onDragOver}
    on:dragleave={onDragLeave}
    on:drop={onDrop}
  >
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

    {#if folderHasMore}
      <div class="banner banner-warn" role="status">
        <span>Showing {folderOffset} of {folderTotal} items.</span>
        <button type="button" on:click={loadMoreFolder}>Load more</button>
      </div>
    {/if}

    {#if selectedIds.length > 0}
      <div class="selection-bar" role="status">
        <span>{selectedIds.length} selected</span>
        <div>
          <button type="button" on:click={batchDownloadSelected}>Download</button>
          <button type="button" on:click={batchMoveSelected}>Move</button>
          <button type="button" on:click={batchDeleteSelected}>Delete</button>
          <button type="button" on:click={clearSelection}>Clear</button>
        </div>
      </div>
    {/if}

    {#if dragOver}
      <div class="drop-overlay" aria-hidden="true">
        Drop to upload to {currentFolderPath()}
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

    {#if searchOpen}
      <div class="modal-backdrop" role="presentation" on:click={closeOnBackdrop(() => (searchOpen = false))}>
        <div
          class="modal-panel search-panel"
          role="dialog"
          aria-modal="true"
          aria-label="Search results"
          tabindex="-1"
          on:keydown={event => closeOnEscape(event, () => (searchOpen = false))}
        >
          <header class="modal-head">
            <div>
              <p class="eyebrow">Search</p>
              <h2>{searchResults.length} result{searchResults.length === 1 ? "" : "s"}</h2>
            </div>
            <button type="button" aria-label="Close search results" on:click={() => (searchOpen = false)}>×</button>
          </header>
          <div class="search-list">
            {#if searchResults.length === 0}
              <p class="muted">No indexed files matched "{searchQuery.trim()}".</p>
            {:else}
              {#each searchResults as entry}
                <button class="search-result" type="button" on:click={() => openSearchResult(entry)}>
                  <span class="result-kind">{entry.type === "dir" ? "Folder" : entry.preview_kind || "File"}</span>
                  <strong>{entry.name}</strong>
                  <span>{entry.path}</span>
                  <small>{entry.type === "file" ? formatBytes(entry.size) : "Folder"} · {formatDate(entry.modified_at)}</small>
                </button>
              {/each}
            {/if}
          </div>
        </div>
      </div>
    {/if}

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
              <article><span>Watcher</span><strong>{stats.watcher.enabled ? "On" : "Off"}</strong><small>{stats.watcher.roots} roots · {stats.watcher.watched_paths} paths</small></article>
              <article><span>Reconcile</span><strong>{stats.reconciliation.enabled ? "On" : "Off"}</strong><small>{stats.reconciliation.enabled ? stats.reconciliation.interval : "disabled"}</small></article>
              <article><span>Preview cfg</span><strong>{stats.preview.workers}</strong><small>{stats.preview.sizes.join(", ")} px</small></article>
            </div>
          {/if}
          <div class="admin-actions">
            <button type="button" disabled={jobRunning()} on:click={() => runAdminJob("reindex")}>Full reindex</button>
            <button type="button" disabled={jobRunning()} on:click={() => runAdminJob("preview")}>Warm previews</button>
            <button type="button" disabled={!adminJob?.cancelable || !jobRunning()} on:click={cancelRunningAdminJob}>Cancel job</button>
            <button type="button" on:click={clearPreviewCacheFromAdmin}>Clear preview cache</button>
            <button type="button" on:click={() => Promise.all([refreshAdmin(), refreshAdminUsers()])}>Refresh</button>
          </div>
          {#if adminJob}
            <section class="job-panel">
              <div>
                <strong>{adminJob.type}</strong>
                <span>{adminJob.status} · {jobProgress(adminJob)} · {adminJob.failed} failed{jobScope(adminJob) ? ` · ${jobScope(adminJob)}` : ""}</span>
              </div>
              {#if adminJob.status === "running" && adminJob.total_known}
                <progress value={progressValue(adminJob)} max="100"></progress>
              {:else if adminJob.status === "running"}
                <progress></progress>
              {/if}
              <p>{adminJob.message}</p>
            </section>
          {:else}
            <p class="muted">No admin job has run yet.</p>
          {/if}
          <section class="admin-section">
            <div class="section-head">
              <div>
                <h3>Users</h3>
                {#if adminBusy}
                  <span>{adminBusy}</span>
                {/if}
              </div>
            </div>
            <form class="user-create" on:submit|preventDefault={createUserFromAdmin}>
              <input bind:value={newUser.username} placeholder="Username" autocomplete="off" required />
              <input bind:value={newUser.password} placeholder="Password" type="password" autocomplete="new-password" required />
              <input bind:value={newUser.home_root} placeholder="Home root" required />
              <label class="check-label"><input bind:checked={newUser.is_admin} type="checkbox" /> Admin</label>
              <label class="check-label"><input bind:checked={newUser.disabled} type="checkbox" /> Disabled</label>
              <button type="submit">Create user</button>
            </form>
            <div class="user-list">
              {#if adminUsers.length === 0}
                <p class="muted">No users found.</p>
              {:else}
                {#each adminUsers as adminUser (adminUser.id)}
                  <article class="user-row">
                    <div class="user-fields">
                      <input bind:value={adminUser.username} aria-label="Username" />
                      <input bind:value={adminUser.home_root} aria-label="Home root" />
                      <label class="check-label"><input bind:checked={adminUser.is_admin} type="checkbox" /> Admin</label>
                      <label class="check-label"><input bind:checked={adminUser.disabled} type="checkbox" /> Disabled</label>
                    </div>
                    <div class="user-actions">
                      <button type="button" on:click={() => saveAdminUser(adminUser)}>Save</button>
                      <input
                        value={passwordReset[adminUser.id] || ""}
                        placeholder="New password"
                        type="password"
                        autocomplete="new-password"
                        on:input={event => (passwordReset = { ...passwordReset, [adminUser.id]: event.currentTarget.value })}
                      />
                      <button type="button" on:click={() => resetAdminPassword(adminUser)}>Set password</button>
                    </div>
                  </article>
                {/each}
              {/if}
            </div>
          </section>
        </div>
      </div>
    {/if}

    {#if viewerFile}
      {@const vkind = viewerFile.previewKind}
      <div
        class="viewer"
        role="dialog"
        aria-modal="true"
        aria-label={viewerFile.name}
        tabindex="-1"
        on:click={thumbnailViewerKinds.has(vkind || "") ? closeOnBackdrop(closeViewer) : undefined}
        on:keydown={viewerKeydown}
      >
        <header>
          <div class="viewer-title">
            <strong>{viewerFile.name}</strong>
            {#if vkind === "image" && viewerIndex >= 0}
              <span>{viewerIndex + 1} / {viewerImages.length} · {formatBytes(viewerFile.size || 0)} · {viewerDate(viewerFile)}</span>
            {:else}
              <span>{formatBytes(viewerFile.size || 0)} · {viewerDate(viewerFile)}</span>
            {/if}
          </div>
          <div>
            {#if vkind === "image"}
              <button type="button" disabled={viewerImages.length < 2} on:click={() => showAdjacentImage(-1)}>Previous</button>
              <button type="button" disabled={viewerImages.length < 2} on:click={() => showAdjacentImage(1)}>Next</button>
              <button type="button" on:click={() => zoomViewer(-0.25)}>Zoom out</button>
              <button type="button" on:click={() => (viewerZoom = 1)}>{Math.round(viewerZoom * 100)}%</button>
              <button type="button" on:click={() => zoomViewer(0.25)}>Zoom in</button>
              <button type="button" class:active={viewerOriginal} on:click={() => (viewerOriginal = !viewerOriginal)}>
                {viewerOriginal ? "Preview" : "Original"}
              </button>
            {/if}
            <button type="button" on:click={downloadViewer}>Download</button>
            <button type="button" on:click={closeViewer}>Close</button>
          </div>
        </header>

        {#if vkind === "image" || vkind === "raw" || vkind === "office"}
          <div
            class="viewer-stage"
            class:pannable={vkind === "image" && viewerZoom > 1}
            class:panning={viewerDragging}
            role="img"
            aria-label={viewerFile.name}
            bind:this={viewerStage}
            on:pointerdown={vkind === "image" ? viewerPointerDown : undefined}
            on:pointermove={vkind === "image" ? viewerPointerMove : undefined}
            on:pointerup={vkind === "image" ? stopViewerDrag : undefined}
            on:pointercancel={vkind === "image" ? stopViewerDrag : undefined}
            on:wheel|preventDefault={vkind === "image" ? viewerWheel : undefined}
          >
            <div class="viewer-canvas" style={`width: ${viewerCanvasSize()}; height: ${viewerCanvasSize()};`}>
              <img
                src={vkind === "image" ? viewerImageURL(viewerFile) : thumbnailURL(viewerFile.id, 2048)}
                alt={viewerFile.name}
                draggable="false"
              />
            </div>
          </div>
          <footer class="viewer-details">
            <span>{viewerFile.id}</span>
            <span>{vkind === "image" && viewerOriginal ? "Original file" : "Cached preview"}</span>
          </footer>

        {:else if vkind === "video"}
          <div class="viewer-media">
            <!-- svelte-ignore a11y-media-has-caption -->
            <video controls src={rawFileURL(viewerFile.id)} class="viewer-video" preload="metadata">
              Your browser does not support video playback.
            </video>
          </div>

        {:else if vkind === "pdf"}
          <div class="viewer-media">
            <iframe
              src={rawFileURL(viewerFile.id)}
              class="viewer-pdf"
              title={viewerFile.name}
            ></iframe>
          </div>

        {:else if vkind === "text" || vkind === "markdown"}
          <div class="viewer-text-wrap">
            {#if viewerTextLoading}
              <p class="viewer-text-notice">Loading…</p>
            {:else if viewerText}
              {#if viewerText.truncated}
                <p class="viewer-text-notice">Showing first {formatBytes(viewerText.max_bytes)} of {formatBytes(viewerText.size)}</p>
              {/if}
              <pre class="viewer-text">{viewerText.content}</pre>
            {:else}
              <p class="viewer-text-notice">Preview unavailable.</p>
            {/if}
          </div>
        {:else if vkind === "3d"}
          <div class="viewer-media">
            {#await loadThreeDViewer()}
              <p class="viewer-text-notice">Loading 3D viewer…</p>
            {:then module}
              <svelte:component this={module.default} src={rawFileURL(viewerFile.id)} name={viewerFile.name} />
            {:catch}
              <p class="viewer-text-notice">3D viewer unavailable.</p>
            {/await}
          </div>
        {/if}
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
          <div class="upload-notice" class:warning={uploadsActive}>
            {uploadQueueNotice()}
          </div>
          <div class="upload-list">
            {#each uploadQueue as item (item.id)}
              <article
                class:failed={item.status === "error"}
                class:complete={item.status === "done"}
                class:interrupted={item.status === "interrupted"}
              >
                <div class="upload-row-head">
                  <strong>{item.name}</strong>
                  <span>{uploadStatusText(item)}</span>
                </div>
                <div class="upload-meta">
                  <span>{formatBytes(item.size)}</span>
                  <span>{item.targetPath}</span>
                </div>
                {#if item.status !== "interrupted"}
                  <progress value={item.progress} max="100"></progress>
                {/if}
                {#if item.finalPath}
                  <div class="upload-final">{item.finalPath}</div>
                {/if}
                {#if item.status === "interrupted"}
                  <div class="upload-error">
                    <span>File handle lost — re-upload required</span>
                  </div>
                {:else if item.error}
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
