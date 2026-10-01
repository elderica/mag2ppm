package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
)

// Flag copy tables (pixel units). dx>=0 means left, dy>=0 means up.
var flagDX = [16]int{0, 1, 2, 4, 0, 1, 0, 1, 2, 0, 1, 2, 0, 1, 2, 0}
var flagDY = [16]int{0, 0, 0, 0, 1, 1, 2, 2, 2, 4, 4, 4, 8, 8, 8, 16}

// Search order for encoder (excluding 0).
var flagSearchOrder = []int{1, 4, 5, 6, 7, 9, 10, 2, 8, 11, 12, 13, 14, 3, 15}

type MagImage struct {
	W, H       int
	Is256      bool
	ScreenMode byte
	Machine    byte
	System     byte
	MachineStr string
	User       string
	Memo       string
	Palette    [][3]byte // RGB
	Indices    []byte    // W*H palette indices (per dot)
}

type DecodeOptions struct {
	PaletteExpand string // auto|x17|fill|none
	Aspect        string // keep|double
}

type EncodeOptions struct {
	Colors       int    // 16 or 256, 0=auto
	Screen200    bool   // true=set 200-line flag
	PaletteDepth int    // 8 or 4
	MachineCode  byte   // header byte 1
	System       byte   // header byte 2
	MachineStr   string // 4 chars for comment area
	User         string // <=18 chars
	Memo         string
}

func expandPalette(raw []byte, mode string) [][3]byte {
	n := len(raw) / 3
	pal := make([][3]byte, n)
	allSmall := true
	for i := 0; i < n; i++ {
		g := raw[3*i]
		r := raw[3*i+1]
		b := raw[3*i+2]
		if g > 0x0F || r > 0x0F || b > 0x0F {
			allSmall = false
			break
		}
	}
	doExpand := false
	fill := false
	switch mode {
	case "x17":
		doExpand = true
	case "fill":
		doExpand = true
		fill = true
	case "none":
		doExpand = false
	default: // auto
		doExpand = allSmall
	}
	for i := 0; i < n; i++ {
		g := raw[3*i]
		r := raw[3*i+1]
		b := raw[3*i+2]
		if doExpand {
			if fill {
				if g != 0 {
					g = (g << 4) | 0x0F
				}
				if r != 0 {
					r = (r << 4) | 0x0F
				}
				if b != 0 {
					b = (b << 4) | 0x0F
				}
			} else {
				g = (g << 4) | g
				r = (r << 4) | r
				b = (b << 4) | b
			}
		}
		pal[i] = [3]byte{r, g, b}
	}
	return pal
}

