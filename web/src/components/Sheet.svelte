<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';

  let {
    open,
    onclose,
    title = '',
    children,
    footer,
    full = false,
  }: { open: boolean; onclose: () => void; title?: string; children: Snippet; footer?: Snippet; full?: boolean } = $props();

  let dragY = $state(0);
  let startY = 0;
  let dragging = false;

  $effect(() => {
    if (!open) return;
    const prev = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onclose();
    window.addEventListener('keydown', onKey);
    return () => {
      document.body.style.overflow = prev;
      window.removeEventListener('keydown', onKey);
    };
  });

  function touchStart(e: TouchEvent) {
    startY = e.touches[0].clientY;
    dragging = true;
  }
  function touchMove(e: TouchEvent) {
    if (!dragging) return;
    dragY = Math.max(0, e.touches[0].clientY - startY);
  }
  function touchEnd() {
    dragging = false;
    if (dragY > 90) onclose();
    dragY = 0;
  }
</script>

{#if open}
  <div class="backdrop" onclick={onclose} role="presentation"></div>
  <div class="sheet" class:full role="dialog" aria-modal="true" aria-label={title} style:transform={dragY ? `translateY(${dragY}px)` : undefined}>
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="head" ontouchstart={touchStart} ontouchmove={touchMove} ontouchend={touchEnd}>
      <div class="handle"></div>
      <div class="title-row">
        <h2>{title}</h2>
        <button class="icon-btn close" onclick={onclose} aria-label="Tutup"><Icon name="x" size={20} /></button>
      </div>
    </div>
    <div class="body">
      {@render children()}
    </div>
    {#if footer}
      <div class="foot">{@render footer()}</div>
    {/if}
  </div>
{/if}

<style>
  .backdrop {
    position: fixed; inset: 0; background: rgba(0, 0, 0, 0.35); z-index: 50;
    animation: fade 0.2s ease;
  }
  .sheet {
    position: fixed; left: 0; right: 0; bottom: 0; z-index: 51;
    max-width: 640px; margin: 0 auto;
    max-height: calc(100dvh - var(--safe-top) - 24px);
    background: var(--bg); border-radius: 22px 22px 0 0; box-shadow: var(--shadow-lg);
    display: flex; flex-direction: column;
    animation: up 0.28s cubic-bezier(0.2, 0.9, 0.3, 1);
    transition: transform 0.15s;
  }
  .sheet.full { height: calc(100dvh - var(--safe-top) - 24px); }
  .head { padding: 6px 16px 0; flex: none; touch-action: none; }
  .handle { width: 38px; height: 5px; border-radius: 3px; background: var(--line-strong); margin: 0 auto 6px; }
  .title-row { display: flex; align-items: center; justify-content: space-between; min-height: 44px; }
  h2 { font-size: 19px; font-weight: 700; }
  .close { background: var(--surface-2); width: 32px; height: 32px; }
  .body { overflow-y: auto; padding: 4px 16px 16px; -webkit-overflow-scrolling: touch; flex: 1; overscroll-behavior: contain; }
  .foot {
    flex: none; padding: 10px 16px calc(var(--safe-bottom) + 12px);
    border-top: 1px solid var(--line); background: var(--bg);
  }
  .sheet:not(:has(.foot)) .body { padding-bottom: calc(var(--safe-bottom) + 20px); }
  @keyframes up { from { transform: translateY(100%); } to { transform: none; } }
  @keyframes fade { from { opacity: 0; } to { opacity: 1; } }
</style>
