<script lang="ts">
  import type { Outlook, Receivable } from '../lib/types';
  import { app, changed, activeAccounts, source, toast, toastError, haptic } from '../lib/state.svelte';
  import { api } from '../lib/api';
  import { back } from '../lib/router.svelte';
  import { rp, rpc, shortDate, addDays } from '../lib/format';
  import Icon from '../components/Icon.svelte';
  import Sheet from '../components/Sheet.svelte';
  import AmountField from '../components/AmountField.svelte';

  let items = $state<Receivable[]>([]);
  let outlook = $state<Outlook | null>(null);
  let loading = $state(true);
  let edit = $state<Partial<Receivable> | null>(null);
  let recv = $state<{ r: Receivable; amount: number; account_id: number; date: string } | null>(null);
  let busy = $state(false);

  async function load() {
    try {
      [items, outlook] = await Promise.all([api.get<Receivable[]>('/receivables'), api.get<Outlook>('/outlook?months=12')]);
    } catch (e) {
      toastError(e);
    } finally {
      loading = false;
    }
  }
  $effect(() => {
    app.dataVersion;
    load();
  });

  const pending = $derived(items.filter((x) => x.status === 'pending'));
  const closed = $derived(items.filter((x) => x.status !== 'pending'));
  const rows = $derived((outlook?.months ?? []).filter((m) => m.incoming || m.debt_due || m.debt_paid).slice(0, 12));
  const clients = $derived([...new Set(items.map((x) => x.client).filter(Boolean).concat(['Fastwork']))]);
  const sources = $derived(app.incomeSources.filter((s) => !s.archived));
  const freelance = $derived(sources.find((s) => /freelance/i.test(s.name))?.id ?? null);

  function open(r?: Receivable) {
    edit = r ? { ...r } : { name: '', client: 'Fastwork', amount: 0, expected_date: '', income_source_id: freelance, note: '', status: 'pending' };
  }

  async function save() {
    if (!edit) return;
    busy = true;
    try {
      if (edit.id) await api.put(`/receivables/${edit.id}`, edit);
      else await api.post('/receivables', edit);
      edit = null;
      toast('Tersimpan');
      await changed();
    } catch (e) {
      toastError(e);
    } finally {
      busy = false;
    }
  }

  async function remove() {
    if (!edit?.id || !confirm(`Hapus ${edit.name}? Transaksi pemasukan yang sudah dibuat tidak ikut terhapus.`)) return;
    try {
      await api.del(`/receivables/${edit.id}`);
      edit = null;
      await changed();
    } catch (e) {
      toastError(e);
    }
  }

  async function cancel() {
    if (!edit?.id || !confirm(`Batalkan ${edit.name}? Nominalnya tidak lagi dihitung sebagai pendapatan ke depan.`)) return;
    edit.status = 'cancelled';
    await save();
  }

  function openReceive(r: Receivable) {
    recv = { r, amount: r.amount, account_id: app.settings.default_account_id || activeAccounts()[0]?.id || 0, date: app.today };
  }

  async function receive() {
    if (!recv) return;
    busy = true;
    try {
      await api.post(`/receivables/${recv.r.id}/receive`, { account_id: recv.account_id, amount: recv.amount, date: recv.date });
      haptic();
      toast(`${rp(recv.amount)} tercatat sebagai pemasukan`);
      recv = null;
      await changed();
    } catch (e) {
      toastError(e);
    } finally {
      busy = false;
    }
  }

  function when(r: Receivable): string {
    if (!r.expected_date) return 'belum ada estimasi tanggal';
    const late = r.status === 'pending' && r.expected_date < app.today;
    return (late ? 'terlambat · ' : 'estimasi ') + shortDate(r.expected_date);
  }
</script>

