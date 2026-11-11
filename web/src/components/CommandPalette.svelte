<script lang="ts">
  import { tick } from 'svelte';
  import type { Transaction } from '../lib/types';
  import { app, ui, acc, cat, activeAccounts, activeCategories, askAssistant, openTx, setDock, switchLedger } from '../lib/state.svelte';
  import { api, qs } from '../lib/api';
  import { go } from '../lib/router.svelte';
  import { rp, shortDate } from '../lib/format';
  import Icon from './Icon.svelte';

  type Item = { id: string; group: string; icon: string; emoji?: string; label: string; hint?: string; kbd?: string; run: () => void };

  let q = $state('');
  let sel = $state(0);
  let input: HTMLInputElement | undefined = $state();
  let listEl: HTMLDivElement | undefined = $state();
  let found = $state<Transaction[]>([]);
  let timer: ReturnType<typeof setTimeout>;

  function setTheme(t: string) {
    try {
      localStorage.setItem('mm_theme', t);
    } catch {
      /* abaikan */
    }
    if (t === 'auto') document.documentElement.removeAttribute('data-theme');
    else document.documentElement.setAttribute('data-theme', t);
  }

  const base = $derived.by<Item[]>(() => {
    const nav = (path: string, icon: string, label: string, kbd = ''): Item => ({ id: 'nav' + path, group: 'Buka halaman', icon, label, kbd, run: () => go(path) });
    const items: Item[] = [
      { id: 'new', group: 'Tindakan', icon: 'plus', label: 'Transaksi baru', kbd: 'N', run: () => openTx() },
      { id: 'new-exp', group: 'Tindakan', icon: 'arrow-out', label: 'Catat pengeluaran', run: () => openTx({ preset: { type: 'expense' } }) },
      { id: 'new-inc', group: 'Tindakan', icon: 'arrow-in', label: 'Catat pemasukan', run: () => openTx({ preset: { type: 'income' } }) },
      { id: 'new-trf', group: 'Tindakan', icon: 'transfer', label: 'Catat transfer antar akun', run: () => openTx({ preset: { type: 'transfer' } }) },
      { id: 'dock', group: 'Tindakan', icon: 'sparkles', label: ui.dock ? 'Tutup panel Asisten AI' : 'Buka panel Asisten AI', kbd: 'Ctrl J', run: () => setDock(!ui.dock) },
      ...app.ledgers.filter((l) => l.id !== app.ledgerId).map((l): Item => ({ id: 'book' + l.id, group: 'Tindakan', icon: 'refresh', label: `Pindah ke buku ${l.name}`, run: () => switchLedger(l.id) })),
      { id: 'export', group: 'Tindakan', icon: 'download', label: 'Ekspor Excel periode ini', run: () => (location.href = '/api/export' + qs({ period: app.period?.key, format: 'xlsx' })) },
      { id: 'th-l', group: 'Tindakan', icon: 'settings', label: 'Tema terang', run: () => setTheme('light') },
      { id: 'th-d', group: 'Tindakan', icon: 'settings', label: 'Tema gelap', run: () => setTheme('dark') },
      { id: 'th-a', group: 'Tindakan', icon: 'settings', label: 'Tema otomatis', run: () => setTheme('auto') },
      nav('/', 'home', 'Beranda', 'G H'),
      nav('/tx', 'list', 'Transaksi', 'G T'),
      nav('/budget', 'pie', 'Budget', 'G B'),
      nav('/reports', 'chart', 'Laporan', 'G L'),
      nav('/accounts', 'wallet', 'Akun & Saldo', 'G A'),
      nav('/debts', 'card', 'Utang & Cicilan', 'G U'),
      nav('/pending', 'calendar', 'Pendapatan Tertunda', 'G P'),
      nav('/import', 'upload', 'Impor Mutasi'),
      nav('/categories', 'tag', 'Kategori'),
      nav('/sources', 'database', 'Sumber Pemasukan & Buku'),
      nav('/settings', 'settings', 'Pengaturan'),
      nav('/ai', 'chat', 'Asisten (layar penuh)'),
      ...activeAccounts().map((a): Item => ({ id: 'acc' + a.id, group: 'Akun', icon: 'wallet', emoji: a.icon, label: a.name, hint: rp(a.balance), run: () => go('/tx', { account: a.id }) })),
      ...activeCategories().map((c): Item => ({ id: 'cat' + c.id, group: 'Kategori', icon: 'tag', emoji: c.icon, label: c.name, hint: c.kind === 'income' ? 'pemasukan' : 'pengeluaran', run: () => go('/tx', { category: c.id }) })),
    ];
    return items;
  });

  const items = $derived.by<Item[]>(() => {
    const t = q.trim().toLowerCase();
    if (!t) return base.filter((i) => i.group === 'Tindakan' || i.group === 'Buka halaman');
    const words = t.split(/\s+/);
    const matched = base.filter((i) => words.every((w) => i.label.toLowerCase().includes(w)));
    const txs = found.map((x): Item => ({
      id: 'tx' + x.id, group: 'Transaksi', icon: 'list', emoji: cat(x.category_id)?.icon ?? (x.type === 'transfer' ? '🔁' : '🧾'),
      label: x.description || cat(x.category_id)?.name || 'Transaksi',
      hint: `${shortDate(x.date)} · ${acc(x.account_id)?.name ?? ''} · ${x.type === 'income' ? '+' : x.type === 'expense' ? '−' : ''}${rp(Math.abs(x.amount))}`,
      run: () => openTx({ tx: x }),
    }));
    const ai: Item = { id: 'ai', group: 'Asisten', icon: 'sparkles', label: `Catat / tanya: “${q.trim()}”`, hint: 'kirim ke Asisten', run: () => askAssistant(q.trim(), (s) => go('/ai', { q: s })) };
    const search: Item = { id: 'find', group: 'Transaksi', icon: 'search', label: `Cari “${q.trim()}” di semua transaksi`, run: () => go('/tx', { q: q.trim() }) };
    // kalimat bernominal (mis. "makan 25rb") hampir pasti ingin dicatat: taruh Asisten paling atas
    const looksLikeEntry = /\d/.test(t);
    return looksLikeEntry ? [ai, ...matched, ...txs, search] : [...matched, ...txs, search, ai];
  });

  $effect(() => {
    const t = q.trim();
    clearTimeout(timer);
    if (t.length < 2) {
      found = [];
      return;
    }
    timer = setTimeout(() => {
      api.get<Transaction[]>('/transactions' + qs({ q: t, limit: 6 })).then((r) => (found = r)).catch(() => {});
    }, 180);
  });

  $effect(() => {
    if (ui.palette) {
      q = '';
      sel = 0;
      tick().then(() => input?.focus());
    }
  });
  $effect(() => {
    items.length;
    if (sel >= items.length) sel = Math.max(0, items.length - 1);
  });

  function close() {
    ui.palette = false;
  }
  function run(it: Item | undefined) {
    if (!it) return;
    close();
    it.run();
  }
  async function onKey(e: KeyboardEvent) {
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      sel = (sel + (e.key === 'ArrowDown' ? 1 : -1) + items.length) % Math.max(items.length, 1);
      await tick();
      listEl?.querySelector('.it.on')?.scrollIntoView({ block: 'nearest' });
    } else if (e.key === 'Enter') {
      e.preventDefault();
      run(items[sel]);
    } else if (e.key === 'Escape') {
      close();
    }
  }
