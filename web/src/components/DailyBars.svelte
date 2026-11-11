<script lang="ts">
  import type { DayTotal } from '../lib/types';
  import { rp, rpShort, shortDate } from '../lib/format';

  let { daily, today, allowance = 0, height = 120 }: { daily: DayTotal[]; today: string; allowance?: number; height?: number } = $props();

  // lebar mengikuti wadah agar teks tidak meregang di layar lebar
  let cw = $state(320);
  const W = $derived(Math.max(cw, 200));
  const H = $derived(height);
  const PAD_TOP = 14;
  // Skala tidak ikut lonjakan tagihan besar (mis. cicilan rumah) agar belanja
  // harian tetap terbaca; batang yang terpotong diberi penanda ▲.
  const max = $derived.by(() => {
    const vals = daily.map((d) => d.expense).filter((v) => v > 0).sort((a, b) => a - b);
    if (!vals.length) return Math.max(allowance * 1.15, 1);
    const p75 = vals[Math.floor((vals.length - 1) * 0.75)];
    const cap = Math.max(allowance * 2, p75 * 2.5);
    return Math.max(Math.min(vals[vals.length - 1], cap), allowance * 1.15, 1);
  });
  const bw = $derived(W / Math.max(daily.length, 1));
  let sel = $state<number | null>(null);
  let hov = $state<number | null>(null);

  function y(v: number) {
    return H - (v / max) * (H - PAD_TOP);
  }
  function barPath(i: number, v: number): string {
    const x = i * bw + 1;
    const w = Math.max(bw - 2, 1);
    const top = y(Math.min(v, max));
    const h = H - top;
    if (h <= 0) return '';
    const r = Math.min(4, w / 2, h);
    return `M${x},${H} V${top + r} Q${x},${top} ${x + r},${top} H${x + w - r} Q${x + w},${top} ${x + w},${top + r} V${H} Z`;
  }
  const active = $derived(hov ?? sel);
  const selected = $derived(active !== null ? daily[active] : null);
</script>

<div class="wrap" bind:clientWidth={cw}>
  <div class="tip" class:show={!!selected}>
    {#if selected}
      <strong>{rp(selected.expense)}</strong> <span class="muted">· {shortDate(selected.date)}{selected.date === today ? ' (hari ini)' : ''}</span>
    {:else}
      <span class="muted">Arahkan kursor atau ketuk batang untuk detail</span>
    {/if}
  </div>
  <svg viewBox="0 0 {W} {H + 16}" style:height="{H + 16}px" onmouseleave={() => (hov = null)} role="img" aria-label="Pengeluaran harian dalam periode">
    <line x1="0" x2={W} y1={H} y2={H} class="base" />
    {#if allowance > 0}
      <line x1="0" x2={W} y1={y(allowance)} y2={y(allowance)} class="ref" />
      <text x={W - 2} y={y(allowance) - 3} text-anchor="end" class="reflabel">jatah {rpShort(allowance)}/hari</text>
    {/if}
    {#each daily as d, i (d.date)}
      <path d={barPath(i, d.expense)} class="bar" class:today={d.date === today} class:future={d.date > today} class:sel={active === i} />
      {#if d.expense > max}<text x={i * bw + bw / 2} y={PAD_TOP - 3} text-anchor="middle" class="clip">▲</text>{/if}
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
        onmouseenter={() => (hov = i)}
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
  svg { width: 100%; overflow: visible; display: block; }
  .base { stroke: var(--line-strong); stroke-width: 1; vector-effect: non-scaling-stroke; }
  .ref { stroke: var(--muted); stroke-dasharray: 4 3; stroke-width: 1; vector-effect: non-scaling-stroke; }
  .reflabel, .axis { font-size: 10px; fill: var(--muted); }
  .bar { fill: var(--series-1); opacity: 0.55; }
  .clip { font-size: 8px; fill: var(--series-1); }
  .bar.today, .bar.sel { opacity: 1; }
  .bar.future { opacity: 0.2; }
</style>
