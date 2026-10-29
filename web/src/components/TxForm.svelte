<script lang="ts">
  import type { Transaction, Draft } from '../lib/types';
  import { app, ledgerName, activeAccounts, activeCategories, changed, toast, toastError, haptic } from '../lib/state.svelte';
  import { api } from '../lib/api';
  import { addDays } from '../lib/format';
  import AmountField from './AmountField.svelte';
  import Icon from './Icon.svelte';

  type Editable = Partial<Transaction> & Partial<Pick<Draft, 'amount_estimated'>>;

  let {
    initial = {},
    mode = 'api',
    onsaved,
    ondelete,
  }: {
    initial?: Editable;
    /** api = simpan ke server; draft = kembalikan nilai saja (untuk draf AI) */
    mode?: 'api' | 'draft';
    onsaved: (tx: Transaction) => void;
    ondelete?: () => void;
  } = $props();

  const editing = $derived(mode === 'api' && !!initial.id);

  // salinan lokal yang diedit (sengaja hanya nilai awal)
  // svelte-ignore state_referenced_locally
  let f = $state({
    type: (initial.type ?? 'expense') as Transaction['type'],
    amount: initial.amount ? Math.abs(initial.amount) : 0,
    description: initial.description ?? '',
    category_id: initial.category_id ?? null,
    account_id: initial.account_id || app.settings.default_account_id || activeAccounts()[0]?.id || 0,
    to_account_id: initial.to_account_id ?? null,
    debt_id: initial.debt_id ?? null,
    income_source_id: initial.income_source_id ?? null,
    date: initial.date || app.today,
    note: initial.note ?? '',
  });
  let busy = $state(false);
  let showAllCats = $state(false);

  const kind = $derived(f.type === 'income' ? 'income' : 'expense');
  const cats = $derived(activeCategories(kind));
  const visibleCats = $derived(showAllCats || cats.length <= 12 ? cats : withSelected(cats.slice(0, 11)));
  const accounts = $derived(activeAccounts());
  const sources = $derived(app.incomeSources.filter((s) => !s.archived));
  const otherAccounts = $derived(app.otherAccounts.filter((a) => !a.archived));
  const debts = $derived(app.debts.filter((d) => !d.archived));
  const quick = [10000, 20000, 50000, 100000];

  function withSelected(list: typeof cats) {
    if (f.category_id && !list.some((c) => c.id === f.category_id)) {
      const sel = cats.find((c) => c.id === f.category_id);
      if (sel) return [sel, ...list];
    }
    return list;
  }

  function setType(t: Transaction['type']) {
    if (f.type === t) return;
    f.type = t;
    const c = activeCategories(t === 'income' ? 'income' : 'expense');
    if (!c.some((x) => x.id === f.category_id)) f.category_id = null;
    if (t !== 'expense') f.debt_id = null;
    if (t !== 'income') f.income_source_id = null;
    if (t === 'transfer' && !f.to_account_id) {
      f.to_account_id = accounts.find((a) => a.id !== f.account_id && a.type === 'cash')?.id ?? accounts.find((a) => a.id !== f.account_id)?.id ?? null;
    }
  }

  function pickDebt(id: number | null) {
    f.debt_id = id;
    if (id) {
      const cicilan = cats.find((c) => /cicilan|paylater|utang/i.test(c.name));
      if (cicilan && !f.category_id) f.category_id = cicilan.id;
      const d = debts.find((x) => x.id === id);
      if (d && !f.amount && d.monthly) f.amount = d.monthly;
      if (d && !f.description) f.description = 'Bayar ' + d.name;
    }
  }

  async function save() {
    if (!f.amount) return toast('Isi nominal dulu', { kind: 'error' });
    if (f.type === 'transfer' && (!f.to_account_id || f.to_account_id === f.account_id))
      return toast('Pilih akun tujuan yang berbeda', { kind: 'error' });
    const body: Partial<Transaction> = {
      type: f.type,
      amount: f.type === 'adjust' && initial.amount && initial.amount < 0 ? -f.amount : f.amount,
      description: f.description.trim(),
      category_id: f.type === 'transfer' || f.type === 'adjust' ? null : f.category_id,
      account_id: f.account_id,
      to_account_id: f.type === 'transfer' ? f.to_account_id : null,
      debt_id: f.type === 'expense' ? f.debt_id : null,
      income_source_id: f.type === 'income' ? f.income_source_id : null,
      date: f.date,
      note: f.note.trim(),
      source: initial.source,
    };
    if (mode === 'draft') {
      onsaved({ ...(initial as Transaction), ...body } as Transaction);
      return;
    }
    busy = true;
    try {
      const tx = editing
        ? await api.put<Transaction>(`/transactions/${initial.id}`, body)
        : await api.post<Transaction>('/transactions', body);
      haptic();
      await changed();
      onsaved(tx);
    } catch (e) {
      toastError(e);
    } finally {
      busy = false;
    }
  }

  async function remove() {
    if (mode === 'draft') return ondelete?.();
    if (!confirm('Hapus transaksi ini?')) return;
    busy = true;
    try {
      await api.del(`/transactions/${initial.id}`);
      await changed();
      toast('Transaksi dihapus');
      ondelete?.();
    } catch (e) {
      toastError(e);
    } finally {
      busy = false;
    }
  }
