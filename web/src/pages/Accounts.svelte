<script lang="ts">
  import type { Account } from '../lib/types';
  import { app, changed, toast, toastError } from '../lib/state.svelte';
  import { api } from '../lib/api';
  import { back, go } from '../lib/router.svelte';
  import { rp } from '../lib/format';
  import Icon from '../components/Icon.svelte';
  import Sheet from '../components/Sheet.svelte';
  import AmountField from '../components/AmountField.svelte';

  const types: [Account['type'], string, string][] = [
    ['bank', 'Bank', '🏦'], ['cash', 'Tunai', '💵'], ['ewallet', 'E-wallet', '📱'],
    ['credit', 'Kartu kredit/Paylater', '💳'], ['saving', 'Tabungan', '🐷'], ['other', 'Lainnya', '👛'],
  ];
  const typeName = Object.fromEntries(types.map(([t, n]) => [t, n]));

  let edit = $state<Partial<Account> | null>(null);
  let negative = $state(false);
  let rec = $state<{ acc: Account; balance: number } | null>(null);
  let busy = $state(false);

  const active = $derived(app.accounts.filter((a) => !a.archived));
  const archived = $derived(app.accounts.filter((a) => a.archived));
  const total = $derived(active.filter((a) => a.type !== 'credit').reduce((s, a) => s + a.balance, 0));

  function open(a?: Account) {
    edit = a ? { ...a, opening_balance: Math.abs(a.opening_balance) } : { name: '', type: 'bank', icon: '🏦', opening_balance: 0, aliases: '', archived: false };
    negative = (a?.opening_balance ?? 0) < 0;
  }

  async function save() {
    if (!edit) return;
    busy = true;
    try {
      const body = { ...edit, opening_balance: negative ? -(edit.opening_balance ?? 0) : edit.opening_balance };
      if (edit.id) await api.put(`/accounts/${edit.id}`, body);
      else await api.post('/accounts', body);
      edit = null;
      await changed();
      toast('Akun disimpan');
    } catch (e) {
      toastError(e);
    } finally {
      busy = false;
    }
  }

  async function remove() {
    if (!edit?.id || !confirm(`Hapus akun ${edit.name}?`)) return;
    try {
      await api.del(`/accounts/${edit.id}`);
      edit = null;
      await changed();
    } catch (e) {
      toastError(e);
    }
  }

  async function reconcile() {
    if (!rec) return;
    busy = true;
    try {
      const r = await api.post<{ transaction: unknown }>(`/accounts/${rec.acc.id}/reconcile`, { balance: rec.balance });
      toast(r.transaction ? 'Saldo disesuaikan' : 'Saldo sudah sama');
      rec = null;
      await changed();
    } catch (e) {
      toastError(e);
    } finally {
      busy = false;
    }
  }
</script>

