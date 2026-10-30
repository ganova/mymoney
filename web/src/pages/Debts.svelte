<script lang="ts">
  import type { Debt, Outlook } from '../lib/types';
  import { app, activeCategories, changed, openTx, toast, toastError } from '../lib/state.svelte';
  import { api } from '../lib/api';
  import { back, go } from '../lib/router.svelte';
  import { rp, rpc, pct, shortDate } from '../lib/format';
  import Icon from '../components/Icon.svelte';
  import Sheet from '../components/Sheet.svelte';
  import AmountField from '../components/AmountField.svelte';

  let edit = $state<Partial<Debt> | null>(null);
  let busy = $state(false);
  let outlook = $state<Outlook | null>(null);

  $effect(() => {
    app.dataVersion;
    app.debts;
    api.get<Outlook>('/outlook?months=24').then((o) => (outlook = o)).catch(() => {});
  });
  const dueMonths = $derived((outlook?.months ?? []).filter((m) => m.debt_due > 0 || m.debt_paid > 0));
  const dueTotal = $derived(dueMonths.reduce((s, m) => s + m.debt_due, 0));

  const active = $derived(app.debts.filter((d) => !d.archived));
  const done = $derived(app.debts.filter((d) => d.archived));
  const totalRemaining = $derived(active.reduce((s, d) => s + d.remaining, 0));
  const totalMonthly = $derived(active.reduce((s, d) => s + d.monthly, 0));

  function open(d?: Debt) {
    edit = d ? { ...d } : { name: '', total: 0, monthly: 0, due_day: 0, note: '', archived: false };
  }

  async function save() {
    if (!edit) return;
    busy = true;
    try {
      if (edit.id) await api.put(`/debts/${edit.id}`, edit);
      else await api.post('/debts', edit);
      edit = null;
      await changed();
      toast('Tersimpan');
    } catch (e) {
      toastError(e);
    } finally {
      busy = false;
    }
  }

  async function remove() {
    if (!edit?.id || !confirm(`Hapus ${edit.name}? Riwayat pembayarannya tetap ada di transaksi.`)) return;
    try {
      await api.del(`/debts/${edit.id}`);
      edit = null;
      await changed();
    } catch (e) {
      toastError(e);
    }
  }

  function pay(d: Debt) {
    const c = activeCategories('expense').find((x) => /cicilan|paylater|utang/i.test(x.name));
    openTx({ preset: { type: 'expense', debt_id: d.id, amount: Math.min(d.monthly || d.remaining, d.remaining), category_id: c?.id ?? null, description: 'Bayar ' + d.name } });
  }
</script>

