<script lang="ts">
  import type { DayTotal } from '../lib/types';
  import { rp, rpShort, shortDate } from '../lib/format';

  let { daily, today, allowance = 0 }: { daily: DayTotal[]; today: string; allowance?: number } = $props();

  const W = 320;
  const H = 120;
  const PAD_TOP = 14;
  const max = $derived(Math.max(allowance * 1.15, ...daily.map((d) => d.expense), 1));
  const bw = $derived(W / Math.max(daily.length, 1));
  let sel = $state<number | null>(null);

  function y(v: number) {
    return H - (v / max) * (H - PAD_TOP);
  }
  function barPath(i: number, v: number): string {
    const x = i * bw + 1;
    const w = Math.max(bw - 2, 1);
    const top = y(v);
    const h = H - top;
    if (h <= 0) return '';
    const r = Math.min(4, w / 2, h);
    return `M${x},${H} V${top + r} Q${x},${top} ${x + r},${top} H${x + w - r} Q${x + w},${top} ${x + w},${top + r} V${H} Z`;
  }
  const selected = $derived(sel !== null ? daily[sel] : null);
</script>

<div class="wrap">
  <div class="tip" class:show={!!selected}>
    {#if selected}
      <strong>{rp(selected.expense)}</strong> <span class="muted">· {shortDate(selected.date)}{selected.date === today ? ' (hari ini)' : ''}</span>
    {:else}
      <span class="muted">Ketuk batang untuk detail</span>
    {/if}
  </div>
  <svg viewBox="0 0 {W} {H + 16}" preserveAspectRatio="none" role="img" aria-label="Pengeluaran harian dalam periode">
    <line x1="0" x2={W} y1={H} y2={H} class="base" />
    {#if allowance > 0}
      <line x1="0" x2={W} y1={y(allowance)} y2={y(allowance)} class="ref" />
      <text x={W - 2} y={y(allowance) - 3} text-anchor="end" class="reflabel">jatah {rpShort(allowance)}/hari</text>
    {/if}
    {#each daily as d, i (d.date)}
      <path d={barPath(i, d.expense)} class="bar" class:today={d.date === today} class:future={d.date > today} class:sel={sel === i} />
      <rect
        x={i * bw}
        y="0"
        width={bw}
        height={H}
        fill="transparent"
        role="button"
        tabindex="-1"
        aria-label="{shortDate(d.date)}: {rp(d.expense)}"
        onclick={() => (sel = sel === i ? null : i)}
        onkeydown={() => {}}
      />
    {/each}
    {#if daily.length}
      <text x="0" y={H + 13} class="axis">{shortDate(daily[0].date)}</text>
      <text x={W} y={H + 13} text-anchor="end" class="axis">{shortDate(daily[daily.length - 1].date)}</text>
    {/if}
  </svg>
</div>

<style>
  .wrap { position: relative; }
  .tip { font-size: 13px; min-height: 20px; margin-bottom: 4px; }
  svg { width: 100%; height: 136px; overflow: visible; }
  .base { stroke: var(--line-strong); stroke-width: 1; vector-effect: non-scaling-stroke; }
  .ref { stroke: var(--muted); stroke-dasharray: 4 3; stroke-width: 1; vector-effect: non-scaling-stroke; }
  .reflabel, .axis { font-size: 9px; fill: var(--muted); }
  .bar { fill: var(--series-1); opacity: 0.55; }
  .bar.today, .bar.sel { opacity: 1; }
  .bar.future { opacity: 0.2; }
</style>
