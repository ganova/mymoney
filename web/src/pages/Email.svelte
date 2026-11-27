<script lang="ts">
  import type { Account } from '../lib/types';
  import { app, changed, toast, toastError } from '../lib/state.svelte';
  import { api } from '../lib/api';
  import { back, go } from '../lib/router.svelte';
  import Icon from '../components/Icon.svelte';

  type Cfg = { enabled: boolean; email: string; has_password: boolean; sender: string; ledger_id: number; account_id: number; since: string; last_check: string; last_error: string };
  type Imp = { id: number; ledger_id: number; subject: string; received_at: string; status: string; tx_ids: string; note: string };

  let cfg = $state<Cfg | null>(null);
  let password = $state('');
  let imports = $state<Imp[]>([]);
  let busy = $state('');
  let test = $state<{ ok: boolean; message: string } | null>(null);

  async function load() {
    try {
      const c = await api.get<Cfg>('/email/config');
      cfg = c;
      imports = await api.get<Imp[]>('/email/imports');
    } catch (e) {
      toastError(e);
    }
  }
  $effect(() => {
    load();
  });

  const allAccounts = $derived<Account[]>([...app.accounts, ...app.otherAccounts].filter((a) => !a.archived));
  const ledgerAccounts = $derived(allAccounts.filter((a) => a.ledger_id === (cfg?.ledger_id ?? 1)));
  const ledgerName = (id: number) => app.ledgers.find((l) => l.id === id)?.name ?? '';

  async function save(enable?: boolean) {
    if (!cfg) return;
    busy = 'save';
    try {
      const body = { ...cfg, enabled: enable ?? cfg.enabled, password };
      cfg = await api.put<Cfg>('/email/config', body);
      password = '';
      toast(cfg.enabled ? 'Tersimpan — email diperiksa tiap 5 menit' : 'Tersimpan');
    } catch (e) {
      toastError(e);
    } finally {
      busy = '';
    }
  }

  async function runTest() {
    busy = 'test';
    test = null;
    try {
      if (password) await save();
      test = await api.post<{ ok: boolean; message: string }>('/email/test');
    } catch (e) {
      test = { ok: false, message: (e as Error).message };
    } finally {
      busy = '';
    }
  }

  async function runNow() {
    busy = 'run';
    try {
      const r = await api.post<{ checked: number; saved: number; skipped: number; failed: number }>('/email/run');
      toast(r.checked ? `${r.checked} email baru: ${r.saved} transaksi tercatat${r.skipped ? `, ${r.skipped} dilewati` : ''}${r.failed ? `, ${r.failed} gagal` : ''}` : 'Tidak ada email baru');
      await changed();
      await load();
    } catch (e) {
      toastError(e);
      await load();
    } finally {
      busy = '';
    }
  }

  async function retry(i: Imp) {
    try {
      await api.del(`/email/imports/${i.id}`);
      toast('Akan diproses ulang pada pemeriksaan berikutnya');
      await load();
    } catch (e) {
      toastError(e);
    }
  }

  const badge = (s: string) => (s === 'tersimpan' ? 'good' : s === 'gagal' ? 'bad' : '');
</script>

