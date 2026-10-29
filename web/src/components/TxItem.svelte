<script lang="ts">
  import type { Transaction } from '../lib/types';
  import { acc, anyAccount, cat, debt, source } from '../lib/state.svelte';
  import { rp, shortDate } from '../lib/format';

  let { tx, onclick, showDate = false }: { tx: Transaction; onclick?: () => void; showDate?: boolean } = $props();

  const c = $derived(cat(tx.category_id));
  const a = $derived(acc(tx.account_id));
  const to = $derived(anyAccount(tx.to_account_id));
  const src = $derived(source(tx.income_source_id));
  const d = $derived(debt(tx.debt_id));
  const icon = $derived(tx.type === 'transfer' ? '🔁' : tx.type === 'adjust' ? '⚖️' : (c?.icon ?? (tx.type === 'income' ? '💵' : '🧾')));
  const sub = $derived.by(() => {
    const parts: string[] = [];
    if (tx.type === 'transfer') parts.push(`${a?.name ?? '?'} → ${to?.name ?? '?'}`);
    else {
      if (src) parts.push(src.name);
      if (c) parts.push(c.name);
      if (a) parts.push(a.name);
    }
    if (d) parts.push(d.name);
    if (showDate) parts.unshift(shortDate(tx.date));
    return parts.join(' · ');
  });
  const sign = $derived(tx.type === 'income' ? '+' : tx.type === 'expense' ? '−' : tx.type === 'adjust' ? (tx.amount >= 0 ? '+' : '−') : '');
</script>

<button class="list-item" {onclick}>
  <div class="bubble">{icon}</div>
  <div class="grow">
    <div class="ellipsis desc">{tx.description || c?.name || 'Transaksi'}</div>
    <div class="ellipsis small muted">{sub}</div>
  </div>
  <div class="amt num" class:inc={tx.type === 'income'} class:trf={tx.type === 'transfer' || tx.type === 'adjust'}>
    {sign}{rp(Math.abs(tx.amount)).replace('-', '')}
  </div>
</button>

<style>
  .desc { font-weight: 550; }
  .amt { font-weight: 650; white-space: nowrap; }
  .inc { color: var(--good); }
  .trf { color: var(--text-2); }
</style>