func DecodeMAG(path string, opt DecodeOptions) (*MagImage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) < 8 || string(data[:8]) != "MAKI02  " {
		return nil, fmt.Errorf("mag: bad check data (want MAKI02  )")
	}
	if len(data) < 30 {
		return nil, fmt.Errorf("mag: truncated file header area")
	}
	machineStr := string(data[8:12])
	userBytes := data[12:30]
	// user is 18 bytes, may contain trailing spaces/zeros
	user := string(bytes.TrimRight(userBytes, "\x00 "))
	// find $1A at >=30
	eofPos := -1
	for i := 30; i < len(data); i++ {
		if data[i] == 0x1A {
			eofPos = i
			break
		}
	}
	if eofPos < 0 {
		return nil, fmt.Errorf("mag: EOF($1A) not found")
	}
	headOff := eofPos + 1
	memoBytes := data[30:eofPos]
	// memo ends before $00? spec says memo from 32nd byte; trailing $00 is header top.
	// Strip trailing zeros.
	memoBytes = bytes.TrimRight(memoBytes, "\x00")
	memo := string(memoBytes) // keep as-is (Shift-JIS bytes preserved)

	if len(data) < headOff+32 {
		return nil, fmt.Errorf("mag: truncated header")
	}
	h := data[headOff:]
	if h[0] != 0x00 {
		return nil, fmt.Errorf("mag: bad header top (want 0x00, got 0x%02x)", h[0])
	}
	machine := h[1]
	system := h[2]
	mode := h[3]
	sx := int(binary.LittleEndian.Uint16(h[4:6]))
	sy := int(binary.LittleEndian.Uint16(h[6:8]))
	ex := int(binary.LittleEndian.Uint16(h[8:10]))
	ey := int(binary.LittleEndian.Uint16(h[10:12]))
	flagAOff := int(binary.LittleEndian.Uint32(h[12:16]))
	flagBOff := int(binary.LittleEndian.Uint32(h[16:20]))
	flagBSize := int(binary.LittleEndian.Uint32(h[20:24]))
	pixelOff := int(binary.LittleEndian.Uint32(h[24:28]))
	pixelSize := int(binary.LittleEndian.Uint32(h[28:32]))

	is256 := (mode & 0x80) != 0
	nColors := 16
	dotsPerPixel := 4
	if is256 {
		nColors = 256
		dotsPerPixel = 2
	}
	xsize := ex - sx + 1
	ysize := ey - sy + 1
	if xsize <= 0 || ysize <= 0 || xsize > 10000 || ysize > 10000 {
		return nil, fmt.Errorf("mag: bad image size %dx%d (sx=%d ex=%d sy=%d ey=%d)", xsize, ysize, sx, ex, sy, ey)
	}
	// palette
	palOff := headOff + 32
	if len(data) < palOff+nColors*3 {
		return nil, fmt.Errorf("mag: truncated palette")
	}
	palRaw := data[palOff : palOff+nColors*3]
	palette := expandPalette(palRaw, opt.PaletteExpand)

	// internal rounded geometry
	var startx, roundedDots, pixPerLine int
	if is256 {
		startx = (sx / 4) * 4
		roundedDots = ((ex+4)/4*4 - startx)
		pixPerLine = roundedDots / 2
	} else {
		startx = (sx / 8) * 8
		roundedDots = ((ex+8)/8*8 - startx)
		pixPerLine = roundedDots / 4
	}
	if roundedDots <= 0 || pixPerLine <= 0 {
		return nil, fmt.Errorf("mag: bad rounded size")
	}
	flagLineBytes := pixPerLine / 2 // 2 flags per byte
	if flagLineBytes <= 0 {
		return nil, fmt.Errorf("mag: bad flag line size")
	}
	// Validate offsets: relative to header top
	flagAAbs := headOff + flagAOff
	flagBAbs := headOff + flagBOff
	pixelAbs := headOff + pixelOff
	if flagAAbs < 0 || flagBAbs < 0 || pixelAbs < 0 ||
		flagAAbs > len(data) || flagBAbs > len(data) || pixelAbs > len(data) {
		return nil, fmt.Errorf("mag: bad offsets")
	}
	// FlagA size derived from offsets
	flagAFileSize := flagBOff - flagAOff
	if flagAFileSize < 0 {
		return nil, fmt.Errorf("mag: bad flagA size")
	}
	if flagAAbs+flagAFileSize > len(data) {
		return nil, fmt.Errorf("mag: truncated flagA")
	}
	if flagBAbs+flagBSize > len(data) {
		return nil, fmt.Errorf("mag: truncated flagB")
	}
	if pixelAbs+pixelSize > len(data) {
		return nil, fmt.Errorf("mag: truncated pixel data")
	}
	flagA := data[flagAAbs : flagAAbs+flagAFileSize]
	flagB := data[flagBAbs : flagBAbs+flagBSize]
	pixel := data[pixelAbs : pixelAbs+pixelSize]

	// Fallback: if header-derived flagLineBytes mismatches FlagA size,
	// try to derive from FlagA size (handles 4-dot vs 8-dot rounding differences).
	expectedFlagBits := flagLineBytes * ysize
	expectedFlagABytes := (expectedFlagBits + 7) / 8
	if flagAFileSize != expectedFlagABytes && flagAFileSize != expectedFlagABytes+1 {
		// +1 allows even padding; otherwise try reverse derivation
		// total flag bytes = flagLineBytes*ysize must satisfy ceil(total/8) <= flagAFileSize <= ceil+1
		// brute-force pixPerLine candidates around current value
		found := false
		for _, candPix := range []int{pixPerLine - 2, pixPerLine - 1, pixPerLine + 1, pixPerLine + 2} {
			if candPix <= 0 || candPix%2 != 0 {
				continue
			}
			candLine := candPix / 2
			candBits := candLine * ysize
			candBytes := (candBits + 7) / 8
			if flagAFileSize == candBytes || flagAFileSize == candBytes+1 {
				// adopt only if it matches 8-dot vs 4-dot alternative
				// 256-color: alternative is 8-dot rounding
				if is256 {
					altStart := (sx / 8) * 8
					altDots := ((ex+8)/8*8 - altStart)
					altPix := altDots / 2
					if candPix == altPix {
						startx = altStart
						roundedDots = altDots
						pixPerLine = altPix
						flagLineBytes = candLine
						found = true
						break
					}
				}
			}
		}
		if !found {
			// keep header-derived but warn via error only if wildly off
			// (allow decode attempt; trailing bits will be zero-padded)
		}
		_ = found
	}

	flagBuf := make([]byte, flagLineBytes)
	rounded := make([]byte, roundedDots*ysize) // palette indices per dot

	flagAPos := 0
	var flagABit byte = 0x80
	var curA byte
	if len(flagA) > 0 {
		curA = flagA[0]
	}
	flagBPos := 0
	pixelPos := 0

	readPixel := func() (d0, d1, d2, d3 byte, nDots int, ok bool) {
		if pixelPos+2 > len(pixel) {
			return 0, 0, 0, 0, 0, false
		}
		b0 := pixel[pixelPos]
		b1 := pixel[pixelPos+1]
		pixelPos += 2
		if is256 {
			return b0, b1, 0, 0, 2, true
		}
		return b0 >> 4, b0 & 0x0F, b1 >> 4, b1 & 0x0F, 4, true
	}

	for y := 0; y < ysize; y++ {
		// expand 1 line of flags
		for x := 0; x < flagLineBytes; x++ {
			if flagAPos < len(flagA) {
				if curA&flagABit != 0 {
					var b byte
					if flagBPos < len(flagB) {
						b = flagB[flagBPos]
						flagBPos++
					}
					flagBuf[x] ^= b
				}
				flagABit >>= 1
				if flagABit == 0 {
					flagABit = 0x80
					flagAPos++
					if flagAPos < len(flagA) {
						curA = flagA[flagAPos]
					}
				}
			} else {
				// FlagA exhausted: treat as 0 (keep previous line's flags)
			}
		}
		// decode pixels of this line
		for p := 0; p < pixPerLine; p++ {
			var f int
			if p%2 == 0 {
				f = int(flagBuf[p/2] >> 4)
			} else {
				f = int(flagBuf[p/2] & 0x0F)
			}
			dstDotX := p * dotsPerPixel
			dstOff := y*roundedDots + dstDotX
			if f == 0 {
				a, b, c, d, _, ok := readPixel()
				if !ok {
					return nil, fmt.Errorf("mag: truncated pixel data at y=%d p=%d", y, p)
				}
				if is256 {
					rounded[dstOff] = a
					rounded[dstOff+1] = b
				} else {
					rounded[dstOff] = a
					rounded[dstOff+1] = b
					rounded[dstOff+2] = c
					rounded[dstOff+3] = d
				}
			} else {
				dx := flagDX[f]
				dy := flagDY[f]
				srcX := p - dx
				srcY := y - dy
				if srcX < 0 || srcY < 0 || srcX >= pixPerLine || srcY > y {
					return nil, fmt.Errorf("mag: invalid copy flag=%d at (%d,%d)", f, p, y)
				}
				srcOff := srcY*roundedDots + srcX*dotsPerPixel
				// copy 1 pixel (dotsPerPixel dots)
				copy(rounded[dstOff:dstOff+dotsPerPixel], rounded[srcOff:srcOff+dotsPerPixel])
			}
		}
	}

	// clip to logical size
	pixOff := sx - startx
	if pixOff < 0 || pixOff+xsize > roundedDots {
		return nil, fmt.Errorf("mag: bad clip offset")
	}
	indices := make([]byte, xsize*ysize)
	for y := 0; y < ysize; y++ {
		copy(indices[y*xsize:(y+1)*xsize], rounded[y*roundedDots+pixOff:y*roundedDots+pixOff+xsize])
	}
	// validate palette indices
	// (tolerate out-of-range by clamping? strict error is better for 16c)
	if !is256 {
		for _, v := range indices {
			if v >= 16 {
				return nil, fmt.Errorf("mag: palette index out of range (%d)", v)
			}
		}
	}

	img := &MagImage{
		W: xsize, H: ysize, Is256: is256,
		ScreenMode: mode, Machine: machine, System: system,
		MachineStr: machineStr, User: user, Memo: memo,
		Palette: palette, Indices: indices,
	}
	// aspect double for 200-line
	if opt.Aspect == "double" && (mode&0x01) != 0 {
		img = magDoubleHeight(img)
	}
	return img, nil
}

