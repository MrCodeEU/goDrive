<script lang="ts">
  export let name = "";
  export let label = "";

  const paths: Record<string, string> = {
    admin: '<path d="M12 15.5A3.5 3.5 0 1 0 12 8a3.5 3.5 0 0 0 0 7.5Z"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09a1.65 1.65 0 0 0-1-1.51 1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.6 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 8.92 4.6a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9c.14.54.57.95 1.11 1H21a2 2 0 1 1 0 4h-.09c-.54.05-.97.46-1.11 1Z"/>',
    chevronDown: '<path d="m6 9 6 6 6-6"/>',
    chevronLeft: '<path d="m15 18-6-6 6-6"/>',
    chevronRight: '<path d="m9 18 6-6-6-6"/>',
    download: '<path d="M12 3v12"/><path d="m7 10 5 5 5-5"/><path d="M5 21h14"/>',
    file: '<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8Z"/><path d="M14 2v6h6"/>',
    filter: '<path d="M3 5h18"/><path d="M6 12h12"/><path d="M10 19h4"/>',
    folder: '<path d="M3 7a2 2 0 0 1 2-2h5l2 2h7a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z"/>',
    grid: '<path d="M3 3h7v7H3Z"/><path d="M14 3h7v7h-7Z"/><path d="M14 14h7v7h-7Z"/><path d="M3 14h7v7H3Z"/>',
    image: '<path d="M21 16V8a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v8"/><path d="m3 16 4-4a2 2 0 0 1 2.8 0L14 16"/><path d="m13 15 2-2a2 2 0 0 1 2.8 0L21 16"/><path d="M3 16v2a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-2"/>',
    list: '<path d="M8 6h13"/><path d="M8 12h13"/><path d="M8 18h13"/><path d="M3 6h.01"/><path d="M3 12h.01"/><path d="M3 18h.01"/>',
    logout: '<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><path d="M16 17l5-5-5-5"/><path d="M21 12H9"/>',
    move: '<path d="M5 9V5h4"/><path d="m5 5 6 6"/><path d="M19 15v4h-4"/><path d="m19 19-6-6"/><path d="M15 5h4v4"/><path d="m19 5-6 6"/><path d="M9 19H5v-4"/><path d="m5 19 6-6"/>',
    plus: '<path d="M12 5v14"/><path d="M5 12h14"/>',
    refresh: '<path d="M21 12a9 9 0 0 1-15 6.7L3 16"/><path d="M3 21v-5h5"/><path d="M3 12a9 9 0 0 1 15-6.7L21 8"/><path d="M21 3v5h-5"/>',
    rename: '<path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z"/>',
    search: '<path d="m21 21-4.3-4.3"/><circle cx="11" cy="11" r="8"/>',
    sort: '<path d="m7 15 5 5 5-5"/><path d="M12 4v16"/>',
    trash: '<path d="M3 6h18"/><path d="M8 6V4h8v2"/><path d="m19 6-1 14H6L5 6"/><path d="M10 11v6"/><path d="M14 11v6"/>',
    keyboard: '<rect width="20" height="16" x="2" y="4" rx="2"/><path d="M6 8h.01M10 8h.01M14 8h.01M18 8h.01M8 12h.01M12 12h.01M16 12h.01M7 16h10"/>',
    upload: '<path d="M12 21V9"/><path d="m7 14 5-5 5 5"/><path d="M5 3h14"/>'
  };
</script>

<svg class="icon" aria-hidden={label ? undefined : "true"} aria-label={label || undefined} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
  {@html paths[name] || paths.file}
</svg>
