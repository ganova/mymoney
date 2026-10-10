const nf = new Intl.NumberFormat('id-ID', { maximumFractionDigits: 0 });

/** Rp25.000 */
export function rp(n: number): string {
  const s = nf.format(Math.abs(Math.round(n)));
  return (n < 0 ? '-Rp' : 'Rp') + s;
}

/** Rp11,2jt untuk nominal ≥ 10 juta, selain itu lengkap — untuk kartu sempit */
export function rpc(n: number): string {
  const a = Math.abs(n);
  if (a < 10_000_000) return rp(n);
  const s = (a / 1e6).toFixed(a >= 100_000_000 ? 0 : 1).replace('.', ',').replace(/,0$/, '');
  return (n < 0 ? '-Rp' : 'Rp') + s + 'jt';
}

/** 25rb, 1,2jt — untuk label sempit */
export function rpShort(n: number): string {
  const a = Math.abs(n);
  const sign = n < 0 ? '-' : '';
  if (a >= 1e9) return sign + trim(a / 1e9) + 'M';
  if (a >= 1e6) return sign + trim(a / 1e6) + 'jt';
  if (a >= 1e3) return sign + trim(a / 1e3) + 'rb';
  return sign + String(a);
}

function trim(v: number): string {
  return (v >= 100 ? Math.round(v).toString() : v.toFixed(1).replace(/\.0$/, '')).replace('.', ',');
}

/** "25.000" tanpa Rp, untuk input */
export function thousands(n: number): string {
  return n ? nf.format(n) : '';
}

/** Membaca input nominal: "25.000", "25rb", "1,5jt" */
export function parseAmount(s: string): number {
  if (!s) return 0;
  const t = s.toLowerCase().replace(/rp\.?/g, '').replace(/\s+/g, '');
  const m = t.match(/^([\d.,]+)(rb|ribu|k|jt|juta|m)?$/);
  if (!m) return Number(t.replace(/\D/g, '')) || 0;
  let num = m[1];
  const unit = m[2];
  const mult = unit === 'rb' || unit === 'ribu' || unit === 'k' ? 1e3 : unit === 'jt' || unit === 'juta' ? 1e6 : unit === 'm' ? 1e9 : 1;
  if (mult === 1) return Number(num.replace(/[.,]/g, '')) || 0;
  num = num.replace(/\./g, '').replace(',', '.');
  return Math.round((Number(num) || 0) * mult);
}

const hari = ['Min', 'Sen', 'Sel', 'Rab', 'Kam', 'Jum', 'Sab'];
const hariPanjang = ['Minggu', 'Senin', 'Selasa', 'Rabu', 'Kamis', 'Jumat', 'Sabtu'];
const bulan = ['Jan', 'Feb', 'Mar', 'Apr', 'Mei', 'Jun', 'Jul', 'Agu', 'Sep', 'Okt', 'Nov', 'Des'];
const bulanPanjang = ['Januari', 'Februari', 'Maret', 'April', 'Mei', 'Juni', 'Juli', 'Agustus', 'September', 'Oktober', 'November', 'Desember'];

export function parseDate(s: string): Date {
  const [y, m, d] = s.split('-').map(Number);
  return new Date(y, m - 1, d);
}

export function isoDate(d: Date): string {
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

export function addDays(s: string, n: number): string {
  const d = parseDate(s);
  d.setDate(d.getDate() + n);
  return isoDate(d);
}

/** "10 Okt" */
export function shortDate(s: string): string {
  const d = parseDate(s);
  return `${d.getDate()} ${bulan[d.getMonth()]}`;
}

/** "Sab, 10 Okt 2026" */
export function longDate(s: string): string {
  const d = parseDate(s);
  return `${hari[d.getDay()]}, ${d.getDate()} ${bulan[d.getMonth()]} ${d.getFullYear()}`;
}

/** "Hari ini", "Kemarin", "Sabtu, 10 Oktober" */
export function dayLabel(s: string, today: string): string {
  if (s === today) return 'Hari ini';
  if (s === addDays(today, -1)) return 'Kemarin';
  const d = parseDate(s);
  const t = parseDate(today);
  const sameYear = d.getFullYear() === t.getFullYear();
  return `${hariPanjang[d.getDay()]}, ${d.getDate()} ${bulanPanjang[d.getMonth()]}${sameYear ? '' : ' ' + d.getFullYear()}`;
}

export function pct(part: number, whole: number): number {
  if (!whole) return 0;
  return Math.round((part / whole) * 100);
}

export function greeting(): string {
  const h = new Date().getHours();
  if (h < 11) return 'Selamat pagi';
  if (h < 15) return 'Selamat siang';
  if (h < 19) return 'Selamat sore';
  return 'Selamat malam';
}

/** Markdown sederhana → HTML aman (judul, tebal, miring, daftar, paragraf). */
export function markdown(src: string): string {
  const esc = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  const inline = (s: string) =>
    esc(s)
      .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
      .replace(/(^|[^*])\*(?!\s)(.+?)\*/g, '$1<em>$2</em>')
      .replace(/`(.+?)`/g, '<code>$1</code>');
  const out: string[] = [];
  let list: string[] | null = null;
  const flush = () => {
    if (list) {
      out.push('<ul>' + list.map((li) => `<li>${li}</li>`).join('') + '</ul>');
      list = null;
    }
  };
  for (const raw of src.split('\n')) {
    const line = raw.trim();
    if (!line) {
      flush();
      continue;
    }
    const h = line.match(/^#{1,4}\s+(.*)$/);
    const li = line.match(/^(?:[-*•]|\d+[.)])\s+(.*)$/);
    if (h) {
      flush();
      out.push(`<h3>${inline(h[1])}</h3>`);
    } else if (li) {
      (list ??= []).push(inline(li[1]));
    } else {
      flush();
      out.push(`<p>${inline(line)}</p>`);
    }
  }
  flush();
  return out.join('');
}
