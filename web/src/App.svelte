<script lang="ts">
  import { onMount } from 'svelte';
  import { app, ui, bootstrap, openTx, setDock } from './lib/state.svelte';
  import { route, go } from './lib/router.svelte';
  import Sidebar from './components/Sidebar.svelte';
  import CommandPalette from './components/CommandPalette.svelte';
  import Sheet from './components/Sheet.svelte';
  import Icon from './components/Icon.svelte';
  import LedgerBar from './components/LedgerBar.svelte';
  import TabBar from './components/TabBar.svelte';
  import Toast from './components/Toast.svelte';
  import TxEditor from './components/TxEditor.svelte';
  import Home from './pages/Home.svelte';
  import Assistant from './pages/Assistant.svelte';
  import Transactions from './pages/Transactions.svelte';
  import BudgetPage from './pages/Budget.svelte';
  import More from './pages/More.svelte';
  import Accounts from './pages/Accounts.svelte';
  import Categories from './pages/Categories.svelte';
  import Debts from './pages/Debts.svelte';
  import Reports from './pages/Reports.svelte';
  import ImportPage from './pages/Import.svelte';
  import SettingsPage from './pages/Settings.svelte';
  import SourcesPage from './pages/Sources.svelte';
  import PendingPage from './pages/Pending.svelte';

  onMount(bootstrap);

  const pages: Record<string, any> = {
    '/': Home,
    '/ai': Assistant,
    '/tx': Transactions,
    '/budget': BudgetPage,
    '/more': More,
    '/accounts': Accounts,
    '/categories': Categories,
    '/debts': Debts,
    '/reports': Reports,
    '/import': ImportPage,
    '/settings': SettingsPage,
    '/sources': SourcesPage,
    '/pending': PendingPage,
  };
  const Page = $derived(pages[route.path] ?? Home);
  const showDock = $derived(ui.desktop && ui.dock && route.path !== '/ai');

  // --- pintasan keyboard (desktop) ---
  const goKeys: Record<string, string> = { h: '/', t: '/tx', b: '/budget', l: '/reports', a: '/accounts', u: '/debts', p: '/pending', s: '/settings' };
  let gPending = 0;
  function onKeydown(e: KeyboardEvent) {
    if (!app.authenticated) return;
    const mod = e.ctrlKey || e.metaKey;
    if (mod && e.key.toLowerCase() === 'k') {
      e.preventDefault();
      ui.palette = !ui.palette;
      return;
    }
    if (mod && e.key.toLowerCase() === 'j') {
      e.preventDefault();
      setDock(!ui.dock);
      return;
    }
    const el = e.target as HTMLElement | null;
    const typing = !!el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.tagName === 'SELECT' || el.isContentEditable);
    if (typing || mod || e.altKey || ui.palette || ui.txEditor || document.querySelector('[aria-modal="true"]')) return;
    const k = e.key.toLowerCase();
    if (gPending && Date.now() - gPending < 1200 && goKeys[k]) {
      gPending = 0;
      e.preventDefault();
      go(goKeys[k]);
      return;
    }
    gPending = 0;
    if (k === 'g') gPending = Date.now();
    else if (k === 'n') {
      e.preventDefault();
      openTx();
    } else if (k === '/') {
      e.preventDefault();
      ui.palette = true;
    } else if (e.key === '?') {
      e.preventDefault();
      ui.help = true;
    }
  }

  const shortcuts: [string, string][] = [
    ['Ctrl K  atau  /', 'Palet perintah: cari transaksi, buka halaman, catat kalimat'],
    ['N', 'Transaksi baru'],
    ['Ctrl J', 'Buka/tutup panel Asisten AI'],
    ['G lalu H', 'Beranda'],
    ['G lalu T', 'Transaksi'],
    ['G lalu B', 'Budget'],
    ['G lalu L', 'Laporan'],
    ['G lalu A', 'Akun & Saldo'],
    ['G lalu U', 'Utang & Cicilan'],
    ['G lalu P', 'Pendapatan Tertunda'],
    ['G lalu S', 'Pengaturan'],
    ['Ctrl V', 'Tempel screenshot di kolom Asisten'],
    ['Esc', 'Tutup dialog'],
    ['?', 'Bantuan ini'],
  ];
