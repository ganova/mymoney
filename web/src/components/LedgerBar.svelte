<script lang="ts">
  import { app, switchLedger, haptic } from '../lib/state.svelte';

  function pick(id: number) {
    haptic();
    switchLedger(id);
  }
</script>

{#if app.ledgers.length > 1}
  <div class="ledger-bar">
    <div class="seg" role="tablist" aria-label="Buku keuangan">
      {#each app.ledgers as l (l.id)}
        <button role="tab" aria-selected={app.ledgerId === l.id} class:on={app.ledgerId === l.id} onclick={() => pick(l.id)}>
          <span aria-hidden="true">{l.icon}</span> {l.name}
        </button>
      {/each}
    </div>
  </div>
{/if}

<style>
  .ledger-bar {
    position: sticky; top: 0; z-index: 30;
    padding: calc(var(--safe-top) + 8px) 16px 4px;
    background: color-mix(in srgb, var(--bg) 88%, transparent);
    backdrop-filter: blur(12px); -webkit-backdrop-filter: blur(12px);
  }
  .seg { max-width: 640px; margin: 0 auto; }
  .seg button { min-height: 34px; font-size: 14px; }
</style>
