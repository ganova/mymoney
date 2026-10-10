<script lang="ts">
  import type { Category } from '../lib/types';
  import { app, changed, toast, toastError } from '../lib/state.svelte';
  import { api } from '../lib/api';
  import { back } from '../lib/router.svelte';
  import Icon from '../components/Icon.svelte';
  import Sheet from '../components/Sheet.svelte';

  let kind = $state<'expense' | 'income'>('expense');
  let edit = $state<Partial<Category> | null>(null);
  let busy = $state(false);

  const groups = $derived.by(() => {
    const m = new Map<string, Category[]>();
    for (const c of app.categories.filter((c) => c.kind === kind && !c.archived)) {
      if (!m.has(c.group)) m.set(c.group, []);
      m.get(c.group)!.push(c);
    }
    return [...m.entries()];
  });
  const archived = $derived(app.categories.filter((c) => c.kind === kind && c.archived));
  const groupNames = $derived([...new Set(app.categories.filter((c) => c.kind === kind).map((c) => c.group))]);

  function open(c?: Category) {
    edit = c ? { ...c } : { name: '', kind, group: groupNames[0] ?? '', icon: '📌', keywords: '', archived: false };
  }

  async function save() {
    if (!edit) return;
    busy = true;
    try {
      if (edit.id) await api.put(`/categories/${edit.id}`, edit);
      else await api.post('/categories', edit);
      edit = null;
      await changed();
      toast('Kategori disimpan');
    } catch (e) {
      toastError(e);
    } finally {
      busy = false;
    }
  }

  async function remove() {
    if (!edit?.id || !confirm(`Hapus kategori ${edit.name}? Transaksinya akan menjadi tanpa kategori. (Lebih aman: arsipkan.)`)) return;
    try {
      await api.del(`/categories/${edit.id}`);
      edit = null;
      await changed();
    } catch (e) {
      toastError(e);
    }
  }
</script>

<div class="page stack">
  <header class="page-head">
    <button class="back" onclick={() => back('/more')}><Icon name="chevron-left" /> Lainnya</button>
    <button class="btn sm primary" onclick={() => open()}><Icon name="plus" size={18} /> Kategori</button>
  </header>
  <h1 class="title">Kategori</h1>
  <div class="seg">
    <button class:on={kind === 'expense'} onclick={() => (kind = 'expense')}>Pengeluaran</button>
    <button class:on={kind === 'income'} onclick={() => (kind = 'income')}>Pemasukan</button>
  </div>
  <p class="tiny muted">Kata kunci membantu AI & mode offline mengenali kategori. AI juga belajar otomatis dari koreksimu.</p>

  {#each groups as [g, list] (g)}
    <div class="section-label">{g}</div>
    <div class="list">
      {#each list as c (c.id)}
        <button class="list-item" onclick={() => open(c)}>
          <div class="bubble">{c.icon}</div>
          <div class="grow">
            <div>{c.name}</div>
            {#if c.keywords}<div class="tiny muted ellipsis">{c.keywords.split(',').join(', ')}</div>{/if}
          </div>
          <span class="chev"><Icon name="chevron-right" /></span>
        </button>
      {/each}
    </div>
  {/each}

  {#if archived.length}
    <div class="section-label">Diarsipkan</div>
    <div class="list">
      {#each archived as c (c.id)}
        <button class="list-item" onclick={() => open(c)}><div class="bubble">{c.icon}</div><div class="grow muted">{c.name}</div></button>
      {/each}
    </div>
  {/if}
</div>

<Sheet open={!!edit} onclose={() => (edit = null)} title={edit?.id ? 'Ubah kategori' : 'Kategori baru'}>
  {#if edit}
    <div class="stack">
      <div class="row">
        <input class="input emoji" bind:value={edit.icon} maxlength="4" aria-label="Ikon" />
        <input class="input grow" bind:value={edit.name} placeholder="Nama kategori" />
      </div>
      <div class="seg">
        <button class:on={edit.kind === 'expense'} onclick={() => edit && (edit.kind = 'expense')}>Pengeluaran</button>
        <button class:on={edit.kind === 'income'} onclick={() => edit && (edit.kind = 'income')}>Pemasukan</button>
      </div>
      <label class="field">
        <span>Grup</span>
        <input class="input" bind:value={edit.group} list="groups" placeholder="mis. Tagihan Bulanan" />
        <datalist id="groups">{#each groupNames as g (g)}<option value={g}></option>{/each}</datalist>
      </label>
      <label class="field">
        <span>Kata kunci (pisahkan koma)</span>
        <textarea class="input" rows="3" bind:value={edit.keywords} placeholder="alfamart, indomaret"></textarea>
      </label>
      {#if edit.id}
        <label class="row between card tight">
          <span>Arsipkan</span>
          <span class="toggle"><input type="checkbox" bind:checked={edit.archived} /><span></span></span>
        </label>
      {/if}
      <div class="row">
        {#if edit.id}<button class="btn danger" onclick={remove} aria-label="Hapus"><Icon name="trash" size={20} /></button>{/if}
        <button class="btn primary grow" onclick={save} disabled={busy || !edit.name}>Simpan</button>
      </div>
    </div>
  {/if}
</Sheet>

<style>
  .title { font-size: 28px; font-weight: 750; margin-top: -8px; }
  .emoji { width: 60px; text-align: center; font-size: 22px; flex: none; }
  .tight { padding: 12px 14px; }
</style>
