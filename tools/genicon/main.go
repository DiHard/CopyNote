//go:build ignore

// Command genicon produces assets/icon.ico — a multi-size Windows
// icon file used as both the exe icon and the tray icon.
//
// The glyph is Lucide's "copy" icon: two overlapping rounded squares
// drawn as outlines over a transparent background. We render it by
// computing the distance from each pixel to the path (line segments
// and quarter-circle arcs) and anti-aliasing via that distance.
//
// Run manually:
//
//	go run tools/genicon/main.go
//
// The output file is committed to version control so normal builds
// don't need to regenerate it.
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
)

// Two stroke colors for two output icons — one for light taskbars
// (dark slate stroke) and one for dark taskbars (muted slate-200
// stroke). The tray swaps between them at runtime based on the
// system's SystemUsesLightTheme setting.
var (
	strokeDark  = color.RGBA{R: 0x1F, G: 0x29, B: 0x37, A: 0xFF}
	strokeLight = color.RGBA{R: 0xE5, G: 0xE7, B: 0xEB, A: 0xFF}
)

// Lucide view-box is 24 units; stroke-width is 2 units. All path
// coordinates below are in that space.
const (
	viewBox    = 24.0
	strokeUnit = 2.0
)

// renderIcon draws the CopyNote copy-icon at the given pixel size on
// a transparent RGBA canvas, using the given stroke color.
func renderIcon(size int, stroke color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	scale := float64(size) / viewBox
	strokeW := strokeUnit * scale

	// Back "L-hook" path, in Lucide viewbox units:
	//
	//   M 4 16
	//   c -1.1 0 -2 -.9 -2 -2   (BL corner arc: (4,16) → (2,14))
	//   V 4                      (vertical:     (2,14) → (2,4))
	//   c 0 -1.1 .9 -2 2 -2      (TL corner arc: (2,4)  → (4,2))
	//   h 10                     (horizontal:   (4,2)  → (14,2))
	//   c 1.1 0 2 .9 2 2         (TR corner arc: (14,2) → (16,4))
	//
	// That's 3 quarter-circle arcs and 2 straight segments, no fill,
	// open-ended (round caps at (4,16) and (16,4)).
	back := []segment{
		arcSeg(4, 14, 2, math.Pi/2, math.Pi),       // BL: from (4,16) CCW to (2,14)
		lineSeg(2, 14, 2, 4),                       // left edge
		arcSeg(4, 4, 2, math.Pi, 3*math.Pi/2),      // TL: from (2,4) CCW to (4,2)
		lineSeg(4, 2, 14, 2),                       // top edge
		arcSeg(14, 4, 2, 3*math.Pi/2, 2*math.Pi),   // TR: from (14,2) CCW to (16,4)
	}

	// Front rounded square: (8,8)-(22,22), corner radius 2.
	front := roundedRectOutline(8, 8, 14, 14, 2)

	paths := append([]segment{}, back...)
	paths = append(paths, front...)

	// Rasterize: for every pixel, find the minimum distance to any
	// segment in the path set. If it's within the stroke's half-width,
	// blend the stroke color with anti-aliased coverage.
	halfW := strokeW / 2
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			// Sample at pixel center, in viewbox coordinates.
			fx := (float64(px) + 0.5) / scale
			fy := (float64(py) + 0.5) / scale

			minDist := math.MaxFloat64
			for _, s := range paths {
				d := s.distance(fx, fy)
				if d < minDist {
					minDist = d
				}
			}
			// Convert viewbox-space distance to pixel-space distance
			// so coverage anti-aliasing uses one-pixel fall-off.
			distPx := minDist * scale
			cov := strokeCoverage(distPx, halfW)
			if cov <= 0 {
				continue
			}
			blendPixel(img, px, py, stroke, cov)
		}
	}

	return img
}

// strokeCoverage returns the antialiased coverage for a pixel whose
// center is distPx pixels away from the nearest point on the stroke
// centerline, for a stroke of half-width halfW pixels. 1 in the solid
// core, linear fall-off over one pixel at the edge, 0 outside.
func strokeCoverage(distPx, halfW float64) float64 {
	edge := halfW
	soft := 0.5
	if distPx <= edge-soft {
		return 1
	}
	if distPx >= edge+soft {
		return 0
	}
	return (edge + soft - distPx) / (2 * soft)
}

