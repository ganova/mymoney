export type TxType = 'expense' | 'income' | 'transfer' | 'adjust';

export interface Settings {
  period_start_day: number;
  default_account_id: number;
  ai_autosave: boolean;
  owner_name: string;
}

export interface Account {
  id: number;
  name: string;
  type: 'bank' | 'cash' | 'ewallet' | 'credit' | 'saving' | 'other';
  icon: string;
  color: string;
  opening_balance: number;
  aliases: string;
  archived: boolean;
  sort: number;
  balance: number;
  tx_count: number;
}

export interface Category {
  id: number;
  name: string;
  kind: 'expense' | 'income';
  group: string;
  icon: string;
  color: string;
  keywords: string;
  archived: boolean;
  sort: number;
}

export interface Debt {
  id: number;
  name: string;
  total: number;
  monthly: number;
  due_day: number;
  note: string;
  archived: boolean;
  created_at: string;
  paid: number;
  remaining: number;
  last_paid: string;
}

export interface Transaction {
  id: number;
  date: string;
  type: TxType;
  amount: number;
  account_id: number;
  to_account_id: number | null;
  category_id: number | null;
  debt_id: number | null;
  description: string;
  note: string;
  source: string;
  raw?: string;
  created_at?: string;
}

export interface Draft {
  type: 'expense' | 'income' | 'transfer';
  amount: number;
  amount_estimated: boolean;
  date: string;
  description: string;
  category_id: number | null;
  account_id: number;
  to_account_id: number | null;
  debt_id: number | null;
  note: string;
  raw?: string;
  confidence: number;
  duplicate_of?: number;
  duplicate_in_scan?: boolean;
  source: string;
}

export interface PeriodInfo {
  key: string;
  label: string;
  start: string;
  end: string;
  range: string;
  days: number;
  prev: string;
  next: string;
}

export interface Budget {
  id: number;
  period: string;
  category_id: number;
  amount: number;
  due_day: number;
  account_id: number | null;
  note: string;
  actual: number;
  kind: 'expense' | 'income';
  due_date?: string;
}

export interface CatTotal {
  category_id: number | null;
  kind: string;
  total: number;
  count: number;
}

export interface DayTotal {
  date: string;
  expense: number;
  income: number;
}

export interface Alert {
  level: 'info' | 'warn' | 'danger';
  icon: string;
  text: string;
}

export interface Dashboard {
  period: PeriodInfo;
  today: string;
  is_current: boolean;
  days_left: number;
  days_elapsed: number;
  income: number;
  expense: number;
  net: number;
  prev_income: number;
  prev_expense: number;
  spent_today: number;
  plan_income: number;
  plan_expense: number;
  budget_spent: number;
  unbudgeted: number;
  flex_plan: number;
  flex_spent: number;
  flex_remaining: number;
  daily_allowance: number;
  bills_total: number;
  bills_paid: number;
  by_category: CatTotal[];
  daily: DayTotal[];
  budgets: Budget[];
  accounts: Account[];
  total_balance: number;
  credit_balance: number;
  debt_remaining: number;
  recent: Transaction[];
  alerts: Alert[];
  budget_auto_copied: boolean;
}

export interface Report {
  period: PeriodInfo;
  income: number;
  expense: number;
  by_category: CatTotal[];
  daily: DayTotal[];
  top: { description: string; category_id: number | null; total: number; count: number }[];
  by_account: { account_id: number; expense: number; income: number }[];
  history: { key: string; label: string; income: number; expense: number }[];
}

export interface Rule {
  id: number;
  pattern: string;
  type: string;
  category_id: number | null;
  account_id: number | null;
  hits: number;
  updated_at: string;
}

export interface APIToken {
  id: number;
  name: string;
  prefix: string;
  created_at: string;
  last_used_at: string | null;
}

export interface ExtractResult {
  source: string;
  account_id: number;
  notes: string;
  drafts: Draft[];
}

export interface BudgetLine {
  label: string;
  category_id: number | null;
  new_category: { name: string; group: string; icon: string; kind: 'expense' | 'income' } | null;
  amount: number;
  due_day: number;
  note: string;
}
