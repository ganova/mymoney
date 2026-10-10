<script lang="ts">
  import type { Draft, Transaction } from '../lib/types';
  import { acc, cat, debt, changed, toast, toastError, haptic } from '../lib/state.svelte';
  import { api } from '../lib/api';
  import { rp, shortDate } from '../lib/format';
  import Sheet from './Sheet.svelte';
  import TxForm from './TxForm.svelte';
  import Icon from './Icon.svelte';

  let {
    drafts = $bindable(),
    autosave = false,
    onsaved,
    compact = false,
  }: { drafts: Draft[]; autosave?: boolean; onsaved?: (txs: Transaction[]) => void; compact?: boolean } = $props();

  // pilih semua kecuali yang kemungkinan duplikat / tanpa nominal
  let selected = $state<boolean[]>(drafts.map((d) => !d.duplicate_of && !d.duplicate_in_scan && d.amount > 0));
  let saved = $state<Transaction[] | null>(null);
  let busy = $state(false);
  let editIndex = $state<number | null>(null);

  const count = $derived(selected.filter(Boolean).length);
  const total = $derived(drafts.reduce((s, d, i) => (selected[i] && d.type === 'expense' ? s + d.amount : s), 0));

  let autoTried = false;
  $effect(() => {
    // simpan otomatis (sekali) bila diizinkan dan semua draf meyakinkan
    if (autosave && !autoTried && drafts.length > 0 && drafts.every((d) => d.amount > 0 && !d.amount_estimated && d.confidence >= 0.75 && !d.duplicate_of)) {
      autoTried = true;
      save();
    }
  });

  async function save() {
    const list = drafts.filter((_, i) => selected[i]);
    if (list.length === 0) return;
    if (list.some((d) => !d.amount)) return toast('Ada draf tanpa nominal — ketuk untuk mengisi', { kind: 'error' });
    busy = true;
    try {
      const txs = await api.post<Transaction[]>('/transactions/bulk', {
        transactions: list.map((d) => ({
          date: d.date, type: d.type, amount: d.amount, account_id: d.account_id, to_account_id: d.to_account_id,
          category_id: d.category_id, debt_id: d.debt_id, description: d.description, note: d.note, source: d.source, raw: d.raw ?? '',
        })),
      });
      saved = txs;
      haptic();
      await changed();
      onsaved?.(txs);
      toast(`${txs.length} transaksi tersimpan`, { action: { label: 'Urungkan', run: undo } });
    } catch (e) {
      toastError(e);
    } finally {
      busy = false;
    }
  }

  async function undo() {
    if (!saved) return;
    try {
      await api.post('/transactions/delete', { ids: saved.map((t) => t.id) });
      saved = null;
      await changed();
      toast('Dibatalkan');
    } catch (e) {
      toastError(e);
    }
  }

  function applyEdit(tx: Transaction) {
    if (editIndex === null) return;
    const d = drafts[editIndex];
    drafts[editIndex] = {
      ...d, type: tx.type as Draft['type'], amount: tx.amount, description: tx.description, category_id: tx.category_id,
      account_id: tx.account_id, to_account_id: tx.to_account_id, debt_id: tx.debt_id, date: tx.date, note: tx.note,
      amount_estimated: false, confidence: 1,
    };
    selected[editIndex] = tx.amount > 0;
    editIndex = null;
  }

  function removeDraft() {
    if (editIndex === null) return;
    drafts.splice(editIndex, 1);
    selected.splice(editIndex, 1);
    editIndex = null;
  }

  function sub(d: Draft): string {
    const parts: string[] = [];
    if (d.type === 'transfer') parts.push(`${acc(d.account_id)?.name ?? '?'} → ${acc(d.to_account_id)?.name ?? '?'}`);
    else {
      const c = cat(d.category_id);
      if (c) parts.push(c.name);
      parts.push(acc(d.account_id)?.name ?? '?');
    }
    const db = debt(d.debt_id);
    if (db) parts.push(db.name);
    parts.push(shortDate(d.date));
    return parts.join(' · ');
  }
