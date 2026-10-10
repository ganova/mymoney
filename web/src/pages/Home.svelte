<script lang="ts">
  import type { Dashboard, Budget } from '../lib/types';
  import { app, ui, cat, openTx, toastError } from '../lib/state.svelte';
  import { api, qs } from '../lib/api';
  import { go } from '../lib/router.svelte';
  import { rp, pct, greeting, shortDate, markdown } from '../lib/format';
  import Icon from '../components/Icon.svelte';
  import TxItem from '../components/TxItem.svelte';
  import DailyBars from '../components/DailyBars.svelte';

  let periodKey = $state(app.period?.key ?? '');
  let d = $state<Dashboard | null>(null);
  let loading = $state(true);
  let text = $state('');
  let fileInput: HTMLInputElement | undefined = $state();
  let insight = $state<{ content: string; created_at: string } | null>(null);
  let insightBusy = $state(false);

  $effect(() => {
    app.dataVersion;
    const key = periodKey;
    const ctrl = new AbortController();
    loading = true;
    api
      .get<Dashboard>('/dashboard' + qs({ period: key }), ctrl.signal)
      .then((r) => (d = r))
      .catch((e) => e.name !== 'AbortError' && toastError(e))
      .finally(() => (loading = false));
    api
      .get<{ insight: typeof insight }>('/ai/insight' + qs({ period: key }), ctrl.signal)
      .then((r) => (insight = r.insight))
      .catch(() => {});
    return () => ctrl.abort();
  });

  const bills = $derived((d?.budgets ?? []).filter((b) => b.kind === 'expense' && b.due_day > 0).sort((a, b) => (a.due_date ?? '').localeCompare(b.due_date ?? '')));
  const topBudgets = $derived(
    (d?.budgets ?? [])
      .filter((b) => b.kind === 'expense' && b.due_day === 0 && b.amount > 0)
      .sort((a, b) => b.actual / b.amount - a.actual / a.amount)
      .slice(0, 4),
  );
  const accounts = $derived((d?.accounts ?? []).filter((a) => !a.archived));
  const hasBudget = $derived((d?.plan_expense ?? 0) > 0);
  const expDelta = $derived(d && d.prev_expense ? Math.round(((d.expense - d.prev_expense) / d.prev_expense) * 100) : null);
  const todayPct = $derived(d && d.daily_allowance ? Math.min(100, pct(d.spent_today, d.daily_allowance)) : 0);

  function submit(e: Event) {
    e.preventDefault();
    const t = text.trim();
    if (!t) return go('/ai');
    go('/ai', { q: t });
  }

  function pickFiles(e: Event) {
    const files = Array.from((e.currentTarget as HTMLInputElement).files ?? []);
    if (!files.length) return;
    ui.pendingScan = files;
    go('/ai');
  }

  function payBill(b: Budget) {
    const c = cat(b.category_id);
    openTx({
      preset: {
        type: 'expense', category_id: b.category_id, amount: Math.max(b.amount - b.actual, 0),
        account_id: b.account_id ?? app.settings.default_account_id, description: c?.name ?? '', source: 'bill',
      },
    });
  }

  async function genInsight() {
    insightBusy = true;
    try {
      const r = await api.post<{ insight: typeof insight }>('/ai/insight' + qs({ period: periodKey }));
      insight = r.insight;
    } catch (e) {
      toastError(e);
    } finally {
      insightBusy = false;
    }
  }

  function billStatus(b: Budget): { cls: string; text: string } {
    if (b.actual >= b.amount) return { cls: 'good', text: '✓ Lunas' };
    if (b.actual > 0) return { cls: 'warn', text: 'Sebagian' };
    if (d?.is_current && b.due_date && b.due_date < d.today) return { cls: 'bad', text: '⏰ Lewat' };
    return { cls: '', text: b.due_date ? shortDate(b.due_date) : 'Belum' };
  }
</script>

