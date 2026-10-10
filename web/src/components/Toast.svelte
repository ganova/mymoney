<script lang="ts">
  import { ui } from '../lib/state.svelte';
</script>

{#if ui.toast}
  {#key ui.toast.id}
    <div class="toast {ui.toast.kind}" role="status">
      <span class="grow">{ui.toast.text}</span>
      {#if ui.toast.action}
        <button
          class="act"
          onclick={() => {
            ui.toast?.action?.run();
            ui.toast = null;
          }}>{ui.toast.action.label}</button
        >
      {/if}
    </div>
  {/key}
{/if}

<style>
  .toast {
    position: fixed; left: 12px; right: 12px; z-index: 60; max-width: 560px; margin: 0 auto;
    bottom: calc(var(--tabbar-h) + var(--safe-bottom) + 14px);
    display: flex; align-items: center; gap: 12px; padding: 12px 16px; border-radius: 14px;
    background: #1f1f1e; color: #f6f6f3; box-shadow: 0 8px 30px rgba(0,0,0,0.25); font-size: 15px;
    animation: pop 0.25s cubic-bezier(0.2, 0.9, 0.3, 1.2);
  }
  .toast.error { background: #8f2424; }
  .act { color: #86b6ef; font-weight: 700; white-space: nowrap; min-height: 32px; }
  @keyframes pop { from { opacity: 0; transform: translateY(12px) scale(0.98); } to { opacity: 1; transform: none; } }
</style>