</script>

<form class="stack" onsubmit={(e) => { e.preventDefault(); save(); }}>
  {#if f.type !== 'adjust'}
    <div class="seg" role="tablist">
      <button type="button" class:on={f.type === 'expense'} onclick={() => setType('expense')}>Keluar</button>
      <button type="button" class:on={f.type === 'income'} onclick={() => setType('income')}>Masuk</button>
      <button type="button" class:on={f.type === 'transfer'} onclick={() => setType('transfer')}>Transfer</button>
    </div>
  {:else}
    <div class="badge">⚖️ Penyesuaian saldo {initial.amount && initial.amount < 0 ? '(pengurangan)' : '(penambahan)'}</div>
  {/if}

  <div class="amount-box">
    <AmountField bind:value={f.amount} autofocus={!f.amount} highlight={!!initial.amount_estimated} />
    {#if initial.amount_estimated}
      <div class="tiny warn center">Nominal ini perkiraan AI — pastikan sudah benar</div>
    {/if}
    <div class="chips quick">
      {#each quick as q (q)}
        <button type="button" class="chip" onclick={() => (f.amount = q)}>{q / 1000}rb</button>
      {/each}
    </div>
  </div>

  <label class="field">
    <span>Deskripsi</span>
    <input class="input" bind:value={f.description} placeholder={f.type === 'transfer' ? 'mis. Tarik tunai' : 'mis. Makan siang'} enterkeyhint="done" />
  </label>

  {#if f.type === 'expense' || f.type === 'income'}
    <div class="field">
      <span>Kategori</span>
      <div class="cat-grid">
        {#each visibleCats as c (c.id)}
          <button type="button" class="cat" class:on={f.category_id === c.id} onclick={() => (f.category_id = c.id)}>
            <span class="emoji">{c.icon || '📌'}</span>
            <span class="nm">{c.name}</span>
          </button>
        {/each}
        {#if !showAllCats && cats.length > 12}
          <button type="button" class="cat more" onclick={() => (showAllCats = true)}>
            <span class="emoji"><Icon name="more" /></span><span class="nm">Semua</span>
          </button>
        {/if}
      </div>
    </div>
  {/if}

  <div class="field">
    <span>{f.type === 'transfer' ? 'Dari akun' : f.type === 'income' ? 'Masuk ke' : 'Dibayar dari'}</span>
    <div class="chips">
      {#each accounts as a (a.id)}
        <button type="button" class="chip" class:on={f.account_id === a.id} onclick={() => (f.account_id = a.id)}>{a.icon} {a.name}</button>
      {/each}
    </div>
  </div>

  {#if f.type === 'income' && sources.length > 0}
    <div class="field">
      <span>Sumber pemasukan</span>
      <div class="chips">
        <button type="button" class="chip" class:on={!f.income_source_id} onclick={() => (f.income_source_id = null)}>Lainnya</button>
        {#each sources as s (s.id)}
          <button type="button" class="chip" class:on={f.income_source_id === s.id} onclick={() => (f.income_source_id = s.id)}>{s.icon} {s.name}</button>
        {/each}
      </div>
    </div>
  {/if}

  {#if f.type === 'transfer'}
    <div class="field">
      <span>Ke akun</span>
      <div class="chips">
        {#each accounts.filter((a) => a.id !== f.account_id) as a (a.id)}
          <button type="button" class="chip" class:on={f.to_account_id === a.id} onclick={() => (f.to_account_id = a.id)}>{a.icon} {a.name}</button>
        {/each}
        {#each otherAccounts as a (a.id)}
          <button type="button" class="chip" class:on={f.to_account_id === a.id} onclick={() => (f.to_account_id = a.id)}>{a.icon} {a.name} · {ledgerName(a.ledger_id)}</button>
        {/each}
      </div>
    </div>
  {/if}

  {#if f.type === 'expense' && debts.length > 0}
    <div class="field">
      <span>Bayar utang/cicilan? (opsional)</span>
      <div class="chips">
        <button type="button" class="chip" class:on={!f.debt_id} onclick={() => pickDebt(null)}>Bukan</button>
        {#each debts as d (d.id)}
          <button type="button" class="chip" class:on={f.debt_id === d.id} onclick={() => pickDebt(d.id)}>💳 {d.name}</button>
        {/each}
      </div>
    </div>
  {/if}

  <div class="field">
    <span>Tanggal</span>
    <div class="row">
      <input class="input grow" type="date" bind:value={f.date} max={addDays(app.today, 366)} />
      <button type="button" class="chip" class:on={f.date === app.today} onclick={() => (f.date = app.today)}>Hari ini</button>
      <button type="button" class="chip" class:on={f.date === addDays(app.today, -1)} onclick={() => (f.date = addDays(app.today, -1))}>Kemarin</button>
    </div>
  </div>

  <label class="field">
    <span>Catatan (opsional)</span>
    <input class="input" bind:value={f.note} placeholder="Tambahan info" />
  </label>

  {#if initial.raw}
    <details class="raw small muted">
      <summary>Teks asli dari bank</summary>
      <pre>{initial.raw}</pre>
    </details>
  {/if}

  <div class="row actions">
    {#if editing || (mode === 'draft' && ondelete)}
      <button type="button" class="btn danger" onclick={remove} disabled={busy} aria-label="Hapus"><Icon name="trash" size={20} /></button>
    {/if}
    <button type="submit" class="btn primary grow" disabled={busy}>
      {#if busy}<span class="spinner"></span>{/if}
      {mode === 'draft' ? 'Terapkan' : editing ? 'Simpan perubahan' : 'Simpan'}
    </button>
  </div>
</form>

<style>
  .amount-box { background: var(--surface); border-radius: var(--radius); padding: 14px 12px 12px; box-shadow: var(--shadow); }
  .quick { justify-content: center; margin-top: 6px; }
  .quick .chip { min-height: 32px; font-size: 13px; }
  .cat-grid {
    display: grid; grid-template-columns: repeat(4, 1fr); gap: 6px;
  }
  .cat {
    display: flex; flex-direction: column; align-items: center; gap: 4px; padding: 8px 4px; min-height: 68px;
    border-radius: 12px; background: var(--surface); border: 1.5px solid transparent; box-shadow: var(--shadow);
  }
  .cat.on { border-color: var(--accent); background: var(--accent-soft); }
  .emoji { font-size: 22px; line-height: 1; display: grid; place-items: center; height: 24px; }
  .nm { font-size: 11px; line-height: 1.2; text-align: center; color: var(--text-2); display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
  .cat.on .nm { color: var(--accent-ink); font-weight: 600; }
  .actions { margin-top: 4px; }
  .raw pre { white-space: pre-wrap; font-size: 12px; background: var(--surface-2); padding: 8px; border-radius: 8px; margin: 6px 0 0; }
  input[type='date'] { min-width: 0; }
</style>