<div class="page stack">
  <header class="page-head">
    <button class="back" onclick={() => back('/more')}><Icon name="chevron-left" /> Lainnya</button>
    <button class="btn sm primary" onclick={() => open()}><Icon name="plus" size={18} /> Proyek</button>
  </header>
  <h1 class="title">Pendapatan Tertunda</h1>

  {#if loading && !outlook}
    <div class="card skeleton" style="height:110px"></div>
  {:else if outlook}
    <div class="card hero">
      <div class="small muted">Total pendapatan yang akan cair</div>
      <div class="big num good">{rp(outlook.pending_total)}</div>
      <div class="tiny muted">{outlook.pending_count} proyek menunggu{outlook.pending_unscheduled ? ` · ${rp(outlook.pending_unscheduled)} belum ada estimasi tanggal` : ''}</div>
    </div>

    {#if rows.length}
      <section class="card">
        <div class="card-title">Proyeksi per periode</div>
        <div class="tbl">
          {#each rows as m (m.key)}
            <div class="tr">
              <div class="row between">
                <b>{m.label}</b>
                <span class="num bold" class:bad={m.net < 0} class:good={m.net > 0}>{m.net > 0 ? '+' : ''}{rp(m.net)}</span>
              </div>
              <div class="row between tiny muted">
                <span>Masuk <span class="num good">{m.incoming ? rp(m.incoming) : '–'}</span></span>
                <span>Cicilan <span class="num">{m.debt_due ? rp(m.debt_due) : m.debt_paid ? 'sudah dibayar' : '–'}</span></span>
              </div>
            </div>
          {/each}
        </div>
        <p class="tiny muted">Masuk = pendapatan tertunda yang diperkirakan cair. Cicilan = utang di buku ini (lihat Utang & Cicilan). Yang terlambat dihitung di periode berjalan.</p>
      </section>
    {/if}
  {/if}

  {#if pending.length === 0 && !loading}
    <div class="card empty">
      <div class="big">⏳</div>
      <p>Belum ada pendapatan tertunda. Tambahkan proyek yang sudah dibayar klien tapi dananya masih ditahan (mis. Fastwork) agar kelihatan berapa pemasukan Anda ke depan.</p>
    </div>
  {/if}

  {#each pending as r (r.id)}
    {@const s = source(r.income_source_id)}
    <div class="card item">
      <button class="row between top" onclick={() => open(r)}>
        <div class="grow left">
          <div class="bold">{r.name}</div>
          <div class="tiny muted">{[r.client, s?.name, when(r)].filter(Boolean).join(' · ')}</div>
        </div>
        <div class="bold num good">{rp(r.amount)}</div>
      </button>
      <button class="btn sm primary" onclick={() => openReceive(r)}>Sudah cair</button>
    </div>
  {/each}

  {#if closed.length}
    <div class="section-label">Riwayat</div>
    <div class="list">
      {#each closed as r (r.id)}
        <button class="list-item" onclick={() => open(r)}>
          <div class="bubble">{r.status === 'received' ? '✅' : '🚫'}</div>
          <div class="grow">
            <div class="ellipsis" class:muted={r.status === 'cancelled'}>{r.name}</div>
            <div class="tiny muted">{r.status === 'received' ? 'cair ' + shortDate(r.received_at) : 'dibatalkan'}{r.client ? ' · ' + r.client : ''}</div>
          </div>
          <div class="num muted">{rp(r.amount)}</div>
        </button>
      {/each}
    </div>
  {/if}
</div>

<Sheet open={!!edit} onclose={() => (edit = null)} title={edit?.id ? 'Ubah proyek' : 'Proyek baru'}>
  {#if edit}
    {@const locked = edit.status === 'received'}
    <div class="stack">
      <label class="field"><span>Nama proyek</span><input class="input" bind:value={edit.name} placeholder="mis. Website toko Budi" /></label>
      <div class="field">
        <span>Klien / platform</span>
        <input class="input" bind:value={edit.client} placeholder="mis. Fastwork" />
        <div class="chips">
          {#each clients as c (c)}<button type="button" class="chip" class:on={edit.client === c} onclick={() => (edit!.client = c)}>{c}</button>{/each}
        </div>
      </div>
      <div class="field">
        <span>Nominal yang akan diterima (bersih)</span>
        {#if locked}<div class="small muted">Sudah cair; nominal tidak bisa diubah.</div>{:else}<div class="card"><AmountField bind:value={edit.amount} /></div>{/if}
      </div>
      {#if !locked}
        <div class="field">
          <span>Perkiraan cair</span>
          <div class="row">
            <input class="input grow" type="date" bind:value={edit.expected_date} />
            <button type="button" class="chip" onclick={() => (edit!.expected_date = addDays(app.today, 7))}>+7 hari</button>
            <button type="button" class="chip" onclick={() => (edit!.expected_date = addDays(app.today, 30))}>+30</button>
          </div>
          <span class="tiny muted">Boleh dikosongkan bila belum tahu.</span>
        </div>
      {/if}
      {#if sources.length}
        <div class="field">
          <span>Sumber pemasukan</span>
          <div class="chips">
            <button type="button" class="chip" class:on={!edit.income_source_id} onclick={() => (edit!.income_source_id = null)}>Lainnya</button>
            {#each sources as s (s.id)}<button type="button" class="chip" class:on={edit.income_source_id === s.id} onclick={() => (edit!.income_source_id = s.id)}>{s.icon} {s.name}</button>{/each}
          </div>
        </div>
      {/if}
      <label class="field"><span>Catatan</span><input class="input" bind:value={edit.note} /></label>
      <div class="row">
        {#if edit.id}<button class="btn danger" onclick={remove} aria-label="Hapus"><Icon name="trash" size={20} /></button>{/if}
        {#if edit.id && edit.status === 'pending'}<button class="btn" onclick={cancel}>Batalkan</button>{/if}
        <button class="btn primary grow" onclick={save} disabled={busy || !edit.name || (!locked && !edit.amount)}>Simpan</button>
      </div>
    </div>
  {/if}
</Sheet>

<Sheet open={!!recv} onclose={() => (recv = null)} title="Dana sudah cair">
  {#if recv}
    <div class="stack">
      <div class="bold">{recv.r.name}{recv.r.client ? ` · ${recv.r.client}` : ''}</div>
      <div class="field">
        <span>Jumlah yang benar-benar masuk</span>
        <div class="card"><AmountField bind:value={recv.amount} /></div>
        {#if recv.amount !== recv.r.amount}<span class="tiny muted">Estimasi {rp(recv.r.amount)}; selisih {rp(recv.amount - recv.r.amount)} (mis. potongan biaya platform).</span>{/if}
      </div>
      <div class="field">
        <span>Masuk ke akun</span>
        <div class="chips">
          {#each activeAccounts() as a (a.id)}<button type="button" class="chip" class:on={recv.account_id === a.id} onclick={() => (recv!.account_id = a.id)}>{a.icon} {a.name}</button>{/each}
        </div>
      </div>
      <label class="field"><span>Tanggal</span><input class="input" type="date" bind:value={recv.date} max={addDays(app.today, 1)} /></label>
      <button class="btn primary" onclick={receive} disabled={busy || !recv.amount}>Catat sebagai pemasukan</button>
    </div>
  {/if}
</Sheet>

<style>
  .title { font-size: 28px; font-weight: 750; margin-top: -8px; }
  .hero .big { font-size: 34px; font-weight: 750; }
  .tbl { display: flex; flex-direction: column; }
  .tr { display: flex; flex-direction: column; gap: 2px; padding: 9px 0; border-top: 1px solid var(--line, rgba(128,128,128,0.18)); font-size: 15px; }
  .tr:first-child { border-top: 0; }
  .item { display: flex; flex-direction: column; gap: 10px; }
  .top { width: 100%; text-align: left; align-items: flex-start; }
  .left { text-align: left; }
</style>
