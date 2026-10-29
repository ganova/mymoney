<script lang="ts">
  import type { IncomeSource, Ledger } from '../lib/types';
  import { app, changed, loadBootstrap, toast, toastError } from '../lib/state.svelte';
  import { api } from '../lib/api';
  import { back } from '../lib/router.svelte';
  import Icon from '../components/Icon.svelte';
  import Sheet from '../components/Sheet.svelte';

  let edit = $state<Partial<IncomeSource> | null>(null);
  let book = $state<Ledger | null>(null);
  let busy = $state(false);

  const active = $derived(app.incomeSources.filter((s) => !s.archived));
  const archived = $derived(app.incomeSources.filter((s) => s.archived));

  function open(s?: IncomeSource) {
    edit = s ? { ...s } : { name: '', icon: '💰', keywords: '', archived: false };
  }

  async function save() {
    if (!edit) return;
    busy = true;
    try {
      if (edit.id) await api.put(`/income-sources/${edit.id}`, edit);
      else await api.post('/income-sources', edit);
      edit = null;
      await changed();
      toast('Tersimpan');
    } catch (e) {
      toastError(e);
    } finally {
      busy = false;
    }
  }

  async function remove() {
    if (!edit?.id || !confirm(`Hapus sumber ${edit.name}? Transaksi lama tetap ada tanpa label sumber.`)) return;
    try {
      await api.del(`/income-sources/${edit.id}`);
      edit = null;
      await changed();
    } catch (e) {
      toastError(e);
    }
  }

  async function saveBook() {
    if (!book) return;
    busy = true;
    try {
      await api.put(`/ledgers/${book.id}`, book);
      book = null;
      await loadBootstrap();
      toast('Tersimpan');
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
    <button class="btn sm primary" onclick={() => open()}><Icon name="plus" size={18} /> Sumber</button>
  </header>
  <h1 class="title">Sumber Pemasukan & Buku</h1>

  <div class="section-label">Sumber pemasukan</div>
  <div class="list">
    {#each active as s (s.id)}
      <button class="list-item" onclick={() => open(s)}>
        <div class="bubble">{s.icon || '💰'}</div>
        <div class="grow"><div class="bold">{s.name}</div><div class="small muted ellipsis">{s.keywords || 'tanpa kata kunci'}</div></div>
        <span class="chev"><Icon name="chevron-right" /></span>
      </button>
    {/each}
    {#each archived as s (s.id)}
      <button class="list-item" onclick={() => open(s)}>
        <div class="bubble">🗄️</div>
        <div class="grow muted">{s.name} (diarsipkan)</div>
      </button>
    {/each}
  </div>
  <p class="tiny muted">Setiap pemasukan bisa diberi sumber (mis. CMW atau Freelance). AI dan pencatatan kalimat menebaknya dari kata kunci, lalu laporan menampilkan total per sumber.</p>

  <div class="section-label">Buku keuangan</div>
  <div class="list">
    {#each app.ledgers as l (l.id)}
      <button class="list-item" onclick={() => (book = { ...l })}>
        <div class="bubble">{l.icon}</div>
        <div class="grow"><div class="bold">{l.name}</div><div class="small muted">{l.id === app.ledgerId ? 'Sedang dipakai' : 'Ganti lewat tombol di atas'}</div></div>
        <span class="chev"><Icon name="chevron-right" /></span>
      </button>
    {/each}
  </div>
  <p class="tiny muted">Tiap buku punya akun, kategori, budget, utang, dan laporan sendiri. Transfer antar buku (mis. Work → Pribadi) bisa dicatat lewat jenis Transfer.</p>
</div>

<Sheet open={!!edit} onclose={() => (edit = null)} title={edit?.id ? 'Ubah sumber' : 'Sumber baru'}>
  {#if edit}
    <div class="stack">
      <label class="field"><span>Nama</span><input class="input" bind:value={edit.name} placeholder="mis. Freelance" /></label>
      <label class="field"><span>Ikon (emoji)</span><input class="input" bind:value={edit.icon} maxlength="4" /></label>
      <label class="field"><span>Kata kunci (pisahkan koma)</span><input class="input" bind:value={edit.keywords} placeholder="mis. freelance, proyek, klien" /></label>
      {#if edit.id}
        <label class="row between card tight"><span>Arsipkan</span><span class="toggle"><input type="checkbox" bind:checked={edit.archived} /><span></span></span></label>
      {/if}
      <div class="row">
        {#if edit.id}<button class="btn danger" onclick={remove} aria-label="Hapus"><Icon name="trash" size={20} /></button>{/if}
        <button class="btn primary grow" onclick={save} disabled={busy || !edit.name}>Simpan</button>
      </div>
    </div>
  {/if}
</Sheet>

<Sheet open={!!book} onclose={() => (book = null)} title="Ubah buku">
  {#if book}
    <div class="stack">
      <label class="field"><span>Nama</span><input class="input" bind:value={book.name} /></label>
      <label class="field"><span>Ikon (emoji)</span><input class="input" bind:value={book.icon} maxlength="4" /></label>
      <button class="btn primary" onclick={saveBook} disabled={busy || !book.name}>Simpan</button>
    </div>
  {/if}
</Sheet>

<style>
  .title { font-size: 28px; font-weight: 750; margin-top: -8px; }
  .tight { padding: 12px 14px; }
</style>
