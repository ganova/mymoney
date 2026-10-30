package ocr

import (
	"bytes"
	"image"
	"image/draw"
	"image/png"

	_ "image/gif"
	_ "image/jpeg"
)

// upscale memperbesar gambar kecil 2x (bilinear) agar teks kecil lebih terbaca
// oleh Tesseract. Gambar yang sudah besar atau gagal didekode dikembalikan apa adanya.
func upscale(data []byte) []byte {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return data
	}
	b := src.Bounds()
	if b.Dx() >= 1800 || b.Dx() < 100 {
		return data
	}
	rgba := image.NewRGBA(b)
	draw.Draw(rgba, b, src, b.Min, draw.Src)
	w, h := b.Dx()*2, b.Dy()*2
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		fy := (float64(y)+0.5)/2 - 0.5
		y0 := int(fy)
		if fy < 0 {
			y0, fy = 0, 0
		}
		y1 := min(y0+1, b.Dy()-1)
		ty := fy - float64(y0)
		for x := 0; x < w; x++ {
			fx := (float64(x)+0.5)/2 - 0.5
			x0 := int(fx)
			if fx < 0 {
				x0, fx = 0, 0
			}
			x1 := min(x0+1, b.Dx()-1)
			tx := fx - float64(x0)
			var out [4]float64
			for _, p := range [4]struct {
				x, y int
				wt   float64
			}{{x0, y0, (1 - tx) * (1 - ty)}, {x1, y0, tx * (1 - ty)}, {x0, y1, (1 - tx) * ty}, {x1, y1, tx * ty}} {

				o := rgba.PixOffset(b.Min.X+p.x, b.Min.Y+p.y)
				for c := 0; c < 4; c++ {
					out[c] += float64(rgba.Pix[o+c]) * p.wt
				}
			}
			d := dst.PixOffset(x, y)
			for c := 0; c < 4; c++ {
				dst.Pix[d+c] = uint8(out[c] + 0.5)
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return data
	}
	return buf.Bytes()
}
