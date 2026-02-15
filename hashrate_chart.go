package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
)

type hashrateChart struct {
	raster    *canvas.Raster
	view      fyne.CanvasObject
	maxPoints int

	mu     sync.Mutex
	points []float64

	axisMin  float64
	axisMax  float64
	axisStep float64

	unitText   *canvas.Text
	tickTop    *canvas.Text
	tickMid    *canvas.Text
	tickBottom *canvas.Text
}

type hashrateChartLayout struct {
	raster     fyne.CanvasObject
	unit       fyne.CanvasObject
	tickTop    fyne.CanvasObject
	tickMid    fyne.CanvasObject
	tickBottom fyne.CanvasObject
}

func (l *hashrateChartLayout) Layout(_ []fyne.CanvasObject, size fyne.Size) {
	if l.raster != nil {
		l.raster.Move(fyne.NewPos(0, 0))
		l.raster.Resize(size)
	}

	const (
		leftPad   float32 = 8
		rightPad  float32 = 8
		topPad    float32 = 8
		bottomPad float32 = 10
	)

	chartW := size.Width - leftPad - rightPad
	chartH := size.Height - topPad - bottomPad
	if chartW < 2 || chartH < 2 {
		return
	}

	maxW := float32(0)
	for _, obj := range []fyne.CanvasObject{l.unit, l.tickTop, l.tickMid, l.tickBottom} {
		if obj == nil || !obj.Visible() {
			continue
		}
		if w := obj.MinSize().Width; w > maxW {
			maxW = w
		}
	}
	if maxW < 1 {
		maxW = 1
	}

	columnX := size.Width - rightPad - maxW - 2
	if columnX < leftPad+2 {
		columnX = leftPad + 2
	}

	place := func(obj fyne.CanvasObject, y float32) {
		if obj == nil || !obj.Visible() {
			return
		}
		if y < 0 {
			y = 0
		}
		h := obj.MinSize().Height
		obj.Resize(fyne.NewSize(maxW, h))
		obj.Move(fyne.NewPos(columnX, y))
	}

	centerOnLine := func(obj fyne.CanvasObject, yLine float32) {
		if obj == nil || !obj.Visible() {
			return
		}
		min := obj.MinSize()
		place(obj, yLine-min.Height/2)
	}

	place(l.unit, topPad+2)

	lineY := func(i float32) float32 {
		return topPad + (chartH-1)*i/4
	}
	centerOnLine(l.tickTop, lineY(1))
	centerOnLine(l.tickMid, lineY(2))
	centerOnLine(l.tickBottom, lineY(3))
}

func (l *hashrateChartLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	min := fyne.NewSize(0, 0)
	for _, obj := range objects {
		if obj == nil {
			continue
		}
		min = min.Max(obj.MinSize())
	}
	return min
}

func newHashrateChart(maxPoints int) *hashrateChart {
	if maxPoints < 2 {
		maxPoints = 2
	}
	c := &hashrateChart{maxPoints: maxPoints}
	c.raster = canvas.NewRaster(func(w, h int) image.Image {
		return c.render(w, h)
	})
	c.raster.ScaleMode = canvas.ImageScaleSmooth
	c.raster.SetMinSize(fyne.NewSize(0, 140))

	tickColor := toNRGBA(theme.Color(theme.ColorNamePlaceHolder))
	tickColor.A = 0xCC

	c.unitText = canvas.NewText("H/s", tickColor)
	c.unitText.Alignment = fyne.TextAlignLeading
	c.unitText.TextStyle = fyne.TextStyle{Bold: true, Monospace: true}
	c.unitText.TextSize = theme.TextSize() * 0.85
	c.unitText.Hide()

	newTick := func() *canvas.Text {
		t := canvas.NewText("", tickColor)
		t.Alignment = fyne.TextAlignLeading
		t.TextStyle = fyne.TextStyle{Monospace: true}
		t.TextSize = theme.TextSize() * 0.85
		t.Hide()
		return t
	}
	c.tickTop = newTick()
	c.tickMid = newTick()
	c.tickBottom = newTick()

	l := &hashrateChartLayout{
		raster:     c.raster,
		unit:       c.unitText,
		tickTop:    c.tickTop,
		tickMid:    c.tickMid,
		tickBottom: c.tickBottom,
	}
	c.view = container.New(l, c.raster, c.unitText, c.tickTop, c.tickMid, c.tickBottom)
	return c
}

func (c *hashrateChart) Object() fyne.CanvasObject {
	return c.view
}

