package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func panel(title string, body fyne.CanvasObject) fyne.CanvasObject {
	header := canvas.NewText(title, theme.Color(theme.ColorNameForeground))
	header.TextStyle = fyne.TextStyle{Bold: true}
	header.TextSize = theme.TextSize() * 1.15

	bg := canvas.NewRectangle(theme.Color(theme.ColorNameButton))
	bg.StrokeColor = theme.Color(theme.ColorNameSeparator)
	bg.StrokeWidth = 1
	bg.CornerRadius = theme.Padding()

	top := container.NewVBox(header, widget.NewSeparator())
	content := container.NewBorder(top, nil, nil, nil, body)
	return container.NewMax(bg, container.NewPadded(content))
}

func panelWithHeader(header fyne.CanvasObject, body fyne.CanvasObject) fyne.CanvasObject {
	bg := canvas.NewRectangle(theme.Color(theme.ColorNameButton))
	bg.StrokeColor = theme.Color(theme.ColorNameSeparator)
	bg.StrokeWidth = 1
	bg.CornerRadius = theme.Padding()

	top := container.NewVBox(header, widget.NewSeparator())
	content := container.NewBorder(top, nil, nil, nil, body)
	return container.NewMax(bg, container.NewPadded(content))
}

func chip(text string, fill color.Color) fyne.CanvasObject {
	label := widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	label.Wrapping = fyne.TextWrapWord

	bg := canvas.NewRectangle(fill)
	bg.StrokeColor = theme.Color(theme.ColorNameSeparator)
	bg.StrokeWidth = 1
	bg.CornerRadius = theme.Padding() * 2

	row := container.NewHBox(layout.NewSpacer(), label, layout.NewSpacer())
	return container.NewMax(bg, container.NewPadded(row))
}

func fieldLabel(text string) *widget.Label {
	l := widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	// Keep labels in a single line so border/grid layouts don't collapse the label column.
	l.Wrapping = fyne.TextWrapOff
	return l
}

func formRow(label string, field fyne.CanvasObject) fyne.CanvasObject {
	l := fieldLabel(label)
	return container.NewBorder(nil, nil, l, nil, field)
}

func metricTile(title string, value fyne.CanvasObject) fyne.CanvasObject {
	titleLabel := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	titleLabel.Wrapping = fyne.TextWrapOff

	return metricTileWithHeader(titleLabel, value)
}

func metricTileWithIcon(title string, icon fyne.Resource, value fyne.CanvasObject) fyne.CanvasObject {
	titleLabel := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	titleLabel.Wrapping = fyne.TextWrapOff

	header := container.NewHBox(widget.NewIcon(icon), titleLabel)
	return metricTileWithHeader(header, value)
}

func metricTileWithHeader(header fyne.CanvasObject, value fyne.CanvasObject) fyne.CanvasObject {
	bg := canvas.NewRectangle(theme.Color(theme.ColorNameInputBackground))
	bg.StrokeColor = theme.Color(theme.ColorNameSeparator)
	bg.StrokeWidth = 1
	bg.CornerRadius = theme.Padding()

	return container.NewMax(bg, container.NewPadded(container.NewVBox(header, value)))
}

func metricTileWithIconBg(title string, icon fyne.Resource, value fyne.CanvasObject) (fyne.CanvasObject, *canvas.Rectangle) {
	titleLabel := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	titleLabel.Wrapping = fyne.TextWrapOff

	header := container.NewHBox(widget.NewIcon(icon), titleLabel)
	return metricTileWithHeaderBg(header, value)
}

func metricTileWithHeaderBg(header fyne.CanvasObject, value fyne.CanvasObject) (fyne.CanvasObject, *canvas.Rectangle) {
	bg := canvas.NewRectangle(theme.Color(theme.ColorNameInputBackground))
	bg.StrokeColor = theme.Color(theme.ColorNameSeparator)
	bg.StrokeWidth = 1
	bg.CornerRadius = theme.Padding()

	tile := container.NewMax(bg, container.NewPadded(container.NewVBox(header, value)))
	return tile, bg
}

type fixedSizeLayout struct {
	size fyne.Size
}

func fixedSize(size fyne.Size, obj fyne.CanvasObject) fyne.CanvasObject {
	return container.New(&fixedSizeLayout{size: size}, obj)
}

func (l *fixedSizeLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, obj := range objects {
		obj.Move(fyne.NewPos(0, 0))
		obj.Resize(size)
	}
}

func (l *fixedSizeLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return l.size
}

type centeredTileRowLayout struct {
	Columns int
}

func (l *centeredTileRowLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) == 0 {
		return
	}

	columns := l.Columns
	if columns <= 0 {
		columns = len(objects)
	}

	padding := theme.Padding()
	tileWidth := float32(0)
	if size.Width > 0 && columns > 0 {
		tileWidth = (size.Width - padding*float32(columns-1)) / float32(columns)
		if tileWidth < 0 {
			tileWidth = 0
		}
	}

	groupWidth := tileWidth*float32(len(objects)) + padding*float32(len(objects)-1)
	x := (size.Width - groupWidth) / 2
	if x < 0 {
		x = 0
	}

	for i, obj := range objects {
		obj.Move(fyne.NewPos(x+float32(i)*(tileWidth+padding), 0))
		obj.Resize(fyne.NewSize(tileWidth, size.Height))
	}
}

func (l *centeredTileRowLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) == 0 {
		return fyne.NewSize(0, 0)
	}
	padding := theme.Padding()
	var maxW, maxH float32
	for _, obj := range objects {
		min := obj.MinSize()
		if min.Width > maxW {
			maxW = min.Width
		}
		if min.Height > maxH {
			maxH = min.Height
		}
	}
	width := maxW*float32(len(objects)) + padding*float32(len(objects)-1)
	return fyne.NewSize(width, maxH)
}

type statsCell struct {
	*fyne.Container
	icon *widget.Icon
	text *widget.Label
}

func newStatsCell() *statsCell {
	icon := widget.NewIcon(nil)
	icon.Hide()
	text := widget.NewLabel("")
	text.Wrapping = fyne.TextWrapOff
	cell := container.NewBorder(nil, nil, icon, nil, text)
	return &statsCell{Container: cell, icon: icon, text: text}
}

type logRowView struct {
	*fyne.Container
	dot     *canvas.Circle
	time    *widget.Label
	message *widget.Label
}

func newLogRowView() *logRowView {
	dot := canvas.NewCircle(theme.Color(theme.ColorNameDisabled))
	dotSize := theme.TextSize() * 0.85
	dotHolder := container.NewVBox(
		layout.NewSpacer(),
		container.NewGridWrap(fyne.NewSize(dotSize, dotSize), dot),
		layout.NewSpacer(),
	)

	timeLabel := widget.NewLabelWithStyle("        ", fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	timeLabel.Wrapping = fyne.TextWrapOff
	timeLabel.Importance = widget.LowImportance

	msg := widget.NewLabel("")
	msg.Wrapping = fyne.TextWrapOff
	msg.TextStyle = fyne.TextStyle{Monospace: true}

	left := container.NewHBox(dotHolder, timeLabel)
	row := container.NewBorder(nil, nil, left, nil, msg)
	return &logRowView{Container: row, dot: dot, time: timeLabel, message: msg}
}