</script>

{#if !app.ready}
  <div class="splash"><div class="logo">💰</div></div>
{:else if !app.authenticated && app.error}
  <div class="page center">
    <div class="empty">
      <div class="big">📡</div>
      <p>{app.error}</p>
      <button class="btn primary" style="margin-top:16px" onclick={() => location.reload()}>Coba lagi</button>
    </div>
  </div>
{:else}
  {#if ui.desktop}<Sidebar />{:else}<LedgerBar />{/if}
  {#key route.path + ':' + app.ledgerId}
    <Page />
  {/key}
  {#if !ui.desktop}<TabBar />{/if}
  {#if showDock}
    {#key app.ledgerId}
      <aside class="dock" aria-label="Asisten AI"><Assistant docked /></aside>
    {/key}
  {:else if ui.desktop && route.path !== '/ai'}
    <button class="dock-tab" onclick={() => setDock(true)} aria-label="Buka Asisten AI" title="Asisten AI (Ctrl J)"><Icon name="sparkles" size={18} /><span>Asisten</span></button>
  {/if}
  <TxEditor />
  <CommandPalette />
  <Sheet open={ui.help} onclose={() => (ui.help = false)} title="Pintasan keyboard">
    <div class="keys">
      {#each shortcuts as [k, d] (k)}
        <div class="krow"><kbd>{k}</kbd><span>{d}</span></div>
      {/each}
    </div>
  </Sheet>
{/if}
<Toast />
<svelte:window onkeydown={onKeydown} />

<style>
  /* bilah buku sudah menangani area aman atas */
  :global(.ledger-bar ~ .page) { padding-top: 8px; min-height: calc(100dvh - 56px); }
  .dock {
    position: fixed; top: 0; right: 0; bottom: 0; width: 392px; z-index: 34; display: flex; flex-direction: column;
    background: var(--bg); border-left: 1px solid var(--line); box-shadow: -12px 0 40px rgba(0, 0, 0, 0.12);
    animation: slidein 0.22s ease;
  }
  @media (min-width: 1280px) { .dock { box-shadow: none; } }
  @keyframes slidein { from { transform: translateX(40px); opacity: 0; } to { transform: none; opacity: 1; } }
  /* tab tipis di tepi kanan: muat di gutter halaman, tidak menutupi isi */
  .dock-tab {
    position: fixed; right: 0; top: 50%; translate: 0 -50%; z-index: 33; width: 30px; padding: 12px 0;
    display: flex; flex-direction: column; align-items: center; gap: 8px; color: #fff; border-radius: 12px 0 0 12px;
    background: linear-gradient(180deg, #3987e5, #6250d6); box-shadow: -4px 4px 18px rgba(57, 135, 229, 0.35);
    transition: width 0.15s;
  }
  .dock-tab span { writing-mode: vertical-rl; font-size: 12px; font-weight: 700; letter-spacing: 0.04em; }
  .dock-tab:hover { width: 36px; }
  .keys { display: flex; flex-direction: column; gap: 10px; }
  .krow { display: grid; grid-template-columns: 150px 1fr; gap: 12px; align-items: center; font-size: 14px; }
  .krow kbd { justify-self: start; font: 600 12px/1 var(--font); padding: 5px 8px; border-radius: 6px; background: var(--surface-2); border: 1px solid var(--line); }
  .splash { min-height: 100dvh; display: grid; place-items: center; }
  .logo { font-size: 56px; animation: pulse 1.2s ease-in-out infinite; }
  @keyframes pulse { 50% { transform: scale(0.9); opacity: 0.6; } }
</style>
