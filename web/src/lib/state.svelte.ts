import type { Account, Category, Debt, PeriodInfo, Settings, Transaction } from './types';
import { api } from './api';

interface Bootstrap {
  settings: Settings;
  accounts: Account[];
  categories: Category[];
  debts: Debt[];
  period: PeriodInfo;
  today: string;
  ai: { enabled: boolean; model: string; ocr: boolean };
  auth_required: boolean;
  version: string;
}

export const app = $state({
  ready: false,
  authRequired: false,
  authenticated: false,
  error: '',
  settings: { period_start_day: 28, default_account_id: 0, ai_autosave: false, owner_name: '' } as Settings,
  accounts: [] as Account[],
  categories: [] as Category[],
  debts: [] as Debt[],
  period: null as PeriodInfo | null,
  today: '',
  ai: { enabled: false, model: '', ocr: false },
  version: '',
  /** naik setiap kali data berubah, agar halaman memuat ulang */
  dataVersion: 0,
});

export interface ToastState {
  id: number;
  text: string;
  kind: 'ok' | 'error' | 'info';
  action?: { label: string; run: () => void };
}

export const ui = $state({
  toast: null as ToastState | null,
  /** editor transaksi global: tx = edit, preset = baru dengan isian awal */
  txEditor: null as null | { tx?: Transaction; preset?: Partial<Transaction> },
  /** screenshot yang dipilih dari Beranda, diproses di halaman Asisten */
  pendingScan: null as File[] | null,
});

export async function bootstrap(): Promise<void> {
  try {
    const st = await api.get<{ auth_required: boolean; authenticated: boolean }>('/auth/status');
    app.authRequired = st.auth_required;
    app.authenticated = true;
    await loadBootstrap();
    app.error = '';
  } catch (e) {
    app.error = (e as Error).message;
  } finally {
    app.ready = true;
  }
}

export async function loadBootstrap(): Promise<void> {
  const b = await api.get<Bootstrap>('/bootstrap');
  app.settings = b.settings;
  app.accounts = b.accounts;
  app.categories = b.categories;
  app.debts = b.debts;
  app.period = b.period;
  app.today = b.today;
  app.ai = b.ai;
  app.authRequired = b.auth_required;
  app.version = b.version;
  app.authenticated = true;
}

/** Panggil setelah ada perubahan data. */
export async function changed(): Promise<void> {
  app.dataVersion++;
  try {
    const [accounts, categories, debts] = await Promise.all([
      api.get<Account[]>('/accounts'),
      api.get<Category[]>('/categories'),
      api.get<Debt[]>('/debts'),
    ]);
    app.accounts = accounts;
    app.categories = categories;
    app.debts = debts;
  } catch {
    /* abaikan; halaman akan menampilkan error sendiri */
  }
}

export function acc(id: number | null | undefined): Account | undefined {
  return id ? app.accounts.find((a) => a.id === id) : undefined;
}

export function cat(id: number | null | undefined): Category | undefined {
  return id ? app.categories.find((c) => c.id === id) : undefined;
}

export function debt(id: number | null | undefined): Debt | undefined {
  return id ? app.debts.find((d) => d.id === id) : undefined;
}

export function activeAccounts(): Account[] {
  return app.accounts.filter((a) => !a.archived);
}

export function activeCategories(kind?: 'expense' | 'income'): Category[] {
  return app.categories.filter((c) => !c.archived && (!kind || c.kind === kind));
}

let toastTimer: ReturnType<typeof setTimeout> | undefined;
let toastSeq = 0;

export function toast(text: string, opts: { kind?: ToastState['kind']; action?: ToastState['action']; ms?: number } = {}): void {
  clearTimeout(toastTimer);
  ui.toast = { id: ++toastSeq, text, kind: opts.kind ?? 'ok', action: opts.action };
  toastTimer = setTimeout(() => (ui.toast = null), opts.ms ?? (opts.action ? 6000 : 3000));
}

export function toastError(e: unknown): void {
  toast((e as Error)?.message || String(e), { kind: 'error', ms: 5000 });
}

export function openTx(arg: { tx?: Transaction; preset?: Partial<Transaction> } = {}): void {
  ui.txEditor = arg;
}

export function haptic(): void {
  try {
    navigator.vibrate?.(10);
  } catch {
    /* iOS tidak mendukung vibrate; tidak apa-apa */
  }
}
