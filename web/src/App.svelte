<script lang="ts">
  import { onMount } from 'svelte';
  import { app, bootstrap } from './lib/state.svelte';
  import { route } from './lib/router.svelte';
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
  <LedgerBar />
  {#key route.path + ':' + app.ledgerId}
    <Page />
  {/key}
  <TabBar />
  <TxEditor />
{/if}
<Toast />

<style>
  /* bilah buku sudah menangani area aman atas */
  :global(.ledger-bar ~ .page) { padding-top: 8px; min-height: calc(100dvh - 56px); }
  .splash { min-height: 100dvh; display: grid; place-items: center; }
  .logo { font-size: 56px; animation: pulse 1.2s ease-in-out infinite; }
  @keyframes pulse { 50% { transform: scale(0.9); opacity: 0.6; } }
</style>
