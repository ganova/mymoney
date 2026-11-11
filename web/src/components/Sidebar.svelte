<script lang="ts">
  import { app, ui, switchLedger, openTx, setDock } from '../lib/state.svelte';
  import { route, go } from '../lib/router.svelte';
  import { rp } from '../lib/format';
  import Icon from './Icon.svelte';

  const groups = [
    {
      label: '',
      items: [
        { path: '/', icon: 'home', label: 'Beranda', key: 'G H' },
        { path: '/tx', icon: 'list', label: 'Transaksi', key: 'G T' },
        { path: '/budget', icon: 'pie', label: 'Budget', key: 'G B' },
        { path: '/reports', icon: 'chart', label: 'Laporan', key: 'G L' },
      ],
    },
    {
      label: 'Aset & kewajiban',
      items: [
        { path: '/accounts', icon: 'wallet', label: 'Akun & Saldo', key: 'G A' },
        { path: '/debts', icon: 'card', label: 'Utang & Cicilan', key: 'G U' },
        { path: '/pending', icon: 'calendar', label: 'Pendapatan Tertunda', key: 'G P' },
      ],
    },
    {
      label: 'Data',
      items: [
        { path: '/import', icon: 'upload', label: 'Impor Mutasi', key: '' },
        { path: '/categories', icon: 'tag', label: 'Kategori', key: '' },
        { path: '/sources', icon: 'database', label: 'Sumber & Buku', key: '' },
      ],
    },
  ];
  const total = $derived(app.accounts.filter((a) => !a.archived && a.type !== 'credit').reduce((s, a) => s + a.balance, 0));
</script>

<aside class="side" aria-label="Navigasi">
  <button class="brand" onclick={() => go('/')}>
    <span class="logo">💰</span>
    <span class="grow"><b>MyMoney</b><span class="tiny muted">{app.period?.label ?? ''}</span></span>
  </button>

  {#if app.ledgers.length > 1}
    <div class="seg books" role="tablist" aria-label="Buku keuangan">
      {#each app.ledgers as l (l.id)}
        <button role="tab" aria-selected={app.ledgerId === l.id} class:on={app.ledgerId === l.id} onclick={() => switchLedger(l.id)}>{l.icon} {l.name}</button>
      {/each}
    </div>
  {/if}

  <button class="new" onclick={() => openTx()}><Icon name="plus" size={18} stroke={2.5} /> <span class="grow">Transaksi baru</span> <kbd>N</kbd></button>
  <button class="find" onclick={() => (ui.palette = true)}><Icon name="search" size={16} /> <span class="grow ellipsis">Cari, perintah…</span> <kbd>Ctrl K</kbd></button>

  <nav>
    {#each groups as g (g.label)}
      {#if g.label}<div class="glabel">{g.label}</div>{/if}
      {#each g.items as it (it.path)}
        <button class="item" class:on={route.path === it.path} onclick={() => go(it.path)} title={it.key ? `Pintasan: ${it.key}` : undefined}>
          <Icon name={it.icon} size={19} stroke={route.path === it.path ? 2.3 : 1.9} />
          <span class="grow">{it.label}</span>
        </button>
      {/each}
    {/each}
  </nav>

  <div class="bottom">
    <div class="worth">
      <div class="tiny muted">Total saldo {app.ledgers.find((l) => l.id === app.ledgerId)?.name ?? ''}</div>
      <div class="num bold">{rp(total)}</div>
    </div>
    <button class="item" class:on={ui.dock || route.path === '/ai'} onclick={() => setDock(!ui.dock)}>
      <Icon name="sparkles" size={19} /> <span class="grow">Asisten AI</span> <kbd>Ctrl J</kbd>
    </button>
    <button class="item" class:on={route.path === '/settings'} onclick={() => go('/settings')}>
      <Icon name="settings" size={19} /> <span class="grow">Pengaturan</span>
    </button>
    <button class="item" onclick={() => (ui.help = true)}>
      <Icon name="info" size={19} /> <span class="grow">Pintasan keyboard</span> <kbd>?</kbd>
    </button>
  </div>
</aside>

<style>
  .side {
    position: fixed; left: 0; top: 0; bottom: 0; width: var(--side-w); z-index: 35;
    display: flex; flex-direction: column; gap: 10px; padding: 16px 12px 12px;
    background: var(--surface); border-right: 1px solid var(--line); overflow-y: auto;
  }
  .brand { display: flex; align-items: center; gap: 10px; padding: 4px 6px; text-align: left; border-radius: 12px; }
  .brand .logo { font-size: 26px; width: 40px; height: 40px; display: grid; place-items: center; background: var(--accent-soft); border-radius: 12px; }
  .brand b { display: block; font-size: 17px; letter-spacing: -0.01em; }
  .brand .tiny { display: block; }
  .books button { min-height: 32px; font-size: 13px; }
  .new {
    display: flex; align-items: center; gap: 8px; min-height: 42px; padding: 0 12px; border-radius: 12px;
    background: var(--accent); color: var(--on-accent); font-weight: 650; text-align: left;
  }
  .new:hover { filter: brightness(1.08); }
  .new kbd { background: rgba(255,255,255,0.18); border-color: transparent; color: #fff; }
  .find {
    display: flex; align-items: center; gap: 8px; min-height: 38px; padding: 0 10px; border-radius: 10px;
    background: var(--surface-2); color: var(--muted); font-size: 14px; text-align: left;
  }
  .find:hover { color: var(--text-2); }
  nav { display: flex; flex-direction: column; gap: 2px; margin-top: 4px; }
  .glabel { font-size: 11px; font-weight: 700; letter-spacing: 0.06em; text-transform: uppercase; color: var(--muted); padding: 12px 10px 4px; }
  .item {
    display: flex; align-items: center; gap: 10px; min-height: 38px; padding: 0 10px; border-radius: 10px;
    color: var(--text-2); font-size: 14.5px; font-weight: 550; text-align: left; width: 100%;
  }
  .item:hover { background: var(--surface-2); color: var(--text); }
  .item.on { background: var(--accent-soft); color: var(--accent-ink); font-weight: 650; }
  .bottom { margin-top: auto; display: flex; flex-direction: column; gap: 2px; padding-top: 10px; border-top: 1px solid var(--line); }
  .worth { padding: 4px 10px 8px; }
  .worth .num { font-size: 17px; letter-spacing: -0.01em; }
</style>