<div class="page stack">
  <header class="page-head">
    <button class="back" onclick={() => back('/more')}><Icon name="chevron-left" /> Lainnya</button>
    <button class="btn sm primary" onclick={() => open()}><Icon name="plus" size={18} /> Akun</button>
  </header>
  <h1 class="title">Akun & Saldo</h1>
  <div class="card total"><div class="small">Total saldo (tanpa kartu kredit)</div><div class="big num">{rp(total)}</div></div>

  <div class="list">
    {#each active as a (a.id)}
      <div class="list-item">
        <button class="bubble" onclick={() => open(a)}>{a.icon}</button>
        <button class="grow left" onclick={() => open(a)}>
          <div class="bold">{a.name}</div>
          <div class="small muted">{typeName[a.type]} · {a.tx_count} transaksi</div>
        </button>
        <div class="right">
          <div class="bold num" class:bad={a.balance < 0}>{rp(a.balance)}</div>
          <button class="link tiny" onclick={() => (rec = { acc: a, balance: a.balance })}>Sesuaikan</button>
        </div>
      </div>
    {/each}
  </div>
  <p class="tiny muted">Saldo dihitung dari saldo awal + semua transaksi. Kalau beda dengan aplikasi bank, ketuk <strong>Sesuaikan</strong> lalu isi saldo sebenarnya.</p>

  {#if archived.length}
    <div class="section-label">Diarsipkan</div>
    <div class="list">
      {#each archived as a (a.id)}
        <button class="list-item" onclick={() => open(a)}>
          <div class="bubble">{a.icon}</div>
          <div class="grow muted">{a.name}</div>
          <div class="num muted">{rp(a.balance)}</div>
        </button>
      {/each}
    </div>
  {/if}
  <button class="btn ghost" onclick={() => go('/tx')}>Lihat semua transaksi →</button>
</div>

<Sheet open={!!edit} onclose={() => (edit = null)} title={edit?.id ? 'Ubah akun' : 'Akun baru'}>
  {#if edit}
    <div class="stack">
      <div class="row">
        <input class="input emoji" bind:value={edit.icon} maxlength="4" aria-label="Ikon" />
        <input class="input grow" bind:value={edit.name} placeholder="Nama akun, mis. BCA" />
      </div>
      <div class="field">
        <span>Jenis</span>
        <div class="chips wrap">
          {#each types as [t, n, ic] (t)}
            <button class="chip" class:on={edit.type === t} onclick={() => { if (edit) { edit.type = t; if (!edit.id) edit.icon = ic; } }}>{ic} {n}</button>
          {/each}
        </div>
      </div>
      <div class="field">
        <span>Saldo awal (saat mulai mencatat)</span>
        <div class="card"><AmountField bind:value={edit.opening_balance} /></div>
        <label class="row small"><input type="checkbox" bind:checked={negative} /> Saldo awal minus (utang kartu kredit)</label>
      </div>
      <label class="field">
        <span>Kata kunci untuk AI (pisahkan koma)</span>
        <input class="input" bind:value={edit.aliases} placeholder="bca, debit, qris, mybca" />
      </label>
      {#if edit.id}
        <label class="row between card tight">
          <span>Arsipkan (sembunyikan)</span>
          <span class="toggle"><input type="checkbox" bind:checked={edit.archived} /><span></span></span>
        </label>
      {/if}
      <div class="row">
        {#if edit.id}<button class="btn danger" onclick={remove} aria-label="Hapus"><Icon name="trash" size={20} /></button>{/if}
        <button class="btn primary grow" onclick={save} disabled={busy || !edit.name}>Simpan</button>
      </div>
    </div>
  {/if}
</Sheet>

<Sheet open={!!rec} onclose={() => (rec = null)} title="Sesuaikan saldo">
  {#if rec}
    <div class="stack">
      <p class="small text-2">Saldo {rec.acc.name} di aplikasi saat ini <strong>{rp(rec.acc.balance)}</strong>. Isi saldo sebenarnya (lihat di m-banking / dompet). Selisihnya dicatat sebagai penyesuaian dan tidak masuk laporan pengeluaran.</p>
      <div class="card"><AmountField bind:value={rec.balance} autofocus /></div>
      {#if rec.balance !== rec.acc.balance}
        <div class="small center">Selisih: <strong class:bad={rec.balance < rec.acc.balance} class:good={rec.balance > rec.acc.balance}>{rp(rec.balance - rec.acc.balance)}</strong></div>
      {/if}
      <button class="btn primary block" onclick={reconcile} disabled={busy}>Simpan saldo</button>
    </div>
  {/if}
</Sheet>

<style>
  .title { font-size: 28px; font-weight: 750; margin-top: -8px; }
  .total { background: var(--accent); color: #fff; }
  .total .big { font-size: 30px; font-weight: 750; }
  .left { text-align: left; }
  .right { text-align: right; display: flex; flex-direction: column; align-items: flex-end; }
  .emoji { width: 60px; text-align: center; font-size: 22px; flex: none; }
  .tight { padding: 12px 14px; }
</style>
