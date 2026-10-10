<script lang="ts">
  import type { PeriodInfo } from '../lib/types';
  import { app } from '../lib/state.svelte';
  import Icon from './Icon.svelte';

  let { period, onchange }: { period: PeriodInfo | null; onchange: (key: string) => void } = $props();
  const isCurrent = $derived(period?.key === app.period?.key);
</script>

{#if period}
  <div class="pnav">
    <button class="icon-btn" onclick={() => onchange(period.prev)} aria-label="Periode sebelumnya"><Icon name="chevron-left" /></button>
    <button class="mid" onclick={() => app.period && onchange(app.period.key)} disabled={isCurrent}>
      <div class="lbl">{period.label}</div>
      <div class="rng">{period.range}{isCurrent ? '' : ' · ketuk utk periode ini'}</div>
    </button>
    <button class="icon-btn" onclick={() => onchange(period.next)} aria-label="Periode berikutnya"><Icon name="chevron-right" /></button>
  </div>
{/if}

<style>
  .pnav { display: flex; align-items: center; justify-content: space-between; background: var(--surface); border-radius: 14px; box-shadow: var(--shadow); padding: 2px; }
  .mid { text-align: center; flex: 1; padding: 4px 0; }
  .mid:disabled { cursor: default; }
  .lbl { font-weight: 700; font-size: 16px; }
  .rng { font-size: 12px; color: var(--muted); }
</style>
