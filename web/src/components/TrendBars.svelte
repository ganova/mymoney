<script lang="ts">
  import { rp, rpShort } from '../lib/format';

  type P = { key: string; label: string; income: number; expense: number };
  let { history }: { history: P[] } = $props();

  const W = 320;
  const H = 130;
  const max = $derived(Math.max(...history.flatMap((h) => [h.income, h.expense]), 1));
  const gw = $derived(W / Math.max(history.length, 1));
  let sel = $state<number | null>(null);
  const bw = $derived(Math.min(18, (gw - 10) / 2));

  function bar(x: number, v: number): string {
    const h = (v / max) * (H - 10);
    if (h <= 0.5) return '';
    const top = H - h;
    const r = Math.min(4, bw / 2, h);
    return `M${x},${H} V${top + r} Q${x},${top} ${x + r},${top} H${x + bw - r} Q${x + bw},${top} ${x + bw},${top + r} V${H} Z`;
  }
  const short = (label: string) => label.slice(0, 3);
  const cur = $derived(sel !== null ? history[sel] : history[history.length - 1]);
</script>

<div class="legend row">
  <span class="row key"><i class="k inc"></i> Pemasukan</span>
  <span class="row key"><i class="k exp"></i> Pengeluaran</span>
</div>
{#if cur}
  <div class="tip small">
    <strong>{cur.label}</strong> · masuk <strong class="num">{rp(cur.income)}</strong> · keluar <strong class="num">{rp(cur.expense)}</strong>
  </div>
{/if}
<svg viewBox="0 0 {W} {H + 18}" role="img" aria-label="Tren pemasukan dan pengeluaran per periode">
  <line x1="0" x2={W} y1={H} y2={H} class="base" />
  {#each history as h, i (h.key)}
    {@const x0 = i * gw + (gw - bw * 2 - 2) / 2}
    <path d={bar(x0, h.income)} class="inc" class:dim={sel !== null && sel !== i} />
    <path d={bar(x0 + bw + 2, h.expense)} class="exp" class:dim={sel !== null && sel !== i} />
    <text x={i * gw + gw / 2} y={H + 13} text-anchor="middle" class="axis" class:on={sel === i}>{short(h.label)}</text>
    <rect x={i * gw} y="0" width={gw} height={H + 18} fill="transparent" role="button" tabindex="-1" aria-label="{h.label}: masuk {rpShort(h.income)}, keluar {rpShort(h.expense)}" onclick={() => (sel = sel === i ? null : i)} onkeydown={() => {}} />
  {/each}
</svg>

<style>
  .legend { gap: 14px; font-size: 12px; color: var(--text-2); margin-bottom: 4px; }
  .key { gap: 6px; }
  .k { width: 10px; height: 10px; border-radius: 3px; display: inline-block; }
  .k.inc, path.inc { background: var(--series-1); fill: var(--series-1); }
  .k.exp, path.exp { background: var(--series-2); fill: var(--series-2); }
  .tip { min-height: 20px; margin-bottom: 4px; color: var(--text-2); }
  svg { width: 100%; height: auto; }
  .base { stroke: var(--line-strong); stroke-width: 1; }
  .axis { font-size: 10px; fill: var(--muted); }
  .axis.on { fill: var(--text); font-weight: 700; }
  .dim { opacity: 0.35; }
</style>
