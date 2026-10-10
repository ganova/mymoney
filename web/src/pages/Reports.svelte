<script lang="ts">
  import type { Report } from '../lib/types';
  import { app, cat, acc, toastError } from '../lib/state.svelte';
  import { api, qs } from '../lib/api';
  import { back, go, route } from '../lib/router.svelte';
  import { rp, rpc, rpShort, markdown } from '../lib/format';
  import Icon from '../components/Icon.svelte';
  import PeriodNav from '../components/PeriodNav.svelte';
  import CategoryBars from '../components/CategoryBars.svelte';
  import DailyBars from '../components/DailyBars.svelte';
  import TrendBars from '../components/TrendBars.svelte';

  let periodKey = $state(route.query.period ?? app.period?.key ?? '');
  let r = $state<Report | null>(null);
  let loading = $state(true);
  let mode = $state<'category' | 'group'>('category');
  let insight = $state<{ content: string; created_at: string } | null>(null);
  let insightBusy = $state(false);

  $effect(() => {
    app.dataVersion;
    const key = periodKey;
    const ctrl = new AbortController();
    loading = true;
    api.get<Report>('/reports' + qs({ period: key }), ctrl.signal)
      .then((x) => (r = x))
      .catch((e) => e.name !== 'AbortError' && toastError(e))
      .finally(() => (loading = false));
    api.get<{ insight: typeof insight }>('/ai/insight' + qs({ period: key }), ctrl.signal).then((x) => (insight = x.insight)).catch(() => {});
    return () => ctrl.abort();
  });

  const expenseItems = $derived.by(() => {
    if (!r) return [];
    const rows = r.by_category.filter((c) => c.kind === 'expense');
    if (mode === 'category') {
      return rows.map((c) => {
        const k = cat(c.category_id);
        return { key: String(c.category_id ?? 0), icon: k?.icon ?? '❔', label: k?.name ?? 'Tanpa kategori', total: c.total, count: c.count };
      });
    }
    const m = new Map<string, number>();
    for (const c of rows) {
      const g = cat(c.category_id)?.group ?? 'Lain-lain';
      m.set(g, (m.get(g) ?? 0) + c.total);
    }
    return [...m.entries()].sort((a, b) => b[1] - a[1]).map(([g, total]) => ({ key: g, icon: '📁', label: g, total }));
  });

  async function genInsight() {
    insightBusy = true;
    try {
      const x = await api.post<{ insight: typeof insight }>('/ai/insight' + qs({ period: periodKey }));
      insight = x.insight;
    } catch (e) {
      toastError(e);
    } finally {
      insightBusy = false;
    }
  }
</script>

