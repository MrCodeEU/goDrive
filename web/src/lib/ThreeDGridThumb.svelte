<script lang="ts">
  import { onMount, onDestroy } from "svelte";

  export let src: string;
  export let name: string;
  export let path: string;

  let container: HTMLDivElement;
  let thumbUrl = "";
  let failed = false;
  let observer: IntersectionObserver | null = null;
  let loaded = false;

  onMount(() => {
    observer = new IntersectionObserver(entries => {
      if (entries[0]?.isIntersecting && !loaded) {
        loaded = true;
        observer?.disconnect();
        void load();
      }
    }, { rootMargin: "120px" });
    observer.observe(container);
  });

  onDestroy(() => observer?.disconnect());

  async function load() {
    const { getThumb } = await import("./ThreeDThumb.js");
    const url = await getThumb(path, src, name);
    if (url) thumbUrl = url;
    else failed = true;
  }
</script>

<div class="threed-thumb" bind:this={container}>
  {#if thumbUrl}
    <img src={thumbUrl} alt={name} />
  {:else if failed}
    <span class="file-icon threed-icon">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" width="28" height="28">
        <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/>
        <polyline points="7.5 4.21 12 6.81 16.5 4.21"/><polyline points="7.5 19.79 7.5 14.6 3 12"/><polyline points="21 12 16.5 14.6 16.5 19.79"/><polyline points="3.27 6.96 12 12.01 20.73 6.96"/><line x1="12" y1="22.08" x2="12" y2="12"/>
      </svg>
      3D
    </span>
  {:else}
    <span class="file-icon threed-icon threed-loading">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" width="28" height="28">
        <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/>
        <polyline points="7.5 4.21 12 6.81 16.5 4.21"/><polyline points="7.5 19.79 7.5 14.6 3 12"/><polyline points="21 12 16.5 14.6 16.5 19.79"/><polyline points="3.27 6.96 12 12.01 20.73 6.96"/><line x1="12" y1="22.08" x2="12" y2="12"/>
      </svg>
      3D
    </span>
  {/if}
</div>

<style>
  .threed-thumb { width: 100%; height: 100%; display: flex; align-items: center; justify-content: center; }
  .threed-thumb img { width: 100%; height: 100%; object-fit: cover; }
  .threed-icon { opacity: 0.7; }
  .threed-loading { animation: pulse 1.4s ease-in-out infinite; }
  @keyframes pulse { 0%, 100% { opacity: 0.4; } 50% { opacity: 0.8; } }
</style>