<div class="page stack">
  <header class="page-head">
    <button class="back" onclick={() => back('/more')}><Icon name="chevron-left" /> Lainnya</button>
    <button class="btn sm primary" onclick={() => open()}><Icon name="plus" size={18} /> Utang</button>
  </header>
  <h1 class="title">Utang & Cicilan</h1>

  <div class="card row sum">
    <div class="grow"><div class="tiny muted">Total sisa</div><div class="big num">{rp(totalRemaining)}</div></div>
    <div class="grow"><div class="tiny muted">Cicilan / bulan</div><div class="big num">{rp(totalMonthly)}</div></div>
  </div>

  {#if outlook && active.length > 0}
    <section class="card">
      <div class="card-title">Proyeksi cicilan per periode</div>
      <div class="stack fc">
        {#each dueMonths as m (m.key)}
          <div class="row between">
            <span>{m.label}</span>
            <span class="num bold">{m.debt_due ? rpc(m.debt_due) : '✓ lunas periode ini'}{m.debt_paid && m.debt_due ? ` (+${rpc(m.debt_paid)} dibayar)` : ''}</span>
          </div>
        {/each}
        {#if dueMonths.length === 0}<div class="small muted">Tidak ada cicilan bulanan terjadwal.</div>{/if}
      </div>
      <div class="tiny muted fc-note">
        Total cicilan terjadwal {rp(dueTotal)}{outlook.debt_free_label ? ` · lunas ${outlook.debt_free_label}` : ''}{outlook.debt_unscheduled ? ` · ${rp(outlook.debt_unscheduled)} tanpa cicilan bulanan (isi "cicilan per bulan" agar masuk jadwal)` : ''}
      </div>
      {#if outlook.debts.some((d) => d.months_left > 0)}
        <div class="stack fc-debts">
          {#each outlook.debts.filter((d) => d.months_left > 0) as d (d.id)}
            <div class="row between tiny"><span>{d.name}</span><span class="muted">{d.months_left} periode lagi · lunas {d.last_label}</span></div>
          {/each}
        </div>
      {/if}
    </section>
  {/if}

  {#if active.length === 0}
    <div class="card empty">
      <div class="big">🎉</div>
      <p>Tidak ada utang aktif. Tambahkan paylater/pinjaman (Kredivo, SPayLater, Indodana, dll.) untuk memantau sisa tagihannya.</p>
    </div>
  {/if}

  {#each active as d (d.id)}
    <div class="card debt">
      <button class="row between head" onclick={() => open(d)}>
        <div class="grow left">
          <div class="bold">💳 {d.name}</div>
          <div class="tiny muted">
            {d.due_day ? `Jatuh tempo tgl ${d.due_day}` : 'Tanpa tanggal'}{d.monthly ? ` · ${rp(d.monthly)}/bln` : ''}{d.last_paid ? ` · terakhir bayar ${shortDate(d.last_paid)}` : ''}
          </div>
        </div>
        <div class="right">
          <div class="bold num">{rp(d.remaining)}</div>
          <div class="tiny muted">dari {rp(d.total)}</div>
        </div>
      </button>
      <div class="meter done"><i style:width="{Math.min(pct(d.paid, d.total), 100)}%"></i></div>
      <div class="row between">
        <span class="tiny muted">Terbayar {pct(d.paid, d.total)}% · {rp(d.paid)}</span>
        <div class="row" style="gap:6px">
          <button class="btn sm" onclick={() => go('/tx', { debt: d.id })}>Riwayat</button>
          <button class="btn sm primary" onclick={() => pay(d)} disabled={d.remaining <= 0}>Bayar</button>
        </div>
      </div>
    </div>
  {/each}

  {#if done.length}
    <div class="section-label">Lunas / diarsipkan</div>
    <div class="list">
      {#each done as d (d.id)}
        <button class="list-item" onclick={() => open(d)}><div class="bubble">✅</div><div class="grow muted">{d.name}</div><div class="num muted">{rp(d.total)}</div></button>
      {/each}
    </div>
  {/if}
  <p class="tiny muted">Pembayaran dicatat sebagai pengeluaran (kategori cicilan) sehingga ikut terhitung di budget. AI otomatis mengenali "bayar kredivo 714rb" sebagai pembayaran utang ini.</p>
</div>

<Sheet open={!!edit} onclose={() => (edit = null)} title={edit?.id ? 'Ubah utang' : 'Utang baru'}>
  {#if edit}
    <div class="stack">
      <label class="field"><span>Nama</span><input class="input" bind:value={edit.name} placeholder="mis. Kredivo" /></label>
      <div class="field"><span>Total utang (sisa saat mulai dicatat)</span><div class="card"><AmountField bind:value={edit.total} /></div></div>
      <div class="field"><span>Cicilan per bulan</span><AmountField bind:value={edit.monthly} big={false} /></div>
      <label class="field">
        <span>Tanggal jatuh tempo</span>
        <select class="input" bind:value={edit.due_day}>
          <option value={0}>Tidak ada</option>
          {#each Array.from({ length: 31 }, (_, i) => i + 1) as n (n)}<option value={n}>Tanggal {n}</option>{/each}
        </select>
      </label>
      <label class="field"><span>Catatan</span><input class="input" bind:value={edit.note} /></label>
      {#if edit.id}
        <label class="row between card tight"><span>Tandai lunas / arsipkan</span><span class="toggle"><input type="checkbox" bind:checked={edit.archived} /><span></span></span></label>
      {/if}
      <div class="row">
        {#if edit.id}<button class="btn danger" onclick={remove} aria-label="Hapus"><Icon name="trash" size={20} /></button>{/if}
        <button class="btn primary grow" onclick={save} disabled={busy || !edit.name}>Simpan</button>
      </div>
    </div>
  {/if}
</Sheet>

<style>
  .title { font-size: 28px; font-weight: 750; margin-top: -8px; }
  .sum .big { font-size: 20px; font-weight: 750; }
  .debt { display: flex; flex-direction: column; gap: 10px; }
  .head { width: 100%; text-align: left; align-items: flex-start; }
  .left { text-align: left; }
  .right { text-align: right; }
  .tight { padding: 12px 14px; }
  .fc { gap: 8px; font-size: 14px; }
  .fc-note { margin-top: 10px; }
  .fc-debts { gap: 4px; margin-top: 8px; }
</style>
