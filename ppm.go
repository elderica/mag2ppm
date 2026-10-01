package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
)

// Image is a truecolor image, RGB packed.
type Image struct {
	W, H int
	Pix  []byte // len W*H*3, RGB
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// token reader that skips whitespace and # comments
type tokenReader struct {
	r *bufio.Reader
}

func newTokenReader(r *bufio.Reader) *tokenReader { return &tokenReader{r: r} }

func (t *tokenReader) next() (string, error) {
	var tok []byte
	for {
		c, err := t.r.ReadByte()
		if err != nil {
			if err == io.EOF && len(tok) > 0 {
				return string(tok), nil
			}
			return "", err
		}
		if c == '#' {
			// skip to end of line
			for {
				cc, err := t.r.ReadByte()
				if err != nil {
					if err == io.EOF {
						break
					}
					return "", err
				}
				if cc == '\n' {
					break
				}
			}
			if len(tok) > 0 {
				// comment ends token? in PPM comments are whitespace-like;
				// return current token, comment already consumed
				return string(tok), nil
			}
			continue
		}
		if isSpace(c) {
			if len(tok) > 0 {
				return string(tok), nil
			}
			continue
		}
		tok = append(tok, c)
	}
}

func ReadPPM(path string) (*Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	br := bufio.NewReader(f)
	tr := newTokenReader(br)
	magic, err := tr.next()
	if err != nil {
		return nil, fmt.Errorf("ppm: missing magic: %w", err)
	}
	if magic != "P3" && magic != "P6" {
		return nil, fmt.Errorf("ppm: unsupported magic %q (want P3/P6)", magic)
	}
	ws, err := tr.next()
	if err != nil {
		return nil, fmt.Errorf("ppm: missing width: %w", err)
	}
	hs, err := tr.next()
	if err != nil {
		return nil, fmt.Errorf("ppm: missing height: %w", err)
	}
	ms, err := tr.next()
	if err != nil {
		return nil, fmt.Errorf("ppm: missing maxval: %w", err)
	}
	w, err := strconv.Atoi(ws)
	if err != nil || w <= 0 || w > 10000 {
		return nil, fmt.Errorf("ppm: bad width %q", ws)
	}
	h, err := strconv.Atoi(hs)
	if err != nil || h <= 0 || h > 10000 {
		return nil, fmt.Errorf("ppm: bad height %q", hs)
	}
	maxv, err := strconv.Atoi(ms)
	if err != nil || maxv <= 0 || maxv > 65535 {
		return nil, fmt.Errorf("ppm: bad maxval %q", ms)
	}
	img := &Image{W: w, H: h, Pix: make([]byte, w*h*3)}
	if magic == "P3" {
		for i := 0; i < w*h*3; i++ {
			ts, err := tr.next()
			if err != nil {
				return nil, fmt.Errorf("ppm: truncated P3 data at sample %d: %w", i, err)
			}
			v, err := strconv.Atoi(ts)
			if err != nil || v < 0 || v > maxv {
				return nil, fmt.Errorf("ppm: bad P3 sample %q", ts)
			}
			if maxv == 255 {
				img.Pix[i] = byte(v)
			} else {
				img.Pix[i] = byte((v*255 + maxv/2) / maxv)
			}
		}
		return img, nil
	}
	// P6: after maxval there is exactly one whitespace byte already consumed
	// by token reader (it consumed delimiter). The reader is positioned at
	// start of raster. Note: token reader read one delimiter byte after maxval.
	// That matches spec (single whitespace). Raster follows directly.
	n := w * h * 3
	raw := make([]byte, n*2) // enough for 2-byte samples
	if maxv < 256 {
		_, err = io.ReadFull(br, raw[:n])
		if err != nil {
			return nil, fmt.Errorf("ppm: truncated P6 data: %w", err)
		}
		if maxv == 255 {
			copy(img.Pix, raw[:n])
		} else {
			for i := 0; i < n; i++ {
				img.Pix[i] = byte((int(raw[i])*255 + maxv/2) / maxv)
			}
		}
		return img, nil
	}
	// maxval > 255: big-endian 2 bytes per sample
	_, err = io.ReadFull(br, raw[:n*2])
	if err != nil {
		return nil, fmt.Errorf("ppm: truncated P6 data (16bit): %w", err)
	}
	for i := 0; i < n; i++ {
		v := int(raw[2*i])<<8 | int(raw[2*i+1])
		img.Pix[i] = byte((v*255 + maxv/2) / maxv)
	}
	return img, nil
}

// WritePPM writes P3 (ascii) or P6 (binary). format must be "p3" or "p6" (case-insensitive).
func WritePPM(path string, img *Image, format string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	bw := bufio.NewWriter(f)
	defer bw.Flush()
	switch format {
	case "P3", "p3":
		fmt.Fprintf(bw, "P3\n# mag2ppm\n%d %d\n255\n", img.W, img.H)
		// 12 samples per line for readability
		for i, v := range img.Pix {
			if i > 0 {
				if i%12 == 0 {
					bw.WriteString("\n")
				} else {
					bw.WriteString(" ")
				}
			}
			fmt.Fprintf(bw, "%d", int(v))
		}
		bw.WriteString("\n")
		return nil
	default: // P6
		fmt.Fprintf(bw, "P6\n# mag2ppm\n%d %d\n255\n", img.W, img.H)
		_, err = bw.Write(img.Pix)
		return err
	}
}