// segment is a one-dimensional primitive (line or arc) that knows how
// to compute the distance from an arbitrary point to itself in
// viewbox-space units.
type segment struct {
	kind   int // 0 = line, 1 = arc
	ax, ay float64
	bx, by float64
	// Arc-only:
	cx, cy     float64
	r          float64
	angA, angB float64
}

func lineSeg(x1, y1, x2, y2 float64) segment {
	return segment{kind: 0, ax: x1, ay: y1, bx: x2, by: y2}
}

// arcSeg creates a quarter-circle (or longer) arc with the given
// center, radius, and angular endpoints in radians. Angles are in
// SVG/math convention: 0 points +x, π/2 points +y (down).
func arcSeg(cx, cy, r, angA, angB float64) segment {
	return segment{kind: 1, cx: cx, cy: cy, r: r, angA: angA, angB: angB}
}

func (s segment) distance(px, py float64) float64 {
	switch s.kind {
	case 0:
		return distLine(px, py, s.ax, s.ay, s.bx, s.by)
	case 1:
		return distArc(px, py, s.cx, s.cy, s.r, s.angA, s.angB)
	}
	return math.MaxFloat64
}

// distLine returns the distance from point p to the line segment
// (a, b).
func distLine(px, py, ax, ay, bx, by float64) float64 {
	dx := bx - ax
	dy := by - ay
	lenSq := dx*dx + dy*dy
	if lenSq == 0 {
		return math.Hypot(px-ax, py-ay)
	}
	t := ((px-ax)*dx + (py-ay)*dy) / lenSq
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	cx := ax + t*dx
	cy := ay + t*dy
	return math.Hypot(px-cx, py-cy)
}

// distArc returns the distance from point p to a circular arc with
// center (cx, cy), radius r, and angular range [angA, angB] in
// radians (normalized to [0, 2π), going counter-clockwise). If the
// point's angular position falls outside that range, the distance is
// to the nearest arc endpoint instead.
func distArc(px, py, cx, cy, r, angA, angB float64) float64 {
	dx := px - cx
	dy := py - cy
	ang := math.Atan2(dy, dx)
	ang = normalizeAngle(ang)
	a := normalizeAngle(angA)
	b := normalizeAngle(angB)

	var inRange bool
	if a <= b {
		inRange = ang >= a && ang <= b
	} else {
		inRange = ang >= a || ang <= b
	}
	if inRange {
		return math.Abs(math.Hypot(dx, dy) - r)
	}
	// Closest endpoint.
	a1x := cx + r*math.Cos(angA)
	a1y := cy + r*math.Sin(angA)
	a2x := cx + r*math.Cos(angB)
	a2y := cy + r*math.Sin(angB)
	d1 := math.Hypot(px-a1x, py-a1y)
	d2 := math.Hypot(px-a2x, py-a2y)
	if d1 < d2 {
		return d1
	}
	return d2
}

func normalizeAngle(a float64) float64 {
	for a < 0 {
		a += 2 * math.Pi
	}
	for a >= 2*math.Pi {
		a -= 2 * math.Pi
	}
	return a
}

// roundedRectOutline returns the 4 lines + 4 corner arcs that form
// the outline of a rounded rectangle at (x, y) with size (w, h) and
// corner radius r, suitable for feeding into the stroke rasterizer.
func roundedRectOutline(x, y, w, h, r float64) []segment {
	return []segment{
		// Top edge: (x+r, y) → (x+w-r, y)
		lineSeg(x+r, y, x+w-r, y),
		// Right edge: (x+w, y+r) → (x+w, y+h-r)
		lineSeg(x+w, y+r, x+w, y+h-r),
		// Bottom edge: (x+w-r, y+h) → (x+r, y+h)
		lineSeg(x+w-r, y+h, x+r, y+h),
		// Left edge: (x, y+h-r) → (x, y+r)
		lineSeg(x, y+h-r, x, y+r),
		// TL corner: center (x+r, y+r), from angle π to 3π/2
		arcSeg(x+r, y+r, r, math.Pi, 3*math.Pi/2),
		// TR corner: center (x+w-r, y+r), from 3π/2 to 2π
		arcSeg(x+w-r, y+r, r, 3*math.Pi/2, 2*math.Pi),
		// BR corner: center (x+w-r, y+h-r), from 0 to π/2
		arcSeg(x+w-r, y+h-r, r, 0, math.Pi/2),
		// BL corner: center (x+r, y+h-r), from π/2 to π
		arcSeg(x+r, y+h-r, r, math.Pi/2, math.Pi),
	}
}

