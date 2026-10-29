
export class ApiError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}

/** Buku keuangan aktif (Pribadi/Work), dikirim ke server di setiap permintaan. */
const LEDGER_KEY = 'mymoney.ledger';
let ledgerId = 0;
try {
  ledgerId = Number(localStorage.getItem(LEDGER_KEY)) || 0;
} catch {
  /* penyimpanan diblokir; pakai default */
}
export function getLedger(): number {
  return ledgerId;
}
export function setLedger(id: number): void {
  ledgerId = id;
  try {
    localStorage.setItem(LEDGER_KEY, String(id));
  } catch {
    /* abaikan */
  }
}

async function request<T>(method: string, path: string, body?: unknown, signal?: AbortSignal): Promise<T> {
  const init: RequestInit = { method, credentials: 'same-origin', signal, headers: {} };
  if (ledgerId > 0) (init.headers as Record<string, string>)['X-Ledger'] = String(ledgerId);
  if (body instanceof FormData) {
    init.body = body;
  } else if (body !== undefined) {
    init.body = JSON.stringify(body);
    (init.headers as Record<string, string>)['Content-Type'] = 'application/json';
  }
  let res: Response;
  try {
    res = await fetch('/api' + path, init);
  } catch (e) {
    if ((e as Error).name === 'AbortError') throw e;
    throw new ApiError('Tidak bisa terhubung ke server. Cek koneksi.', 0);
  }
  const text = await res.text();
  let data: any = null;
  try {
    data = text ? JSON.parse(text) : null;
  } catch {
    data = text;
  }
  if (!res.ok) {
    const msg = (data && typeof data === 'object' && data.error) || (typeof data === 'string' && data) || `Gagal (${res.status})`;
    throw new ApiError(msg, res.status);
  }
  return data as T;
}

export const api = {
  get: <T>(path: string, signal?: AbortSignal) => request<T>('GET', path, undefined, signal),
  post: <T>(path: string, body?: unknown, signal?: AbortSignal) => request<T>('POST', path, body ?? {}, signal),
  put: <T>(path: string, body?: unknown) => request<T>('PUT', path, body ?? {}),
  del: <T>(path: string) => request<T>('DELETE', path),
};

export function qs(params: Record<string, string | number | undefined | null | false>): string {
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== null && v !== '' && v !== false && v !== 0) p.set(k, String(v));
  }
  const s = p.toString();
  return s ? '?' + s : '';
}
