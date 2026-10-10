<script lang="ts">
  import type { Dashboard, Budget, BudgetLine, Category } from '../lib/types';
  import { app, cat, activeCategories, changed, toast, toastError } from '../lib/state.svelte';
  import { api, qs } from '../lib/api';
  import { route, go } from '../lib/router.svelte';
  import { rp, rpc, pct } from '../lib/format';
  import Icon from '../components/Icon.svelte';
  import PeriodNav from '../components/PeriodNav.svelte';
  import Sheet from '../components/Sheet.svelte';
  import AmountField from '../components/AmountField.svelte';

  let periodKey = $state(route.query.period ?? app.period?.key ?? '');
  let d = $state<Dashboard | null>(null);
  let loading = $state(true);

  // editor baris
  let edit = $state<null | { id?: number; category_id: number; amount: number; due_day: number; note: string; isBill: boolean }>(null);
  let saving = $state(false);

  // AI
  let aiOpen = $state(false);
  let aiText = $state('');
  let aiFile = $state<File | null>(null);
  let aiBusy = $state(false);
  let aiLines = $state<(BudgetLine & { on: boolean })[]>([]);
  let aiSummary = $state('');
  let aiReplace = $state(true);

  $effect(() => {
    app.dataVersion;
    const key = periodKey;
    const ctrl = new AbortController();
    loading = true;
    api
      .get<Dashboard>('/dashboard' + qs({ period: key }), ctrl.signal)
      .then((r) => (d = r))
      .catch((e) => e.name !== 'AbortError' && toastError(e))
      .finally(() => (loading = false));
    return () => ctrl.abort();
  });

  const expenseLines = $derived((d?.budgets ?? []).filter((b) => b.kind === 'expense'));
  const incomeLines = $derived((d?.budgets ?? []).filter((b) => b.kind === 'income'));
  const groups = $derived.by(() => {
    const m = new Map<string, Budget[]>();
    for (const b of expenseLines) {
      const g = cat(b.category_id)?.group || 'Lain-lain';
      if (!m.has(g)) m.set(g, []);
      m.get(g)!.push(b);
    }
    return [...m.entries()].map(([name, items]) => ({
      name, items,
      plan: items.reduce((s, b) => s + b.amount, 0),
      actual: items.reduce((s, b) => s + b.actual, 0),
    }));
  });
  const unbudgeted = $derived.by(() => {
    if (!d) return [];
    const set = new Set(d.budgets.map((b) => b.category_id));
    return d.by_category.filter((c) => c.kind === 'expense' && (c.category_id === null || !set.has(c.category_id)));
  });
  const actualIncome = $derived(d?.income ?? 0);
  const sisaBudget = $derived((d?.plan_expense ?? 0) - (d?.budget_spent ?? 0));
  const freeCats = $derived(activeCategories().filter((c) => !(d?.budgets ?? []).some((b) => b.category_id === c.id)));

  function openEdit(b?: Budget, categoryId?: number, amount?: number) {
    if (b) edit = { id: b.id, category_id: b.category_id, amount: b.amount, due_day: b.due_day, note: b.note, isBill: b.due_day > 0 };
    else edit = { category_id: categoryId ?? 0, amount: amount ?? 0, due_day: 0, note: '', isBill: false };
  }

  async function saveLine() {
    if (!edit || !d) return;
    if (!edit.category_id) return toast('Pilih kategori dulu', { kind: 'error' });
    saving = true;
    try {
      await api.put('/budgets', {
        period: d.period.key, category_id: edit.category_id, amount: edit.amount,
        due_day: edit.isBill ? edit.due_day || app.settings.period_start_day : 0, note: edit.note,
      });
      edit = null;
      await changed();
      toast('Budget disimpan');
    } catch (e) {
      toastError(e);
    } finally {
      saving = false;
    }
  }

  async function deleteLine() {
    if (!edit?.id) return;
    if (!confirm('Hapus baris budget ini?')) return;
    try {
      await api.del(`/budgets/${edit.id}`);
      edit = null;
      await changed();
    } catch (e) {
      toastError(e);
    }
  }

  async function copyPrev() {
    if (!d) return;
    const overwrite = d.budgets.length > 0 && confirm('Timpa budget periode ini dengan budget periode sebelumnya?');
    try {
      const r = await api.post<{ copied: number }>('/budgets/copy', { from: d.period.prev, to: d.period.key, overwrite });
      await changed();
      toast(r.copied ? `${r.copied} baris disalin` : 'Tidak ada yang disalin (periode lalu kosong?)');
    } catch (e) {
      toastError(e);
    }
  }

  async function askAI() {
    if (!aiText.trim() && !aiFile) return toast('Tempel daftar budget atau pilih file', { kind: 'error' });
    aiBusy = true;
    try {
      const fd = new FormData();
      fd.append('text', aiText);
      if (aiFile) fd.append('file', aiFile);
      const r = await api.post<{ lines: BudgetLine[]; summary: string }>('/ai/budget', fd);
      aiLines = r.lines.map((l) => ({ ...l, on: l.amount > 0 }));
      aiSummary = r.summary;
    } catch (e) {
      toastError(e);
    } finally {
      aiBusy = false;
    }
  }

  async function applyAI() {
    if (!d) return;
    const lines = aiLines.filter((l) => l.on);
    if (!lines.length) return;
    aiBusy = true;
    try {
      const r = await api.post<{ saved: number }>('/budgets/apply', { period: d.period.key, replace: aiReplace, lines });
      aiOpen = false;
      aiLines = [];
      aiText = '';
      aiFile = null;
      await changed();
      toast(`${r.saved} baris budget tersimpan`);
    } catch (e) {
      toastError(e);
    } finally {
      aiBusy = false;
    }
  }

  function lineName(l: BudgetLine): string {
    if (l.category_id) return cat(l.category_id)?.name ?? l.label;
    return l.new_category?.name ?? l.label;
  }
  function lineIcon(l: BudgetLine): string {
    if (l.category_id) return cat(l.category_id)?.icon ?? '📌';
    return l.new_category?.icon ?? '📌';
  }
  function groupCats(list: Category[]) {
    const m = new Map<string, Category[]>();
    for (const c of list) {
      if (!m.has(c.group)) m.set(c.group, []);
      m.get(c.group)!.push(c);
    }
    return [...m.entries()];
  }