// blendPixel over-composites a source color with coverage onto the
// existing pixel at (x, y).
func blendPixel(img *image.RGBA, x, y int, c color.RGBA, cov float64) {
	if cov <= 0 {
		return
	}
	if cov > 1 {
		cov = 1
	}
	sr := float64(c.R) / 255
	sg := float64(c.G) / 255
	sb := float64(c.B) / 255
	sa := float64(c.A) / 255 * cov

	existing := img.RGBAAt(x, y)
	er := float64(existing.R) / 255
	eg := float64(existing.G) / 255
	eb := float64(existing.B) / 255
	ea := float64(existing.A) / 255

	outA := sa + ea*(1-sa)
	if outA == 0 {
		return
	}
	outR := (sr*sa + er*ea*(1-sa)) / outA
	outG := (sg*sa + eg*ea*(1-sa)) / outA
	outB := (sb*sa + eb*ea*(1-sa)) / outA
	img.SetRGBA(x, y, color.RGBA{
		R: uint8(outR*255 + 0.5),
		G: uint8(outG*255 + 0.5),
		B: uint8(outB*255 + 0.5),
		A: uint8(outA*255 + 0.5),
	})
}

// writeICO wraps a slice of PNG-encoded images in a Windows ICO
// container. Each entry describes one image; the actual bitmap data
// is appended after the directory.
func writeICO(w *bytes.Buffer, pngs [][]byte, sizes []int) {
	if len(pngs) != len(sizes) {
		panic("pngs/sizes length mismatch")
	}
	count := uint16(len(pngs))
	binary.Write(w, binary.LittleEndian, uint16(0)) // reserved
	binary.Write(w, binary.LittleEndian, uint16(1)) // type = icon
	binary.Write(w, binary.LittleEndian, count)

	offset := 6 + 16*int(count)
	for i, data := range pngs {
		sz := sizes[i]
		var bw, bh byte
		if sz >= 256 {
			bw, bh = 0, 0
		} else {
			bw, bh = byte(sz), byte(sz)
		}
		w.WriteByte(bw)                                         // width
		w.WriteByte(bh)                                         // height
		w.WriteByte(0)                                          // color count
		w.WriteByte(0)                                          // reserved
		binary.Write(w, binary.LittleEndian, uint16(1))         // planes
		binary.Write(w, binary.LittleEndian, uint16(32))        // bit count
		binary.Write(w, binary.LittleEndian, uint32(len(data))) // size
		binary.Write(w, binary.LittleEndian, uint32(offset))    // offset
		offset += len(data)
	}

	for _, data := range pngs {
		w.Write(data)
	}
}

func writeVariant(path string, stroke color.RGBA, sizes []int) {
	pngs := make([][]byte, 0, len(sizes))
	for _, s := range sizes {
		img := renderIcon(s, stroke)
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			log.Fatalf("png encode %d: %v", s, err)
		}
		pngs = append(pngs, buf.Bytes())
	}
	var icoBuf bytes.Buffer
	writeICO(&icoBuf, pngs, sizes)
	if err := os.WriteFile(path, icoBuf.Bytes(), 0o644); err != nil {
		log.Fatalf("write %s: %v", path, err)
	}
	log.Printf("wrote %s (%d bytes, %d sizes)", path, icoBuf.Len(), len(sizes))
}

func main() {
	sizes := []int{16, 24, 32, 48, 64, 128, 256}
	if err := os.MkdirAll("assets", 0o755); err != nil {
		log.Fatalf("mkdir assets: %v", err)
	}
	writeVariant(filepath.Join("assets", "icon-dark.ico"), strokeDark, sizes)
	writeVariant(filepath.Join("assets", "icon-light.ico"), strokeLight, sizes)
}