func magDoubleHeight(m *MagImage) *MagImage {
	nm := &MagImage{
		W: m.W, H: m.H * 2, Is256: m.Is256,
		ScreenMode: m.ScreenMode, Machine: m.Machine, System: m.System,
		MachineStr: m.MachineStr, User: m.User, Memo: m.Memo,
		Palette: m.Palette, Indices: make([]byte, m.W*m.H*2),
	}
	for y := 0; y < m.H; y++ {
		copy(nm.Indices[2*y*m.W:(2*y+1)*m.W], m.Indices[y*m.W:(y+1)*m.W])
		copy(nm.Indices[(2*y+1)*m.W:(2*y+2)*m.W], m.Indices[y*m.W:(y+1)*m.W])
	}
	return nm
}

// MagImage -> truecolor Image
func (m *MagImage) ToRGB() *Image {
	img := &Image{W: m.W, H: m.H, Pix: make([]byte, m.W*m.H*3)}
	for i, idx := range m.Indices {
		var c [3]byte
		if int(idx) < len(m.Palette) {
			c = m.Palette[idx]
		}
		img.Pix[3*i] = c[0]
		img.Pix[3*i+1] = c[1]
		img.Pix[3*i+2] = c[2]
	}
	return img
}

// EncodeMAG builds a MAG file from truecolor image.
func EncodeMAG(img *Image, opt EncodeOptions) ([]byte, error) {
	if img.W <= 0 || img.H <= 0 {
		return nil, fmt.Errorf("mag encode: bad image size")
	}
	target := opt.Colors
	if target == 0 {
		// auto: <=16 unique -> 16, else 256
		seen := map[[3]byte]struct{}{}
		for i := 0; i < img.W*img.H; i++ {
			seen[[3]byte{img.Pix[3*i], img.Pix[3*i+1], img.Pix[3*i+2]}] = struct{}{}
			if len(seen) > 16 {
				break
			}
		}
		if len(seen) <= 16 {
			target = 16
		} else {
			target = 256
		}
	}
	if target != 16 && target != 256 {
		return nil, fmt.Errorf("mag encode: --colors must be 16 or 256")
	}
	depth := opt.PaletteDepth
	if depth == 0 {
		depth = 8
	}
	if depth != 4 && depth != 8 {
		return nil, fmt.Errorf("mag encode: --palette-depth must be 4 or 8")
	}
	is256 := target == 256
	dotsPerPixel := 4
	if is256 {
		dotsPerPixel = 2
	}
	// pad width to alignment: 16c -> 8 dots, 256c -> 4 dots
	align := 8
	if is256 {
		align = 4
	}
	encW := ((img.W + align - 1) / align) * align
	encH := img.H
	// build padded RGB
	padded := make([]byte, encW*encH*3)
	for y := 0; y < encH; y++ {
		copy(padded[y*encW*3:y*encW*3+img.W*3], img.Pix[y*img.W*3:(y+1)*img.W*3])
		// replicate last column for padding
		if encW > img.W {
			lr, lg, lb := img.Pix[y*img.W*3+(img.W-1)*3], img.Pix[y*img.W*3+(img.W-1)*3+1], img.Pix[y*img.W*3+(img.W-1)*3+2]
			for x := img.W; x < encW; x++ {
				padded[y*encW*3+x*3] = lr
				padded[y*encW*3+x*3+1] = lg
				padded[y*encW*3+x*3+2] = lb
			}
		}
	}
	palRGB, indices := quantizeMedianCut(padded, target)
	// indices len encW*encH (per dot)
	// build pixel array (per pixel words) for flag search
	pixPerLine := encW / dotsPerPixel
	nPixels := pixPerLine * encH
	_ = nPixels
	// helper to get pixel dots
	getPixel := func(px, py int) []byte {
		// returns dotsPerPixel indices
		base := py*encW + px*dotsPerPixel
		return indices[base : base+dotsPerPixel]
	}
	pixelEqual := func(ax, ay, bx, by int) bool {
		a := getPixel(ax, ay)
		b := getPixel(bx, by)
		for i := 0; i < dotsPerPixel; i++ {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	// flag search
	flags := make([]byte, pixPerLine*encH) // 4bit each
	for y := 0; y < encH; y++ {
		for x := 0; x < pixPerLine; x++ {
			chosen := 0
			// vertical bias: try upper line's flag first
			if y > 0 {
				upFlag := int(flags[(y-1)*pixPerLine+x])
				if upFlag != 0 {
					dx := flagDX[upFlag]
					dy := flagDY[upFlag]
					sxx := x - dx
					syy := y - dy
					if sxx >= 0 && syy >= 0 {
						if pixelEqual(x, y, sxx, syy) {
							chosen = upFlag
						}
					}
				}
			}
			if chosen == 0 {
				for _, f := range flagSearchOrder {
					dx := flagDX[f]
					dy := flagDY[f]
					sxx := x - dx
					syy := y - dy
					if sxx < 0 || syy < 0 {
						continue
					}
					// same-line left referencess are fine; upper lines fine.
					// Also must ensure source is already decoded (raster order):
					// srcY < y, or srcY==y && srcX < x. Since dx>=0, dy>=0 and not both 0,
					// and f!=0, this holds when sxx>=0&&syy>=0 except f with dy=0,dx=0? none.
					if syy == y && sxx >= x {
						continue
					}
					if pixelEqual(x, y, sxx, syy) {
						chosen = f
						break
					}
				}
			}
			flags[y*pixPerLine+x] = byte(chosen)
		}
	}
	// pack flags into bytes (2 per byte)
	flagLineBytes := pixPerLine / 2
	flagBytes := make([]byte, flagLineBytes*encH)
	for y := 0; y < encH; y++ {
		for i := 0; i < flagLineBytes; i++ {
			hi := flags[y*pixPerLine+2*i]
			lo := flags[y*pixPerLine+2*i+1]
			flagBytes[y*flagLineBytes+i] = hi<<4 | lo
		}
	}
	// vertical XOR: T[y] = F[y] ^ F[y-1]
	xorBytes := make([]byte, len(flagBytes))
	copy(xorBytes[0:flagLineBytes], flagBytes[0:flagLineBytes])
	for y := 1; y < encH; y++ {
		for i := 0; i < flagLineBytes; i++ {
			xorBytes[y*flagLineBytes+i] = flagBytes[y*flagLineBytes+i] ^ flagBytes[(y-1)*flagLineBytes+i]
		}
	}
	// FlagA/B
	var flagA []byte
	var flagB []byte
	bits := 0
	for _, b := range xorBytes {
		bit := byte(0)
		if b != 0 {
			bit = 1
			flagB = append(flagB, b)
		}
		if bits%8 == 0 {
			flagA = append(flagA, 0)
		}
		// MSB first: first byte -> 0x80
		shift := 7 - (bits % 8)
		if bit != 0 {
			flagA[len(flagA)-1] |= 1 << uint(shift)
		}
		bits++
	}
	// even padding
	if len(flagA)%2 == 1 {
		flagA = append(flagA, 0x00)
	}
	if len(flagB)%2 == 1 {
		flagB = append(flagB, 0x00)
	}
	// pixel data: for flags==0 in raster order
	var pixelData []byte
	for y := 0; y < encH; y++ {
		for x := 0; x < pixPerLine; x++ {
			if flags[y*pixPerLine+x] == 0 {
				dots := getPixel(x, y)
				if is256 {
					pixelData = append(pixelData, dots[0], dots[1])
				} else {
					pixelData = append(pixelData, dots[0]<<4|dots[1], dots[2]<<4|dots[3])
				}
			}
		}
	}
	if len(pixelData)%2 == 1 {
		pixelData = append(pixelData, 0x00)
	}
	// palette GRB
	nColors := target
	palRaw := make([]byte, nColors*3)
	for i := 0; i < nColors; i++ {
		var r, g, b byte
		if i < len(palRGB) {
			r, g, b = palRGB[i][0], palRGB[i][1], palRGB[i][2]
		}
		if depth == 4 {
			r >>= 4
			g >>= 4
			b >>= 4
		}
		palRaw[3*i] = g
		palRaw[3*i+1] = r
		palRaw[3*i+2] = b
	}
	// header
	var mode byte
	if is256 {
		mode |= 0x80
	}
	if opt.Screen200 {
		mode |= 0x01
	}
	machineCode := opt.MachineCode
	system := opt.System
	// offsets relative to header top
	flagAOff := 32 + len(palRaw)
	flagBOff := flagAOff + len(flagA)
	pixelOff := flagBOff + len(flagB)
	// comment area
	machineStr := opt.MachineStr
	if machineStr == "" {
		machineStr = "PC98"
	}
	// ensure 4 bytes
	mb := []byte(machineStr)
	if len(mb) < 4 {
		tmp := make([]byte, 4)
		copy(tmp, mb)
		for i := len(mb); i < 4; i++ {
			tmp[i] = ' '
		}
		mb = tmp
	} else if len(mb) > 4 {
		mb = mb[:4]
	}
	user := opt.User
	if user == "" {
		user = "mag2ppm"
	}
	ub := []byte(user)
	if len(ub) > 18 {
		ub = ub[:18]
	} else if len(ub) < 18 {
		tmp := make([]byte, 18)
		copy(tmp, ub)
		for i := len(ub); i < 18; i++ {
			tmp[i] = ' '
		}
		ub = tmp
	}
	memo := []byte(opt.Memo)
	// build file
	var out bytes.Buffer
	out.WriteString("MAKI02  ")
	out.Write(mb)
	out.Write(ub)
	out.Write(memo)
	out.WriteByte(0x1A)
	headPos := out.Len() // header top
	// header 32B
	hdr := make([]byte, 32)
	hdr[0] = 0x00
	hdr[1] = machineCode
	hdr[2] = system
	hdr[3] = mode
	binary.LittleEndian.PutUint16(hdr[4:6], 0)
	binary.LittleEndian.PutUint16(hdr[6:8], 0)
	binary.LittleEndian.PutUint16(hdr[8:10], uint16(encW-1))
	binary.LittleEndian.PutUint16(hdr[10:12], uint16(encH-1))
	binary.LittleEndian.PutUint32(hdr[12:16], uint32(flagAOff))
	binary.LittleEndian.PutUint32(hdr[16:20], uint32(flagBOff))
	binary.LittleEndian.PutUint32(hdr[20:24], uint32(len(flagB)))
	binary.LittleEndian.PutUint32(hdr[24:28], uint32(pixelOff))
	binary.LittleEndian.PutUint32(hdr[28:32], uint32(len(pixelData)))
	out.Write(hdr)
	out.Write(palRaw)
	out.Write(flagA)
	out.Write(flagB)
	out.Write(pixelData)
	_ = headPos
	return out.Bytes(), nil
}
