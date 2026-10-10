<script lang="ts">
  import type { APIToken, Rule, Settings } from '../lib/types';
  import { app, acc, cat, activeAccounts, changed, toast, toastError } from '../lib/state.svelte';
  import { api } from '../lib/api';
  import { back } from '../lib/router.svelte';
  import { rp } from '../lib/format';
  import Icon from '../components/Icon.svelte';

  type Usage = { model: string; calls: number; errors: number; input_tokens: number; output_tokens: number; cache_read_tokens: number; cache_write_tokens: number };

  let s = $state<Settings>({ ...app.settings });
  let saving = $state(false);
  let tokens = $state<APIToken[]>([]);
  let newToken = $state('');
  let rules = $state<Rule[]>([]);
  let showRules = $state(false);
  let usage = $state<{ since: string; usage: Usage[] } | null>(null);
  let theme = $state(localStorage.getItem('mm_theme') ?? 'auto');

  // harga per juta token (USD): input, output — perkiraan untuk tampilan saja
  const price: Record<string, [number, number]> = {
    'claude-opus-5-5': [4, 20], 'claude-sonnet-5-5': [2, 10], 'claude-haiku-5-5': [0.1, 0.5],
    'claude-fable-5-1': [10, 50], 'claude-opus-5': [5, 25], 'claude-opus-4-8': [5, 25],
  };

  $effect(() => {
    api.get<APIToken[]>('/tokens').then((t) => (tokens = t)).catch(() => {});
    api.get<Rule[]>('/rules').then((r) => (rules = r)).catch(() => {});
    api.get<typeof usage>('/ai/usage').then((u) => (usage = u)).catch(() => {});
  });

  const preview = $derived.by(() => {
    const d = s.period_start_day;
    if (d === 1) return 'Periode = bulan kalender (1 – akhir bulan)';
    return `Contoh: periode Oktober = ${d} September – ${d - 1} Oktober`;
  });
  const shortcutURL = $derived(location.origin + '/api/shortcut/ask');
  const scanURL = $derived(location.origin + '/api/shortcut/scan');

  function cost(u: Usage): number {
    const p = price[u.model];
    if (!p) return 0;
    return (u.input_tokens * p[0] + u.cache_write_tokens * p[0] * 1.25 + u.cache_read_tokens * p[0] * 0.1 + u.output_tokens * p[1]) / 1e6;
  }

  async function save() {
    saving = true;
    try {
      app.settings = await api.put<Settings>('/settings', s);
      await changed();
      const b = await api.get<{ period: typeof app.period }>('/bootstrap');
      app.period = b.period;
      toast('Pengaturan disimpan');
    } catch (e) {
      toastError(e);
    } finally {
      saving = false;
    }
  }

  async function createToken() {
    try {
      const r = await api.post<{ token: string; info: APIToken }>('/tokens', { name: 'iPhone Shortcut' });
      newToken = r.token;
      tokens = [r.info, ...tokens];
    } catch (e) {
      toastError(e);
    }
  }

  async function revoke(t: APIToken) {
    if (!confirm(`Cabut token ${t.prefix}…? Shortcut yang memakainya berhenti bekerja.`)) return;
    await api.del(`/tokens/${t.id}`);
    tokens = tokens.filter((x) => x.id !== t.id);
  }

  async function deleteRule(r: Rule) {
    await api.del(`/rules/${r.id}`);
    rules = rules.filter((x) => x.id !== r.id);
  }

  async function copy(text: string) {
    try {
      await navigator.clipboard.writeText(text);
      toast('Disalin');
    } catch {
      prompt('Salin manual:', text);
    }
  }

  function setTheme(t: string) {
    theme = t;
    try {
      localStorage.setItem('mm_theme', t);
    } catch {
      /* abaikan */
    }
    if (t === 'auto') document.documentElement.removeAttribute('data-theme');
    else document.documentElement.setAttribute('data-theme', t);
  }
</script>

