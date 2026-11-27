<script lang="ts">
  import { rp } from '../lib/format';

  export type DonutLine = { key: string; name: string; icon: string; amount: number; mandatory: boolean };

  // Komposisi rencana periode. Cincin dalam: wajib / fleksibel / belum dialokasikan.
  // Cincin luar: tiap kategori, diwarnai gradasi kelompoknya (biru = wajib,
  // oranye = fleksibel) agar tetap terbaca walau kategorinya banyak.
  let { income, lines }: { income: number; lines: DonutLine[] } = $props();

  const items = $derived(lines.filter((l) => l.amount > 0));
  const wajib = $derived(items.filter((l) => l.mandatory).sort((a, b) => b.amount - a.amount));
  const fleks = $derived(items.filter((l) => !l.mandatory).sort((a, b) => b.amount - a.amount));
  const mandatory = $derived(wajib.reduce((s, l) => s + l.amount, 0));
  const flexible = $derived(fleks.reduce((s, l) => s + l.amount, 0));
  const expense = $derived(mandatory + flexible);
  const leftover = $derived(Math.max(income - expense, 0));
  const whole = $derived(Math.max(income, expense, 1));
  const pctOf = (v: number, base: number) => (base > 0 ? Math.round((v / base) * 1000) / 10 : 0);

  const REST = 'color-mix(in srgb, var(--muted) 45%, var(--surface))';
  // gradasi dalam satu kelompok: terbesar paling pekat, makin kecil makin pudar
  const shade = (base: string, i: number, n: number) => `color-mix(in srgb, ${base} ${n <= 1 ? 100 : Math.round(100 - (i / (n - 1)) * 45)}%, var(--surface))`;

  type Seg = { key: string; label: string; icon?: string; value: number; color: string; group: string };
  const outer = $derived<Seg[]>([
    ...wajib.map((l, i) => ({ key: l.key, label: l.name, icon: l.icon, value: l.amount, color: shade('var(--series-1)', i, wajib.length), group: 'wajib' })),
    ...fleks.map((l, i) => ({ key: l.key, label: l.name, icon: l.icon, value: l.amount, color: shade('var(--series-2)', i, fleks.length), group: 'fleks' })),
    ...(leftover > 0 ? [{ key: 'sisa', label: 'Belum dialokasikan', value: leftover, color: REST, group: 'sisa' }] : []),
  ]);
  const inner = $derived<Seg[]>(
    [
      { key: 'g-wajib', label: 'Wajib', value: mandatory, color: 'var(--series-1)', group: 'wajib' },
      { key: 'g-fleks', label: 'Fleksibel', value: flexible, color: 'var(--series-2)', group: 'fleks' },
      { key: 'g-sisa', label: 'Belum dialokasikan', value: leftover, color: REST, group: 'sisa' },
    ].filter((s) => s.value > 0),
  );

  let hov = $state<string | null>(null);
  const active = $derived([...outer, ...inner].find((s) => s.key === hov) ?? null);
  const isDim = (s: Seg) => hov !== null && hov !== s.key && !(hov === 'g-' + s.group) && !(s.key.startsWith('g-') && active && !active.key.startsWith('g-') && active.group === s.group);

  const C = 100;
  function arc(R: number, r: number, a0: number, a1: number): string {
    const p = (rad: number, ang: number) => `${(C + rad * Math.sin(ang)).toFixed(2)},${(C - rad * Math.cos(ang)).toFixed(2)}`;
    if (a1 - a0 >= Math.PI * 2 - 1e-6) {
      return `M${p(R, 0)} A${R},${R} 0 1 1 ${p(R, Math.PI)} A${R},${R} 0 1 1 ${p(R, 0)} M${p(r, 0)} A${r},${r} 0 1 0 ${p(r, Math.PI)} A${r},${r} 0 1 0 ${p(r, 0)} Z`;
    }
    const large = a1 - a0 > Math.PI ? 1 : 0;
    return `M${p(R, a0)} A${R},${R} 0 ${large} 1 ${p(R, a1)} L${p(r, a1)} A${r},${r} 0 ${large} 0 ${p(r, a0)} Z`;
  }
  function ring(segs: Seg[], R: number, r: number) {
    const total = segs.reduce((s, x) => s + x.value, 0) || 1;
    let a = 0;
    return segs.map((s) => {
      const a0 = a;
      a += (s.value / total) * Math.PI * 2;
      return { ...s, d: arc(R, r, a0, a) };
    });
  }
  const outerPaths = $derived(ring(outer, 96, 70));
  const innerPaths = $derived(ring(inner, 66, 50));

  const toggle = (k: string) => (hov = hov === k ? null : k);
</script>

