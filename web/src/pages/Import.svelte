<script lang="ts">
  import type { ExtractResult } from '../lib/types';
  import { app, activeAccounts, toastError } from '../lib/state.svelte';
  import { api } from '../lib/api';
  import { back } from '../lib/router.svelte';
  import { imagesToForm } from '../lib/image';
  import Icon from '../components/Icon.svelte';
  import DraftList from '../components/DraftList.svelte';

  let tab = $state<'image' | 'text' | 'file'>('image');
  let accountId = $state(0);
  let periodKey = $state(app.period?.key ?? '');
  let text = $state('');
  let images = $state<File[]>([]);
  let file = $state<File | null>(null);
  let busy = $state(false);
  let result = $state<ExtractResult | null>(null);

  const periods = $derived.by(() => {
    const out: { key: string; label: string }[] = [];
    if (!app.period) return out;
    const [y, m] = app.period.key.split('-').map(Number);
    const bulan = ['Januari', 'Februari', 'Maret', 'April', 'Mei', 'Juni', 'Juli', 'Agustus', 'September', 'Oktober', 'November', 'Desember'];
    for (let i = 0; i < 12; i++) {
      const d = new Date(y, m - 1 - i, 1);
      out.push({ key: `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`, label: `${bulan[d.getMonth()]} ${d.getFullYear()}` });
    }
    return out;
  });

  async function run() {
    busy = true;
    result = null;
    try {
      let fd: FormData;
      if (tab === 'image') {
        if (!images.length) throw new Error('Pilih screenshot dulu');
        fd = await imagesToForm(images, { account_id: accountId ? String(accountId) : '', period: periodKey });
        result = await api.post<ExtractResult>('/ai/scan', fd);
      } else {
        fd = new FormData();
        if (accountId) fd.append('account_id', String(accountId));
        fd.append('period', periodKey);
        if (tab === 'text') {
          if (!text.trim()) throw new Error('Tempel teks mutasi dulu');
          fd.append('text', text);
        } else {
          if (!file) throw new Error('Pilih file dulu');
          fd.append('file', file);
        }
        result = await api.post<ExtractResult>('/ai/import', fd);
      }
    } catch (e) {
      toastError(e);
    } finally {
      busy = false;
    }
  }
</script>

<div class="page stack">
  <header class="page-head">
    <button class="back" onclick={() => back('/more')}><Icon name="chevron-left" /> Lainnya</button>
  </header>
  <h1 class="title">Impor Mutasi</h1>
  <p class="small text-2">AI membaca riwayat transaksi, merapikan deskripsi, memilih kategori, mengenali top-up/tarik tunai sebagai transfer, dan menandai yang sudah pernah dicatat.</p>

  {#if !app.ai.enabled && !app.ai.ocr}
    <div class="card empty"><p>Impor butuh AI atau OCR. Jalankan lewat Docker, atau isi <code>ANTHROPIC_API_KEY</code>.</p></div>
  {:else}
    {#if !app.ai.enabled}<div class="card small text-2">🆓 Mode gratis: screenshot dibaca dengan OCR di komputer Anda. Hasilnya bisa kurang rapi dibanding AI — periksa sebelum menyimpan. PDF belum didukung tanpa AI.</div>{/if}
    <div class="seg">
      <button class:on={tab === 'image'} onclick={() => (tab = 'image')}>Screenshot</button>
      <button class:on={tab === 'text'} onclick={() => (tab = 'text')}>Tempel teks</button>
      <button class:on={tab === 'file'} onclick={() => (tab = 'file')}>File</button>
    </div>

    <div class="card stack">
      {#if tab === 'image'}
        <label class="drop">
          <Icon name="image" size={28} />
          <span>{images.length ? `${images.length} gambar dipilih` : 'Pilih screenshot dari Foto (bisa banyak)'}</span>
          <input type="file" accept="image/*" multiple class="hidden" onchange={(e) => (images = Array.from((e.currentTarget as HTMLInputElement).files ?? []).slice(0, 10))} />
        </label>
      {:else if tab === 'text'}
        <textarea class="input" rows="8" bind:value={text} placeholder={'Tempel mutasi dari KlikBCA / myBCA / e-statement, mis:\nTRANSAKSI DEBIT TGL: 0506 QR 914 Nasi Bebek  44.000 DB\nTRSF E-BANKING DB 0705/FTFVA 70001/GOPAY TOPUP 21.000'}></textarea>
      {:else}
        <label class="drop">
          <Icon name="file" size={28} />
          <span>{file ? file.name : 'Excel (.xlsx), CSV, atau PDF e-statement'}</span>
          <input type="file" accept=".xlsx,.xlsm,.csv,.txt,.pdf,image/*" class="hidden" onchange={(e) => (file = (e.currentTarget as HTMLInputElement).files?.[0] ?? null)} />
        </label>
        <p class="tiny muted">Bisa juga file Google Sheet lama (File → Download → .xlsx). Untuk tanggal yang hanya berupa angka hari, AI memakai periode di bawah.</p>
      {/if}

      <label class="field">
        <span>Akun sumber</span>
        <select class="input" bind:value={accountId}>
          <option value={0}>✨ Deteksi otomatis</option>
          {#each activeAccounts() as a (a.id)}<option value={a.id}>{a.icon} {a.name}</option>{/each}
        </select>
      </label>
      <label class="field">
        <span>Periode acuan</span>
        <select class="input" bind:value={periodKey}>
          {#each periods as p (p.key)}<option value={p.key}>{p.label}</option>{/each}
        </select>
      </label>
      <button class="btn primary block" onclick={run} disabled={busy}>
        {#if busy}<span class="spinner"></span> AI sedang membaca…{:else}<Icon name="sparkles" size={18} /> Baca transaksi{/if}
      </button>
    </div>

    {#if result}
      <div class="section-label">Hasil{result.source ? ` · ${result.source}` : ''}</div>
      {#if result.notes}<div class="card small text-2">{result.notes}</div>{/if}
      {#if result.drafts.length}
        {#key result}
          <DraftList bind:drafts={result.drafts} />
        {/key}
      {:else}
        <div class="card empty">Tidak ada transaksi yang terbaca.</div>
      {/if}
    {/if}
  {/if}
</div>

<style>
  .title { font-size: 28px; font-weight: 750; margin-top: -8px; }
  .drop {
    display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 26px 16px; border-radius: 14px;
    border: 2px dashed var(--line-strong); color: var(--text-2); text-align: center; cursor: pointer;
  }
</style>
