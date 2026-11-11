<script lang="ts">
  import type { Transaction, PeriodInfo } from '../lib/types';
  import { app, ui, activeAccounts, activeCategories, acc, anyAccount, cat, source, changed, openTx, toast, toastError } from '../lib/state.svelte';
  import { api, qs } from '../lib/api';
  import { route } from '../lib/router.svelte';
  import { rp, dayLabel, shortDate } from '../lib/format';
  import Icon from '../components/Icon.svelte';
  import TxItem from '../components/TxItem.svelte';
  import PeriodNav from '../components/PeriodNav.svelte';

  let periodKey = $state(route.query.period ?? app.period?.key ?? '');
  let period = $state<PeriodInfo | null>(null);
  let q = $state(route.query.q ?? '');
  let type = $state('');
  let accountId = $state(Number(route.query.account) || 0);
  let categoryId = $state(Number(route.query.category) || 0);
  let allTime = $state(!!route.query.q);
  let list = $state<Transaction[]>([]);
  let loading = $state(true);
  // svelte-ignore state_referenced_locally
  let showFilters = $state(!!(accountId || categoryId));
  let debounced = $state('');
  let timer: ReturnType<typeof setTimeout>;

  $effect(() => {
    const v = q;
    clearTimeout(timer);
    timer = setTimeout(() => (debounced = v), 250);
  });

  $effect(() => {
    if (!periodKey) return;
    api.get<PeriodInfo>('/period' + qs({ period: periodKey })).then((r) => (period = r)).catch(() => {});
  });

  $effect(() => {
    app.dataVersion;
    const params = {
      period: allTime ? undefined : periodKey,
      q: debounced,
      type,
      account: accountId,
      category: categoryId,
      limit: 1000,
    };
    const ctrl = new AbortController();
    loading = true;
    api
      .get<Transaction[]>('/transactions' + qs(params), ctrl.signal)
      .then((r) => (list = r))
      .catch((e) => e.name !== 'AbortError' && toastError(e))
      .finally(() => (loading = false));
    return () => ctrl.abort();
  });

  const groups = $derived.by(() => {
    const out: { date: string; items: Transaction[]; out: number; in: number }[] = [];
    for (const t of list) {
      let g = out[out.length - 1];
      if (!g || g.date !== t.date) {
        g = { date: t.date, items: [], out: 0, in: 0 };
        out.push(g);
      }
      g.items.push(t);
      if (t.type === 'expense') g.out += t.amount;
      if (t.type === 'income') g.in += t.amount;
    }
    return out;
  });
  const totalOut = $derived(list.reduce((s, t) => (t.type === 'expense' ? s + t.amount : s), 0));
  const totalIn = $derived(list.reduce((s, t) => (t.type === 'income' ? s + t.amount : s), 0));
  const filterCount = $derived((accountId ? 1 : 0) + (categoryId ? 1 : 0) + (allTime ? 1 : 0));

  // ---- tampilan tabel (desktop): urutkan, pilih banyak, aksi massal ----
  type SortKey = 'date' | 'description' | 'category' | 'account' | 'amount';
  let sortKey = $state<SortKey>('date');
  let sortDir = $state<1 | -1>(-1);
  let picked = $state<Record<number, boolean>>({});
  let bulkBusy = $state(false);

  function sortBy(k: SortKey) {
    if (sortKey === k) sortDir = sortDir === 1 ? -1 : 1;
    else {
      sortKey = k;
      sortDir = k === 'date' || k === 'amount' ? -1 : 1;
    }
  }
  const signed = (t: Transaction) => (t.type === 'income' ? t.amount : t.type === 'expense' ? -t.amount : t.type === 'adjust' ? t.amount : 0);
  const rows = $derived.by(() => {
    const val = (t: Transaction): string | number => {
      switch (sortKey) {
        case 'description': return (t.description || '').toLowerCase();
        case 'category': return (cat(t.category_id)?.name ?? '~').toLowerCase();
        case 'account': return (acc(t.account_id)?.name ?? '').toLowerCase();
        case 'amount': return Math.abs(t.amount);
        default: return t.date;
      }
    };
    return [...list].sort((a, b) => {
      const x = val(a), y = val(b);
      if (x < y) return -sortDir;
      if (x > y) return sortDir;
      return b.id - a.id;
    });
  });
  const pickedList = $derived(list.filter((t) => picked[t.id]));
  const allPicked = $derived(list.length > 0 && pickedList.length === list.length);
  const pickedOut = $derived(pickedList.reduce((s, t) => (t.type === 'expense' ? s + t.amount : s), 0));
  const pickedIn = $derived(pickedList.reduce((s, t) => (t.type === 'income' ? s + t.amount : s), 0));
  $effect(() => {
    list; // daftar berubah (filter/periode): buang pilihan yang sudah tidak tampil
    const ids = new Set(list.map((t) => t.id));
    for (const k of Object.keys(picked)) if (!ids.has(Number(k))) delete picked[Number(k)];
  });
  function toggleAll() {
    if (allPicked) picked = {};
    else picked = Object.fromEntries(list.map((t) => [t.id, true]));
  }
  async function bulkDelete() {
    const ids = pickedList.map((t) => t.id);
    if (!ids.length || !confirm(`Hapus ${ids.length} transaksi yang dipilih?`)) return;
    bulkBusy = true;
    try {
      await api.post('/transactions/delete', { ids });
      picked = {};
      toast(`${ids.length} transaksi dihapus`);
      await changed();
    } catch (e) {
      toastError(e);
    } finally {
      bulkBusy = false;
    }
  }
  async function bulkCategory(e: Event) {
    const sel = e.currentTarget as HTMLSelectElement;
    const c = cat(Number(sel.value));
    sel.value = '';
    if (!c) return;
    const targets = pickedList.filter((t) => t.type === c.kind);
    if (!targets.length) return toast(`Tidak ada transaksi ${c.kind === 'income' ? 'pemasukan' : 'pengeluaran'} yang dipilih`, { kind: 'error' });
    bulkBusy = true;
    try {
      for (const t of targets) await api.put(`/transactions/${t.id}`, { ...t, category_id: c.id });
      toast(`${targets.length} transaksi dipindah ke ${c.name}`);
      await changed();
    } catch (err) {
      toastError(err);
    } finally {
      bulkBusy = false;
    }
  }
  const arrow = (k: SortKey) => (sortKey === k ? (sortDir === 1 ? ' ▲' : ' ▼') : '');

  function addNew() {
    openTx({ preset: { account_id: accountId || undefined, category_id: categoryId || null, type: cat(categoryId)?.kind === 'income' ? 'income' : 'expense' } });
  }
