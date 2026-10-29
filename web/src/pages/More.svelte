<script lang="ts">
  import { app, toastError } from '../lib/state.svelte';
  import { api, qs } from '../lib/api';
  import { go } from '../lib/router.svelte';
  import Icon from '../components/Icon.svelte';

  const items = [
    { path: '/accounts', icon: '🏦', title: 'Akun & Saldo', sub: 'Rekening, tunai, e-wallet, sesuaikan saldo' },
    { path: '/sources', icon: '💼', title: 'Sumber Pemasukan & Buku', sub: 'CMW, Freelance, nama buku Pribadi/Work' },
    { path: '/categories', icon: '🏷️', title: 'Kategori', sub: 'Kelola kategori & kata kunci' },
    { path: '/debts', icon: '💳', title: 'Utang & Cicilan', sub: 'Paylater, pinjaman, sisa tagihan' },
    { path: '/reports', icon: '📊', title: 'Laporan', sub: 'Grafik, tren, pengeluaran terbesar' },
    { path: '/import', icon: '📥', title: 'Impor Mutasi', sub: 'Screenshot, teks, Excel, PDF e-statement' },
    { path: '/settings', icon: '⚙️', title: 'Pengaturan', sub: 'Periode, AI, iPhone Shortcut, backup' },
  ];

  async function logout() {
    try {
      await api.post('/auth/logout');
      app.authenticated = false;
    } catch (e) {
      toastError(e);
    }
  }
</script>

<div class="page stack">
  <header class="page-head"><h1>Lainnya</h1></header>
  <div class="list">
    {#each items as it (it.path)}
      <button class="list-item" onclick={() => go(it.path)}>
        <div class="bubble">{it.icon}</div>
        <div class="grow"><div class="bold">{it.title}</div><div class="small muted">{it.sub}</div></div>
        <span class="chev"><Icon name="chevron-right" /></span>
      </button>
    {/each}
  </div>

  <div class="section-label">Ekspor</div>
  <div class="list">
    <a class="list-item" href={'/api/export' + qs({ period: app.period?.key, format: 'xlsx' })}>
      <div class="bubble">📗</div>
      <div class="grow"><div class="bold">Excel periode ini</div><div class="small muted">Transaksi + budget {app.period?.label}</div></div>
      <span class="chev"><Icon name="download" size={20} /></span>
    </a>
    <a class="list-item" href="/api/export?all=1&format=xlsx">
      <div class="bubble">📚</div>
      <div class="grow"><div class="bold">Excel semua transaksi</div><div class="small muted">Seluruh riwayat</div></div>
      <span class="chev"><Icon name="download" size={20} /></span>
    </a>
    <a class="list-item" href="/api/backup">
      <div class="bubble">💾</div>
      <div class="grow"><div class="bold">Backup database</div><div class="small muted">File SQLite lengkap untuk disimpan</div></div>
      <span class="chev"><Icon name="download" size={20} /></span>
    </a>
  </div>

  {#if app.authRequired}
    <button class="btn danger block" onclick={logout} style="margin-top:16px"><Icon name="logout" size={18} /> Keluar</button>
  {/if}
  <p class="tiny muted center">MyMoney {app.version} · AI {app.ai.enabled ? `aktif (${app.ai.model})` : 'nonaktif'}</p>
</div>
