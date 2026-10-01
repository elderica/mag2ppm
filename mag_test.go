package main

import (
	"os"
	_path "path/filepath"
	"testing"
)

func writeTempPPM(t *testing.T, w, h int, fn func(x, y int) (byte, byte, byte)) string {
	t.Helper()
	img := &Image{W: w, H: h, Pix: make([]byte, w*h*3)}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b := fn(x, y)
			img.Pix[(y*w+x)*3] = r
			img.Pix[(y*w+x)*3+1] = g
			img.Pix[(y*w+x)*3+2] = b
		}
	}
	p := _path.Join(t.TempDir(), "in.ppm")
	if err := WritePPM(p, img, "p6"); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRoundtrip16Lossless(t *testing.T) {
	pal := [][3]byte{{255, 0, 0}, {0, 255, 0}, {0, 0, 255}, {255, 255, 255}}
	in := writeTempPPM(t, 32, 16, func(x, y int) (byte, byte, byte) {
		c := pal[(x/4+y)%len(pal)]
		return c[0], c[1], c[2]
	})
	ppm, err := ReadPPM(in)
	if err != nil {
		t.Fatal(err)
	}
	blob, err := EncodeMAG(ppm, EncodeOptions{Colors: 16})
	if err != nil {
		t.Fatal(err)
	}
	mp := _path.Join(t.TempDir(), "a.mag")
	if err := os.WriteFile(mp, blob, 0644); err != nil {
		t.Fatal(err)
	}
	mag, err := DecodeMAG(mp, DecodeOptions{PaletteExpand: "auto"})
	if err != nil {
		t.Fatal(err)
	}
	if mag.W != 32 || mag.H != 16 || mag.Is256 {
		t.Fatalf("size/mode mismatch: %+v", mag)
	}
	got := mag.ToRGB()
	if len(got.Pix) != len(ppm.Pix) {
		t.Fatalf("len mismatch")
	}
	for i := range got.Pix {
		if got.Pix[i] != ppm.Pix[i] {
			t.Fatalf("pixel mismatch at %d", i)
		}
	}
}

func TestRoundtrip256Lossless(t *testing.T) {
	in := writeTempPPM(t, 64, 16, func(x, y int) (byte, byte, byte) {
		i := (x + y*64) % 256
		return byte(i), byte(255 - i), byte((i * 7) % 256)
	})
	ppm, _ := ReadPPM(in)
	blob, err := EncodeMAG(ppm, EncodeOptions{Colors: 256})
	if err != nil {
		t.Fatal(err)
	}
	mp := _path.Join(t.TempDir(), "a.mag")
	os.WriteFile(mp, blob, 0644)
	mag, err := DecodeMAG(mp, DecodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := mag.ToRGB()
	for i := range got.Pix {
		if got.Pix[i] != ppm.Pix[i] {
			t.Fatalf("pixel mismatch at %d", i)
		}
	}
}

func TestPPMFormats(t *testing.T) {
	img := &Image{W: 4, H: 1, Pix: []byte{255, 0, 0, 0, 255, 0, 0, 0, 255, 255, 255, 255}}
	for _, f := range []string{"p3", "p6"} {
		p := _path.Join(t.TempDir(), "a.ppm")
		if err := WritePPM(p, img, f); err != nil {
			t.Fatal(err)
		}
		back, err := ReadPPM(p)
		if err != nil {
			t.Fatal(err)
		}
		for i := range img.Pix {
			if back.Pix[i] != img.Pix[i] {
				t.Fatalf("%s mismatch", f)
			}
		}
	}
}

func TestScreen200Aspect(t *testing.T) {
	in := writeTempPPM(t, 16, 8, func(x, y int) (byte, byte, byte) { return byte(x * 16), byte(y * 32), 0 })
	ppm, _ := ReadPPM(in)
	blob, _ := EncodeMAG(ppm, EncodeOptions{Colors: 16, Screen200: true})
	mp := _path.Join(t.TempDir(), "a.mag")
	os.WriteFile(mp, blob, 0644)
	m1, _ := DecodeMAG(mp, DecodeOptions{Aspect: "keep"})
	if m1.H != 8 {
		t.Fatalf("keep height")
	}
	m2, _ := DecodeMAG(mp, DecodeOptions{Aspect: "double"})
	if m2.H != 16 {
		t.Fatalf("double height")
	}
}
