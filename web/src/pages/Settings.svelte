<script lang="ts">
  import type { APIToken, Rule, Settings } from '../lib/types';
  import { app, acc, cat, activeAccounts, changed, loadBootstrap, toast, toastError } from '../lib/state.svelte';
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
  let backup = $state<{ dir: string; count: number; keep: number; every_hours: number; last?: { name: string; size: number; mod_time: string } } | null>(null);
  let backing = $state(false);
  let aiTest = $state<{ ok: boolean; message: string; ms?: number } | null>(null);
  let aiTesting = $state(false);
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
    api.get<typeof backup>('/backup/status').then((b) => (backup = b)).catch(() => {});
  });

  async function backupNow() {
    backing = true;
    try {
      await api.post('/backup/now');
      backup = await api.get<typeof backup>('/backup/status');
      toast('Backup tersimpan');
    } catch (e) {
      toastError(e);
    } finally {
      backing = false;
    }
  }

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

  async function setAIMode(mode: 'ai' | 'offline') {
    if (s.ai_mode === mode) return;
    s.ai_mode = mode;
    aiTest = null;
    try {
      app.settings = await api.put<Settings>('/settings', { ...s, ai_mode: mode });
      await loadBootstrap();
      toast(mode === 'ai' ? 'AI dipakai (otomatis tanpa AI bila gagal)' : 'Mode tanpa AI');
    } catch (e) {
      toastError(e);
    }
  }

  async function setAICLI(cli: 'auto' | 'api' | 'codex' | 'claude') {
    if (s.ai_cli === cli) return;
    s.ai_cli = cli;
    aiTest = null;
    try {
      app.settings = await api.put<Settings>('/settings', { ...s, ai_cli: cli });
      await loadBootstrap();
      toast('Sumber AI diubah');
    } catch (e) {
      toastError(e);
    }
  }

  async function testAI() {
    aiTesting = true;
    aiTest = null;
    try {
      aiTest = await api.post<{ ok: boolean; message: string; ms?: number }>('/ai/test');
    } catch (e) {
      aiTest = { ok: false, message: (e as Error).message };
    } finally {
      aiTesting = false;
    }
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
      <span><strong>Jatah mingguan</strong><br /><span class="small muted">Uang fleksibel (pendapatan − budget wajib) dibagi 4 minggu: 3 minggu × 8 hari, minggu ke-4 sisanya. Berlaku per buku.</span></span>
      <span class="toggle"><input type="checkbox" bind:checked={s.weekly_allowance} /><span></span></span>
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
    <div class="field">
      <span>Cara mencatat (bawaan)</span>
      <div class="seg" role="tablist">
        <button class:on={s.ai_mode === 'ai'} onclick={() => setAIMode('ai')}>Pakai AI</button>
        <button class:on={s.ai_mode === 'offline'} onclick={() => setAIMode('offline')}>Tanpa AI</button>
      </div>
      <span class="tiny muted">
        {#if s.ai_mode === 'ai'}
          {app.ai.available ? 'AI membaca kalimat & screenshot. Bila AI sedang tidak bisa diakses, otomatis dibantu pencocokan kata kunci + OCR (tanpa AI).' : 'AI belum terpasang, jadi sementara memakai pencocokan kata kunci + OCR.'}
        {:else}
          Selalu memakai pencocokan kata kunci, kebiasaan, dan OCR di komputer ini; tidak memakai kuota AI. Tanya-jawab keuangan tidak tersedia.
        {/if}
      </span>
    </div>
    {#if s.ai_mode === 'ai'}
      <div class="field">
        <span>Sumber AI</span>
        <div class="seg" role="tablist">
          <button class:on={s.ai_cli === 'auto'} onclick={() => setAICLI('auto')}>Otomatis</button>
          {#if app.ai.status.codex_found}<button class:on={s.ai_cli === 'codex'} onclick={() => setAICLI('codex')}>Codex</button>{/if}
          {#if app.ai.status.claude_found}<button class:on={s.ai_cli === 'claude'} onclick={() => setAICLI('claude')}>Claude CLI</button>{/if}
          {#if app.ai.status.api_key}<button class:on={s.ai_cli === 'api'} onclick={() => setAICLI('api')}>API key</button>{/if}
        </div>
        <span class="tiny muted">
          {#if s.ai_cli === 'auto'}Otomatis: API key bila ada, lalu Codex, lalu Claude CLI — yang terpasang di PC ini.
          {:else if s.ai_cli === 'codex'}Memakai Codex CLI (langganan ChatGPT). Kuota Claude tidak terpakai.
          {:else if s.ai_cli === 'claude'}Memakai Claude CLI (langganan Claude Pro/Max) di PC ini. Screenshot dibaca dengan OCR, bukan AI.
          {:else}Memakai API key Claude (berbayar per pemakaian).{/if}
        </span>
        <div class="tiny">
          <span class:good={app.ai.status.codex_found} class:bad={!app.ai.status.codex_found}>{app.ai.status.codex_found ? '● Codex terpasang' : '○ Codex belum terpasang'}</span>
          &nbsp;·&nbsp;
          <span class:good={app.ai.status.claude_found} class:bad={!app.ai.status.claude_found}>{app.ai.status.claude_found ? '● Claude CLI terpasang' : '○ Claude CLI belum terpasang'}</span>
        </div>
        {#if !app.ai.status.codex_found && !app.ai.status.claude_found && !app.ai.status.api_key}
          <p class="tiny text-2">Pasang salah satu: <code>npm i -g @anthropic-ai/claude-code</code> lalu <code>claude</code> dan ketik <code>/login</code>; atau <code>npm i -g @openai/codex</code> lalu <code>codex login</code>. Jalankan ulang MyMoney setelahnya.</p>
        {/if}
      </div>
    {/if}
    <div class="row between">
      <span>Status AI</span>
      {#if !app.ai.available}<span class="badge bad">Belum terpasang</span>
      {:else if app.ai.enabled}<span class="badge good">● Aktif</span>
      {:else}<span class="badge">Dimatikan</span>{/if}
    </div>
    {#if app.ai.available}
      <div class="row between small"><span class="muted">Sumber</span><code>{app.ai.model}</code></div>
      <button class="btn block" onclick={testAI} disabled={aiTesting}>{#if aiTesting}<span class="spinner"></span>{/if} Tes koneksi AI</button>
      {#if aiTest}
        <div class="alert {aiTest.ok ? 'info' : 'warn'}"><span class="grow small">{aiTest.ok ? '✅' : '⚠️'} {aiTest.message}{aiTest.ms ? ` (${(aiTest.ms / 1000).toFixed(1)} dtk)` : ''}</span></div>
      {/if}
    {:else}
      <p class="small text-2">Pasang Codex CLI (<code>npm i -g @openai/codex</code>, lalu <code>codex login</code>) atau isi <code>ANTHROPIC_API_KEY</code> di <code>.env</code>, lalu jalankan ulang MyMoney. Tanpa AI, input teks tetap bisa dengan pencocokan kata kunci.</p>
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

  <div class="section-label">Backup</div>
  <div class="card stack">
    {#if backup}
      <div class="small text-2">Disimpan otomatis tiap {backup.every_hours} jam ke <code>{backup.dir}</code>, {backup.keep} versi terakhir.</div>
      {#if backup.last}
        <div class="row between small"><span class="muted">Terakhir</span><span class="num">{backup.last.name.replace('mymoney-', '').replace('.db', '')}</span></div>
      {:else}
        <div class="small text-2">Belum ada backup.</div>
      {/if}
      <button class="btn block" onclick={backupNow} disabled={backing}>{#if backing}<span class="spinner"></span>{/if} Buat backup sekarang</button>
      <p class="tiny muted">Tip: isi <code>BACKUP_DIR</code> di .env dengan folder yang ikut tersinkron (mis. Google Drive) agar salinannya aman di luar PC.</p>
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