func (c *hashrateChart) Add(mhs float64) {
	if mhs < 0 || math.IsNaN(mhs) || math.IsInf(mhs, 0) {
		return
	}
	c.mu.Lock()
	c.points = append(c.points, mhs)
	if len(c.points) > c.maxPoints {
		c.points = c.points[len(c.points)-c.maxPoints:]
	}
	axisMin, axisMax, axisStep := c.axisRangeLocked()
	c.axisMin = axisMin
	c.axisMax = axisMax
	c.axisStep = axisStep
	c.mu.Unlock()
	c.setScale(axisMin, axisMax, axisStep)
	c.raster.Refresh()
}

func (c *hashrateChart) Average() (float64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.points) == 0 {
		return 0, false
	}
	var sum float64
	for _, v := range c.points {
		sum += v
	}
	return sum / float64(len(c.points)), true
}

func (c *hashrateChart) Reset() {
	c.mu.Lock()
	c.points = nil
	c.axisMin = 0
	c.axisMax = 0
	c.axisStep = 0
	c.mu.Unlock()
	c.unitText.Hide()
	c.tickTop.Hide()
	c.tickMid.Hide()
	c.tickBottom.Hide()
	c.unitText.Refresh()
	c.tickTop.Refresh()
	c.tickMid.Refresh()
	c.tickBottom.Refresh()
	c.raster.Refresh()
}

func (c *hashrateChart) axisRangeLocked() (axisMin, axisMax, axisStep float64) {
	if len(c.points) == 0 {
		return 0, 0, 0
	}
	dataMin, dataMax := c.points[0], c.points[0]
	for _, v := range c.points[1:] {
		if v < dataMin {
			dataMin = v
		}
		if v > dataMax {
			dataMax = v
		}
	}
	if dataMax-dataMin < 1e-6 {
		dataMax = dataMin + 1
	}
	rng := dataMax - dataMin
	pad := rng * 0.10
	minP := dataMin - pad
	maxP := dataMax + pad
	if minP < 0 {
		minP = 0
	}
	if maxP <= minP {
		maxP = minP + 1
	}

	target := (maxP - minP) / 2
	step := niceStep(target)
	axisMin = math.Floor(minP/step) * step
	axisMax = axisMin + 2*step
	for i := 0; i < 12 && axisMax < maxP; i++ {
		step = niceStep(step * 2)
		axisMin = math.Floor(minP/step) * step
		axisMax = axisMin + 2*step
	}
	return axisMin, axisMax, step
}

func (c *hashrateChart) setScale(axisMin, axisMax, axisStep float64) {
	if axisMax <= axisMin || axisStep <= 0 || math.IsNaN(axisStep) || math.IsInf(axisStep, 0) {
		c.unitText.Hide()
		c.tickTop.Hide()
		c.tickMid.Hide()
		c.tickBottom.Hide()
		c.unitText.Refresh()
		c.tickTop.Refresh()
		c.tickMid.Refresh()
		c.tickBottom.Refresh()
		return
	}

	rng := axisMax - axisMin
	decimals := decimalsForStep(rng / 4)
	formatValue := func(v float64) string {
		if v < 0 {
			v = 0
		}
		if math.Abs(v) < 1e-9 {
			v = 0
		}
		return fmt.Sprintf("%.*f", decimals, v)
	}
	top := formatValue(axisMax - rng*0.25)
	mid := formatValue(axisMax - rng*0.50)
	bottom := formatValue(axisMax - rng*0.75)

	c.unitText.Show()
	c.tickTop.Show()
	c.tickMid.Show()
	c.tickBottom.Show()

	c.unitText.Text = "H/s"
	c.tickTop.Text = top
	c.tickMid.Text = mid
	c.tickBottom.Text = bottom

	c.unitText.Refresh()
	c.tickTop.Refresh()
	c.tickMid.Refresh()
	c.tickBottom.Refresh()
}