<div class="page stack">
  <header class="row between head">
    <div class="grow">
      <div class="small muted">{greeting()}{app.settings.owner_name ? `, ${app.settings.owner_name}` : ''} 👋</div>
      <div class="row pchip">
        <button class="icon-btn sm" onclick={() => d && (periodKey = d.period.prev)} aria-label="Periode sebelumnya"><Icon name="chevron-left" size={18} /></button>
        <button class="ptitle" onclick={() => (periodKey = app.period?.key ?? '')}>
          <span class="bold">{d?.period.label ?? app.period?.label}</span>
          <span class="tiny muted">{d?.period.range ?? app.period?.range}</span>
        </button>
        <button class="icon-btn sm" onclick={() => d && (periodKey = d.period.next)} aria-label="Periode berikutnya"><Icon name="chevron-right" size={18} /></button>
      </div>
    </div>
    <button class="icon-btn filled" onclick={() => go('/settings')} aria-label="Pengaturan"><Icon name="settings" /></button>
  </header>

  <!-- Bilah AI -->
  <form class="aibar" onsubmit={submit}>
    <span class="spark"><Icon name="sparkles" size={20} /></span>
    <input
      class="grow"
      bind:value={text}
      placeholder="Catat atau tanya… mis. makan siang 25rb"
      enterkeyhint="send"
      autocomplete="off"
      aria-label="Catat transaksi atau tanya AI"
    />
    <button type="button" class="icon-btn" onclick={() => fileInput?.click()} aria-label="Scan screenshot mutasi"><Icon name="camera" /></button>
    <button type="submit" class="send" aria-label="Kirim"><Icon name="up" size={20} stroke={2.5} /></button>
    <input bind:this={fileInput} type="file" accept="image/*" multiple class="hidden" onchange={pickFiles} />
  </form>

  {#if !d && loading}
    <div class="card skeleton" style="height:170px"></div>
    <div class="card skeleton" style="height:90px"></div>
  {:else if d}
    <!-- Kartu utama -->
    <section class="card hero" class:dim={loading}>
      {#if hasBudget && d.is_current}
        <div class="small muted">Jatah aman per hari</div>
        <div class="big num">{rp(d.daily_allowance)}</div>
        <div class="row between small">
          <span class="text-2">Hari ini terpakai <strong class="num">{rp(d.spent_today)}</strong></span>
          {#if d.spent_today > d.daily_allowance && d.daily_allowance > 0}
            <span class="badge bad">Lewat {rp(d.spent_today - d.daily_allowance)}</span>
          {/if}
        </div>
        <div class="meter" class:over={d.spent_today > d.daily_allowance}><i style:width="{todayPct}%"></i></div>
        <div class="row between tiny muted foot">
          <span>Sisa uang fleksibel {rp(d.flex_remaining)}</span>
          <span>{d.days_left} hari lagi</span>
        </div>
      {:else}
        <div class="small muted">Total saldo semua akun</div>
        <div class="big num">{rp(d.total_balance)}</div>
        {#if d.debt_remaining > 0 || d.credit_balance < 0}
          <div class="small text-2">Utang/cicilan tersisa {rp(d.debt_remaining - Math.min(d.credit_balance, 0))}</div>
        {/if}
        {#if !hasBudget}
          <button class="link small cta" onclick={() => go('/budget')}>Atur budget untuk dapat jatah harian otomatis →</button>
        {/if}
      {/if}
    </section>

    <!-- KPI -->
    <section class="kpis">
      <div class="card kpi">
        <div class="tiny muted">Pemasukan</div>
        <div class="v num good">{rp(d.income)}</div>
        {#if d.plan_income}<div class="tiny muted">dari {rp(d.plan_income)}</div>{/if}
      </div>
      <div class="card kpi">
        <div class="tiny muted">Pengeluaran</div>
        <div class="v num">{rp(d.expense)}</div>
        {#if expDelta !== null}
          <div class="tiny" class:bad={expDelta > 0} class:good={expDelta <= 0}>{expDelta > 0 ? '▲' : '▼'} {Math.abs(expDelta)}% vs lalu</div>
        {/if}
      </div>
      <div class="card kpi">
        <div class="tiny muted">Selisih</div>
        <div class="v num" class:bad={d.net < 0}>{rp(d.net)}</div>
        {#if d.plan_expense}<div class="tiny muted">budget {pct(d.budget_spent, d.plan_expense)}%</div>{/if}
      </div>
    </section>

    {#if d.alerts.length}
      <section class="stack alerts">
        {#each d.alerts as a, i (i)}
          <div class="alert {a.level}"><span>{a.icon}</span><span class="grow small">{a.text}</span></div>
        {/each}
      </section>
    {/if}

    {#if bills.length}
      <section class="card">
        <div class="card-title">Tagihan periode ini <span class="tiny">{rp(d.bills_paid)} / {rp(d.bills_total)}</span></div>
        <div class="bills">
          {#each bills as b (b.id)}
            {@const st = billStatus(b)}
            {@const c = cat(b.category_id)}
            <div class="bill row">
              <div class="bubble sm">{c?.icon ?? '🧾'}</div>
              <div class="grow">
                <div class="ellipsis">{c?.name}</div>
                <div class="tiny muted num">{rp(b.amount)}{b.actual > 0 && b.actual < b.amount ? ` · dibayar ${rp(b.actual)}` : ''}</div>
              </div>
              {#if b.actual < b.amount && d.is_current}
                <span class="badge {st.cls}">{st.text}</span>
                <button class="btn sm primary" onclick={() => payBill(b)}>Bayar</button>
              {:else}
                <span class="badge {st.cls}">{st.text}</span>
              {/if}
            </div>
          {/each}
        </div>
      </section>
    {/if}

    {#if topBudgets.length}
      <section class="card">
        <div class="card-title">Budget fleksibel <button onclick={() => go('/budget', { period: periodKey })}>Semua</button></div>
        <div class="stack">
          {#each topBudgets as b (b.id)}
            {@const c = cat(b.category_id)}
            {@const p = pct(b.actual, b.amount)}
            <button class="bud" onclick={() => go('/tx', { period: periodKey, category: b.category_id })}>
              <div class="row between small">
                <span class="row ellipsis"><span>{c?.icon}</span> {c?.name}</span>
                <span class="num"><strong>{rp(b.actual)}</strong> <span class="muted">/ {rp(b.amount)}</span></span>
              </div>
              <div class="meter" class:over={b.actual > b.amount}><i style:width="{Math.min(p, 100)}%"></i></div>
              <div class="tiny" class:bad={b.actual > b.amount} class:muted={b.actual <= b.amount}>
                {b.actual > b.amount ? `Lewat ${rp(b.actual - b.amount)}` : `Sisa ${rp(b.amount - b.actual)} · ${p}%`}
              </div>
            </button>
          {/each}
        </div>
      </section>
    {/if}

    <section>
      <div class="section-label row between"><span>Akun</span><button class="link small" onclick={() => go('/accounts')}>Kelola</button></div>
      <div class="accs">
        <div class="acc total">
          <div class="tiny">Total</div>
          <div class="bold num">{rp(d.total_balance)}</div>
        </div>
        {#each accounts as a (a.id)}
          <button class="acc" onclick={() => go('/tx', { account: a.id })}>
            <div class="tiny muted ellipsis">{a.icon} {a.name}</div>
            <div class="bold num" class:bad={a.balance < 0}>{rp(a.balance)}</div>
          </button>
        {/each}
      </div>
    </section>

    {#if d.daily.length}
      <section class="card">
        <div class="card-title">Pengeluaran harian</div>
        <DailyBars daily={d.daily} today={d.today} allowance={d.is_current ? d.daily_allowance : 0} />
      </section>
    {/if}

    <section>
      <div class="section-label row between"><span>Transaksi terakhir</span><button class="link small" onclick={() => go('/tx')}>Semua</button></div>
      {#if d.recent.length}
        <div class="list">
          {#each d.recent as t (t.id)}
            <TxItem tx={t} showDate onclick={() => openTx({ tx: t })} />
          {/each}
        </div>
      {:else}
        <div class="card empty">
          <div class="big">✨</div>
          <p>Belum ada transaksi. Coba ketik <strong>“makan siang 25rb”</strong> di atas, atau scan screenshot mutasi bank.</p>
        </div>
      {/if}
    </section>

    {#if app.ai.enabled}
      <section class="card">
        <div class="card-title">
          <span class="row" style="gap:6px"><Icon name="brain" size={16} /> Analisis AI</span>
          <button onclick={genInsight} disabled={insightBusy}>{insight ? 'Perbarui' : 'Buat'}</button>
        </div>
        {#if insightBusy}
          <div class="row muted small"><span class="spinner"></span> Menganalisis keuanganmu…</div>
        {:else if insight}
          <div class="md small">{@html markdown(insight.content)}</div>
          <div class="tiny muted" style="margin-top:8px">Dibuat {insight.created_at.slice(0, 16)} UTC</div>
        {:else}
          <p class="small muted">Dapatkan ringkasan pola belanja dan saran hemat untuk periode ini.</p>
        {/if}
      </section>
    {/if}
  {/if}
</div>

<style>
  .head { align-items: flex-start; }
  .pchip { gap: 0; margin-left: -10px; }
  .icon-btn.sm { width: 32px; height: 36px; }
  .ptitle { display: flex; flex-direction: column; align-items: flex-start; line-height: 1.2; font-size: 20px; }
  .ptitle .bold { font-size: 22px; font-weight: 750; letter-spacing: -0.01em; }
  .aibar {
    display: flex; align-items: center; gap: 4px; padding: 4px 4px 4px 12px; border-radius: 26px;
    background: var(--surface); box-shadow: var(--shadow); border: 1.5px solid transparent;
  }
  .aibar:focus-within { border-color: var(--accent); }
  .aibar input { border: 0; background: transparent; min-height: 44px; font-size: 16px; outline: none; padding: 0 4px; }
  .spark { color: #6250d6; }
  .send { width: 40px; height: 40px; border-radius: 20px; background: var(--accent); color: #fff; display: grid; place-items: center; flex: none; }
  .hero { padding: 18px; display: flex; flex-direction: column; gap: 8px; transition: opacity 0.2s; }
  .hero .big { font-size: 40px; font-weight: 750; letter-spacing: -0.03em; line-height: 1.1; }
  .hero .foot { margin-top: 2px; }
  .cta { text-align: left; }
  .dim { opacity: 0.6; }
  .kpis { display: grid; grid-template-columns: repeat(3, 1fr); gap: 8px; }
  .kpi { padding: 12px; min-width: 0; }
  .kpi .v { font-weight: 700; font-size: 15px; margin: 2px 0; overflow-wrap: anywhere; }
  .alerts { gap: 8px; }
  .alert { display: flex; gap: 10px; align-items: flex-start; padding: 12px 14px; border-radius: 14px; background: var(--surface); box-shadow: var(--shadow); }
  .alert.warn { background: var(--warn-soft); }
  .alert.danger { background: var(--bad-soft); }
  .alert.info { background: var(--accent-soft); }
  .bills { display: flex; flex-direction: column; gap: 10px; }
  .bud { text-align: left; display: flex; flex-direction: column; gap: 6px; width: 100%; }
  .bud .row { gap: 6px; }
  .accs { display: flex; gap: 8px; overflow-x: auto; scrollbar-width: none; margin: 0 -16px; padding: 2px 16px 6px; }
  .accs::-webkit-scrollbar { display: none; }
  .acc { flex: none; min-width: 128px; max-width: 170px; text-align: left; background: var(--surface); border-radius: 14px; padding: 12px; box-shadow: var(--shadow); }
  .acc.total { background: var(--accent); color: #fff; }
  .acc.total .tiny { opacity: 0.85; }
</style>
