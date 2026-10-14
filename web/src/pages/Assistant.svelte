<script lang="ts">
  import { onMount, tick } from 'svelte';
  import type { Draft, ExtractResult } from '../lib/types';
  import { app, ui, activeAccounts, openTx, toastError } from '../lib/state.svelte';
  import { api } from '../lib/api';
  import { route, go } from '../lib/router.svelte';
  import { imagesToForm } from '../lib/image';
  import { markdown } from '../lib/format';
  import Icon from '../components/Icon.svelte';
  import DraftList from '../components/DraftList.svelte';
  import Sheet from '../components/Sheet.svelte';

  type Msg = {
    id: number;
    role: 'user' | 'assistant';
    text: string;
    drafts?: Draft[];
    pending?: boolean;
    error?: boolean;
    offline?: boolean;
  };

  const STORE_KEY = 'mm_chat_v1';
  let seq = Date.now();
  let messages = $state<Msg[]>(loadHistory());
  let text = $state('');
  let busy = $state(false);
  let ta: HTMLTextAreaElement | undefined = $state();
  let fileInput: HTMLInputElement | undefined = $state();
  let scanFiles = $state<File[] | null>(null);
  let bottom: HTMLDivElement | undefined = $state();

  const suggestions = [
    'makan siang 25rb',
    'tarik tunai 300rb',
    'bensin 50rb cash',
    'topup gopay 100rb',
    'sisa budget periode ini?',
    'pengeluaran terbesarku apa saja?',
    'bandingkan dengan periode lalu',
  ];

  function loadHistory(): Msg[] {
    try {
      const raw = localStorage.getItem(STORE_KEY);
      if (!raw) return [];
      return (JSON.parse(raw) as Msg[]).filter((m) => m.text && !m.pending).map((m) => ({ ...m, drafts: undefined }));
    } catch {
      return [];
    }
  }

  function persist() {
    try {
      const keep = messages.filter((m) => !m.pending && m.text).slice(-40).map(({ id, role, text }) => ({ id, role, text }));
      localStorage.setItem(STORE_KEY, JSON.stringify(keep));
    } catch {
      /* penyimpanan lokal tidak tersedia */
    }
  }

  async function scrollDown() {
    await tick();
    bottom?.scrollIntoView({ behavior: 'smooth', block: 'end' });
  }

  onMount(() => {
    const q = route.query.q;
    if (q) {
      history.replaceState(null, '', '#/ai');
      send(q);
    } else if (ui.pendingScan) {
      scanFiles = ui.pendingScan;
      ui.pendingScan = null;
    } else {
      setTimeout(() => ta?.focus(), 300);
    }
    scrollDown();
  });

  function autoGrow() {
    if (!ta) return;
    ta.style.height = 'auto';
    ta.style.height = Math.min(ta.scrollHeight, 140) + 'px';
  }

  async function send(input?: string) {
    const t = (input ?? text).trim();
    if (!t || busy) return;
    text = '';
    autoGrow();
    const history = messages
      .filter((m) => !m.pending && !m.error && m.text)
      .slice(-10)
      .map((m) => ({ role: m.role, text: m.text }));
    messages.push({ id: ++seq, role: 'user', text: t });
    const pendingId = ++seq;
    messages.push({ id: pendingId, role: 'assistant', text: '', pending: true });
    busy = true;
    scrollDown();
    try {
      const res = await api.post<{ reply: string; drafts: Draft[]; mode: string }>('/ai/ask', { text: t, history });
      replace(pendingId, { text: res.reply, drafts: res.drafts.length ? res.drafts : undefined, offline: res.mode === 'offline' });
    } catch (e) {
      replace(pendingId, { text: (e as Error).message, error: true });
    } finally {
      busy = false;
      persist();
      scrollDown();
    }
  }

  function replace(id: number, patch: Partial<Msg>) {
    const i = messages.findIndex((m) => m.id === id);
    if (i >= 0) messages[i] = { ...messages[i], ...patch, pending: false };
  }

  function pickFiles(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const files = Array.from(input.files ?? []);
    input.value = '';
    if (files.length) scanFiles = files.slice(0, 10);
  }

  async function startScan(accountId: number) {
    const files = scanFiles;
    scanFiles = null;
    if (!files) return;
    if (!app.ai.enabled && !app.ai.ocr) {
      messages.push({ id: ++seq, role: 'assistant', text: 'Pembaca screenshot belum tersedia. Jalankan lewat Docker (OCR sudah terpasang) atau isi ANTHROPIC_API_KEY.', error: true });
      return;
    }
    const isPdf = files.some((f) => f.type === 'application/pdf' || f.name.toLowerCase().endsWith('.pdf'));
    messages.push({ id: ++seq, role: 'user', text: isPdf ? `📄 ${files[0].name}` : `📷 ${files.length} screenshot mutasi` });
    const pendingId = ++seq;
    messages.push({ id: pendingId, role: 'assistant', text: '', pending: true });
    busy = true;
    scrollDown();
    try {
      let res: ExtractResult;
      if (isPdf) {
        const pdf = files.find((f) => f.type === 'application/pdf' || f.name.toLowerCase().endsWith('.pdf'))!;
        const send = (pw: string) => {
          const fd = new FormData();
          fd.append('file', pdf);
          if (accountId) fd.append('account_id', String(accountId));
          if (pw) fd.append('pdf_password', pw);
          return api.post<ExtractResult>('/ai/import', fd);
        };
        try {
          res = await send('');
        } catch (err) {
          if (!/terkunci/i.test((err as Error).message)) throw err;
          const pw = prompt('PDF ini dikunci. Masukkan kata sandi PDF (mis. tanggal lahir DDMMYYYY):') ?? '';
          if (!pw) throw err;
          res = await send(pw);
        }
      } else {
        const fd = await imagesToForm(files, { account_id: accountId ? String(accountId) : '' });
        res = await api.post<ExtractResult>('/ai/scan', fd);
      }
      const n = res.drafts.length;
      const dup = res.drafts.filter((d) => d.duplicate_of || d.duplicate_in_scan).length;
      let msg = n ? `Ketemu **${n} transaksi**${res.source ? ` dari ${res.source}` : ''}.` : 'Tidak ada transaksi yang terbaca di screenshot ini.';
      if (dup) msg += ` ${dup} kemungkinan sudah tercatat, jadi tidak dicentang.`;
      if (res.notes) msg += `\n\n${res.notes}`;
      replace(pendingId, { text: msg, drafts: n ? res.drafts : undefined });
    } catch (e) {
      replace(pendingId, { text: (e as Error).message, error: true });
    } finally {
      busy = false;
      persist();
      scrollDown();
    }
  }

  function clearChat() {
    if (!confirm('Hapus riwayat percakapan?')) return;
    messages = [];
    persist();
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
      e.preventDefault();
      send();
    }
  }
