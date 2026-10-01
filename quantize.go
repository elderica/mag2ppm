package main

import "sort"

// quantizeMedianCut maps RGB image to at most target colors.
// Returns palette [][3]byte (RGB) and indices []byte (len W*H).
func quantizeMedianCut(pix []byte, target int) (pal [][3]byte, idx []byte) {
	n := len(pix) / 3
	uniq := map[[3]byte]int{}
	uniqList := [][3]byte{}
	for i := 0; i < n; i++ {
		c := [3]byte{pix[3*i], pix[3*i+1], pix[3*i+2]}
		if _, ok := uniq[c]; !ok {
			uniq[c] = len(uniqList)
			uniqList = append(uniqList, c)
		}
	}
	if len(uniqList) <= target {
		// keep first-appearance order for stability
		pal = uniqList
		idx = make([]byte, n)
		for i := 0; i < n; i++ {
			c := [3]byte{pix[3*i], pix[3*i+1], pix[3*i+2]}
			idx[i] = byte(uniq[c])
		}
		return pal, idx
	}

	type box struct {
		colors [][3]byte
	}
	boxes := []box{{colors: uniqList}}
	for len(boxes) < target {
		// pick box with largest channel range that has >=2 colors
		best := -1
		bestRange := -1
		var bestCh int
		for bi, b := range boxes {
			if len(b.colors) < 2 {
				continue
			}
			var mn, mx [3]int
			for ch := 0; ch < 3; ch++ {
				mn[ch] = 255
				mx[ch] = 0
			}
			for _, c := range b.colors {
				for ch := 0; ch < 3; ch++ {
					v := int(c[ch])
					if v < mn[ch] {
						mn[ch] = v
					}
					if v > mx[ch] {
						mx[ch] = v
					}
				}
			}
			for ch := 0; ch < 3; ch++ {
				if mx[ch]-mn[ch] > bestRange {
					bestRange = mx[ch] - mn[ch]
					best = bi
					bestCh = ch
				}
			}
		}
		if best < 0 {
			break
		}
		b := boxes[best]
		sort.Slice(b.colors, func(i, j int) bool {
			if b.colors[i][bestCh] != b.colors[j][bestCh] {
				return b.colors[i][bestCh] < b.colors[j][bestCh]
			}
			if b.colors[i][(bestCh+1)%3] != b.colors[j][(bestCh+1)%3] {
				return b.colors[i][(bestCh+1)%3] < b.colors[j][(bestCh+1)%3]
			}
			return b.colors[i][(bestCh+2)%3] < b.colors[j][(bestCh+2)%3]
		})
		mid := len(b.colors) / 2
		boxes[best] = box{colors: b.colors[:mid]}
		boxes = append(boxes, box{colors: b.colors[mid:]})
	}

	pal = make([][3]byte, len(boxes))
	for i, b := range boxes {
		var sr, sg, sb, cnt int
		for _, c := range b.colors {
			sr += int(c[0])
			sg += int(c[1])
			sb += int(c[2])
			cnt++
		}
		if cnt == 0 {
			continue
		}
		pal[i] = [3]byte{byte((sr + cnt/2) / cnt), byte((sg + cnt/2) / cnt), byte((sb + cnt/2) / cnt)}
	}
	idx = make([]byte, n)
	for i := 0; i < n; i++ {
		r, g, b := int(pix[3*i]), int(pix[3*i+1]), int(pix[3*i+2])
		best := 0
		bestD := 1 << 30
		for pi, c := range pal {
			dr := r - int(c[0])
			dg := g - int(c[1])
			db := b - int(c[2])
			d := dr*dr + dg*dg + db*db
			if d < bestD {
				bestD = d
				best = pi
				if d == 0 {
					break
				}
			}
		}
		idx[i] = byte(best)
	}
	return pal, idx
}