func (c *hashrateChart) render(w, h int) image.Image {
	if w < 2 || h < 2 {
		return image.NewNRGBA(image.Rect(0, 0, 2, 2))
	}

	img := image.NewNRGBA(image.Rect(0, 0, w, h))

	bg := toNRGBA(theme.Color(theme.ColorNameInputBackground))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, bg)
		}
	}

	c.mu.Lock()
	data := append([]float64(nil), c.points...)
	axisMin := c.axisMin
	axisMax := c.axisMax
	c.mu.Unlock()
	if len(data) == 0 || axisMax <= axisMin {
		return img
	}

	leftPad, rightPad := 8, 8
	topPad, bottomPad := 8, 10
	chartW := w - leftPad - rightPad
	chartH := h - topPad - bottomPad
	if chartW < 2 || chartH < 2 {
		return img
	}

	minV, maxV := axisMin, axisMax

	grid := toNRGBA(theme.Color(theme.ColorNameSeparator))
	grid.A = 0x45
	for i := 1; i <= 3; i++ {
		y := topPad + int(float64(chartH-1)*float64(i)/4.0)
		drawHLine(img, leftPad, leftPad+chartW-1, y, grid)
	}

	line := toNRGBA(theme.Color(theme.ColorNamePrimary))
	fill := line
	fill.A = 0x2A

	ys := make([]int, chartW)
	n := float64(len(data) - 1)
	for x := 0; x < chartW; x++ {
		t := float64(x) / float64(chartW-1) * n
		i0 := int(math.Floor(t))
		if i0 < 0 {
			i0 = 0
		}
		i1 := i0 + 1
		if i1 >= len(data) {
			i1 = len(data) - 1
		}
		f := t - float64(i0)
		v := data[i0]*(1-f) + data[i1]*f
		yy := (maxV - v) / (maxV - minV)
		y := topPad + int(math.Round(yy*float64(chartH-1)))
		if y < topPad {
			y = topPad
		}
		if y > topPad+chartH-1 {
			y = topPad + chartH - 1
		}
		ys[x] = y
	}

	for x := 0; x < chartW; x++ {
		y := ys[x]
		drawVLine(img, leftPad+x, y, topPad+chartH-1, fill)
	}
	for x := 1; x < chartW; x++ {
		drawLine(img, leftPad+x-1, ys[x-1], leftPad+x, ys[x], line)
	}

	// Highlight last point.
	lastX := leftPad + chartW - 1
	lastY := ys[chartW-1]
	drawCircle(img, lastX, lastY, 3, line)

	return img
}

func niceStep(target float64) float64 {
	if target <= 0 || math.IsNaN(target) || math.IsInf(target, 0) {
		return 1
	}
	exp := math.Floor(math.Log10(target))
	base := math.Pow(10, exp)
	fraction := target / base
	var niceFraction float64
	switch {
	case fraction <= 1:
		niceFraction = 1
	case fraction <= 2:
		niceFraction = 2
	case fraction <= 5:
		niceFraction = 5
	default:
		niceFraction = 10
	}
	return niceFraction * base
}

func decimalsForStep(step float64) int {
	if step <= 0 || math.IsNaN(step) || math.IsInf(step, 0) {
		return 2
	}
	exp := math.Floor(math.Log10(step))
	if exp >= 0 {
		return 0
	}
	decimals := int(-exp)
	if decimals > 4 {
		return 4
	}
	return decimals
}

func toNRGBA(c color.Color) color.NRGBA {
	nrgba, ok := c.(color.NRGBA)
	if ok {
		return nrgba
	}
	r, g, b, a := c.RGBA()
	return color.NRGBA{
		R: uint8(r >> 8),
		G: uint8(g >> 8),
		B: uint8(b >> 8),
		A: uint8(a >> 8),
	}
}

func drawHLine(img *image.NRGBA, x0, x1, y int, c color.NRGBA) {
	if y < 0 || y >= img.Bounds().Dy() {
		return
	}
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if x0 < 0 {
		x0 = 0
	}
	if x1 >= img.Bounds().Dx() {
		x1 = img.Bounds().Dx() - 1
	}
	for x := x0; x <= x1; x++ {
		img.SetNRGBA(x, y, c)
	}
}

func drawVLine(img *image.NRGBA, x, y0, y1 int, c color.NRGBA) {
	if x < 0 || x >= img.Bounds().Dx() {
		return
	}
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	if y0 < 0 {
		y0 = 0
	}
	if y1 >= img.Bounds().Dy() {
		y1 = img.Bounds().Dy() - 1
	}
	for y := y0; y <= y1; y++ {
		img.SetNRGBA(x, y, c)
	}
}

func drawLine(img *image.NRGBA, x0, y0, x1, y1 int, c color.NRGBA) {
	dx := int(math.Abs(float64(x1 - x0)))
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	dy := -int(math.Abs(float64(y1 - y0)))
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		if x0 >= 0 && x0 < img.Bounds().Dx() && y0 >= 0 && y0 < img.Bounds().Dy() {
			img.SetNRGBA(x0, y0, c)
			if x0+1 < img.Bounds().Dx() {
				img.SetNRGBA(x0+1, y0, c)
			}
		}
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func drawCircle(img *image.NRGBA, cx, cy, r int, c color.NRGBA) {
	if r < 1 {
		return
	}
	for y := -r; y <= r; y++ {
		for x := -r; x <= r; x++ {
			if x*x+y*y > r*r {
				continue
			}
			px := cx + x
			py := cy + y
			if px < 0 || py < 0 || px >= img.Bounds().Dx() || py >= img.Bounds().Dy() {
				continue
			}
			img.SetNRGBA(px, py, c)
		}
	}
}
