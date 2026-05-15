<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { EditorView, basicSetup } from 'codemirror';
  import { EditorState } from '@codemirror/state';
  import { oneDark } from '@codemirror/theme-one-dark';
  import { javascript } from '@codemirror/lang-javascript';
  import { css } from '@codemirror/lang-css';
  import { html } from '@codemirror/lang-html';
  import { json } from '@codemirror/lang-json';
  import { markdown } from '@codemirror/lang-markdown';
  import { python } from '@codemirror/lang-python';
  import { cpp } from '@codemirror/lang-cpp';
  import { java } from '@codemirror/lang-java';
  import { xml } from '@codemirror/lang-xml';

  export let content: string = '';
  export let filename: string = '';
  export let onChange: ((value: string) => void) | null = null;

  let container: HTMLElement;
  let view: EditorView | null = null;

  function langForFile(name: string) {
    const ext = name.split('.').pop()?.toLowerCase() ?? '';
    switch (ext) {
      case 'js': case 'jsx': case 'ts': case 'tsx': case 'mjs': case 'cjs':
        return javascript({ jsx: ext.includes('x'), typescript: ext.startsWith('t') });
      case 'css': case 'scss': case 'less': return css();
      case 'html': case 'htm': case 'svelte': return html();
      case 'json': case 'jsonc': return json();
      case 'md': case 'markdown': return markdown();
      case 'py': return python();
      case 'c': case 'cpp': case 'cc': case 'h': case 'hpp': return cpp();
      case 'java': return java();
      case 'xml': case 'svg': return xml();
      default: return null;
    }
  }

  onMount(() => {
    const lang = langForFile(filename);
    const extensions = [
      basicSetup,
      oneDark,
      EditorView.lineWrapping,
      EditorView.updateListener.of(update => {
        if (update.docChanged && onChange) {
          onChange(update.state.doc.toString());
        }
      }),
    ];
    if (lang) extensions.push(lang);

    view = new EditorView({
      state: EditorState.create({ doc: content, extensions }),
      parent: container,
    });
  });

  onDestroy(() => view?.destroy());

  export function getValue(): string {
    return view?.state.doc.toString() ?? content;
  }
</script>

<div class="cm-host" bind:this={container}></div>

<style>
  .cm-host {
    height: 100%;
    overflow: hidden;
  }
  .cm-host :global(.cm-editor) {
    height: 100%;
    background: #0b0b0e !important;
  }
  .cm-host :global(.cm-scroller) {
    font-family: 'Fira Code', monospace;
    font-size: 13px;
    line-height: 1.7;
  }
  .cm-host :global(.cm-gutters) {
    background: #0b0b0e !important;
    border-right: 1px solid rgba(255,255,255,0.06) !important;
  }
  .cm-host :global(.cm-activeLineGutter),
  .cm-host :global(.cm-activeLine) {
    background: rgba(249,115,22,0.04) !important;
  }
  .cm-host :global(.cm-cursor) {
    border-left-color: #f97316 !important;
  }
  .cm-host :global(.cm-selectionBackground) {
    background: rgba(249,115,22,0.15) !important;
  }
</style>
