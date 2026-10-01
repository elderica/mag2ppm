# mag2ppm — MAG ⇔ PPM 相互変換 (Go)

MAGフォーマット (MAKIchan Graphic loader) と PPM (P3/P6) を相互変換するCLIツール。
Go標準ライブラリのみで動作。

仕様根拠: `magbible.doc`, `01.doc`, `impl_note.txt`, magjs実装。

## ビルド

```sh
go build -o mag2ppm .
```

## 使い方

```sh
# MAG -> PPM (デコード)。入力マジックで自動判定
mag2ppm [options] input.mag output.ppm

# PPM -> MAG (エンコード)。P3/P6どちらも読める
mag2ppm [options] input.ppm output.mag
```

オプションはファイル引数の前後どちらでも指定可。

### 主なオプション

| オプション | 対象 | 既定 | 説明 |
|---|---|---|---|
| `--ppm-format p6\|p3` | decode | `p6` | PPM出力形式。P6=バイナリ, P3=アスキー |
| `--colors 16\|256\|auto` | encode | `auto` | 色数。autoはユニーク色<=16なら16 |
| `--screen 400\|200` | encode | `400` | 200で200ラインflag (縦横比2:1) |
| `--palette-depth 8\|4` | encode | `8` | GRB格納深度。4は仕様厳密(0-15), 8は現代互換 |
| `--palette-expand auto\|x17\|fill\|none` | decode | `auto` | 12bpp->24bpp拡張。autoは全成分<=0x0F時のみx17 |
| `--aspect keep\|double` | decode | `keep` | 200ライン時の縦2倍表示 |
| `--machine-code N` | encode | `0` | ヘッダ機種コード |
| `--machine STR` | encode | `PC98` | コメント機種名4文字 |
| `--user STR` | encode | `mag2ppm` | ユーザ名 |
| `--memo STR` | encode | `` | メモ |

例:

```sh
mag2ppm input.mag output.ppm
mag2ppm --ppm-format p3 input.mag output.ppm
mag2ppm --colors 256 input.ppm output.mag
mag2ppm --colors 16 --screen 200 --memo "test" input.ppm output.mag
```

## 仕様メモ

* MAGヘッダ32BはLE。オフセットはヘッダ先頭相対・偶数境界。
* フラグ対応表 (ピクセル単位, 16色=4dot/256色=2dot):
  `0:新規, 1:(-1,0), 2:(-2,0), 3:(-4,0), 4:(0,-1), 5:(-1,-1), 6:(0,-2), 7:(-1,-2), 8:(-2,-2), 9:(0,-4), 10:(-1,-4), 11:(-2,-4), 12:(0,-8), 13:(-1,-8), 14:(-2,-8), 15:(0,-16)`。FlagAはMSB first。
* エンコード探索順は `1,4,5,6,7,9,10,2,8,11,12,13,14,3,15` + 縦バイアス (真上と同フラグ優先)。
* 横幅丸め: 16色8dot / 256色4dot (マルチペイント修正を採用)。デコードは8dot丸めファイル (xv流) もFlagAサイズから救済。
* エンコード時、幅がアラインに満たない場合は最終列複製でパディングし、ヘッダはパディング後サイズになる (例 30px→32px)。
* PPM入力はP3/P6・`#`コメント・maxval!=255 (255にスケーリング) に対応。出力はmaxval=255。
* 256色超の画像はMedian-Cutで減色 (ディザなし)。
* 200ライン (`mode&0x01`) は `--aspect double` でのみ縦2倍。`--screen 200` で設定。
* SC8訂正 (`0x81`=200ライン256色) 対応。

## テスト

```sh
go test ./...
go vet ./...
```
