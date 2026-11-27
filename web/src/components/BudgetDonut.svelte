<script lang="ts">
  import { rp } from '../lib/format';

  // Komposisi rencana periode: wajib, fleksibel, dan sisa pendapatan yang belum dialokasikan.
  let { income, mandatory, flexible }: { income: number; mandatory: number; flexible: number } = $props();

  const expense = $derived(mandatory + flexible);
  const leftover = $derived(Math.max(income - expense, 0));
  const whole = $derived(Math.max(income, expense, 1));
  const parts = $derived(
    [
      { key: 'wajib', label: 'Wajib', value: mandatory, cls: 's1' },
      { key: 'fleks', label: 'Fleksibel', value: flexible, cls: 's2' },
      { key: 'sisa', label: 'Belum dialokasikan', value: leftover, cls: 'rest' },
    ].filter((p) => p.value > 0),
  );
  const pctOf = (v: number, base: number) => (base > 0 ? Math.round((v / base) * 1000) / 10 : 0);

  let hov = $state<string | null>(null);
  const active = $derived(parts.find((p) => p.key === hov) ?? null);

  // geometri donut
  const R = 60;
  const r = 40;
  const C = 70;
  function arc(a0: number, a1: number): string {
    const large = a1 - a0 > Math.PI ? 1 : 0;
    const p = (rad: number, ang: number) => `${C + rad * Math.sin(ang)},${C - rad * Math.cos(ang)}`;
    if (a1 - a0 >= Math.PI * 2 - 1e-6) {
      // satu bagian penuh: dua setengah lingkaran
      return `M${p(R, 0)} A${R},${R} 0 1 1 ${p(R, Math.PI)} A${R},${R} 0 1 1 ${p(R, 0)} M${p(r, 0)} A${r},${r} 0 1 0 ${p(r, Math.PI)} A${r},${r} 0 1 0 ${p(r, 0)} Z`;
    }
    return `M${p(R, a0)} A${R},${R} 0 ${large} 1 ${p(R, a1)} L${p(r, a1)} A${r},${r} 0 ${large} 0 ${p(r, a0)} Z`;
  }
  const segs = $derived.by(() => {
    const total = parts.reduce((s, p) => s + p.value, 0) || 1;
    let a = 0;
    return parts.map((p) => {
      const a0 = a;
      a += (p.value / total) * Math.PI * 2;
      return { ...p, d: arc(a0, a) };
    });
  });
</script>

<div class="donut-box">
<div class="donut-wrap">
  <svg viewBox="0 0 140 140" role="img" aria-label="Komposisi rencana: wajib {pctOf(mandatory, expense)}% dari rencana pengeluaran" onmouseleave={() => (hov = null)}>
    {#each segs as s (s.key)}
      <path
        d={s.d}
        class="seg {s.cls}"
        class:dim={hov !== null && hov !== s.key}
        role="button"
        tabindex="0"
        aria-label="{s.label}: {rp(s.value)}"
        onmouseenter={() => (hov = s.key)}
        onfocus={() => (hov = s.key)}
        onblur={() => (hov = null)}
        onclick={() => (hov = hov === s.key ? null : s.key)}
        onkeydown={(e) => (e.key === 'Enter' || e.key === ' ') && (hov = hov === s.key ? null : s.key)}
      />
    {/each}
    <text x="70" y="66" text-anchor="middle" class="c-big">{active ? `${pctOf(active.value, whole)}%` : `${pctOf(mandatory, expense)}%`}</text>
    <text x="70" y="82" text-anchor="middle" class="c-small">{active ? active.label.toLowerCase() : 'wajib / keluar'}</text>
  </svg>

  <div class="legend">
    {#each parts as p (p.key)}
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div class="lrow" class:on={hov === p.key} onmouseenter={() => (hov = p.key)} onmouseleave={() => (hov = null)}>
        <i class="sw {p.cls}"></i>
        <div class="grow">
          <div class="row between"><span class="nm">{p.label}</span><span class="num bold">{rp(p.value)}</span></div>
          <div class="tiny muted">
            {#if p.key === 'sisa'}
              {pctOf(p.value, income)}% dari pendapatan
            {:else}
              {pctOf(p.value, expense)}% dari rencana keluar{income > 0 ? ` · ${pctOf(p.value, income)}% dari pendapatan` : ''}
            {/if}
          </div>
        </div>
      </div>
    {/each}
    {#if income > 0 && expense > income}
      <div class="tiny bad">Rencana keluar melebihi pendapatan {rp(expense - income)}</div>
    {/if}
  </div>
</div>
</div>

<style>
  .donut-box { container-type: inline-size; }
  .donut-wrap { display: flex; align-items: center; gap: 18px; }
  svg { width: 140px; height: 140px; flex: none; overflow: visible; }
  .seg { stroke: var(--surface); stroke-width: 2; stroke-linejoin: round; cursor: pointer; transition: opacity 0.15s; }
  .seg.dim { opacity: 0.35; }
  .seg:focus-visible { outline: none; stroke: var(--text); }
  .s1 { fill: var(--series-1); background: var(--series-1); }
  .s2 { fill: var(--series-2); background: var(--series-2); }
  /* sisa = abu netral yang tetap terlihat di tema terang & gelap */
  .rest { fill: color-mix(in srgb, var(--muted) 45%, var(--surface)); background: color-mix(in srgb, var(--muted) 45%, var(--surface)); }
  .c-big { font-size: 20px; font-weight: 750; fill: var(--text); }
  .c-small { font-size: 9.5px; fill: var(--muted); }
  .legend { flex: 1; min-width: 0; max-width: 460px; display: flex; flex-direction: column; gap: 10px; }
  .lrow { display: flex; gap: 10px; align-items: flex-start; border-radius: 8px; }
  .lrow.on .nm { color: var(--text); font-weight: 650; }
  .sw { width: 12px; height: 12px; border-radius: 3px; flex: none; margin-top: 4px; }
  .nm { color: var(--text-2); font-size: 14px; }
  .bold { font-weight: 650; font-size: 14px; }
  /* wadah sempit (ponsel): donut di atas, legenda selebar kartu */
  @container (max-width: 460px) {
    .donut-wrap { flex-direction: column; align-items: stretch; gap: 14px; }
    svg { align-self: center; }
    .legend { max-width: none; }
  }
</style>