<div class="page stack">
  <header class="page-head">
    <button class="back" onclick={() => back('/more')}><Icon name="chevron-left" /> Lainnya</button>
  </header>
  <h1 class="title">Pengaturan</h1>

  <div class="card stack">
    <label class="field"><span>Nama panggilan</span><input class="input" bind:value={s.owner_name} placeholder="mis. Ghanni" /></label>
    <label class="field">
      <span>Tanggal mulai periode (tanggal gajian)</span>
      <select class="input" bind:value={s.period_start_day}>
        {#each Array.from({ length: 31 }, (_, i) => i + 1) as n (n)}<option value={n}>Tanggal {n}</option>{/each}
      </select>
      <small class="muted">{preview}</small>
    </label>
    <label class="field">
      <span>Akun default (bila tidak disebut)</span>
      <select class="input" bind:value={s.default_account_id}>
        {#each activeAccounts() as a (a.id)}<option value={a.id}>{a.icon} {a.name}</option>{/each}
      </select>
    </label>
    <label class="row between">
      <span><strong>Simpan otomatis</strong><br /><span class="small muted">Draf AI yang yakin (nominal jelas) langsung tersimpan, tetap bisa diurungkan</span></span>
      <span class="toggle"><input type="checkbox" bind:checked={s.ai_autosave} /><span></span></span>
    </label>
    <button class="btn primary block" onclick={save} disabled={saving}>{#if saving}<span class="spinner"></span>{/if} Simpan pengaturan</button>
  </div>

  <div class="section-label">Tampilan</div>
  <div class="seg">
    <button class:on={theme === 'auto'} onclick={() => setTheme('auto')}>Otomatis</button>
    <button class:on={theme === 'light'} onclick={() => setTheme('light')}>Terang</button>
    <button class:on={theme === 'dark'} onclick={() => setTheme('dark')}>Gelap</button>
  </div>

  <div class="section-label">AI</div>
  <div class="card stack">
    <div class="row between">
      <span>Status</span>
      {#if app.ai.enabled}<span class="badge good">● Aktif</span>{:else}<span class="badge bad">Nonaktif</span>{/if}
    </div>
    {#if app.ai.enabled}
      <div class="row between small"><span class="muted">Model</span><code>{app.ai.model}</code></div>
    {:else}
      <p class="small text-2">Isi <code>ANTHROPIC_API_KEY</code> di file <code>.env</code> lalu jalankan ulang <code>docker compose up -d</code>. Tanpa AI, input teks tetap bisa dengan pencocokan kata kunci.</p>
    {/if}
    {#if usage && usage.usage.length}
      <div class="tiny muted">Pemakaian sejak {usage.since}:</div>
      {#each usage.usage as u (u.model)}
        <div class="row between small">
          <span>{u.model}<br /><span class="tiny muted">{u.calls} panggilan{u.errors ? `, ${u.errors} gagal` : ''} · {Math.round((u.input_tokens + u.cache_read_tokens + u.cache_write_tokens) / 1000)}rb token masuk · {Math.round(u.output_tokens / 1000)}rb keluar</span></span>
          {#if cost(u)}<span class="num bold">≈ ${cost(u).toFixed(2)}</span>{/if}
        </div>
      {/each}
    {/if}
  </div>

  <div class="section-label">iPhone Shortcut & Siri</div>
  <div class="card stack">
    <p class="small text-2">Catat tanpa membuka aplikasi: "Hey Siri, catat pengeluaran" → ucapkan "makan siang 25 ribu" → tersimpan. Atau bagikan screenshot mutasi dari Foto → langsung tercatat.</p>
    {#if newToken}
      <div class="tokenbox">
        <div class="tiny muted">Token baru (hanya ditampilkan sekali):</div>
        <code class="tok">{newToken}</code>
        <button class="btn sm" onclick={() => copy(newToken)}><Icon name="copy" size={16} /> Salin token</button>
      </div>
    {/if}
    <button class="btn block" onclick={createToken}><Icon name="key" size={18} /> Buat token Shortcut</button>
    {#each tokens as t (t.id)}
      <div class="row between small">
        <span><code>{t.prefix}…</code> {t.name}<br /><span class="tiny muted">{t.last_used_at ? `terakhir dipakai ${t.last_used_at}` : 'belum dipakai'}</span></span>
        <button class="btn sm danger" onclick={() => revoke(t)}>Cabut</button>
      </div>
    {/each}
    <details class="small">
      <summary class="link">Cara membuat Shortcut</summary>
      <ol class="steps">
        <li>Buka app <strong>Pintasan (Shortcuts)</strong> → <strong>+</strong> → beri nama "Catat Pengeluaran".</li>
        <li>Tambah aksi <strong>Minta Input (Ask for Input)</strong> tipe Teks, pertanyaan "Catat apa?".</li>
        <li>Tambah aksi <strong>Dapatkan Isi URL (Get Contents of URL)</strong>:
          <br />URL: <button class="url" onclick={() => copy(shortcutURL)}>{shortcutURL}</button>
          <br />Metode: <strong>POST</strong> · Header: <code>Authorization</code> = <code>Bearer TOKEN_ANDA</code>
          <br />Isi permintaan: <strong>JSON</strong>, kunci <code>text</code> = <em>Input yang Diberikan</em>.</li>
        <li>Tambah aksi <strong>Tampilkan Hasil (Show Result)</strong>.</li>
        <li>Untuk screenshot: buat Shortcut kedua dengan "Tampilkan di Lembar Berbagi" (gambar) → <strong>Konversi Gambar</strong> ke JPEG → Get Contents of URL POST ke <button class="url" onclick={() => copy(scanURL)}>{scanURL}</button>, Isi permintaan <strong>Formulir</strong>, kunci <code>image</code> (File).</li>
      </ol>
      <p class="tiny muted">Agar bisa diakses dari luar rumah, jalankan server lewat Tailscale atau Cloudflare Tunnel (lihat README).</p>
    </details>
  </div>

  <div class="section-label row between">
    <span>Yang dipelajari AI ({rules.length})</span>
    <button class="link small" onclick={() => (showRules = !showRules)}>{showRules ? 'Sembunyikan' : 'Lihat'}</button>
  </div>
  {#if showRules}
    <div class="list">
      {#each rules as r (r.id)}
        <div class="list-item plain">
          <div class="grow">
            <div>"{r.pattern}"</div>
            <div class="tiny muted">→ {r.type === 'transfer' ? 'Transfer' : (cat(r.category_id)?.name ?? '-')} · {acc(r.account_id)?.name ?? '-'} · {r.hits}×</div>
          </div>
          <button class="icon-btn" onclick={() => deleteRule(r)} aria-label="Hapus aturan"><Icon name="trash" size={18} /></button>
        </div>
      {:else}
        <div class="empty small">Belum ada. AI belajar setiap kali kamu menyimpan atau mengoreksi transaksi.</div>
      {/each}
    </div>
  {/if}

  <p class="tiny muted center">MyMoney {app.version} · total saldo {rp(app.accounts.reduce((x, a) => x + (a.archived || a.type === 'credit' ? 0 : a.balance), 0))}</p>
</div>

<style>
  .title { font-size: 28px; font-weight: 750; margin-top: -8px; }
  .tokenbox { background: var(--warn-soft); border-radius: 12px; padding: 12px; display: flex; flex-direction: column; gap: 8px; align-items: flex-start; }
  .tok { word-break: break-all; font-size: 13px; }
  .steps { padding-left: 18px; display: flex; flex-direction: column; gap: 8px; margin: 10px 0; }
  code { font-size: 13px; background: var(--surface-2); padding: 1px 5px; border-radius: 5px; }
  .url { word-break: break-all; text-align: left; font-family: ui-monospace, monospace; font-size: 13px; background: var(--surface-2); padding: 1px 5px; border-radius: 5px; color: var(--accent-ink); }
</style>