</script>

<div class="drafts" class:compact>
  {#each drafts as d, i (i)}
    <div class="draft" class:off={!selected[i] && !saved} class:done={!!saved}>
      {#if !saved}
        <button class="check" class:on={selected[i]} onclick={() => (selected[i] = !selected[i])} aria-label="Pilih">
          {#if selected[i]}<Icon name="check" size={16} stroke={3} />{/if}
        </button>
      {/if}
      <button class="main" onclick={() => !saved && (editIndex = i)} disabled={!!saved}>
        <div class="bubble sm">{d.type === 'transfer' ? '🔁' : (cat(d.category_id)?.icon ?? '🧾')}</div>
        <div class="grow">
          <div class="ellipsis bold desc">{d.description || 'Tanpa deskripsi'}</div>
          <div class="ellipsis tiny muted">{sub(d)}</div>
          {#if !saved && (d.amount_estimated || d.duplicate_of || d.duplicate_in_scan || d.confidence < 0.6)}
            <div class="flags">
              {#if d.amount_estimated}<span class="badge warn">{d.amount ? 'nominal perkiraan' : 'isi nominal'}</span>{/if}
              {#if d.duplicate_of || d.duplicate_in_scan}<span class="badge bad">mungkin sudah tercatat</span>{/if}
              {#if d.confidence < 0.6 && !d.amount_estimated}<span class="badge">cek kategori</span>{/if}
            </div>
          {/if}
        </div>
        <div class="amt num" class:inc={d.type === 'income'}>
          {d.type === 'income' ? '+' : d.type === 'expense' ? '−' : ''}{d.amount ? rp(d.amount) : 'Rp?'}
        </div>
      </button>
    </div>
  {/each}

  {#if saved}
    <div class="saved row between">
      <span class="good bold row"><Icon name="check" size={18} stroke={3} /> {saved.length} tersimpan</span>
      <button class="btn sm ghost" onclick={undo}><Icon name="undo" size={16} /> Urungkan</button>
    </div>
  {:else if drafts.length > 0}
    <button class="btn primary block" onclick={save} disabled={busy || count === 0}>
      {#if busy}<span class="spinner"></span>{/if}
      Simpan {count} transaksi{total ? ` · ${rp(total)}` : ''}
    </button>
    <div class="tiny muted center">Ketuk transaksi untuk mengubah kategori, akun, atau nominal</div>
  {/if}
</div>

<Sheet open={editIndex !== null} onclose={() => (editIndex = null)} title="Ubah draf">
  {#if editIndex !== null}
    {#key editIndex}
      <TxForm initial={drafts[editIndex]} mode="draft" onsaved={applyEdit} ondelete={removeDraft} />
    {/key}
  {/if}
</Sheet>

<style>
  .drafts { display: flex; flex-direction: column; gap: 8px; }
  .draft {
    display: flex; align-items: center; gap: 8px; background: var(--surface); border-radius: 14px;
    padding: 8px 10px 8px 8px; box-shadow: var(--shadow); transition: opacity 0.2s;
  }
  .draft.off { opacity: 0.5; }
  .check {
    width: 26px; height: 26px; border-radius: 13px; border: 2px solid var(--line-strong); flex: none;
    display: grid; place-items: center; color: #fff;
  }
  .check.on { background: var(--accent); border-color: var(--accent); }
  .main { display: flex; align-items: center; gap: 10px; flex: 1; min-width: 0; text-align: left; padding: 2px 0; }
  .desc { font-size: 15px; }
  .flags { display: flex; gap: 4px; flex-wrap: wrap; margin-top: 4px; }
  .amt { font-weight: 700; white-space: nowrap; font-size: 15px; }
  .inc { color: var(--good); }
  .saved { padding: 6px 4px; }
  .done { box-shadow: none; background: var(--surface-2); }
</style>
