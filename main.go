package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func usage() {
	fmt.Fprintf(os.Stderr, `mag2ppm: MAG <=> PPM 相互変換 (Go)

使い方:
  mag2ppm [options] <input> <output>
    入出力は拡張子または内容で自動判定:
      .mag -> .ppm (デコード), .ppm/.pgm? -> .mag (エンコード)
      .ppm 入力は P3/P6 両対応

オプション:
  --ppm-format p6|p3   MAG->PPM 出力形式 (default p6)
  --colors 16|256|auto PPM->MAG 色数 (default auto)
  --screen 400|200     PPM->MAG 画面モード (default 400)
  --palette-depth 8|4  PPM->MAG パレット格納深度 (default 8)
  --palette-expand auto|x17|fill|none  MAG->PPM パレット拡張 (default auto)
  --aspect keep|double  MAG->PPM 200ライン時の縦倍率 (default keep)
  --machine-code N      PPM->MAG ヘッダ機種コード (default 0)
  --machine STR         PPM->MAG コメント機種名4文字 (default PC98)
  --user STR            PPM->MAG ユーザ名 (default mag2ppm)
  --memo STR            PPM->MAG メモ
`)
}

func detectDirection(inPath, outPath string) (string, error) {
	// prefer magic when input exists
	if b, err := os.ReadFile(inPath); err == nil && len(b) >= 2 {
		if len(b) >= 8 && string(b[:8]) == "MAKI02  " {
			return "decode", nil
		}
		if b[0] == 'P' && len(b) >= 2 && (b[1] == '3' || b[1] == '6') {
			return "encode", nil
		}
	}
	ie := strings.ToLower(filepath.Ext(inPath))
	oe := strings.ToLower(filepath.Ext(outPath))
	if ie == ".mag" && (oe == ".ppm" || oe == ".pgm") {
		return "decode", nil
	}
	if (ie == ".ppm" || ie == ".pgm") && oe == ".mag" {
		return "encode", nil
	}
	if ie == ".mag" {
		return "decode", nil
	}
	if oe == ".mag" {
		return "encode", nil
	}
	return "", fmt.Errorf("方向を判定できません: 入力のマジック/拡張子を確認してください (.mag/.ppm)")
}

func reorderArgs(args []string) []string {
	var flgs, pos []string
	i := 0
	for i < len(args) {
		a := args[i]
		if len(a) >= 2 && a[0] == '-' && a != "-" && a != "--" {
			if strings.Contains(a, "=") {
				flgs = append(flgs, a)
				i++
				continue
			}
			name := strings.TrimLeft(a, "-")
			if name == "h" || name == "help" {
				flgs = append(flgs, a)
				i++
				continue
			}
			// all other options take a value
			if i+1 < len(args) {
				flgs = append(flgs, a, args[i+1])
				i += 2
			} else {
				flgs = append(flgs, a)
				i++
			}
			continue
		}
		if a == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		pos = append(pos, a)
		i++
	}
	return append(flgs, pos...)
}

func main() {
	// flags may appear before or after file args
	os.Args = append([]string{os.Args[0]}, reorderArgs(os.Args[1:])...)

	var ppmFormat string
	var colorsStr string
	var screen int
	var palDepth int
	var palExpand string
	var aspect string
	var machineCode int
	var machineStr, userStr, memoStr string

	flag.StringVar(&ppmFormat, "ppm-format", "p6", "PPM output format")
	flag.StringVar(&colorsStr, "colors", "auto", "colors")
	flag.IntVar(&screen, "screen", 400, "screen mode")
	flag.IntVar(&palDepth, "palette-depth", 8, "palette depth")
	flag.StringVar(&palExpand, "palette-expand", "auto", "palette expand")
	flag.StringVar(&aspect, "aspect", "keep", "aspect")
	flag.IntVar(&machineCode, "machine-code", 0, "machine code")
	flag.StringVar(&machineStr, "machine", "PC98", "machine string")
	flag.StringVar(&userStr, "user", "mag2ppm", "user")
	flag.StringVar(&memoStr, "memo", "", "memo")
	flag.Usage = usage
	flag.Parse()

	args := flag.Args()
	if len(args) != 2 {
		usage()
		os.Exit(1)
	}
	inPath, outPath := args[0], args[1]

	dir, err := detectDirection(inPath, outPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	switch dir {
	case "decode":
		decOpt := DecodeOptions{PaletteExpand: strings.ToLower(palExpand), Aspect: strings.ToLower(aspect)}
		mag, err := DecodeMAG(inPath, decOpt)
		if err != nil {
			fmt.Fprintln(os.Stderr, "decode error:", err)
			os.Exit(2)
		}
		rgb := mag.ToRGB()
		fmtFmt := strings.ToLower(ppmFormat)
		if fmtFmt != "p3" && fmtFmt != "p6" {
			fmt.Fprintln(os.Stderr, "error: --ppm-format は p3|p6")
			os.Exit(1)
		}
		if err := WritePPM(outPath, rgb, fmtFmt); err != nil {
			fmt.Fprintln(os.Stderr, "write error:", err)
			os.Exit(1)
		}
		cols := 16
		if mag.Is256 {
			cols = 256
		}
		fmt.Fprintf(os.Stderr, "decode: %dx%d %d色 mode=0x%02x -> %s (%s)\n", mag.W, mag.H, cols, mag.ScreenMode, outPath, strings.ToUpper(fmtFmt))
	case "encode":
		ppm, err := ReadPPM(inPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "ppm read error:", err)
			os.Exit(2)
		}
		var colors int
		switch strings.ToLower(colorsStr) {
		case "auto", "0":
			colors = 0
		case "16":
			colors = 16
		case "256":
			colors = 256
		default:
			fmt.Fprintln(os.Stderr, "error: --colors は 16|256|auto")
			os.Exit(1)
		}
		encOpt := EncodeOptions{
			Colors: colors, Screen200: screen == 200,
			PaletteDepth: palDepth, MachineCode: byte(machineCode),
			MachineStr: machineStr, User: userStr, Memo: memoStr,
		}
		if screen != 400 && screen != 200 {
			fmt.Fprintln(os.Stderr, "error: --screen は 400|200")
			os.Exit(1)
		}
		blob, err := EncodeMAG(ppm, encOpt)
		if err != nil {
			fmt.Fprintln(os.Stderr, "encode error:", err)
			os.Exit(2)
		}
		if err := os.WriteFile(outPath, blob, 0644); err != nil {
			fmt.Fprintln(os.Stderr, "write error:", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "encode: %dx%d -> %s (%d bytes)\n", ppm.W, ppm.H, outPath, len(blob))
	}
}
