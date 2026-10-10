<script lang="ts">
  import { thousands } from '../lib/format';

  let {
    value = $bindable(0),
    autofocus = false,
    big = true,
    placeholder = '0',
    highlight = false,
  }: { value?: number; autofocus?: boolean; big?: boolean; placeholder?: string; highlight?: boolean } = $props();

  let el: HTMLInputElement | undefined = $state();

  $effect(() => {
    if (autofocus && el) setTimeout(() => el?.focus(), 250);
  });

  function onInput(e: Event) {
    const t = e.currentTarget as HTMLInputElement;
    const digits = t.value.replace(/\D/g, '').slice(0, 13);
    value = Number(digits) || 0;
    t.value = thousands(value);
  }
</script>

<div class="amount" class:big class:highlight>
  <span class="cur">Rp</span>
  <input
    bind:this={el}
    class={big ? 'amount-input' : 'input num'}
    inputmode="numeric"
    enterkeyhint="done"
    autocomplete="off"
    {placeholder}
    value={thousands(value)}
    style:width={big ? `${Math.max(1, (thousands(value) || placeholder).length) * 0.62 + 0.4}em` : undefined}
    oninput={onInput}
    aria-label="Nominal"
  />
</div>

<style>
  .amount { display: flex; align-items: baseline; justify-content: center; gap: 6px; }
  .amount:not(.big) { align-items: center; }
  .amount:not(.big) .cur { position: absolute; margin-left: 14px; color: var(--muted); }
  .amount:not(.big) { position: relative; justify-content: flex-start; }
  .amount:not(.big) input { padding-left: 42px; }
  .cur { font-size: 20px; font-weight: 650; color: var(--muted); }
  .big input { max-width: 82%; min-width: 1.2em; text-align: left; }
  .highlight { border-radius: 14px; background: var(--warn-soft); }
</style>