<div class="page stack">
  <header class="page-head">
    <button class="back" onclick={() => back('/more')}><Icon name="chevron-left" /> Lainnya</button>
    <a class="btn sm" href={'/api/export' + qs({ period: periodKey, format: 'xlsx' })}><Icon name="download" size={16} /> Excel</a>
  </header>
  <h1 class="title">Laporan</h1>
  <PeriodNav period={r?.period ?? null} onchange={(k) => (periodKey = k)} />

  {#if r}
    <div class="kpis" class:dim={loading}>
      <div class="card"><div class="tiny muted">Pemasukan</div><div class="bold num good">{rpc(r.income)}</div></div>
      <div class="card"><div class="tiny muted">Pengeluaran</div><div class="bold num">{rpc(r.expense)}</div></div>
      <div class="card"><div class="tiny muted">Selisih</div><div class="bold num" class:bad={r.income - r.expense < 0}>{rpc(r.income - r.expense)}</div></div>
    </div>

    <section class="card">
      <div class="card-title">
        <span>Pengeluaran per {mode === 'category' ? 'kategori' : 'grup'}</span>
        <button onclick={() => (mode = mode === 'category' ? 'group' : 'category')}>{mode === 'category' ? 'Per grup' : 'Per kategori'}</button>
      </div>
      {#if expenseItems.length}
        <CategoryBars items={expenseItems} total={r.expense} onpick={(k) => mode === 'category' && go('/tx', { period: periodKey, category: k === '0' ? undefined : k })} />
      {:else}
        <p class="muted small">Belum ada pengeluaran.</p>
      {/if}
    </section>

    <section class="card">
      <div class="card-title">Pengeluaran harian</div>
      <DailyBars daily={r.daily} today={app.today} />
    </section>

    <section class="card">
      <div class="card-title">Tren 6 periode</div>
      <TrendBars history={r.history} />
      <table class="tbl small">
        <thead><tr><th>Periode</th><th>Masuk</th><th>Keluar</th><th>Selisih</th></tr></thead>
        <tbody>
          {#each [...r.history].reverse() as h (h.key)}
            <tr><td>{h.label.slice(0, 3)} {h.label.slice(-2)}</td><td class="num">{rpShort(h.income)}</td><td class="num">{rpShort(h.expense)}</td><td class="num" class:bad={h.income - h.expense < 0}>{rpShort(h.income - h.expense)}</td></tr>
          {/each}
        </tbody>
      </table>
    </section>

    {#if r.top.length}
      <section class="card">
        <div class="card-title">Pengeluaran terbesar</div>
        <div class="stack top">
          {#each r.top as t, i (i)}
            <div class="row">
              <span class="rank">{i + 1}</span>
              <span class="ic">{cat(t.category_id)?.icon ?? '🧾'}</span>
              <span class="grow ellipsis">{t.description || 'Tanpa deskripsi'} {#if t.count > 1}<span class="muted tiny">×{t.count}</span>{/if}</span>
              <span class="num bold small">{rp(t.total)}</span>
            </div>
          {/each}
        </div>
      </section>
    {/if}

    {#if r.by_account.length}
      <section class="card">
        <div class="card-title">Per akun</div>
        <div class="stack top">
          {#each r.by_account as a (a.account_id)}
            {@const x = acc(a.account_id)}
            <div class="row">
              <span class="ic">{x?.icon}</span>
              <span class="grow">{x?.name}</span>
              <span class="num small"><span class="good">+{rp(a.income)}</span> · −{rp(a.expense)}</span>
            </div>
          {/each}
        </div>
      </section>
    {/if}

    {#if app.ai.enabled}
      <section class="card">
        <div class="card-title">
          <span class="row" style="gap:6px"><Icon name="brain" size={16} /> Analisis AI</span>
          <button onclick={genInsight} disabled={insightBusy}>{insight ? 'Perbarui' : 'Buat'}</button>
        </div>
        {#if insightBusy}
          <div class="row muted small"><span class="spinner"></span> Menganalisis…</div>
        {:else if insight}
          <div class="md small">{@html markdown(insight.content)}</div>
        {:else}
          <p class="small muted">Minta AI merangkum pola belanja dan memberi saran.</p>
        {/if}
      </section>
    {/if}
  {:else}
    <div class="card skeleton" style="height:200px"></div>
  {/if}
</div>

<style>
  .title { font-size: 28px; font-weight: 750; margin-top: -8px; }
  .kpis { display: grid; grid-template-columns: repeat(3, 1fr); gap: 8px; }
  .kpis .card { padding: 12px; }
  .kpis .bold { font-size: 14px; white-space: nowrap; letter-spacing: -0.01em; }
  .dim { opacity: 0.6; }
  .tbl { width: 100%; border-collapse: collapse; margin-top: 12px; }
  .tbl th { text-align: left; font-weight: 600; color: var(--muted); font-size: 12px; padding: 6px 4px; border-bottom: 1px solid var(--line); }
  .tbl td { padding: 6px 4px; border-bottom: 1px solid var(--line); }
  .tbl th:not(:first-child), .tbl td:not(:first-child) { text-align: right; }
  .top { gap: 10px; }
  .rank { width: 18px; color: var(--muted); font-size: 13px; text-align: right; flex: none; }
  .ic { width: 22px; text-align: center; flex: none; }
</style>