</script>

{#if ui.palette}
  <div class="backdrop" onclick={close} role="presentation"></div>
  <div class="pal" role="dialog" aria-modal="true" aria-label="Palet perintah">
    <div class="q">
      <Icon name="search" size={20} />
      <input bind:this={input} bind:value={q} oninput={() => (sel = 0)} onkeydown={onKey} placeholder="Cari transaksi, halaman, atau ketik “makan siang 25rb”…" aria-label="Cari atau perintah" autocomplete="off" />
      <kbd>Esc</kbd>
    </div>
    <div class="list" bind:this={listEl}>
      {#each items as it, i (it.id)}
        {#if i === 0 || items[i - 1].group !== it.group}<div class="g">{it.group}</div>{/if}
        <!-- svelte-ignore a11y_mouse_events_have_key_events -->
        <button class="it" class:on={i === sel} onmouseover={() => (sel = i)} onclick={() => run(it)}>
          <span class="ic">{#if it.emoji}{it.emoji}{:else}<Icon name={it.icon} size={18} />{/if}</span>
          <span class="grow ellipsis">{it.label}</span>
          {#if it.hint}<span class="hint num ellipsis">{it.hint}</span>{/if}
          {#if it.kbd}<kbd>{it.kbd}</kbd>{/if}
        </button>
      {/each}
      {#if items.length === 0}<div class="none">Tidak ada hasil.</div>{/if}
    </div>
    <div class="foot"><span><kbd>↑</kbd> <kbd>↓</kbd> pilih</span><span><kbd>Enter</kbd> jalankan</span><span><kbd>Esc</kbd> tutup</span></div>
  </div>
{/if}

<style>
  .backdrop { position: fixed; inset: 0; background: rgba(0, 0, 0, 0.45); z-index: 70; backdrop-filter: blur(2px); animation: fade 0.15s ease; }
  .pal {
    position: fixed; z-index: 71; left: 50%; top: 12vh; translate: -50% 0; width: min(640px, calc(100vw - 32px));
    max-height: 70vh; display: flex; flex-direction: column; overflow: hidden;
    background: var(--surface); border-radius: 18px; box-shadow: 0 24px 80px rgba(0, 0, 0, 0.4); border: 1px solid var(--line);
    animation: pop 0.16s cubic-bezier(0.2, 0.9, 0.3, 1);
  }
  .q { display: flex; align-items: center; gap: 10px; padding: 14px 16px; border-bottom: 1px solid var(--line); color: var(--muted); }
  .q input { flex: 1; border: 0; background: transparent; outline: none; font-size: 17px; color: var(--text); min-width: 0; }
  .list { overflow-y: auto; padding: 6px; }
  .g { font-size: 11px; font-weight: 700; letter-spacing: 0.06em; text-transform: uppercase; color: var(--muted); padding: 10px 10px 4px; }
  .it { display: flex; align-items: center; gap: 10px; width: 100%; text-align: left; min-height: 40px; padding: 0 10px; border-radius: 10px; font-size: 15px; }
  .it.on { background: var(--accent-soft); color: var(--accent-ink); }
  .ic { width: 24px; display: grid; place-items: center; flex: none; color: var(--text-2); font-size: 17px; }
  .it.on .ic { color: inherit; }
  .hint { color: var(--muted); font-size: 13px; max-width: 46%; }
  .none { padding: 24px; text-align: center; color: var(--muted); }
  .foot { display: flex; gap: 16px; padding: 8px 14px; border-top: 1px solid var(--line); font-size: 12px; color: var(--muted); }
  kbd { font: 600 11px/1 var(--font); padding: 3px 5px; border-radius: 5px; background: var(--surface-2); border: 1px solid var(--line); color: var(--text-2); white-space: nowrap; }
  @keyframes fade { from { opacity: 0; } to { opacity: 1; } }
  @keyframes pop { from { opacity: 0; scale: 0.97; } to { opacity: 1; scale: 1; } }
</style>
