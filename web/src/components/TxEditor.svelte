<script lang="ts">
  import { ui, toast } from '../lib/state.svelte';
  import Sheet from './Sheet.svelte';
  import TxForm from './TxForm.svelte';

  const ed = $derived(ui.txEditor);
  const title = $derived(ed?.tx ? 'Ubah transaksi' : 'Transaksi baru');
  function close() {
    ui.txEditor = null;
  }
</script>

<Sheet open={!!ed} onclose={close} {title}>
  {#if ed}
    <TxForm
      initial={ed.tx ?? ed.preset ?? {}}
      onsaved={() => {
        toast(ed?.tx ? 'Perubahan disimpan' : 'Transaksi tersimpan');
        close();
      }}
      ondelete={close}
    />
  {/if}
</Sheet>