<div class="donut-box">
  <div class="donut-wrap">
    <svg viewBox="0 0 200 200" role="img" aria-label="Komposisi rencana per kategori: wajib {pctOf(mandatory, expense)}% dari rencana pengeluaran" onmouseleave={() => (hov = null)}>
      {#each [...outerPaths, ...innerPaths] as s (s.key)}
        <path
          d={s.d}
          class="seg"
          class:dim={isDim(s)}
          style:fill={s.color}
          role="button"
          tabindex="0"
          aria-label="{s.label}: {rp(s.value)}"
          onmouseenter={() => (hov = s.key)}
          onfocus={() => (hov = s.key)}
          onblur={() => (hov = null)}
          onclick={() => toggle(s.key)}
          onkeydown={(e) => (e.key === 'Enter' || e.key === ' ') && toggle(s.key)}
        />
      {/each}
      {#if active}
        <text x="100" y="96" text-anchor="middle" class="c-big">{pctOf(active.value, active.group === 'sisa' ? whole : expense)}%</text>
        <text x="100" y="111" text-anchor="middle" class="c-small">{active.label.length > 16 ? active.label.slice(0, 15) + '…' : active.label}</text>
        <text x="100" y="123" text-anchor="middle" class="c-small">{rp(active.value)}</text>
      {:else}
        <text x="100" y="100" text-anchor="middle" class="c-big">{pctOf(mandatory, expense)}%</text>
        <text x="100" y="115" text-anchor="middle" class="c-small">wajib / keluar</text>
      {/if}
    </svg>

    <div class="legend">
      {#each [{ key: 'wajib', title: 'Wajib', total: mandatory, list: outer.filter((s) => s.group === 'wajib') }, { key: 'fleks', title: 'Fleksibel', total: flexible, list: outer.filter((s) => s.group === 'fleks') }] as g (g.key)}
        {#if g.list.length}
          <div class="grp">
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <div class="gh" onmouseenter={() => (hov = 'g-' + g.key)} onmouseleave={() => (hov = null)}>
              <i class="sw" style:background={g.key === 'wajib' ? 'var(--series-1)' : 'var(--series-2)'}></i>
              <span class="grow">{g.title} <span class="muted">{pctOf(g.total, expense)}%</span></span>
              <span class="num">{rp(g.total)}</span>
            </div>
            {#each g.list as s (s.key)}
              <!-- svelte-ignore a11y_no_static_element_interactions -->
              <div class="li" class:on={hov === s.key} onmouseenter={() => (hov = s.key)} onmouseleave={() => (hov = null)}>
                <i class="sw sm" style:background={s.color}></i>
                <span class="grow ellipsis">{s.icon} {s.label}</span>
                <span class="pc muted">{pctOf(s.value, expense)}%</span>
                <span class="num amt">{rp(s.value)}</span>
              </div>
            {/each}
          </div>
        {/if}
      {/each}
      {#if leftover > 0}
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div class="gh rest" onmouseenter={() => (hov = 'sisa')} onmouseleave={() => (hov = null)}>
          <i class="sw" style:background={REST}></i>
          <span class="grow">Belum dialokasikan <span class="muted">{pctOf(leftover, income)}% pendapatan</span></span>
          <span class="num">{rp(leftover)}</span>
        </div>
      {/if}
      {#if income > 0 && expense > income}
        <div class="tiny bad">Rencana keluar melebihi pendapatan {rp(expense - income)}</div>
      {/if}
      <div class="tiny muted">Persentase kategori = porsi dari rencana pengeluaran {rp(expense)}{income > 0 ? `; wajib = ${pctOf(mandatory, income)}% dari pendapatan` : ''}.</div>
    </div>
  </div>
</div>

<style>
  .donut-box { container-type: inline-size; }
  .donut-wrap { display: flex; align-items: flex-start; gap: 24px; }
  svg { width: 220px; height: 220px; flex: none; overflow: visible; }
  .seg { stroke: var(--surface); stroke-width: 1.5; stroke-linejoin: round; cursor: pointer; transition: opacity 0.15s; }
  .seg.dim { opacity: 0.3; }
  .seg:focus-visible { outline: none; stroke: var(--text); }
  .c-big { font-size: 22px; font-weight: 750; fill: var(--text); }
  .c-small { font-size: 10px; fill: var(--muted); }
  .legend { flex: 1; min-width: 0; display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px 24px; align-content: start; }
  .legend > .tiny, .legend > .rest { grid-column: 1 / -1; }
  .grp { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
  .gh { display: flex; align-items: center; gap: 8px; font-weight: 650; font-size: 14px; padding-bottom: 4px; border-bottom: 1px solid var(--line); }
  .gh.rest { border-bottom: 0; }
  .li { display: flex; align-items: center; gap: 8px; font-size: 13.5px; color: var(--text-2); padding: 2px 4px; margin: 0 -4px; border-radius: 6px; }
  .li.on { background: var(--surface-2); color: var(--text); }
  .sw { width: 12px; height: 12px; border-radius: 3px; flex: none; }
  .sw.sm { width: 10px; height: 10px; }
  .pc { font-size: 12px; width: 44px; text-align: right; flex: none; }
  .amt { width: 104px; text-align: right; flex: none; }
  .num { font-variant-numeric: tabular-nums; white-space: nowrap; }
  /* wadah sempit (ponsel): donut di atas, legenda selebar kartu */
  @container (max-width: 560px) {
    .donut-wrap { flex-direction: column; align-items: stretch; gap: 16px; }
    svg { align-self: center; width: 200px; height: 200px; }
    .legend { grid-template-columns: 1fr; }
  }
</style>