</script>

<div class="page chat-page">
  <header class="page-head">
    <div>
      <h1>Asisten</h1>
      <div class="small muted">{app.ai.enabled ? 'Catat atau tanya apa saja' : 'Mode gratis (tanpa AI) — catat & scan screenshot'}</div>
    </div>
    <div class="row" style="gap:4px">
      {#if messages.length}
        <button class="icon-btn" onclick={clearChat} aria-label="Bersihkan percakapan"><Icon name="trash" size={20} /></button>
      {/if}
      <button class="btn sm" onclick={() => openTx()}><Icon name="plus" size={18} /> Manual</button>
    </div>
  </header>

  {#if messages.length === 0}
    <div class="welcome fade-in">
      <div class="orb"><Icon name="sparkles" size={30} /></div>
      <h2>Mau catat apa hari ini?</h2>
      <p class="muted small">Tulis bebas seperti chat. Aku yang menentukan kategori, akun, dan tanggalnya. Bisa juga kirim screenshot mutasi bank.</p>
      <div class="chips wrap sugg">
        {#each suggestions as s (s)}
          <button class="chip" onclick={() => send(s)}>{s}</button>
        {/each}
        <button class="chip" onclick={() => fileInput?.click()}>📷 Scan screenshot / 📄 PDF mutasi</button>
      </div>
    </div>
  {/if}

  <div class="msgs">
    {#each messages as m (m.id)}
      <div class="msg {m.role}" class:err={m.error}>
        {#if m.pending}
          <div class="bubble-msg typing"><span></span><span></span><span></span></div>
        {:else}
          {#if m.text}
            <div class="bubble-msg">
              {#if m.role === 'assistant'}
                <div class="md">{@html markdown(m.text)}</div>
              {:else}
                {m.text}
              {/if}
            </div>
          {/if}
          {#if m.drafts?.length}
            <div class="draft-wrap">
              <DraftList bind:drafts={m.drafts} autosave={app.settings.ai_autosave && m.drafts.length <= 3} />
            </div>
          {/if}
        {/if}
      </div>
    {/each}
    <div bind:this={bottom} class="spacer"></div>
  </div>
</div>

<form class="composer" onsubmit={(e) => { e.preventDefault(); send(); }}>
  <button type="button" class="icon-btn cam" onclick={() => fileInput?.click()} aria-label="Scan screenshot" disabled={busy}><Icon name="camera" /></button>
  <textarea
    bind:this={ta}
    bind:value={text}
    rows="1"
    placeholder="mis. makan siang 25rb pakai gopay"
    enterkeyhint="send"
    oninput={autoGrow}
    onkeydown={onKey}
    aria-label="Pesan"
  ></textarea>
  <button type="submit" class="send" disabled={busy || !text.trim()} aria-label="Kirim">
    {#if busy}<span class="spinner"></span>{:else}<Icon name="up" size={20} stroke={2.5} />{/if}
  </button>
  <input bind:this={fileInput} type="file" accept="image/*,application/pdf,.pdf" multiple class="hidden" onchange={pickFiles} />
</form>

<Sheet open={!!scanFiles} onclose={() => (scanFiles = null)} title="Mutasi dari akun mana?">
  <p class="small muted" style="margin-bottom:12px">
    {scanFiles?.length} file dipilih. Pilih akunnya agar transaksi masuk ke rekening yang benar, atau biarkan AI mendeteksi dari tampilan aplikasinya.
  </p>
  <div class="list">
    <button class="list-item" onclick={() => startScan(0)}>
      <div class="bubble">✨</div>
      <div class="grow"><div class="bold">Deteksi otomatis</div><div class="small muted">AI mengenali logo/nama aplikasi</div></div>
      <span class="chev"><Icon name="chevron-right" /></span>
    </button>
    {#each activeAccounts() as a (a.id)}
      <button class="list-item" onclick={() => startScan(a.id)}>
        <div class="bubble">{a.icon}</div>
        <div class="grow bold">{a.name}</div>
        <span class="chev"><Icon name="chevron-right" /></span>
      </button>
    {/each}
  </div>
  <button class="btn ghost block" style="margin-top:8px" onclick={() => { scanFiles = null; go('/import'); }}>Impor teks / Excel / PDF mutasi →</button>
</Sheet>

<style>
  .chat-page { padding-bottom: calc(var(--tabbar-h) + var(--safe-bottom) + 90px); }
  .welcome { text-align: center; padding: 24px 8px 8px; display: flex; flex-direction: column; align-items: center; gap: 10px; }
  .welcome h2 { font-size: 22px; font-weight: 750; }
  .orb { width: 64px; height: 64px; border-radius: 32px; display: grid; place-items: center; color: #fff; background: linear-gradient(135deg, #3987e5, #6250d6); box-shadow: 0 8px 24px rgba(57, 135, 229, 0.35); }
  .sugg { justify-content: center; margin-top: 8px; }
  .msgs { display: flex; flex-direction: column; gap: 10px; }
  .msg { display: flex; flex-direction: column; gap: 8px; max-width: 100%; }
  .msg.user { align-items: flex-end; }
  .msg.assistant { align-items: flex-start; }
  .bubble-msg {
    max-width: 86%; padding: 10px 14px; border-radius: 20px; font-size: 16px; line-height: 1.4;
    background: var(--surface); box-shadow: var(--shadow); overflow-wrap: anywhere;
  }
  .user .bubble-msg { background: var(--accent); color: #fff; border-bottom-right-radius: 6px; white-space: pre-wrap; }
  .assistant .bubble-msg { border-bottom-left-radius: 6px; }
  .err .bubble-msg { background: var(--bad-soft); color: var(--bad); }
  .bubble-msg :global(.md p:first-child) { margin-top: 0; }
  .bubble-msg :global(.md p:last-child) { margin-bottom: 0; }
  .draft-wrap { width: 100%; }
  .typing { display: flex; gap: 4px; padding: 14px 16px; }
  .typing span { width: 8px; height: 8px; border-radius: 4px; background: var(--muted); animation: blink 1.2s infinite; }
  .typing span:nth-child(2) { animation-delay: 0.2s; }
  .typing span:nth-child(3) { animation-delay: 0.4s; }
  @keyframes blink { 0%, 80%, 100% { opacity: 0.25; } 40% { opacity: 1; } }
  .spacer { height: 1px; }
  .composer {
    position: fixed; left: 0; right: 0; z-index: 30; max-width: 640px; margin: 0 auto;
    bottom: calc(var(--tabbar-h) + var(--safe-bottom));
    display: flex; align-items: flex-end; gap: 6px; padding: 8px 10px;
    background: color-mix(in srgb, var(--bg) 88%, transparent);
    backdrop-filter: blur(16px); -webkit-backdrop-filter: blur(16px);
  }
  .composer textarea {
    flex: 1; min-height: 44px; max-height: 140px; padding: 11px 14px; border-radius: 22px; resize: none;
    border: 1px solid var(--line); background: var(--surface); font-size: 16px; line-height: 1.35; outline: none;
  }
  .composer textarea:focus { border-color: var(--accent); }
  .cam { background: var(--surface); box-shadow: var(--shadow); }
  .send { width: 44px; height: 44px; border-radius: 22px; background: var(--accent); color: #fff; display: grid; place-items: center; flex: none; }
  .send:disabled { opacity: 0.4; }
</style>
