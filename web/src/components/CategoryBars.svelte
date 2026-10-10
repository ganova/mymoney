<script lang="ts">
  import { rp } from '../lib/format';

  type Item = { key: string; icon: string; label: string; total: number; count?: number };
  let { items, total, onpick }: { items: Item[]; total: number; onpick?: (key: string) => void } = $props();
  const max = $derived(Math.max(...items.map((i) => i.total), 1));
</script>

<div class="rows">
  {#each items as it (it.key)}
    <button class="r" onclick={() => onpick?.(it.key)}>
      <div class="row between">
        <span class="row lbl"><span class="ic">{it.icon}</span><span class="ellipsis">{it.label}</span></span>
        <span class="num val">{rp(it.total)} <span class="muted small">{total ? Math.round((it.total / total) * 100) : 0}%</span></span>
      </div>
      <div class="track"><i style:width="{(it.total / max) * 100}%"></i></div>
    </button>
  {/each}
</div>

<style>
  .rows { display: flex; flex-direction: column; gap: 12px; }
  .r { text-align: left; width: 100%; }
  .lbl { gap: 8px; min-width: 0; font-weight: 550; font-size: 15px; }
  .ic { width: 22px; text-align: center; flex: none; }
  .val { font-weight: 650; font-size: 14px; white-space: nowrap; }
  .track { height: 6px; margin-top: 6px; margin-left: 30px; }
  .track i { display: block; height: 100%; border-radius: 3px; background: var(--series-1); min-width: 4px; }
</style>
