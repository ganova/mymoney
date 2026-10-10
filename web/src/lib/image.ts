// Mengecilkan gambar di perangkat sebelum diunggah: hemat kuota & lebih cepat.
// Batas mengikuti resolusi tinggi Claude (2576 px sisi panjang, ±3,7 MP).
const MAX_EDGE = 2576;
const MAX_PIXELS = 3_700_000;

export async function prepareImage(file: File): Promise<Blob> {
  // HEIC dari iPhone biasanya sudah dikonversi Safari saat dipilih; tetap kita
  // gambar ulang ke JPEG agar formatnya pasti didukung.
  const bitmap = await loadBitmap(file);
  let { width, height } = bitmap;
  const scale = Math.min(1, MAX_EDGE / Math.max(width, height), Math.sqrt(MAX_PIXELS / (width * height)));
  width = Math.round(width * scale);
  height = Math.round(height * scale);
  const canvas = document.createElement('canvas');
  canvas.width = width;
  canvas.height = height;
  const ctx = canvas.getContext('2d');
  if (!ctx) return file;
  ctx.fillStyle = '#fff';
  ctx.fillRect(0, 0, width, height);
  ctx.drawImage(bitmap, 0, 0, width, height);
  if ('close' in bitmap) (bitmap as ImageBitmap).close();
  return await new Promise<Blob>((resolve) => canvas.toBlob((b) => resolve(b ?? file), 'image/jpeg', 0.86));
}

async function loadBitmap(file: File): Promise<ImageBitmap | HTMLImageElement> {
  if ('createImageBitmap' in window) {
    try {
      return await createImageBitmap(file);
    } catch {
      /* lanjut ke cara lama */
    }
  }
  return await new Promise((resolve, reject) => {
    const img = new Image();
    img.onload = () => resolve(img);
    img.onerror = () => reject(new Error('Gambar tidak bisa dibaca'));
    img.src = URL.createObjectURL(file);
  });
}

export async function imagesToForm(files: File[], extra: Record<string, string>): Promise<FormData> {
  const fd = new FormData();
  for (const [k, v] of Object.entries(extra)) if (v) fd.append(k, v);
  let i = 0;
  for (const f of files) {
    const blob = await prepareImage(f);
    fd.append('images', blob, `screenshot-${++i}.jpg`);
  }
  return fd;
}