</script>

<div class="page wide">
<div class="stack txp">
  <header class="page-head">
    <h1>Transaksi</h1>
    <button class="btn sm primary desk-only" onclick={addNew}><Icon name="plus" size={18} /> Transaksi <kbd class="k">N</kbd></button>
    <button class="icon-btn filled mob-only" class:active={filterCount > 0} onclick={() => (showFilters = !showFilters)} aria-label="Filter">
      <Icon name="filter" size={20} />
      {#if filterCount}<span class="dot">{filterCount}</span>{/if}
    </button>
  </header>

  <div class="tools">
  {#if !allTime}
    <PeriodNav {period} onchange={(k) => (periodKey = k)} />
  {/if}

  <div class="search">
    <Icon name="search" size={18} />
    <input bind:value={q} placeholder="Cari deskripsi, catatan, nominal…" type="search" enterkeyhint="search" />
  </div>

  <div class="chips">
    {#each [['', 'Semua'], ['expense', 'Keluar'], ['income', 'Masuk'], ['transfer', 'Transfer']] as [v, l] (v)}
      <button class="chip" class:on={type === v} onclick={() => (type = v)}>{l}</button>
    {/each}
  </div>

  {#if showFilters || ui.desktop}
    <div class="card stack filters fade-in">
      <label class="field">
        <span>Akun</span>
        <select class="input" bind:value={accountId}>
          <option value={0}>Semua akun</option>
          {#each activeAccounts() as a (a.id)}<option value={a.id}>{a.icon} {a.name}</option>{/each}
        </select>
      </label>
      <label class="field">
        <span>Kategori</span>
        <select class="input" bind:value={categoryId}>
          <option value={0}>Semua kategori</option>
          {#each activeCategories() as c (c.id)}<option value={c.id}>{c.icon} {c.name}</option>{/each}
        </select>
      </label>
      <label class="row between">
        <span>Semua periode</span>
        <span class="toggle"><input type="checkbox" bind:checked={allTime} /><span></span></span>
      </label>
    </div>
  {/if}

  </div>

  {#if accountId || categoryId}
    <div class="chips mob-only">
      {#if accountId}<button class="chip on" onclick={() => (accountId = 0)}>{acc(accountId)?.name} ✕</button>{/if}
      {#if categoryId}<button class="chip on" onclick={() => (categoryId = 0)}>{cat(categoryId)?.name} ✕</button>{/if}
    </div>
  {/if}

  <div class="summary row">
    <div class="grow"><div class="tiny muted">Masuk</div><div class="bold num good">{rp(totalIn)}</div></div>
    <div class="grow"><div class="tiny muted">Keluar</div><div class="bold num">{rp(totalOut)}</div></div>
    <div class="grow"><div class="tiny muted">Jumlah</div><div class="bold num">{list.length}</div></div>
  </div>

  <div class:dim={loading}>
    {#if groups.length === 0 && !loading}
      <div class="card empty">
        <div class="big">🗒️</div>
        <p>Belum ada transaksi{q ? ' yang cocok' : ' di periode ini'}.</p>
      </div>
    {/if}
    {#if ui.desktop && rows.length}
      <div class="card flush tablewrap">
        <table class="tx">
          <thead>
            <tr>
              <th class="ck"><input type="checkbox" checked={allPicked} onchange={toggleAll} aria-label="Pilih semua" /></th>
              <th><button onclick={() => sortBy('date')}>Tanggal{arrow('date')}</button></th>
              <th><button onclick={() => sortBy('description')}>Deskripsi{arrow('description')}</button></th>
              <th><button onclick={() => sortBy('category')}>Kategori{arrow('category')}</button></th>
              <th><button onclick={() => sortBy('account')}>Akun{arrow('account')}</button></th>
              <th class="r"><button onclick={() => sortBy('amount')}>Nominal{arrow('amount')}</button></th>
            </tr>
          </thead>
          <tbody>
            {#each rows as t (t.id)}
              {@const c = cat(t.category_id)}
              {@const s = source(t.income_source_id)}
              {@const v = signed(t)}
              <!-- svelte-ignore a11y_click_events_have_key_events -->
              <tr class:sel={picked[t.id]} onclick={() => openTx({ tx: t })} tabindex="0" onkeydown={(e) => e.key === 'Enter' && openTx({ tx: t })}>
                <td class="ck" onclick={(e) => e.stopPropagation()}><input type="checkbox" bind:checked={picked[t.id]} aria-label="Pilih transaksi" /></td>
                <td class="dt num">{shortDate(t.date)}<span class="tiny muted"> {dayLabel(t.date, app.today).split(',')[0]}</span></td>
                <td class="ds">
                  <span class="em">{t.type === 'transfer' ? '🔁' : t.type === 'adjust' ? '⚖️' : (c?.icon ?? (t.type === 'income' ? '💵' : '🧾'))}</span>
                  <span class="grow ellipsis"><b>{t.description || c?.name || 'Transaksi'}</b>{#if t.note}<span class="muted">&nbsp;— {t.note}</span>{/if}</span>
                  {#if t.source && t.source !== 'manual'}<span class="badge">{t.source === 'ai_text' ? 'AI' : t.source === 'ai_image' ? 'scan' : t.source === 'import' ? 'impor' : t.source === 'shortcut' ? 'siri' : t.source}</span>{/if}
                </td>
                <td class="ellipsis">{#if t.type === 'transfer'}<span class="muted">Transfer</span>{:else if t.type === 'adjust'}<span class="muted">Penyesuaian</span>{:else}{c?.name ?? '—'}{#if s}<span class="muted">&nbsp;· {s.name}</span>{/if}{/if}</td>
                <td class="ellipsis">{acc(t.account_id)?.name ?? '?'}{#if t.type === 'transfer'}<span class="muted">&nbsp;→ {anyAccount(t.to_account_id)?.name ?? '?'}</span>{/if}</td>
                <td class="r num am" class:good={v > 0} class:muted={v === 0}>{v > 0 ? '+' : v < 0 ? '−' : ''}{rp(Math.abs(t.amount))}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
    {#each ui.desktop ? [] : groups as g (g.date)}
      <div class="day row between">
        <span class="bold small">{dayLabel(g.date, app.today)}</span>
        <span class="tiny muted num">
          {#if g.in}<span class="good">+{rp(g.in)}</span>{/if}
          {#if g.out}−{rp(g.out)}{/if}
        </span>
      </div>
      <div class="list">
        {#each g.items as t (t.id)}
          <TxItem tx={t} onclick={() => openTx({ tx: t })} />
        {/each}
      </div>
    {/each}
  </div>
</div>

</div>

{#if ui.desktop && pickedList.length}
  <div class="bulk fade-in">
    <b>{pickedList.length} dipilih</b>
    <span class="num small">{#if pickedIn}<span class="good">+{rp(pickedIn)}</span> {/if}{#if pickedOut}−{rp(pickedOut)}{/if}</span>
    <span class="grow"></span>
    <select class="input bsel" onchange={bulkCategory} disabled={bulkBusy} aria-label="Ubah kategori">
      <option value="">Ubah kategori…</option>
      {#each activeCategories() as c (c.id)}<option value={c.id}>{c.icon} {c.name}</option>{/each}
    </select>
    <button class="btn sm danger" onclick={bulkDelete} disabled={bulkBusy}><Icon name="trash" size={16} /> Hapus</button>
    <button class="btn sm" onclick={() => (picked = {})}>Batal</button>
  </div>
{/if}

<button class="fab mob-only" onclick={addNew} aria-label="Tambah transaksi"><Icon name="plus" size={26} /></button>

<style>
  .search {
    display: flex; align-items: center; gap: 8px; padding: 0 14px; border-radius: 12px; background: var(--surface-2); color: var(--muted);
  }
  .search input { border: 0; background: transparent; min-height: 44px; flex: 1; outline: none; font-size: 16px; color: var(--text); }
  .summary { background: var(--surface); border-radius: 14px; padding: 12px 16px; box-shadow: var(--shadow); }
  .day { margin: 16px 4px 6px; }
  .dim { opacity: 0.6; transition: opacity 0.2s; }
  .icon-btn { position: relative; }
  .icon-btn.active { color: var(--accent); }
  .dot { position: absolute; top: 4px; right: 4px; min-width: 16px; height: 16px; border-radius: 8px; background: var(--accent); color: #fff; font-size: 10px; font-weight: 700; display: grid; place-items: center; }
  .filters { gap: 10px; }
  .tools { display: contents; }
  .k { background: rgba(255,255,255,0.2); border-color: transparent; color: #fff; margin-left: 2px; }

  /* ---- desktop: bilah alat sebaris + tabel ---- */
  @container page (min-width: 760px) {
    .tools { display: grid; grid-template-columns: minmax(260px, 320px) minmax(180px, 1fr) auto; gap: 12px; align-items: center; }
    .tools > .filters { grid-column: 1 / -1; flex-direction: row; align-items: end; gap: 12px; padding: 12px 14px; }
    .tools > .filters > .field { flex: 1; }
    .tools > .filters > .row { min-height: 48px; gap: 10px; white-space: nowrap; }
    .summary { padding: 14px 20px; }
    .summary .bold { font-size: 18px; }
  }
  .tablewrap { overflow: hidden; }
  table.tx { width: 100%; border-collapse: collapse; font-size: 14.5px; table-layout: fixed; }
  .tx th { text-align: left; font-size: 12px; font-weight: 700; letter-spacing: 0.04em; text-transform: uppercase; color: var(--muted); padding: 10px 12px; border-bottom: 1px solid var(--line); background: var(--surface); position: sticky; top: 0; z-index: 1; }
  .tx th button { font: inherit; color: inherit; letter-spacing: inherit; text-transform: inherit; }
  .tx th button:hover { color: var(--text); }
  .tx td { padding: 10px 12px; border-bottom: 1px solid var(--line); vertical-align: middle; }
  .tx tbody tr { cursor: pointer; }
  .tx tbody tr:hover { background: var(--surface-2); }
  .tx tbody tr.sel { background: var(--accent-soft); }
  .tx tbody tr:last-child td { border-bottom: 0; }
  .tx tbody tr:focus-visible { outline: 2px solid var(--accent); outline-offset: -2px; }
  .tx .ck { width: 44px; text-align: center; }
  .tx .ck input { width: 17px; height: 17px; accent-color: var(--accent); cursor: pointer; }
  .tx th:nth-child(2) { width: 132px; }
  .tx th:nth-child(4) { width: 21%; }
  .tx th:nth-child(5) { width: 17%; }
  .tx th:nth-child(6) { width: 150px; }
  .tx .r { text-align: right; }
  .tx .dt { white-space: nowrap; }
  .tx .dt .tiny { margin-left: 6px; }
  .tx .ds { display: flex; align-items: center; gap: 10px; min-width: 0; }
  .tx .ds b { font-weight: 600; }
  .tx .em { width: 28px; height: 28px; border-radius: 8px; background: var(--surface-2); display: grid; place-items: center; flex: none; font-size: 15px; }
  .tx .am { font-weight: 650; white-space: nowrap; }
  .bulk {
    position: fixed; left: calc(var(--side-w) + 36px); right: calc(var(--dock-w) + 36px); bottom: 24px; z-index: 32;
    display: flex; align-items: center; gap: 14px; padding: 10px 14px 10px 18px; border-radius: 16px;
    background: var(--surface); border: 1px solid var(--line-strong); box-shadow: 0 12px 40px rgba(0, 0, 0, 0.22);
    max-width: 900px; margin: 0 auto;
  }
  .bsel { width: 200px; min-height: 36px; padding: 6px 32px 6px 10px; font-size: 14px; }
</style>