</script>

<div class="page stack">
  <header class="page-head">
    <h1>Budget</h1>
    <button class="btn sm" onclick={() => (aiOpen = true)}><Icon name="sparkles" size={16} /> Susun dgn AI</button>
  </header>

  <PeriodNav period={d?.period ?? null} onchange={(k) => (periodKey = k)} />

  {#if d}
    <section class="card summary" class:dim={loading}>
      <div class="grid3">
        <div><div class="tiny muted">Rencana masuk</div><div class="bold num">{rpc(d.plan_income)}</div></div>
        <div><div class="tiny muted">Rencana keluar</div><div class="bold num">{rpc(d.plan_expense)}</div></div>
        <div><div class="tiny muted">Sisa rencana</div><div class="bold num" class:bad={d.plan_income - d.plan_expense < 0}>{rpc(d.plan_income - d.plan_expense)}</div></div>
      </div>
      <hr class="divider" />
      <div class="grid3">
        <div><div class="tiny muted">Masuk aktual</div><div class="bold num good">{rpc(actualIncome)}</div></div>
        <div><div class="tiny muted">Keluar aktual</div><div class="bold num">{rpc(d.expense)}</div></div>
        <div><div class="tiny muted">Sisa budget</div><div class="bold num" class:bad={sisaBudget < 0}>{rpc(sisaBudget)}</div></div>
      </div>
      {#if d.plan_expense}
        <div class="meter big" class:over={d.budget_spent > d.plan_expense}><i style:width="{Math.min(pct(d.budget_spent, d.plan_expense), 100)}%"></i></div>
        <div class="tiny muted row between">
          <span>Terpakai {pct(d.budget_spent, d.plan_expense)}% dari budget</span>
          {#if d.unbudgeted}<span>Di luar budget {rp(d.unbudgeted)}</span>{/if}
        </div>
      {/if}
    </section>

    {#if d.budgets.length === 0 && !loading}
      <div class="card empty">
        <div class="big">🎯</div>
        <p><strong>Belum ada budget untuk {d.period.label}.</strong></p>
        <p class="small">Atur sekali, nanti otomatis terbawa ke periode berikutnya. Paling cepat: tempel daftar budget dari Google Sheet ke AI.</p>
        <div class="stack" style="margin-top:14px">
          <button class="btn primary" onclick={() => (aiOpen = true)}><Icon name="sparkles" size={18} /> Susun dengan AI</button>
          <button class="btn" onclick={copyPrev}>Salin dari periode lalu</button>
          <button class="btn ghost" onclick={() => openEdit()}>+ Tambah manual</button>
        </div>
      </div>
    {/if}

    {#if incomeLines.length}
      <div class="section-label">Pemasukan</div>
      <div class="list">
        {#each incomeLines as b (b.id)}
          {@const c = cat(b.category_id)}
          <button class="list-item" onclick={() => openEdit(b)}>
            <div class="bubble">{c?.icon}</div>
            <div class="grow">
              <div class="row between"><span class="ellipsis">{c?.name}</span><span class="num small"><strong>{rp(b.actual)}</strong> / {rp(b.amount)}</span></div>
              <div class="meter done" style="margin-top:6px"><i style:width="{Math.min(pct(b.actual, b.amount), 100)}%"></i></div>
            </div>
          </button>
        {/each}
      </div>
    {/if}

    {#each groups as g (g.name)}
      <div class="section-label row between"><span>{g.name}</span><span class="num">{rp(g.actual)} / {rp(g.plan)}</span></div>
      <div class="list">
        {#each g.items as b (b.id)}
          {@const c = cat(b.category_id)}
          {@const p = pct(b.actual, b.amount)}
          {@const over = b.actual > b.amount}
          {@const paid = b.due_day > 0 && b.actual >= b.amount && b.amount > 0}
          <button class="list-item" onclick={() => openEdit(b)}>
            <div class="bubble">{c?.icon}</div>
            <div class="grow">
              <div class="row name">
                <span class="ellipsis">{c?.name}</span>
                {#if b.due_day}<span class="badge {paid ? 'good' : ''}">{paid ? '✓ lunas' : `tgl ${b.due_day}`}</span>{/if}
              </div>
              <div class="meter" class:over class:done={paid} style="margin-top:6px"><i style:width="{Math.min(p, 100)}%"></i></div>
              <div class="small row between" style="margin-top:4px">
                <span class="num"><strong>{rp(b.actual)}</strong><span class="muted"> / {rp(b.amount)}</span></span>
                <span class="tiny" class:bad={over} class:muted={!over}>{over ? `Lewat ${rp(b.actual - b.amount)}` : `Sisa ${rp(b.amount - b.actual)}`}</span>
              </div>
              {#if b.note}<div class="tiny muted ellipsis">{b.note}</div>{/if}
            </div>
          </button>
        {/each}
      </div>
    {/each}

    {#if unbudgeted.length}
      <div class="section-label">Di luar budget</div>
      <div class="list">
        {#each unbudgeted as u (u.category_id ?? 0)}
          {@const c = cat(u.category_id)}
          <div class="list-item">
            <div class="bubble">{c?.icon ?? '❔'}</div>
            <button class="grow left" onclick={() => go('/tx', { period: d?.period.key ?? '', category: u.category_id ?? undefined })}>
              <div class="ellipsis">{c?.name ?? 'Tanpa kategori'}</div>
              <div class="tiny muted">{u.count} transaksi · {rp(u.total)}</div>
            </button>
            {#if c}
              <button class="btn sm" onclick={() => openEdit(undefined, c.id, Math.ceil(u.total / 50000) * 50000)}>+ Budget</button>
            {/if}
          </div>
        {/each}
      </div>
    {/if}

    {#if d.budgets.length}
      <div class="row actions">
        <button class="btn grow" onclick={() => openEdit()}><Icon name="plus" size={18} /> Tambah baris</button>
        <button class="btn grow" onclick={copyPrev}><Icon name="copy" size={18} /> Salin periode lalu</button>
      </div>
      <p class="tiny muted center">Budget otomatis disalin ke periode baru setiap tanggal {app.settings.period_start_day}. Baris dengan tanggal jatuh tempo muncul sebagai tagihan di Beranda.</p>
    {/if}
  {:else}
    <div class="card skeleton" style="height:160px"></div>
  {/if}
</div>

<!-- Editor baris -->
<Sheet open={!!edit} onclose={() => (edit = null)} title={edit?.id ? 'Ubah budget' : 'Tambah budget'}>
  {#if edit}
    <div class="stack">
      {#if !edit.id}
        <label class="field">
          <span>Kategori</span>
          <select class="input" bind:value={edit.category_id}>
            <option value={0}>Pilih kategori…</option>
            {#each groupCats(freeCats) as [g, list] (g)}
              <optgroup label={g}>
                {#each list as c (c.id)}<option value={c.id}>{c.icon} {c.name}</option>{/each}
              </optgroup>
            {/each}
          </select>
        </label>
      {:else}
        <div class="row"><div class="bubble">{cat(edit.category_id)?.icon}</div><div class="bold">{cat(edit.category_id)?.name}</div></div>
      {/if}
      <div class="card"><AmountField bind:value={edit.amount} autofocus={!edit.amount} /></div>
      {#if cat(edit.category_id)?.kind !== 'income'}
        <label class="row between card tight">
          <span><strong>Tagihan tetap</strong><br /><span class="small muted">Punya tanggal bayar, mis. cicilan rumah tgl 28</span></span>
          <span class="toggle"><input type="checkbox" bind:checked={edit.isBill} /><span></span></span>
        </label>
        {#if edit.isBill}
          <label class="field">
            <span>Tanggal jatuh tempo</span>
            <select class="input" bind:value={edit.due_day}>
              <option value={0}>Pilih tanggal…</option>
              {#each Array.from({ length: 31 }, (_, i) => i + 1) as n (n)}<option value={n}>Tanggal {n}</option>{/each}
            </select>
          </label>
        {/if}
      {/if}
      <label class="field">
        <span>Catatan</span>
        <input class="input" bind:value={edit.note} placeholder="mis. 4 minggu × 500rb" />
      </label>
      <div class="row">
        {#if edit.id}<button class="btn danger" onclick={deleteLine} aria-label="Hapus"><Icon name="trash" size={20} /></button>{/if}
        <button class="btn primary grow" onclick={saveLine} disabled={saving}>{#if saving}<span class="spinner"></span>{/if} Simpan</button>
      </div>
    </div>
  {/if}
</Sheet>

<!-- Susun budget dengan AI -->
<Sheet open={aiOpen} onclose={() => (aiOpen = false)} title="Susun budget dengan AI" full>
  {#if !app.ai.enabled}
    <div class="card empty"><p>Fitur ini butuh AI. Isi <code>ANTHROPIC_API_KEY</code> di file .env lalu restart.</p></div>
  {:else if aiLines.length === 0}
    <div class="stack">
      <p class="small text-2">Tempel daftar rencana anggaranmu (copy dari Google Sheet, catatan, atau ketik bebas). AI akan memetakan ke kategori, mengenali tagihan bertanggal, dan membuat kategori baru bila perlu.</p>
      <textarea class="input" rows="9" bind:value={aiText} placeholder={'Gaji 11.217.866\nBayar Rumah 4.638.000 tgl 28\nJaga anak 1.300.000 tgl 28\nTK 550rb\nListrik + pulsa 500rb\nInternet 189rb tgl 28\nUang sehari-hari 500rb/minggu\n...'}></textarea>
      <label class="field">
        <span>…atau pilih file Excel (.xlsx) dari Google Sheet</span>
        <input class="input" type="file" accept=".xlsx,.csv,.txt" onchange={(e) => (aiFile = (e.currentTarget as HTMLInputElement).files?.[0] ?? null)} />
      </label>
      <button class="btn primary block" onclick={askAI} disabled={aiBusy}>
        {#if aiBusy}<span class="spinner"></span> AI sedang menyusun…{:else}<Icon name="sparkles" size={18} /> Susun budget{/if}
      </button>
    </div>
  {:else}
    <div class="stack">
      {#if aiSummary}<div class="card small">{aiSummary}</div>{/if}
      <div class="list">
        {#each aiLines as l, i (i)}
          <label class="list-item">
            <input type="checkbox" bind:checked={l.on} class="cb" />
            <div class="bubble sm">{lineIcon(l)}</div>
            <div class="grow">
              <div class="ellipsis">{lineName(l)} {#if !l.category_id}<span class="badge accent">baru</span>{/if}</div>
              <div class="tiny muted ellipsis">{l.label}{l.due_day ? ` · tgl ${l.due_day}` : ''}{l.note ? ` · ${l.note}` : ''}</div>
            </div>
            <div class="num bold small">{rp(l.amount)}</div>
          </label>
        {/each}
      </div>
      <label class="row between card tight">
        <span class="small">Ganti seluruh budget {d?.period.label}</span>
        <span class="toggle"><input type="checkbox" bind:checked={aiReplace} /><span></span></span>
      </label>
      <div class="row">
        <button class="btn" onclick={() => (aiLines = [])}>Ulangi</button>
        <button class="btn primary grow" onclick={applyAI} disabled={aiBusy}>
          {#if aiBusy}<span class="spinner"></span>{/if} Terapkan {aiLines.filter((l) => l.on).length} baris
        </button>
      </div>
    </div>
  {/if}
</Sheet>

<style>
  .summary { display: flex; flex-direction: column; gap: 8px; transition: opacity 0.2s; }
  .grid3 { display: grid; grid-template-columns: repeat(3, 1fr); gap: 8px; }
  .grid3 .bold { font-size: 15px; white-space: nowrap; letter-spacing: -0.01em; }
  .divider { margin: 4px 0; }
  .meter.big { height: 10px; border-radius: 5px; margin-top: 6px; }
  .dim { opacity: 0.6; }
  .name { gap: 6px; }
  .actions { margin-top: 8px; }
  .left { text-align: left; }
  .tight { padding: 12px 14px; }
  .cb { width: 22px; height: 22px; accent-color: var(--accent); flex: none; }
</style>
