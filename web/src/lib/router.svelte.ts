// Router hash sederhana: #/budget?period=2026-10

export const route = $state({ path: '/', query: {} as Record<string, string> });

function parse(): void {
  const h = location.hash.replace(/^#/, '') || '/';
  const [path, q = ''] = h.split('?');
  route.path = path || '/';
  route.query = Object.fromEntries(new URLSearchParams(q));
}

parse();
window.addEventListener('hashchange', () => {
  parse();
  window.scrollTo(0, 0);
});

export function go(path: string, query?: Record<string, string | number | undefined>): void {
  let h = path;
  if (query) {
    const p = new URLSearchParams();
    for (const [k, v] of Object.entries(query)) if (v !== undefined && v !== '') p.set(k, String(v));
    const s = p.toString();
    if (s) h += '?' + s;
  }
  location.hash = h;
}

export function back(fallback = '/'): void {
  if (history.length > 1) history.back();
  else go(fallback);
}