<div class="page stack">
  <header class="page-head">
    <button class="back" onclick={() => back('/more')}><Icon name="chevron-left" /> Lainnya</button>
  </header>
  <h1 class="title">Email Bank Otomatis</h1>
  <p class="small text-2">MyMoney membaca email notifikasi transaksi (mis. dari BCA) di Gmail Anda setiap 5 menit, lalu mencatatnya otomatis. Email tidak ditandai sudah dibaca dan tidak diubah.</p>

  {#if cfg}
    <div class="card stack">
      <label class="row between">
        <span><strong>Aktif</strong><br /><span class="small muted">{cfg.enabled ? 'Memeriksa email tiap 5 menit' : 'Tidak memeriksa email'}</span></span>
        <span class="toggle"><input type="checkbox" checked={cfg.enabled} onchange={(e) => save((e.currentTarget as HTMLInputElement).checked)} /><span></span></span>
      </label>
      <label class="field"><span>Alamat Gmail</span><input class="input" type="email" bind:value={cfg.email} placeholder="nama@gmail.com" autocomplete="off" /></label>
      <label class="field">
        <span>App Password Google</span>
        <input class="input" type="password" bind:value={password} placeholder={cfg.has_password ? '•••• tersimpan (isi untuk mengganti)' : '16 huruf, mis. abcd efgh ijkl mnop'} autocomplete="new-password" />
      </label>
      <details class="small">
        <summary class="link">Cara membuat App Password</summary>
        <ol class="help">
          <li>Aktifkan <strong>Verifikasi 2 Langkah</strong> di akun Google Anda.</li>
          <li>Buka <a href="https://myaccount.google.com/apppasswords" target="_blank" rel="noopener">myaccount.google.com/apppasswords</a>, buat sandi baru bernama "MyMoney", lalu salin 16 hurufnya ke sini.</li>
          <li>Pastikan IMAP aktif: Gmail → Setelan → Lihat semua setelan → Penerusan dan POP/IMAP → <strong>Aktifkan IMAP</strong>.</li>
        </ol>
        <p class="tiny muted">App Password disimpan hanya di PC ini (berkas terpisah), tidak ikut backup, dan bisa dicabut kapan saja dari akun Google.</p>
      </details>
      <label class="field"><span>Email dari pengirim yang mengandung</span><input class="input" bind:value={cfg.sender} placeholder="bca.co.id" /></label>
      <div class="row two">
        <label class="field grow">
          <span>Catat ke buku</span>
          <select class="input" bind:value={cfg.ledger_id} onchange={() => cfg && (cfg.account_id = 0)}>
            {#each app.ledgers as l (l.id)}<option value={l.id}>{l.icon} {l.name}</option>{/each}
          </select>
        </label>
        <label class="field grow">
          <span>Akun</span>
          <select class="input" bind:value={cfg.account_id}>
            <option value={0}>Deteksi otomatis</option>
            {#each ledgerAccounts as a (a.id)}<option value={a.id}>{a.icon} {a.name}</option>{/each}
          </select>
        </label>
      </div>
      <label class="field"><span>Ambil email sejak tanggal</span><input class="input" type="date" bind:value={cfg.since} /></label>
      <div class="row wrap" style="gap:8px">
        <button class="btn primary grow" onclick={() => save()} disabled={!!busy}>{#if busy === 'save'}<span class="spinner"></span>{/if} Simpan</button>
        <button class="btn grow" onclick={runTest} disabled={!!busy}>{#if busy === 'test'}<span class="spinner"></span>{/if} Tes koneksi</button>
        <button class="btn grow" onclick={runNow} disabled={!!busy || !cfg.enabled}>{#if busy === 'run'}<span class="spinner"></span>{/if} Periksa sekarang</button>
      </div>
      {#if test}<div class="alert {test.ok ? 'info' : 'warn'}"><span class="grow small">{test.ok ? '✅' : '⚠️'} {test.message}</span></div>{/if}
      {#if cfg.last_check}
        <div class="tiny muted">Pemeriksaan terakhir: {cfg.last_check}{cfg.last_error ? '' : ' · berhasil'}</div>
      {/if}
      {#if cfg.last_error}<div class="alert warn"><span class="grow small">⚠️ {cfg.last_error}</span></div>{/if}
    </div>

    <div class="section-label">Riwayat email</div>
    {#if imports.length === 0}
      <div class="card empty"><p class="small">Belum ada email yang diproses.</p></div>
    {:else}
      <div class="list">
        {#each imports as i (i.id)}
          <div class="list-item">
            <div class="grow">
              <div class="row" style="gap:6px"><span class="badge {badge(i.status)}">{i.status}</span><span class="ellipsis small bold">{i.subject || '(tanpa subjek)'}</span></div>
              <div class="tiny muted">{i.received_at} · {ledgerName(i.ledger_id)}{i.note ? ` · ${i.note}` : ''}</div>
            </div>
            {#if i.status === 'tersimpan'}
              <button class="btn sm" onclick={() => go('/tx')}>Lihat</button>
            {:else}
              <button class="btn sm" onclick={() => retry(i)} title="Proses ulang email ini">Ulangi</button>
            {/if}
          </div>
        {/each}
      </div>
      <p class="tiny muted">Transaksi dari email ditandai <span class="badge">email</span> di daftar transaksi dan bisa diubah atau dihapus seperti biasa.</p>
    {/if}
  {:else}
    <div class="card skeleton" style="height:240px"></div>
  {/if}
</div>

<style>
  .title { font-size: 28px; font-weight: 750; margin-top: -8px; }
  .help { margin: 8px 0 4px; padding-left: 18px; display: flex; flex-direction: column; gap: 4px; }
  .two { gap: 10px; align-items: flex-end; }
  .two .field { min-width: 0; }
</style>
