<script lang="ts">
  import { route, go } from '../lib/router.svelte';
  import Icon from './Icon.svelte';

  const tabs = [
    { path: '/', icon: 'home', label: 'Beranda' },
    { path: '/tx', icon: 'list', label: 'Transaksi' },
    { path: '/ai', icon: 'sparkles', label: 'Catat', center: true },
    { path: '/budget', icon: 'pie', label: 'Budget' },
    { path: '/more', icon: 'grid', label: 'Lainnya' },
  ];
  const moreRoutes = ['/more', '/accounts', '/categories', '/debts', '/reports', '/import', '/settings'];
  function active(p: string): boolean {
    if (p === '/more') return moreRoutes.includes(route.path);
    return route.path === p;
  }
</script>

<nav class="tabbar">
  {#each tabs as t (t.path)}
    {#if t.center}
      <button class="center" onclick={() => go(t.path)} aria-label="Catat dengan AI">
        <span class="orb"><Icon name={t.icon} size={26} /></span>
      </button>
    {:else}
      <button class="tab" class:on={active(t.path)} onclick={() => go(t.path)}>
        <Icon name={t.icon} size={24} stroke={active(t.path) ? 2.4 : 2} />
        <span>{t.label}</span>
      </button>
    {/if}
  {/each}
</nav>

<style>
  .tabbar {
    position: fixed; left: 0; right: 0; bottom: 0; z-index: 40;
    display: grid; grid-template-columns: repeat(5, 1fr); align-items: center;
    height: calc(var(--tabbar-h) + var(--safe-bottom)); padding-bottom: var(--safe-bottom);
    background: color-mix(in srgb, var(--surface) 82%, transparent);
    backdrop-filter: saturate(180%) blur(20px); -webkit-backdrop-filter: saturate(180%) blur(20px);
    border-top: 0.5px solid var(--line);
  }
  .tab { display: flex; flex-direction: column; align-items: center; gap: 2px; color: var(--muted); font-size: 11px; font-weight: 600; height: 100%; justify-content: center; }
  .tab.on { color: var(--accent); }
  .center { display: grid; place-items: center; height: 100%; }
  .orb {
    width: 54px; height: 54px; border-radius: 27px; display: grid; place-items: center; color: #fff; margin-top: -18px;
    background: linear-gradient(135deg, #3987e5, #6250d6); box-shadow: 0 6px 18px rgba(57, 135, 229, 0.45);
    border: 3px solid var(--bg);
  }
  .center:active .orb { transform: scale(0.95); }
</style>
