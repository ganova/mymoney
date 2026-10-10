<script lang="ts">
  import type { Transaction, PeriodInfo } from '../lib/types';
  import { app, activeAccounts, activeCategories, acc, cat, openTx, toastError } from '../lib/state.svelte';
  import { api, qs } from '../lib/api';
  import { route } from '../lib/router.svelte';
  import { rp, dayLabel } from '../lib/format';
  import Icon from '../components/Icon.svelte';
  import TxItem from '../components/TxItem.svelte';
  import PeriodNav from '../components/PeriodNav.svelte';

  let periodKey = $state(route.query.period ?? app.period?.key ?? '');
  let period = $state<PeriodInfo | null>(null);
  let q = $state('');
  let type = $state('');
  let accountId = $state(Number(route.query.account) || 0);
  let categoryId = $state(Number(route.query.category) || 0);
  let allTime = $state(false);
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

  function addNew() {
    openTx({ preset: { account_id: accountId || undefined, category_id: categoryId || null, type: cat(categoryId)?.kind === 'income' ? 'income' : 'expense' } });
  }
</script>

<div class="page stack">
  <header class="page-head">
    <h1>Transaksi</h1>
    <button class="icon-btn filled" class:active={filterCount > 0} onclick={() => (showFilters = !showFilters)} aria-label="Filter">
      <Icon name="filter" size={20} />
      {#if filterCount}<span class="dot">{filterCount}</span>{/if}
    </button>
  </header>

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

  {#if showFilters}
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

  {#if accountId || categoryId}
    <div class="chips">
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
    {#each groups as g (g.date)}
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

<button class="fab" onclick={addNew} aria-label="Tambah transaksi"><Icon name="plus" size={26} /></button>

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
</style>
