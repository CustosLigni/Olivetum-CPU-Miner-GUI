package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"io"
	"math"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	appName            = "Olivetum Miner"
	configDirName      = "olivetum-miner-gui"
	configFileName     = "config.json"
	defaultStratumHost = "pool.olivetumchain.org"
	defaultStratumPort = 8008
	defaultRPCURL      = "http://127.0.0.1:8545"

	defaultNodeDataDirHint = "~/.olivetum/node"
	defaultNodeRPCPort     = 8545
	defaultNodeP2PPort     = 31333
	defaultNodeVerbosity   = 3
	defaultNodeBootnodes   = "enode://9862175626bb4e6b983e3f50d8dcb9bd2b2fa1d9bd9ad38840f026ba4f4a87ea451e375945426cdb4fb6ac58a1624da4f8241f2b67e87f05c6f4922e97682279@pool.olivetumchain.org:31333"

	modeStratum    = "stratum"
	modeRPCLocal   = "rpc-local"
	modeRPCGateway = "rpc-gateway"

	backendAuto   = "auto"
	backendCUDA   = "cuda"
	backendOpenCL = "opencl"

	nodeModeSync = "sync"
	nodeModeMine = "mine"
)

type Config struct {
	Mode            string `json:"mode"`
	Backend         string `json:"backend"`
	StratumHost     string `json:"stratumHost"`
	StratumPort     int    `json:"stratumPort"`
	RPCURL          string `json:"rpcUrl"`
	WalletAddress   string `json:"walletAddress"`
	WorkerName      string `json:"workerName"`
	SelectedDevices []int  `json:"selectedDevices"`
	ReportHashrate  bool   `json:"reportHashrate"`
	DisplayInterval int    `json:"displayInterval"`
	HWMon           bool   `json:"hwMon"`

	NodeEnabled    bool   `json:"nodeEnabled"`
	NodeMode       string `json:"nodeMode"`
	NodeDataDir    string `json:"nodeDataDir"`
	NodeRPCPort    int    `json:"nodeRpcPort"`
	NodeP2PPort    int    `json:"nodeP2pPort"`
	NodeBootnodes  string `json:"nodeBootnodes"`
	NodeVerbosity  int    `json:"nodeVerbosity"`
	NodeEtherbase  string `json:"nodeEtherbase"`
	NodeCleanStart bool   `json:"nodeCleanStart"`

	WatchdogEnabled         bool `json:"watchdogEnabled"`
	WatchdogNoJobTimeoutSec int  `json:"watchdogNoJobTimeoutSec"`
	WatchdogRestartDelaySec int  `json:"watchdogRestartDelaySec"`
	WatchdogRetryWindowMin  int  `json:"watchdogRetryWindowMin"`
}

type Device struct {
	Index int
	PCI   string
	Name  string
}

type Stat struct {
	Version      string
	UptimeMin    int
	TotalKHs     int64
	Accepted     int64
	Rejected     int64
	Invalid      int64
	PoolSwitches int64
	PerGPU_KHs   []int64
	PerGPU_Power []float64
	Temps        []int
	Fans         []int
	Pool         string
	Difficulty   float64
}

func main() {
	a := app.NewWithID("org.olivetum.miner")
	a.Settings().SetTheme(olivetumDarkTheme{})
	w := a.NewWindow(appName)
	w.SetFixedSize(false)
	w.SetFullScreen(false)
	w.Resize(fyne.NewSize(1120, 760))

	cfg := loadConfig()

	ethminerPath, ethminerErr := findEthminer()

	modeLabels := []string{
		"Solo Pool (Stratum)",
		"Solo (Local RPC)",
		"Solo (RPC gateway)",
	}
	modeKeyForLabel := map[string]string{
		modeLabels[0]: modeStratum,
		modeLabels[1]: modeRPCLocal,
		modeLabels[2]: modeRPCGateway,
	}
	modeLabelForKey := map[string]string{
		modeStratum:    modeLabels[0],
		modeRPCLocal:   modeLabels[1],
		modeRPCGateway: modeLabels[2],
	}

	modeSelect := widget.NewSelect(modeLabels, nil)
	if initial, ok := modeLabelForKey[cfg.Mode]; ok && initial != "" {
		modeSelect.SetSelected(initial)
	} else {
		modeSelect.SetSelected(modeLabels[0])
	}

	selectedMode := func() string {
		if v, ok := modeKeyForLabel[strings.TrimSpace(modeSelect.Selected)]; ok {
			return v
		}
		return modeStratum
	}

	backendLabels := []string{
		"Auto (recommended)",
		"CUDA (NVIDIA)",
		"OpenCL (AMD/NVIDIA)",
	}
	backendKeyForLabel := map[string]string{
		backendLabels[0]: backendAuto,
		backendLabels[1]: backendCUDA,
		backendLabels[2]: backendOpenCL,
	}
	backendLabelForKey := map[string]string{
		backendAuto:   backendLabels[0],
		backendCUDA:   backendLabels[1],
		backendOpenCL: backendLabels[2],
	}
	backendSelect := widget.NewSelect(backendLabels, nil)
	if initial, ok := backendLabelForKey[cfg.Backend]; ok && initial != "" {
		backendSelect.SetSelected(initial)
	} else {
		backendSelect.SetSelected(backendLabels[0])
	}

	selectedBackend := func() string {
		if v, ok := backendKeyForLabel[strings.TrimSpace(backendSelect.Selected)]; ok {
			return v
		}
		return backendAuto
	}

	hostEntry := widget.NewEntry()
	hostEntry.SetText(cfg.StratumHost)
	hostEntry.SetPlaceHolder(defaultStratumHost)

	portEntry := widget.NewEntry()
	portEntry.SetText(strconv.Itoa(cfg.StratumPort))
	portEntry.SetPlaceHolder(strconv.Itoa(defaultStratumPort))

	walletEntry := widget.NewEntry()
	walletEntry.SetText(cfg.WalletAddress)
	walletEntry.SetPlaceHolder("0x...")

	workerEntry := widget.NewEntry()
	workerEntry.SetText(cfg.WorkerName)
	workerEntry.SetPlaceHolder("optional (e.g. rig1)")

	rpcEntry := widget.NewEntry()
	rpcEntry.SetText(cfg.RPCURL)
	rpcEntry.SetPlaceHolder(defaultRPCURL)

	nodeEnabledCheck := widget.NewCheck("Run a node", nil)
	nodeEnabledCheck.SetChecked(cfg.NodeEnabled)

	nodeModeLabels := []string{
		"Sync only",
		"Sync + mining service (CPU 1 thread)",
	}
	nodeModeKeyForLabel := map[string]string{
		nodeModeLabels[0]: nodeModeSync,
		nodeModeLabels[1]: nodeModeMine,
	}
	nodeModeLabelForKey := map[string]string{
		nodeModeSync: nodeModeLabels[0],
		nodeModeMine: nodeModeLabels[1],
	}
	nodeModeSelect := widget.NewSelect(nodeModeLabels, nil)
	if initial, ok := nodeModeLabelForKey[cfg.NodeMode]; ok && initial != "" {
		nodeModeSelect.SetSelected(initial)
	} else {
		nodeModeSelect.SetSelected(nodeModeLabels[0])
	}
	selectedNodeMode := func() string {
		if v, ok := nodeModeKeyForLabel[strings.TrimSpace(nodeModeSelect.Selected)]; ok {
			return v
		}
		return nodeModeSync
	}

	nodeEtherbaseEntry := widget.NewEntry()
	nodeEtherbaseEntry.SetText(cfg.NodeEtherbase)
	nodeEtherbaseEntry.SetPlaceHolder("0x...")

	nodeDataDirEntry := widget.NewEntry()
	if strings.TrimSpace(cfg.NodeDataDir) != "" {
		nodeDataDirEntry.SetText(cfg.NodeDataDir)
	}
	nodeDataDirEntry.SetPlaceHolder(defaultNodeDataDirHint)

	nodeRPCPortEntry := widget.NewEntry()
	if cfg.NodeRPCPort > 0 {
		nodeRPCPortEntry.SetText(strconv.Itoa(cfg.NodeRPCPort))
	}
	nodeRPCPortEntry.SetPlaceHolder(strconv.Itoa(defaultNodeRPCPort))

	nodeP2PPortEntry := widget.NewEntry()
	if cfg.NodeP2PPort > 0 {
		nodeP2PPortEntry.SetText(strconv.Itoa(cfg.NodeP2PPort))
	}
	nodeP2PPortEntry.SetPlaceHolder(strconv.Itoa(defaultNodeP2PPort))

	nodeBootnodesEntry := widget.NewMultiLineEntry()
	nodeBootnodesEntry.SetText(cfg.NodeBootnodes)
	nodeBootnodesEntry.SetPlaceHolder(defaultNodeBootnodes)

	nodeVerbosityEntry := widget.NewEntry()
	if cfg.NodeVerbosity > 0 {
		nodeVerbosityEntry.SetText(strconv.Itoa(cfg.NodeVerbosity))
	}
	nodeVerbosityEntry.SetPlaceHolder(strconv.Itoa(defaultNodeVerbosity))

	nodeCleanStartCheck := widget.NewCheck("Start with clean database (next start)", nil)
	nodeCleanStartCheck.SetChecked(cfg.NodeCleanStart)

	watchdogEnabledCheck := widget.NewCheck("Enable watchdog (restart miner if jobs stop)", nil)
	watchdogEnabledCheck.SetChecked(cfg.WatchdogEnabled)

	watchdogNoJobEntry := widget.NewEntry()
	watchdogNoJobEntry.SetText(strconv.Itoa(cfg.WatchdogNoJobTimeoutSec))
	watchdogNoJobEntry.SetPlaceHolder("120")

	watchdogRestartDelayEntry := widget.NewEntry()
	watchdogRestartDelayEntry.SetText(strconv.Itoa(cfg.WatchdogRestartDelaySec))
	watchdogRestartDelayEntry.SetPlaceHolder("10")

	watchdogRetryWindowEntry := widget.NewEntry()
	watchdogRetryWindowEntry.SetText(strconv.Itoa(cfg.WatchdogRetryWindowMin))
	watchdogRetryWindowEntry.SetPlaceHolder("10")

	reportHashrateCheck := widget.NewCheck("Report hashrate to pool (-R)", nil)
	reportHashrateCheck.SetChecked(cfg.ReportHashrate)

	displayIntervalEntry := widget.NewEntry()
	displayIntervalEntry.SetText(strconv.Itoa(cfg.DisplayInterval))
	displayIntervalEntry.SetPlaceHolder("10")

	hwmonCheck := widget.NewCheck("Enable hardware monitoring (temps/fans/power)", nil)
	hwmonCheck.SetChecked(cfg.HWMon)

	statusDot := canvas.NewCircle(theme.Color(theme.ColorNameDisabled))
	statusDot.Resize(fyne.NewSize(10, 10))
	statusDotHolder := container.NewVBox(
		layout.NewSpacer(),
		container.NewGridWrap(fyne.NewSize(10, 10), statusDot),
		layout.NewSpacer(),
	)
	statusValue := widget.NewLabelWithStyle("Stopped", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	statusValue.Wrapping = fyne.TextWrapOff

	connectionBadgeLabel := widget.NewLabelWithStyle("Conn: Offline", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	connectionBadgeLabel.Wrapping = fyne.TextWrapOff
	connectionBadgeBg := canvas.NewRectangle(theme.Color(theme.ColorNameDisabledButton))
	connectionBadgeBg.StrokeColor = theme.Color(theme.ColorNameSeparator)
	connectionBadgeBg.StrokeWidth = 1
	connectionBadgeBg.CornerRadius = theme.Padding() * 2
	connectionBadge := container.NewMax(
		connectionBadgeBg,
		container.NewPadded(container.NewCenter(connectionBadgeLabel)),
	)
	connOfflineColor := theme.Color(theme.ColorNameDisabledButton)
	connConnectingColor := theme.Color(theme.ColorNameHover)
	connLiveColor := theme.Color(theme.ColorNamePrimary)
	setConnectionBadge := func(text string, fill color.Color) {
		connectionBadgeLabel.SetText(text)
		connectionBadgeBg.FillColor = fill
		connectionBadgeBg.Refresh()
	}

	nodeBadgeLabel := widget.NewLabelWithStyle("Node: Off", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	nodeBadgeLabel.Wrapping = fyne.TextWrapOff
	nodeBadgeBg := canvas.NewRectangle(theme.Color(theme.ColorNameDisabledButton))
	nodeBadgeBg.StrokeColor = theme.Color(theme.ColorNameSeparator)
	nodeBadgeBg.StrokeWidth = 1
	nodeBadgeBg.CornerRadius = theme.Padding() * 2
	nodeBadge := container.NewMax(
		nodeBadgeBg,
		container.NewPadded(container.NewCenter(nodeBadgeLabel)),
	)
	setNodeBadge := func(text string, fill color.Color) {
		nodeBadgeLabel.SetText(text)
		nodeBadgeBg.FillColor = fill
		nodeBadgeBg.Refresh()
	}

	hashrateValue := canvas.NewText("—", theme.Color(theme.ColorNameForeground))
	hashrateValue.Alignment = fyne.TextAlignLeading
	hashrateValue.TextStyle = fyne.TextStyle{Bold: true}
	hashrateValue.TextSize = theme.TextSize() * 2.6

	setStatusDot := func(fill color.Color) {
		statusDot.FillColor = fill
		statusDot.Refresh()
	}

	setStatusText := func(text string) {
		statusValue.SetText(text)
	}

	sharesValue := widget.NewLabel("—")
	poolValue := widget.NewLabel("—")
	poolValue.Wrapping = fyne.TextWrapWord
	uptimeValue := widget.NewLabel("—")
	backendInUseValue := widget.NewLabel("—")

	currentBlockValue := widget.NewLabelWithStyle("—", fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	currentDifficultyValue := widget.NewLabelWithStyle("—", fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	lastFoundBlockValue := widget.NewLabelWithStyle("—", fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})

	sharesTile, sharesTileBg := metricTileWithIconBg("Shares", theme.ConfirmIcon(), sharesValue)
	hashrateHistory := newHashrateChart(300) // ~10 minutes at 2s polling
	avgHashrateValue := widget.NewLabelWithStyle("Avg —", fyne.TextAlignTrailing, fyne.TextStyle{Monospace: true})
	avgHashrateValue.Wrapping = fyne.TextWrapOff
	avgHashrateValue.Importance = widget.MediumImportance

	blendColor := func(a, b color.NRGBA, t float32) color.NRGBA {
		if t < 0 {
			t = 0
		}
		if t > 1 {
			t = 1
		}
		return color.NRGBA{
			R: uint8(float32(a.R)*(1-t) + float32(b.R)*t),
			G: uint8(float32(a.G)*(1-t) + float32(b.G)*t),
			B: uint8(float32(a.B)*(1-t) + float32(b.B)*t),
			A: 0xFF,
		}
	}

	var sharesHighlight *fyne.Animation
	highlightShares := func() {
		if sharesTileBg == nil {
			return
		}
		if sharesHighlight != nil {
			sharesHighlight.Stop()
		}
		base := toNRGBA(theme.Color(theme.ColorNameInputBackground))
		accent := toNRGBA(theme.Color(theme.ColorNamePrimary))
		flash := blendColor(base, accent, 0.35)
		sharesHighlight = canvas.NewColorRGBAAnimation(base, flash, canvas.DurationShort*2, func(c color.Color) {
			sharesTileBg.FillColor = c
			sharesTileBg.Refresh()
		})
		sharesHighlight.AutoReverse = true
		sharesHighlight.RepeatCount = 0
		sharesHighlight.Curve = fyne.AnimationEaseOut
		sharesHighlight.Start()
	}

	modeHint := widget.NewLabel("")
	modeHint.Wrapping = fyne.TextWrapWord
	modeHint.TextStyle = fyne.TextStyle{Italic: true}

	backendHint := widget.NewLabel("Tip: Auto uses CUDA on NVIDIA and OpenCL on AMD/Intel.")
	backendHint.Wrapping = fyne.TextWrapWord
	backendHint.TextStyle = fyne.TextStyle{Italic: true}

	backendResolvedHint := widget.NewLabel("")
	backendResolvedHint.Wrapping = fyne.TextWrapWord
	backendResolvedHint.TextStyle = fyne.TextStyle{Italic: true}

	devicesBox := container.NewVBox()
	devicesActivity := widget.NewActivity()
	devicesActivity.Hide()
	devicesLoadingLabel := widget.NewLabel("Detecting GPUs...")
	devicesLoadingRow := container.NewHBox(devicesActivity, devicesLoadingLabel)
	var (
		devMu        sync.Mutex
		devices      []Device
		deviceChecks []*widget.Check
	)
	deviceLabelByIndex := make(map[int]string)

	minerLogBuf := newRingLogs(5000)
	nodeLogBuf := newRingLogs(5000)

	minerFollowTailCheck := widget.NewCheck("Follow tail", nil)
	minerFollowTailCheck.SetChecked(true)
	nodeFollowTailCheck := widget.NewCheck("Follow tail", nil)
	nodeFollowTailCheck.SetChecked(true)
	wrapLogsCheck := widget.NewCheck("Wrap long lines", nil)
	wrapLogsCheck.SetChecked(true)
	var wrapLogsEnabled atomic.Bool
	wrapLogsEnabled.Store(wrapLogsCheck.Checked)
	logLineHeight := fyne.MeasureText("M", theme.TextSize(), fyne.TextStyle{Monospace: true}).Height
	baseLogRowHeight := logLineHeight + theme.Padding()*2
	const maxLogWrapLines = 12

	var (
		logSensorMu sync.RWMutex
		logSensors  = make(map[int]deviceSensors)
	)
	var (
		logsTabActive   atomic.Bool
		minerLogsActive atomic.Bool
		nodeLogsActive  atomic.Bool
	)

	var (
		minerLogSnapshotMu sync.RWMutex
		minerLogSnapshot   []string
		nodeLogSnapshotMu  sync.RWMutex
		nodeLogSnapshot    []string
	)
	minerLogLen := func() int {
		if !minerFollowTailCheck.Checked {
			minerLogSnapshotMu.RLock()
			defer minerLogSnapshotMu.RUnlock()
			if minerLogSnapshot != nil {
				return len(minerLogSnapshot)
			}
		}
		return minerLogBuf.Len()
	}
	minerLogAt := func(idx int) string {
		if !minerFollowTailCheck.Checked {
			minerLogSnapshotMu.RLock()
			defer minerLogSnapshotMu.RUnlock()
			if idx >= 0 && idx < len(minerLogSnapshot) {
				return minerLogSnapshot[idx]
			}
		}
		return minerLogBuf.At(idx)
	}
	nodeLogLen := func() int {
		if !nodeFollowTailCheck.Checked {
			nodeLogSnapshotMu.RLock()
			defer nodeLogSnapshotMu.RUnlock()
			if nodeLogSnapshot != nil {
				return len(nodeLogSnapshot)
			}
		}
		return nodeLogBuf.Len()
	}
	nodeLogAt := func(idx int) string {
		if !nodeFollowTailCheck.Checked {
			nodeLogSnapshotMu.RLock()
			defer nodeLogSnapshotMu.RUnlock()
			if idx >= 0 && idx < len(nodeLogSnapshot) {
				return nodeLogSnapshot[idx]
			}
		}
		return nodeLogBuf.At(idx)
	}

	logLineRe := regexp.MustCompile(`^([a-zA-Z])\s+(\d{2}:\d{2}:\d{2})\s+(.*)$`)
	logInfoColor := color.NRGBA{R: 0x60, G: 0xA5, B: 0xFA, A: 0xFF}
	logMinerColor := color.NRGBA{R: 0x7C, G: 0xB3, B: 0x42, A: 0xFF}
	logWarnColor := color.NRGBA{R: 0xFA, G: 0xCC, B: 0x15, A: 0xFF}
	logErrorColor := color.NRGBA{R: 0xF8, G: 0x71, B: 0x71, A: 0xFF}

	type logRowRefs struct {
		dot     *canvas.Circle
		time    *widget.Label
		message *widget.Label
		prefix  fyne.CanvasObject
	}
	var logRowRefsByContainer sync.Map

	parseLogLine := func(line string) (level, ts, msg string) {
		line = strings.TrimRight(line, "\r\n")
		if strings.TrimSpace(line) == "" {
			return "", "", ""
		}
		if m := logLineRe.FindStringSubmatch(line); m != nil {
			return strings.ToLower(m[1]), m[2], m[3]
		}
		return "", "", line
	}

	logColorForLine := func(level, raw string) color.Color {
		lower := strings.ToLower(raw)
		switch {
		case strings.Contains(lower, "error") || strings.Contains(lower, "fatal") || strings.Contains(lower, "failed"):
			return logErrorColor
		case strings.Contains(lower, "warn"):
			return logWarnColor
		}
		switch level {
		case "e":
			return logErrorColor
		case "w":
			return logWarnColor
		case "m":
			return logMinerColor
		case "i":
			return logInfoColor
		default:
			return theme.Color(theme.ColorNameDisabled)
		}
	}

	newLogRow := func() fyne.CanvasObject {
		dot := canvas.NewCircle(theme.Color(theme.ColorNameDisabled))
		dotSize := theme.TextSize() * 0.85
		dotHolder := container.NewVBox(
			layout.NewSpacer(),
			container.NewGridWrap(fyne.NewSize(dotSize, dotSize), dot),
			layout.NewSpacer(),
		)

		timeLabel := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
		timeLabel.Wrapping = fyne.TextWrapOff
		timeLabel.Importance = widget.LowImportance

		msg := widget.NewLabel("")
		msg.Wrapping = fyne.TextWrapOff
		msg.TextStyle = fyne.TextStyle{Monospace: true}

		prefix := container.NewHBox(dotHolder, timeLabel)
		row := container.NewBorder(nil, nil, prefix, nil, msg)
		logRowRefsByContainer.Store(row, &logRowRefs{dot: dot, time: timeLabel, message: msg, prefix: prefix})
		return row
	}

	applyLogRow := func(list *widget.List, id widget.ListItemID, item fyne.CanvasObject, line string) {
		row, ok := item.(*fyne.Container)
		if !ok {
			return
		}
		v, ok := logRowRefsByContainer.Load(row)
		if !ok {
			return
		}
		refs, ok := v.(*logRowRefs)
		if !ok || refs == nil {
			return
		}

		level, ts, msg := parseLogLine(line)
		refs.dot.FillColor = logColorForLine(level, line)
		refs.dot.Refresh()
		if ts != "" {
			refs.time.SetText(ts + " ")
		} else {
			refs.time.SetText("")
		}
		wrap := wrapLogsEnabled.Load()
		if wrap {
			refs.message.Wrapping = fyne.TextWrapBreak
		} else {
			refs.message.Wrapping = fyne.TextWrapOff
		}
		refs.message.SetText(msg)
		refs.message.Refresh()

		if list == nil {
			return
		}

		height := baseLogRowHeight
		if wrap && strings.TrimSpace(msg) != "" {
			rowWidth := item.Size().Width
			if rowWidth <= 0 {
				rowWidth = list.Size().Width
			}
			availableWidth := rowWidth
			if refs.prefix != nil {
				availableWidth -= refs.prefix.MinSize().Width
			}
			availableWidth -= theme.Padding() * 2
			if availableWidth < theme.TextSize()*4 {
				availableWidth = rowWidth
			}

			measured := fyne.MeasureText(msg, theme.TextSize(), refs.message.TextStyle)
			lines := int(math.Ceil(float64(measured.Width) / float64(availableWidth)))
			if lines < 1 {
				lines = 1
			}
			if lines > maxLogWrapLines {
				lines = maxLogWrapLines
			}
			height = float32(lines)*logLineHeight + theme.Padding()*2
			if height < baseLogRowHeight {
				height = baseLogRowHeight
			}
		}
		list.SetItemHeight(id, height)
	}

	var minerLogList *widget.List
	minerLogList = widget.NewList(
		func() int { return minerLogLen() },
		func() fyne.CanvasObject {
			return newLogRow()
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			applyLogRow(minerLogList, id, item, minerLogAt(int(id)))
		},
	)
	var nodeLogList *widget.List
	nodeLogList = widget.NewList(
		func() int { return nodeLogLen() },
		func() fyne.CanvasObject {
			return newLogRow()
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			applyLogRow(nodeLogList, id, item, nodeLogAt(int(id)))
		},
	)

	wrapLogsCheck.OnChanged = func(enabled bool) {
		wrapLogsEnabled.Store(enabled)
		minerLogList.Refresh()
		nodeLogList.Refresh()
	}

	minerFollowTailCheck.OnChanged = func(enabled bool) {
		if enabled {
			minerLogSnapshotMu.Lock()
			minerLogSnapshot = nil
			minerLogSnapshotMu.Unlock()
			minerLogList.Refresh()
			minerLogList.ScrollToBottom()
			return
		}
		minerLogSnapshotMu.Lock()
		minerLogSnapshot = minerLogBuf.Snapshot()
		minerLogSnapshotMu.Unlock()
		minerLogList.Refresh()
	}
	nodeFollowTailCheck.OnChanged = func(enabled bool) {
		if enabled {
			nodeLogSnapshotMu.Lock()
			nodeLogSnapshot = nil
			nodeLogSnapshotMu.Unlock()
			nodeLogList.Refresh()
			nodeLogList.ScrollToBottom()
			return
		}
		nodeLogSnapshotMu.Lock()
		nodeLogSnapshot = nodeLogBuf.Snapshot()
		nodeLogSnapshotMu.Unlock()
		nodeLogList.Refresh()
	}

	type statsHeaderCell struct {
		Label string
		Icon  fyne.Resource
	}
	statsHeader := []statsHeaderCell{
		{Label: "GPU"},
		{Label: "Name"},
		{Label: "Hashrate", Icon: iconHash},
		{Label: "Temp", Icon: iconThermometer},
		{Label: "Fan", Icon: iconFan},
		{Label: "Power", Icon: iconBolt},
	}
	statsColWidths := []float32{60, 260, 130, 90, 90, 100}
	statsHeaderHeight := theme.TextSize() * 1.8
	statsHeaderRow := func() fyne.CanvasObject {
		iconSize := theme.TextSize() * 1.1
		cells := make([]fyne.CanvasObject, 0, len(statsHeader))
		for i, cell := range statsHeader {
			label := widget.NewLabelWithStyle(cell.Label, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			label.Wrapping = fyne.TextWrapOff
			var content fyne.CanvasObject = label
			if cell.Icon != nil {
				icon := widget.NewIcon(cell.Icon)
				content = container.NewHBox(container.NewGridWrap(fyne.NewSize(iconSize, iconSize), icon), label)
			}
			width := float32(120)
			if i >= 0 && i < len(statsColWidths) {
				width = statsColWidths[i]
			}
			cells = append(cells, fixedSize(fyne.NewSize(width, statsHeaderHeight), content))
		}
		return container.NewHBox(cells...)
	}()
	type statsRow struct {
		Index    int
		Name     string
		Hashrate float64
		Temp     int
		Fan      int
		Power    float64
	}
	var (
		statsMu    sync.RWMutex
		statsRows  []statsRow
		lastStat   *Stat
		lastStatMu sync.RWMutex
	)

	statsTable := widget.NewTable(
		func() (int, int) {
			statsMu.RLock()
			defer statsMu.RUnlock()
			return len(statsRows), len(statsHeader)
		},
		func() fyne.CanvasObject {
			l := widget.NewLabel("")
			l.Wrapping = fyne.TextWrapOff
			return l
		},
		func(id widget.TableCellID, obj fyne.CanvasObject) {
			text := obj.(*widget.Label)
			text.Alignment = fyne.TextAlignLeading
			text.TextStyle = fyne.TextStyle{}
			row := id.Row
			statsMu.RLock()
			var data statsRow
			if row >= 0 && row < len(statsRows) {
				data = statsRows[row]
			}
			statsMu.RUnlock()

			switch id.Col {
			case 0:
				text.SetText(fmt.Sprintf("#%d", data.Index))
			case 1:
				text.SetText(data.Name)
			case 2:
				text.Alignment = fyne.TextAlignTrailing
				text.TextStyle = fyne.TextStyle{Monospace: true}
				if data.Hashrate > 0 {
					text.SetText(fmt.Sprintf("%.2f MH/s", data.Hashrate))
				} else {
					text.SetText("—")
				}
			case 3:
				text.Alignment = fyne.TextAlignTrailing
				text.TextStyle = fyne.TextStyle{Monospace: true}
				if data.Temp > 0 {
					text.SetText(fmt.Sprintf("%d°C", data.Temp))
				} else {
					text.SetText("—")
				}
			case 4:
				text.Alignment = fyne.TextAlignTrailing
				text.TextStyle = fyne.TextStyle{Monospace: true}
				if data.Fan > 0 {
					text.SetText(fmt.Sprintf("%d%%", data.Fan))
				} else {
					text.SetText("—")
				}
			case 5:
				text.Alignment = fyne.TextAlignTrailing
				text.TextStyle = fyne.TextStyle{Monospace: true}
				if data.Power >= 0 {
					text.SetText(fmt.Sprintf("%.0f W", data.Power))
				} else {
					text.SetText("—")
				}
			default:
				text.SetText("")
			}
			text.Refresh()
		},
	)
	for i, w := range statsColWidths {
		statsTable.SetColumnWidth(i, w)
	}

	updateStatsTable := func(s Stat) {
		devMu.Lock()
		labelMap := make(map[int]string, len(deviceLabelByIndex))
		maxIndex := -1
		for idx, label := range deviceLabelByIndex {
			labelMap[idx] = label
			if idx > maxIndex {
				maxIndex = idx
			}
		}
		devMu.Unlock()

		logSensorMu.RLock()
		fallbackSensors := make(map[int]deviceSensors, len(logSensors))
		for idx, sensor := range logSensors {
			fallbackSensors[idx] = sensor
			if idx > maxIndex {
				maxIndex = idx
			}
		}
		logSensorMu.RUnlock()

		maxCount := len(s.PerGPU_KHs)
		if len(s.Temps) > maxCount {
			maxCount = len(s.Temps)
		}
		if len(s.Fans) > maxCount {
			maxCount = len(s.Fans)
		}
		if len(s.PerGPU_Power) > maxCount {
			maxCount = len(s.PerGPU_Power)
		}
		if maxIndex+1 > maxCount {
			maxCount = maxIndex + 1
		}
		if maxCount < 0 {
			maxCount = 0
		}

		rows := make([]statsRow, 0, maxCount)
		for i := 0; i < maxCount; i++ {
			name := labelMap[i]
			if name == "" {
				name = fmt.Sprintf("GPU %d", i)
			}
			hashrate := -1.0
			if i < len(s.PerGPU_KHs) {
				hashrate = float64(s.PerGPU_KHs[i]) / 1000.0
			}
			temp := 0
			if i < len(s.Temps) && s.Temps[i] > 0 {
				temp = s.Temps[i]
			}
			fan := 0
			if i < len(s.Fans) && s.Fans[i] > 0 {
				fan = s.Fans[i]
			}
			power := -1.0
			if i < len(s.PerGPU_Power) && s.PerGPU_Power[i] >= 0 {
				power = s.PerGPU_Power[i]
			}
			if fallback, ok := fallbackSensors[i]; ok {
				if temp == 0 && fallback.Temp > 0 {
					temp = fallback.Temp
				}
				if fan == 0 && fallback.Fan > 0 {
					fan = fallback.Fan
				}
				if power < 0 && fallback.Power >= 0 {
					power = fallback.Power
				}
			}
			rows = append(rows, statsRow{
				Index:    i,
				Name:     name,
				Hashrate: hashrate,
				Temp:     temp,
				Fan:      fan,
				Power:    power,
			})
		}

		sort.Slice(rows, func(i, j int) bool { return rows[i].Index < rows[j].Index })

		statsMu.Lock()
		statsRows = rows
		statsMu.Unlock()
		statsTable.Refresh()
	}

	refreshStatsTable := func() {
		lastStatMu.RLock()
		var snapshot Stat
		if lastStat != nil {
			snapshot = *lastStat
		}
		lastStatMu.RUnlock()
		updateStatsTable(snapshot)
	}

	type logEvent struct {
		text  string
		reset bool
	}

	var (
		minerStartedAt            atomic.Int64
		lastJobAt                 atomic.Int64
		currentJobBlock           atomic.Int64
		lastFoundBlock            atomic.Int64
		jobDifficulty             atomic.Value
		nodeChainIssueDialogShown atomic.Bool
		nodeChainIssueCount       atomic.Int64
		nodeChainIssueFirstAt     atomic.Int64
	)
	jobDifficulty.Store("")

	minerLogEvents := make(chan logEvent, 4096)
	nodeLogEvents := make(chan logEvent, 4096)

	resetMinerLog := func() {
		logSensorMu.Lock()
		logSensors = make(map[int]deviceSensors)
		logSensorMu.Unlock()
		select {
		case minerLogEvents <- logEvent{reset: true}:
		default:
		}
	}
	resetNodeLog := func() {
		select {
		case nodeLogEvents <- logEvent{reset: true}:
		default:
		}
	}

	var resetNodeDataAndResync func(startAfter bool, requireConfirm bool)

	appendMinerLog := func(text string) {
		text = sanitizeLogLine(text)
		handleLine := func(line string) {
			if strings.Contains(line, "cu") || strings.Contains(line, "cl") {
				if m := gpuStatLine.FindStringSubmatch(line); len(m) == 5 {
					idx, _ := strconv.Atoi(m[1])
					temp, _ := strconv.Atoi(m[2])
					fan, _ := strconv.Atoi(m[3])
					power, _ := strconv.ParseFloat(m[4], 64)
					logSensorMu.Lock()
					logSensors[idx] = deviceSensors{Temp: temp, Fan: fan, Power: power}
					logSensorMu.Unlock()
				}
			}
			if m := jobBlockLine.FindStringSubmatch(line); len(m) == 2 {
				if block, err := strconv.ParseInt(m[1], 10, 64); err == nil && block > 0 {
					currentJobBlock.Store(block)
				}
				lastJobAt.Store(time.Now().UnixNano())
			}
			select {
			case minerLogEvents <- logEvent{text: line}:
			default:
				// Drop logs if the UI can't keep up.
			}
		}
		if strings.IndexByte(text, '\n') == -1 {
			handleLine(text)
			return
		}
		for _, line := range strings.Split(text, "\n") {
			handleLine(line)
		}
	}

	appendNodeLog := func(text string) {
		text = sanitizeLogLine(text)
		handleLine := func(line string) {
			if m := nodeMinedPotentialBlockLine.FindStringSubmatch(line); len(m) == 2 {
				n := strings.ReplaceAll(m[1], ",", "")
				if block, err := strconv.ParseInt(n, 10, 64); err == nil && block > 0 {
					lastFoundBlock.Store(block)
					fyne.Do(func() { lastFoundBlockValue.SetText(fmt.Sprintf("%d", block)) })
				}
			} else if m := nodeSealedNewBlockLine.FindStringSubmatch(line); len(m) == 2 {
				n := strings.ReplaceAll(m[1], ",", "")
				if block, err := strconv.ParseInt(n, 10, 64); err == nil && block > 0 {
					lastFoundBlock.Store(block)
					fyne.Do(func() { lastFoundBlockValue.SetText(fmt.Sprintf("%d", block)) })
				}
			}
			lower := strings.ToLower(line)
			isLocalDBIssue := (strings.Contains(lower, "failed to read") && strings.Contains(lower, "last block")) ||
				strings.Contains(lower, "missing trie node") ||
				(strings.Contains(lower, "failed to restore") && strings.Contains(lower, "runtime")) ||
				(strings.Contains(lower, "database") && strings.Contains(lower, "corrupt")) ||
				strings.Contains(lower, "chaindata is corrupt") ||
				strings.Contains(lower, "corruption") ||
				strings.Contains(lower, "fatal")
			if isLocalDBIssue && resetNodeDataAndResync != nil {
				isFatal := (strings.Contains(lower, "failed to read") && strings.Contains(lower, "last block")) ||
					(strings.Contains(lower, "database") && strings.Contains(lower, "corrupt")) ||
					strings.Contains(lower, "chaindata is corrupt") ||
					strings.Contains(lower, "corruption") ||
					strings.Contains(lower, "fatal")

				now := time.Now().UnixNano()
				const issueWindow = 45 * time.Second
				const issueThreshold = int64(3)

				firstAt := nodeChainIssueFirstAt.Load()
				if firstAt == 0 || now-firstAt > int64(issueWindow) {
					nodeChainIssueFirstAt.Store(now)
					nodeChainIssueCount.Store(1)
				} else {
					nodeChainIssueCount.Add(1)
				}

				shouldPrompt := isFatal || nodeChainIssueCount.Load() >= issueThreshold
				if shouldPrompt && nodeChainIssueDialogShown.CompareAndSwap(false, true) {
					fyne.Do(func() {
						msg := widget.NewLabel("A potential local database issue was detected.\n\nIf syncing continues normally, you can ignore this.\nIf the issue repeats after restart or the node cannot sync, a resync may help.")
						msg.Wrapping = fyne.TextWrapWord
						d := dialog.NewCustomConfirm(appName, "Reset node data & resync", "Dismiss", msg, func(ok bool) {
							if ok {
								resetNodeDataAndResync(true, false)
								return
							}
							nodeChainIssueDialogShown.Store(false)
						}, w)
						d.Show()
					})
				}
			}
			select {
			case nodeLogEvents <- logEvent{text: line}:
			default:
				// Drop logs if the UI can't keep up.
			}
		}
		if strings.IndexByte(text, '\n') == -1 {
			handleLine(text)
			return
		}
		for _, line := range strings.Split(text, "\n") {
			handleLine(line)
		}
	}

	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		dirty := false
		for {
			select {
			case ev := <-minerLogEvents:
				if ev.reset {
					minerLogBuf.Clear()
					dirty = true
					continue
				}
				minerLogBuf.Append(ev.text)
				dirty = true

			case <-ticker.C:
				if !dirty || !logsTabActive.Load() || !minerLogsActive.Load() {
					continue
				}
				minerLogSnapshotMu.RLock()
				paused := minerLogSnapshot != nil
				minerLogSnapshotMu.RUnlock()
				dirty = false
				if paused {
					continue
				}
				fyne.Do(func() {
					minerLogList.Refresh()
					minerLogList.ScrollToBottom()
				})
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		dirty := false
		for {
			select {
			case ev := <-nodeLogEvents:
				if ev.reset {
					nodeLogBuf.Clear()
					dirty = true
					continue
				}
				nodeLogBuf.Append(ev.text)
				dirty = true

			case <-ticker.C:
				if !dirty || !logsTabActive.Load() || !nodeLogsActive.Load() {
					continue
				}
				nodeLogSnapshotMu.RLock()
				paused := nodeLogSnapshot != nil
				nodeLogSnapshotMu.RUnlock()
				dirty = false
				if paused {
					continue
				}
				fyne.Do(func() {
					nodeLogList.Refresh()
					nodeLogList.ScrollToBottom()
				})
			}
		}
	}()

	refreshBtn := widget.NewButtonWithIcon("Refresh GPUs", theme.ViewRefreshIcon(), nil)

	quickPoolRow := container.NewGridWithColumns(2, hostEntry, portEntry)
	modeRow := formRow("Mode", modeSelect)
	walletRow := formRow("Wallet", walletEntry)
	workerRow := formRow("Worker", workerEntry)
	poolRow := formRow("Pool", quickPoolRow)
	rpcRow := formRow("RPC URL", rpcEntry)

	applyModeUI := func() {
		mode := selectedMode()
		switch mode {
		case modeStratum:
			poolRow.Show()
			workerRow.Show()
			walletRow.Show()
			rpcRow.Hide()
			rpcEntry.Enable()
			reportHashrateCheck.Enable()
			modeHint.SetText("Solo Pool (Stratum): solo mining; no node required; reward goes to the wallet above.")
		case modeRPCLocal:
			poolRow.Hide()
			workerRow.Hide()
			walletRow.Hide()
			rpcRow.Show()
			rpcEntry.Enable()
			reportHashrateCheck.Disable()
			modeHint.SetText("RPC local: mines to node coinbase; wallet/worker ignored. If you enable Run a node, set the mining address in Node settings.")
		case modeRPCGateway:
			poolRow.Hide()
			workerRow.Hide()
			walletRow.Show()
			rpcRow.Show()
			rpcEntry.Enable()
			reportHashrateCheck.Disable()
			modeHint.SetText("RPC gateway: node must support olivetumhash_getWorkFor; reward goes to wallet above.")
		default:
			modeHint.SetText("")
		}
	}
	modeSelect.OnChanged = func(_ string) {
		applyModeUI()
	}
	applyModeUI()

	refreshDevices := func() {
		if ethminerErr != nil {
			dialog.ShowError(fmt.Errorf("ethminer not found: %w", ethminerErr), w)
			return
		}
		refreshBtn.Disable()
		devicesActivity.Show()
		devicesActivity.Start()
		devicesBox.Objects = []fyne.CanvasObject{devicesLoadingRow}
		devicesBox.Refresh()

		go func() {
			backendSelection := selectedBackend()
			backend := resolveBackend(ethminerPath, backendSelection)
			list, out, err := listEthminerDevices(ethminerPath, backend)
			if err != nil {
				appendMinerLog(fmt.Sprintf("[devices] %v\n", err))
				fyne.Do(func() {
					devicesActivity.Stop()
					devicesActivity.Hide()
					if backendSelection == backendAuto {
						backendResolvedHint.SetText(fmt.Sprintf("Auto resolved to: %s", strings.ToUpper(backend)))
					} else {
						backendResolvedHint.SetText("")
					}
					devicesBox.Objects = []fyne.CanvasObject{
						widget.NewLabel("Failed to detect GPUs. See Logs for details."),
					}
					devicesBox.Refresh()
					refreshBtn.Enable()
				})
				return
			}

			selected := make(map[int]bool, len(cfg.SelectedDevices))
			for _, idx := range cfg.SelectedDevices {
				selected[idx] = true
			}
			var (
				newObjects []fyne.CanvasObject
				newChecks  []*widget.Check
			)
			if len(list) == 0 {
				backendName := "OpenCL"
				if backend == backendCUDA {
					backendName = "CUDA"
				}
				if strings.TrimSpace(out) != "" {
					appendMinerLog(fmt.Sprintf("[devices] %s --list-devices output:\n%s\n", backendName, strings.TrimSpace(out)))
				}
				newObjects = []fyne.CanvasObject{
					widget.NewLabel(fmt.Sprintf("No %s devices found. If you use NVIDIA, pick CUDA. For AMD/Intel, install drivers with OpenCL support.", backendName)),
				}
			} else {
				newObjects = make([]fyne.CanvasObject, 0, len(list))
				newChecks = make([]*widget.Check, 0, len(list))
				for _, d := range list {
					d := d
					check := widget.NewCheck(fmt.Sprintf("[%d] %s (%s)", d.Index, d.Name, d.PCI), nil)
					check.SetChecked(selected[d.Index])
					newChecks = append(newChecks, check)
					newObjects = append(newObjects, check)
				}
			}

			devMu.Lock()
			devices = list
			deviceChecks = newChecks
			deviceLabelByIndex = make(map[int]string, len(list))
			for _, d := range list {
				label := d.Name
				if strings.TrimSpace(d.PCI) != "" {
					label = fmt.Sprintf("%s (%s)", d.Name, d.PCI)
				}
				deviceLabelByIndex[d.Index] = label
			}
			devMu.Unlock()

			fyne.Do(func() {
				devicesActivity.Stop()
				devicesActivity.Hide()
				if backendSelection == backendAuto {
					backendResolvedHint.SetText(fmt.Sprintf("Auto resolved to: %s", strings.ToUpper(backend)))
				} else {
					backendResolvedHint.SetText("")
				}
				devicesBox.Objects = newObjects
				devicesBox.Refresh()
				refreshBtn.Enable()
				refreshStatsTable()
			})
		}()
	}
	refreshBtn.OnTapped = refreshDevices
	backendSelect.OnChanged = func(_ string) { refreshDevices() }

	var (
		procMu             sync.Mutex
		minerCmd           *exec.Cmd
		minerCtx           context.Context
		minerCancel        context.CancelFunc
		apiPort            int
		pollCancel         context.CancelFunc
		waitingForStats    atomic.Bool
		lastAccepted       atomic.Int64
		watchdogCancel     context.CancelFunc
		watchdogRestarting atomic.Bool

		nodeCmd     *exec.Cmd
		nodeCtx     context.Context
		nodeCancel  context.CancelFunc
		nodeRunMode string
	)

	var startBtn *widget.Button
	var stopBtn *widget.Button
	var nodeStartBtn *widget.Button
	var nodeStopBtn *widget.Button

	setRunningUI := func(running bool) {
		if running {
			if waitingForStats.Load() {
				setStatusText("Starting")
				setConnectionBadge("Conn: Connecting", connConnectingColor)
			} else {
				setStatusText("Running")
				setConnectionBadge("Conn: Live", connLiveColor)
			}
			setStatusDot(theme.Color(theme.ColorNamePrimary))
			if startBtn != nil {
				startBtn.Disable()
			}
			if stopBtn != nil {
				stopBtn.Enable()
			}
		} else {
			waitingForStats.Store(false)
			lastAccepted.Store(0)
			lastJobAt.Store(0)
			currentJobBlock.Store(0)
			lastFoundBlock.Store(0)
			jobDifficulty.Store("")
			setStatusText("Stopped")
			setStatusDot(theme.Color(theme.ColorNameDisabled))
			setConnectionBadge("Conn: Offline", connOfflineColor)
			hashrateValue.Text = "—"
			hashrateValue.Refresh()
			sharesValue.SetText("—")
			poolValue.SetText("—")
			uptimeValue.SetText("—")
			backendInUseValue.SetText("—")
			currentBlockValue.SetText("—")
			currentDifficultyValue.SetText("—")
			lastFoundBlockValue.SetText("—")
			hashrateHistory.Reset()
			avgHashrateValue.SetText("Avg —")
			lastStatMu.Lock()
			lastStat = nil
			lastStatMu.Unlock()
			updateStatsTable(Stat{})
			if startBtn != nil {
				startBtn.Enable()
			}
			if stopBtn != nil {
				stopBtn.Disable()
			}
		}
	}

	saveFromUI := func() error {
		mode := selectedMode()
		var err error

		host := strings.TrimSpace(hostEntry.Text)
		if host == "" {
			host = defaultStratumHost
		}

		var port int
		portText := strings.TrimSpace(portEntry.Text)
		if portText == "" {
			port = defaultStratumPort
		} else {
			port, err = strconv.Atoi(portText)
			if mode == modeStratum && (err != nil || port < 1 || port > 65535) {
				return errors.New("invalid stratum port")
			}
			if err != nil || port < 1 || port > 65535 {
				port = cfg.StratumPort
			}
		}

		rpcURLText := strings.TrimSpace(rpcEntry.Text)
		rpcURL := cfg.RPCURL
		if mode != modeStratum {
			rpcURL, err = normalizeRPCURL(rpcURLText)
			if err != nil {
				return err
			}
		} else if rpcURLText != "" {
			if normalized, err := normalizeRPCURL(rpcURLText); err == nil {
				rpcURL = normalized
			}
		}
		if rpcURL == "" {
			rpcURL = defaultRPCURL
		}

		wallet := strings.TrimSpace(walletEntry.Text)
		if mode != modeRPCLocal {
			if !isHexAddress(wallet) {
				return errors.New("invalid wallet address (expected 0x + 40 hex chars)")
			}
		}

		worker := strings.TrimSpace(workerEntry.Text)
		if mode == modeStratum {
			if worker != "" && !regexp.MustCompile(`^[0-9A-Za-z_-]{1,16}$`).MatchString(worker) {
				return errors.New("invalid worker name (allowed: 0-9 A-Z a-z _ -; max 16)")
			}
		}

		displayIntv := 10
		if strings.TrimSpace(displayIntervalEntry.Text) != "" {
			displayIntv, err = strconv.Atoi(strings.TrimSpace(displayIntervalEntry.Text))
			if err != nil || displayIntv < 1 || displayIntv > 1800 {
				return errors.New("invalid display interval (1..1800)")
			}
		}

		var selected []int
		devMu.Lock()
		for i, c := range deviceChecks {
			if c.Checked && i < len(devices) {
				selected = append(selected, devices[i].Index)
			}
		}
		devMu.Unlock()

		cfg.Mode = mode
		cfg.Backend = selectedBackend()
		cfg.StratumHost = host
		cfg.StratumPort = port
		cfg.RPCURL = rpcURL
		if isHexAddress(wallet) {
			cfg.WalletAddress = strings.ToLower(wallet)
		} else {
			cfg.WalletAddress = wallet
		}
		cfg.WorkerName = worker
		cfg.SelectedDevices = selected
		cfg.ReportHashrate = reportHashrateCheck.Checked
		cfg.DisplayInterval = displayIntv
		cfg.HWMon = hwmonCheck.Checked

		cfg.NodeEnabled = nodeEnabledCheck.Checked
		cfg.NodeMode = selectedNodeMode()

		cfg.NodeDataDir = strings.TrimSpace(nodeDataDirEntry.Text)

		nodeRPCPort := defaultNodeRPCPort
		if strings.TrimSpace(nodeRPCPortEntry.Text) != "" {
			nodeRPCPort, err = strconv.Atoi(strings.TrimSpace(nodeRPCPortEntry.Text))
			if err != nil || nodeRPCPort < 1 || nodeRPCPort > 65535 {
				return errors.New("invalid node RPC port")
			}
		}
		cfg.NodeRPCPort = nodeRPCPort

		nodeP2PPort := defaultNodeP2PPort
		if strings.TrimSpace(nodeP2PPortEntry.Text) != "" {
			nodeP2PPort, err = strconv.Atoi(strings.TrimSpace(nodeP2PPortEntry.Text))
			if err != nil || nodeP2PPort < 1 || nodeP2PPort > 65535 {
				return errors.New("invalid node P2P port")
			}
		}
		cfg.NodeP2PPort = nodeP2PPort

		nodeBootnodes := strings.TrimSpace(nodeBootnodesEntry.Text)
		if nodeBootnodes == "" {
			nodeBootnodes = defaultNodeBootnodes
		}
		cfg.NodeBootnodes = nodeBootnodes

		nodeVerbosity := defaultNodeVerbosity
		if strings.TrimSpace(nodeVerbosityEntry.Text) != "" {
			nodeVerbosity, err = strconv.Atoi(strings.TrimSpace(nodeVerbosityEntry.Text))
			if err != nil || nodeVerbosity < 0 || nodeVerbosity > 5 {
				return errors.New("invalid node verbosity (0..5)")
			}
		}
		cfg.NodeVerbosity = nodeVerbosity
		nodeEtherbase := strings.TrimSpace(nodeEtherbaseEntry.Text)
		if nodeEtherbase != "" && !isHexAddress(nodeEtherbase) {
			if cfg.NodeEnabled {
				return errors.New("invalid node mining address (expected 0x + 40 hex chars)")
			}
			cfg.NodeEtherbase = ""
		} else if isHexAddress(nodeEtherbase) {
			cfg.NodeEtherbase = strings.ToLower(nodeEtherbase)
		} else {
			cfg.NodeEtherbase = ""
		}
		cfg.NodeCleanStart = nodeCleanStartCheck.Checked

		cfg.WatchdogEnabled = watchdogEnabledCheck.Checked
		if text := strings.TrimSpace(watchdogNoJobEntry.Text); text != "" {
			if v, err := strconv.Atoi(text); err == nil && v >= 5 && v <= 3600 {
				cfg.WatchdogNoJobTimeoutSec = v
			} else if cfg.WatchdogEnabled {
				return errors.New("invalid watchdog no-job timeout (5..3600 seconds)")
			}
		}
		if text := strings.TrimSpace(watchdogRestartDelayEntry.Text); text != "" {
			if v, err := strconv.Atoi(text); err == nil && v >= 1 && v <= 600 {
				cfg.WatchdogRestartDelaySec = v
			} else if cfg.WatchdogEnabled {
				return errors.New("invalid watchdog restart delay (1..600 seconds)")
			}
		}
		if text := strings.TrimSpace(watchdogRetryWindowEntry.Text); text != "" {
			if v, err := strconv.Atoi(text); err == nil && v >= 1 && v <= 1440 {
				cfg.WatchdogRetryWindowMin = v
			} else if cfg.WatchdogEnabled {
				return errors.New("invalid watchdog retry window (1..1440 minutes)")
			}
		}
		return saveConfig(cfg)
	}

	saveDraftFromUI := func() {
		cfg.Mode = selectedMode()
		cfg.Backend = selectedBackend()

		if host := strings.TrimSpace(hostEntry.Text); host != "" {
			cfg.StratumHost = host
		} else if cfg.StratumHost == "" {
			cfg.StratumHost = defaultStratumHost
		}

		if portText := strings.TrimSpace(portEntry.Text); portText != "" {
			if port, err := strconv.Atoi(portText); err == nil && port >= 1 && port <= 65535 {
				cfg.StratumPort = port
			}
		} else if cfg.StratumPort == 0 {
			cfg.StratumPort = defaultStratumPort
		}

		if rpc := strings.TrimSpace(rpcEntry.Text); rpc != "" {
			cfg.RPCURL = rpc
		} else if cfg.RPCURL == "" {
			cfg.RPCURL = defaultRPCURL
		}

		cfg.WalletAddress = strings.TrimSpace(walletEntry.Text)
		cfg.WorkerName = strings.TrimSpace(workerEntry.Text)
		cfg.ReportHashrate = reportHashrateCheck.Checked
		cfg.HWMon = hwmonCheck.Checked

		if diText := strings.TrimSpace(displayIntervalEntry.Text); diText != "" {
			if di, err := strconv.Atoi(diText); err == nil && di >= 1 && di <= 1800 {
				cfg.DisplayInterval = di
			}
		} else if cfg.DisplayInterval == 0 {
			cfg.DisplayInterval = 10
		}

		var selected []int
		devMu.Lock()
		for i, c := range deviceChecks {
			if c.Checked && i < len(devices) {
				selected = append(selected, devices[i].Index)
			}
		}
		devMu.Unlock()
		cfg.SelectedDevices = selected

		cfg.NodeEnabled = nodeEnabledCheck.Checked
		cfg.NodeMode = selectedNodeMode()

		cfg.NodeDataDir = strings.TrimSpace(nodeDataDirEntry.Text)

		if portText := strings.TrimSpace(nodeRPCPortEntry.Text); portText != "" {
			if port, err := strconv.Atoi(portText); err == nil && port >= 1 && port <= 65535 {
				cfg.NodeRPCPort = port
			}
		} else if cfg.NodeRPCPort == 0 {
			cfg.NodeRPCPort = defaultNodeRPCPort
		}

		if portText := strings.TrimSpace(nodeP2PPortEntry.Text); portText != "" {
			if port, err := strconv.Atoi(portText); err == nil && port >= 1 && port <= 65535 {
				cfg.NodeP2PPort = port
			}
		} else if cfg.NodeP2PPort == 0 {
			cfg.NodeP2PPort = defaultNodeP2PPort
		}

		if bootnodes := strings.TrimSpace(nodeBootnodesEntry.Text); bootnodes != "" {
			cfg.NodeBootnodes = bootnodes
		} else if cfg.NodeBootnodes == "" {
			cfg.NodeBootnodes = defaultNodeBootnodes
		}

		if vText := strings.TrimSpace(nodeVerbosityEntry.Text); vText != "" {
			if v, err := strconv.Atoi(vText); err == nil && v >= 0 && v <= 5 {
				cfg.NodeVerbosity = v
			}
		} else if cfg.NodeVerbosity == 0 {
			cfg.NodeVerbosity = defaultNodeVerbosity
		}

		if wallet := strings.TrimSpace(nodeEtherbaseEntry.Text); isHexAddress(wallet) {
			cfg.NodeEtherbase = strings.ToLower(wallet)
		} else if wallet == "" {
			cfg.NodeEtherbase = ""
		}

		cfg.WatchdogEnabled = watchdogEnabledCheck.Checked
		if text := strings.TrimSpace(watchdogNoJobEntry.Text); text != "" {
			if v, err := strconv.Atoi(text); err == nil && v >= 5 && v <= 3600 {
				cfg.WatchdogNoJobTimeoutSec = v
			}
		}
		if text := strings.TrimSpace(watchdogRestartDelayEntry.Text); text != "" {
			if v, err := strconv.Atoi(text); err == nil && v >= 1 && v <= 600 {
				cfg.WatchdogRestartDelaySec = v
			}
		}
		if text := strings.TrimSpace(watchdogRetryWindowEntry.Text); text != "" {
			if v, err := strconv.Atoi(text); err == nil && v >= 1 && v <= 1440 {
				cfg.WatchdogRetryWindowMin = v
			}
		}

		_ = saveConfig(cfg)
	}

	type nodeStartSettings struct {
		Enabled    bool
		CleanStart bool
		Mode       string
		DataDir    string
		RPCPort    int
		P2PPort    int
		Bootnodes  string
		Verbosity  int
		Wallet     string
	}

	snapshotNodeConfigFromUI := func(requireMiningService bool) (nodeStartSettings, error) {
		var err error
		settings := nodeStartSettings{
			Enabled:    nodeEnabledCheck.Checked,
			CleanStart: nodeCleanStartCheck.Checked,
			Mode:       selectedNodeMode(),
		}

		settings.DataDir = strings.TrimSpace(nodeDataDirEntry.Text)

		nodeRPCPort := defaultNodeRPCPort
		if strings.TrimSpace(nodeRPCPortEntry.Text) != "" {
			nodeRPCPort, err = strconv.Atoi(strings.TrimSpace(nodeRPCPortEntry.Text))
			if err != nil || nodeRPCPort < 1 || nodeRPCPort > 65535 {
				return settings, errors.New("invalid node RPC port")
			}
		}
		settings.RPCPort = nodeRPCPort

		nodeP2PPort := defaultNodeP2PPort
		if strings.TrimSpace(nodeP2PPortEntry.Text) != "" {
			nodeP2PPort, err = strconv.Atoi(strings.TrimSpace(nodeP2PPortEntry.Text))
			if err != nil || nodeP2PPort < 1 || nodeP2PPort > 65535 {
				return settings, errors.New("invalid node P2P port")
			}
		}
		settings.P2PPort = nodeP2PPort

		nodeBootnodes := strings.TrimSpace(nodeBootnodesEntry.Text)
		if nodeBootnodes == "" {
			nodeBootnodes = defaultNodeBootnodes
		}
		settings.Bootnodes = nodeBootnodes

		nodeVerbosity := defaultNodeVerbosity
		if strings.TrimSpace(nodeVerbosityEntry.Text) != "" {
			nodeVerbosity, err = strconv.Atoi(strings.TrimSpace(nodeVerbosityEntry.Text))
			if err != nil || nodeVerbosity < 0 || nodeVerbosity > 5 {
				return settings, errors.New("invalid node verbosity (0..5)")
			}
		}
		settings.Verbosity = nodeVerbosity

		wallet := strings.TrimSpace(nodeEtherbaseEntry.Text)
		if wallet == "" {
			wallet = strings.TrimSpace(walletEntry.Text)
		}
		if settings.Enabled && (settings.Mode == nodeModeMine || requireMiningService) {
			if !isHexAddress(wallet) {
				return settings, errors.New("mining address is required for node mining (expected 0x + 40 hex chars)")
			}
			settings.Wallet = strings.ToLower(wallet)
		} else if isHexAddress(wallet) {
			settings.Wallet = strings.ToLower(wallet)
		}

		cfg.NodeEnabled = settings.Enabled
		cfg.NodeMode = settings.Mode
		cfg.NodeDataDir = settings.DataDir
		cfg.NodeRPCPort = settings.RPCPort
		cfg.NodeP2PPort = settings.P2PPort
		cfg.NodeBootnodes = settings.Bootnodes
		cfg.NodeVerbosity = settings.Verbosity
		cfg.NodeCleanStart = settings.CleanStart
		if etherbase := strings.TrimSpace(nodeEtherbaseEntry.Text); isHexAddress(etherbase) {
			cfg.NodeEtherbase = strings.ToLower(etherbase)
		} else {
			cfg.NodeEtherbase = ""
		}
		return settings, saveConfig(cfg)
	}

	setNodeButtons := func(running bool) {
		if nodeStartBtn != nil {
			if running || !nodeEnabledCheck.Checked {
				nodeStartBtn.Disable()
			} else {
				nodeStartBtn.Enable()
			}
		}
		if nodeStopBtn != nil {
			if running {
				nodeStopBtn.Enable()
			} else {
				nodeStopBtn.Disable()
			}
		}
	}

	startNodeWithSettings := func(settings nodeStartSettings, requireMiningService bool) error {
		procMu.Lock()
		if nodeCmd != nil && nodeCmd.Process != nil {
			procMu.Unlock()
			return nil
		}
		procMu.Unlock()

		gethPath, err := findGeth()
		if err != nil {
			return fmt.Errorf("geth not found: %w", err)
		}
		genesisPath, err := ensureGenesisFile()
		if err != nil {
			return fmt.Errorf("failed to prepare genesis file: %w", err)
		}

		dataDir := strings.TrimSpace(settings.DataDir)
		if dataDir == "" {
			dataDir = defaultNodeDataDir()
		}
		dataDir, err = expandUserPath(dataDir)
		if err != nil {
			return err
		}
		if dataDir == "" {
			return errors.New("node data directory is required")
		}
		if err := os.MkdirAll(dataDir, 0o755); err != nil {
			return err
		}

		if settings.CleanStart {
			appendNodeLog("\n[node] Cleaning local chain data...\n")
			if err := wipeNodeData(dataDir); err != nil {
				return err
			}
			settings.CleanStart = false
			cfg.NodeCleanStart = false
			_ = saveConfig(cfg)
			fyne.Do(func() { nodeCleanStartCheck.SetChecked(false) })
		}

		if !isGethInitialized(dataDir) {
			appendNodeLog("\n[node] Initializing chain data...\n")
			out, err := runGethInit(gethPath, dataDir, genesisPath)
			if strings.TrimSpace(out) != "" {
				appendNodeLog(out + "\n")
			}
			if err != nil {
				return err
			}
		}

		effectiveMode := settings.Mode
		if requireMiningService {
			effectiveMode = nodeModeMine
		}

		args := []string{
			"--datadir", dataDir,
			"--http", "--http.addr", "127.0.0.1", "--http.port", strconv.Itoa(settings.RPCPort),
			"--http.api", "eth,net,web3,miner,olivetumhash,olivetum",
			"--port", strconv.Itoa(settings.P2PPort),
			"--syncmode", "snap",
			"--gcmode", "full",
			"--bootnodes", strings.TrimSpace(settings.Bootnodes),
			"--verbosity", strconv.Itoa(settings.Verbosity),
		}
		if effectiveMode == nodeModeMine {
			if !isHexAddress(settings.Wallet) {
				return errors.New("wallet address is required for node mining")
			}
			args = append(args,
				"--mine",
				"--miner.threads", "1",
				"--miner.recommit=1s",
				"--miner.etherbase", settings.Wallet,
			)
		}

		nodeCtx, nodeCancel = context.WithCancel(context.Background())
		cmd := exec.CommandContext(nodeCtx, gethPath, args...)
		configureChildProcess(cmd)
		cmd.Env = append(os.Environ(), "LC_ALL=C")

		stdout, _ := cmd.StdoutPipe()
		stderr, _ := cmd.StderrPipe()

		appendNodeLog(fmt.Sprintf("\nStarting node: %s %s\n\n", gethPath, strings.Join(args, " ")))
		if err := cmd.Start(); err != nil {
			nodeCancel()
			nodeCtx = nil
			nodeCancel = nil
			fyne.Do(func() {
				setNodeBadge("Node: Off", connOfflineColor)
				setNodeButtons(false)
			})
			return err
		}

		procMu.Lock()
		nodeCmd = cmd
		nodeRunMode = effectiveMode
		procMu.Unlock()

		go streamLines(stdout, appendNodeLog)
		go streamLines(stderr, appendNodeLog)

		go func(ctx context.Context, port int) {
			ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()
			for {
				conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 750*time.Millisecond)
				if err == nil {
					_ = conn.Close()
					fyne.Do(func() { setNodeBadge("Node: Running", connLiveColor) })
					return
				}
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}(nodeCtx, settings.RPCPort)

		go func() {
			err := cmd.Wait()
			procMu.Lock()
			nodeCmd = nil
			nodeRunMode = ""
			if nodeCancel != nil {
				nodeCancel()
				nodeCancel = nil
			}
			procMu.Unlock()

			fyne.Do(func() {
				setNodeBadge("Node: Off", connOfflineColor)
				setNodeButtons(false)
			})
			if err != nil && !errors.Is(err, context.Canceled) {
				appendNodeLog(fmt.Sprintf("\n[node exit] %v\n", err))
			} else {
				appendNodeLog("\n[node exit] node stopped\n")
			}
		}()
		return nil
	}

	startNodeAsync := func(requireMiningService bool) error {
		settings, err := snapshotNodeConfigFromUI(requireMiningService)
		if err != nil {
			return err
		}
		if !settings.Enabled {
			return errors.New("node is disabled")
		}

		procMu.Lock()
		alreadyRunning := nodeCmd != nil && nodeCmd.Process != nil
		runningMode := nodeRunMode
		procMu.Unlock()
		if alreadyRunning {
			if requireMiningService && runningMode != nodeModeMine {
				return errors.New("node is running without mining service enabled; stop the node and start it again with mining enabled")
			}
			return nil
		}

		setNodeBadge("Node: Starting", connConnectingColor)
		setNodeButtons(true)
		go func(settings nodeStartSettings) {
			if err := startNodeWithSettings(settings, requireMiningService); err != nil {
				fyne.Do(func() {
					setNodeBadge("Node: Off", connOfflineColor)
					setNodeButtons(false)
					dialog.ShowError(err, w)
				})
			}
		}(settings)
		return nil
	}

	stopNode := func() {
		procMu.Lock()
		defer procMu.Unlock()
		if nodeCmd == nil || nodeCmd.Process == nil {
			return
		}
		appendNodeLog("\nStopping node...\n")
		cmd := nodeCmd
		proc := nodeCmd.Process
		if err := sendProcessInterrupt(proc); err != nil {
			appendNodeLog(fmt.Sprintf("[node] interrupt failed: %v\n", err))
		}
		go func(cmd *exec.Cmd, p *os.Process) {
			time.Sleep(60 * time.Second)
			procMu.Lock()
			still := nodeCmd == cmd
			procMu.Unlock()
			if still {
				appendNodeLog("[node] Force-killing node (timeout)\n")
				_ = p.Kill()
			}
		}(cmd, proc)
	}

	redactPath := func(p string) string {
		p = strings.TrimSpace(p)
		if p == "" {
			return ""
		}
		p = filepath.Clean(p)
		home, err := os.UserHomeDir()
		if err == nil && home != "" {
			home = filepath.Clean(home)
			if strings.HasPrefix(strings.ToLower(p), strings.ToLower(home+string(os.PathSeparator))) || strings.EqualFold(p, home) {
				rel, err := filepath.Rel(home, p)
				if err == nil && rel != "" && rel != "." {
					return filepath.Join("~", rel)
				}
				return "~"
			}
		}
		base := filepath.Base(p)
		dir := filepath.Dir(p)
		parent := filepath.Base(dir)
		if parent != "" && parent != "." && parent != string(os.PathSeparator) {
			return filepath.Join("…", parent, base)
		}
		return filepath.Join("…", base)
	}

	resetNodeDataAndResync = func(startAfter bool, requireConfirm bool) {
		if !nodeEnabledCheck.Checked {
			dialog.ShowInformation(appName, "Node is disabled", w)
			return
		}
		dataDir := strings.TrimSpace(nodeDataDirEntry.Text)
		if dataDir == "" {
			dataDir = defaultNodeDataDir()
		}
		dataDir, err := expandUserPath(dataDir)
		if err != nil {
			dialog.ShowError(err, w)
			return
		}
		if dataDir == "" {
			dialog.ShowError(errors.New("node data directory is required"), w)
			return
		}

		doReset := func() {
			go func(dataDir string) {
				fyne.Do(func() {
					setNodeBadge("Node: Resetting", connConnectingColor)
					setNodeButtons(true)
				})

				stopNode()
				deadline := time.Now().Add(90 * time.Second)
				for {
					procMu.Lock()
					running := nodeCmd != nil && nodeCmd.Process != nil
					procMu.Unlock()
					if !running {
						break
					}
					if time.Now().After(deadline) {
						fyne.Do(func() {
							setNodeBadge("Node: Off", connOfflineColor)
							setNodeButtons(false)
							dialog.ShowError(errors.New("node did not stop in time"), w)
						})
						return
					}
					time.Sleep(250 * time.Millisecond)
				}

				appendNodeLog("\n[node] Removing local chain data...\n")
				if err := wipeNodeData(dataDir); err != nil {
					fyne.Do(func() {
						setNodeBadge("Node: Off", connOfflineColor)
						setNodeButtons(false)
						dialog.ShowError(err, w)
					})
					return
				}
				nodeChainIssueDialogShown.Store(false)
				nodeChainIssueCount.Store(0)
				nodeChainIssueFirstAt.Store(0)

				if startAfter {
					fyne.Do(func() {
						if err := startNodeAsync(false); err != nil {
							setNodeBadge("Node: Off", connOfflineColor)
							setNodeButtons(false)
							dialog.ShowError(err, w)
						}
					})
				} else {
					fyne.Do(func() {
						setNodeBadge("Node: Off", connOfflineColor)
						setNodeButtons(false)
					})
				}
			}(dataDir)
		}

		if !requireConfirm {
			doReset()
			return
		}

		msg := widget.NewLabel(fmt.Sprintf("This will delete local chain data in %s and resync from scratch.\n\nAccounts (keystore) will be kept.", redactPath(dataDir)))
		msg.Wrapping = fyne.TextWrapWord
		d := dialog.NewCustomConfirm(appName, "Reset node data & resync", "Cancel", msg, func(ok bool) {
			if ok {
				doReset()
			}
		}, w)
		d.Show()
	}

	type minerStartOrigin int

	const (
		minerStartOriginUser minerStartOrigin = iota
		minerStartOriginWatchdog
	)

	type minerStopOrigin int

	const (
		minerStopOriginUser minerStopOrigin = iota
		minerStopOriginWatchdog
	)

	type watchdogSettings struct {
		NoJobTimeout time.Duration
		RestartDelay time.Duration
		RetryWindow  time.Duration
	}

	stopWatchdogSession := func() {
		procMu.Lock()
		cancel := watchdogCancel
		watchdogCancel = nil
		watchdogRestarting.Store(false)
		procMu.Unlock()
		if cancel != nil {
			cancel()
		}
	}

	var startMinerWithOrigin func(origin minerStartOrigin) error
	var stopMinerWithOrigin func(origin minerStopOrigin)

	waitForMinerExit := func(ctx context.Context, timeout time.Duration) bool {
		deadline := time.Now().Add(timeout)
		for {
			procMu.Lock()
			running := minerCmd != nil && minerCmd.Process != nil
			procMu.Unlock()
			if !running {
				return true
			}
			if time.Now().After(deadline) {
				return false
			}
			select {
			case <-ctx.Done():
				return false
			case <-time.After(200 * time.Millisecond):
			}
		}
	}

	startWatchdogSession := func(settings watchdogSettings) {
		procMu.Lock()
		if watchdogCancel != nil {
			procMu.Unlock()
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		watchdogCancel = cancel
		procMu.Unlock()

		appendMinerLog(fmt.Sprintf("[watchdog] Enabled (no-job %s, retry %s)\n",
			settings.NoJobTimeout, settings.RetryWindow))

		go func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()

			var (
				outageStart  time.Time
				lastSeenJob  int64
				restartCount int
			)

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}

				jobAt := lastJobAt.Load()
				if jobAt != 0 && jobAt != lastSeenJob {
					lastSeenJob = jobAt
					outageStart = time.Time{}
					restartCount = 0
					continue
				}

				refAt := jobAt
				if refAt == 0 {
					refAt = minerStartedAt.Load()
				}
				if refAt == 0 {
					refAt = time.Now().UnixNano()
				}
				elapsed := time.Since(time.Unix(0, refAt))
				if elapsed <= settings.NoJobTimeout {
					continue
				}

				if outageStart.IsZero() {
					outageStart = time.Now()
				}
				if settings.RetryWindow > 0 && time.Since(outageStart) > settings.RetryWindow {
					appendMinerLog(fmt.Sprintf("[watchdog] No jobs for %s (retry window reached). Stopping miner.\n", elapsed))
					stopMinerWithOrigin(minerStopOriginUser)
					return
				}

				if !watchdogRestarting.CompareAndSwap(false, true) {
					continue
				}
				restartCount++
				appendMinerLog(fmt.Sprintf("[watchdog] No jobs for %s. Restarting miner (attempt %d).\n", elapsed, restartCount))

				stopMinerWithOrigin(minerStopOriginWatchdog)
				_ = waitForMinerExit(ctx, 25*time.Second)

				select {
				case <-ctx.Done():
					watchdogRestarting.Store(false)
					return
				case <-time.After(settings.RestartDelay):
				}

				minerStartedAt.Store(time.Now().UnixNano())
				lastJobAt.Store(0)
				currentJobBlock.Store(0)

				if ctx.Err() != nil {
					watchdogRestarting.Store(false)
					return
				}

				startErrCh := make(chan error, 1)
				fyne.Do(func() {
					if ctx.Err() != nil {
						startErrCh <- ctx.Err()
						return
					}
					startErrCh <- startMinerWithOrigin(minerStartOriginWatchdog)
				})
				if err := <-startErrCh; err != nil {
					appendMinerLog(fmt.Sprintf("[watchdog] Restart failed: %v\n", err))
				}
				watchdogRestarting.Store(false)
			}
		}()
	}

	errMinerAlreadyRunning := errors.New("miner already running")

	stopMinerWithOrigin = func(origin minerStopOrigin) {
		if origin == minerStopOriginUser {
			stopWatchdogSession()
		}
		procMu.Lock()
		defer procMu.Unlock()
		if minerCmd == nil || minerCmd.Process == nil {
			return
		}
		appendMinerLog("\nStopping miner...\n")
		cmd := minerCmd
		proc := minerCmd.Process
		if err := sendProcessInterrupt(proc); err != nil {
			appendMinerLog(fmt.Sprintf("[miner] interrupt failed: %v\n", err))
		}
		go func(cmd *exec.Cmd, p *os.Process) {
			time.Sleep(10 * time.Second)
			procMu.Lock()
			still := minerCmd == cmd
			procMu.Unlock()
			if still {
				appendMinerLog("[miner] Force-killing miner (timeout)\n")
				_ = p.Kill()
			}
		}(cmd, proc)
	}

	startMinerWithOrigin = func(origin minerStartOrigin) error {
		if ethminerErr != nil {
			return fmt.Errorf("ethminer not found: %w", ethminerErr)
		}
		if origin == minerStartOriginUser {
			if err := saveFromUI(); err != nil {
				return err
			}
		}

		procMu.Lock()
		if minerCmd != nil && minerCmd.Process != nil {
			procMu.Unlock()
			return errMinerAlreadyRunning
		}

		port, err := pickFreePort()
		if err != nil {
			procMu.Unlock()
			return err
		}
		apiPort = port

		poolURL, err := buildPoolURL(cfg)
		if err != nil {
			procMu.Unlock()
			return err
		}

		if cfg.Mode == modeRPCLocal {
			nodeRunning := nodeCmd != nil && nodeCmd.Process != nil
			runningMode := nodeRunMode
			if cfg.NodeEnabled {
				if !nodeRunning {
					procMu.Unlock()
					return errors.New("node is enabled but not running; start it in Setup → Node")
				}
				if runningMode != nodeModeMine {
					procMu.Unlock()
					return errors.New("node is running without mining service enabled; restart the node with mining service enabled")
				}
			}

			u, err := url.Parse(poolURL)
			if err != nil || u.Host == "" {
				procMu.Unlock()
				return errors.New("invalid RPC URL")
			}
			host := u.Host
			if !strings.Contains(host, ":") {
				host += ":80"
			}
			conn, err := net.DialTimeout("tcp", host, 750*time.Millisecond)
			if err != nil {
				procMu.Unlock()
				return fmt.Errorf("RPC is not reachable at %s", host)
			}
			_ = conn.Close()
		}

		backendSelection := cfg.Backend
		backend := resolveBackend(ethminerPath, backendSelection)
		args := []string{
			"-G",
			"--olivetum",
			"--nocolor",
			"-P", poolURL,
			"--api-bind", fmt.Sprintf("127.0.0.1:-%d", apiPort),
			"--display-interval", strconv.Itoa(cfg.DisplayInterval),
		}
		if cfg.HWMon {
			args = append(args, "--HWMON", "2")
		}
		if backend == backendCUDA {
			args[0] = "-U"
		}
		if cfg.Mode == modeStratum && cfg.ReportHashrate {
			args = append(args, "--report-hashrate")
		}
		if len(cfg.SelectedDevices) > 0 {
			if backend == backendCUDA {
				args = append(args, "--cu-devices")
			} else {
				args = append(args, "--cl-devices")
			}
			for _, idx := range cfg.SelectedDevices {
				args = append(args, strconv.Itoa(idx))
			}
		}

		minerStartedAt.Store(time.Now().UnixNano())
		lastJobAt.Store(0)
		currentJobBlock.Store(0)
		lastFoundBlock.Store(0)
		jobDifficulty.Store("")

		minerCtx, minerCancel = context.WithCancel(context.Background())
		cmd := exec.CommandContext(minerCtx, ethminerPath, args...)
		configureChildProcess(cmd)
		cmd.Env = append(os.Environ(), "LC_ALL=C")

		stdout, _ := cmd.StdoutPipe()
		stderr, _ := cmd.StderrPipe()

		resetMinerLog()
		appendMinerLog(fmt.Sprintf("Starting: %s %s\n\n", ethminerPath, strings.Join(args, " ")))

		if err := cmd.Start(); err != nil {
			minerCancel()
			minerCtx = nil
			minerCancel = nil
			procMu.Unlock()
			return err
		}
		minerCmd = cmd
		waitingForStats.Store(true)
		lastAccepted.Store(0)

		pollCtx, pollCancelFn := context.WithCancel(context.Background())
		pollCancel = pollCancelFn
		procMu.Unlock()

		setRunningUI(true)
		if backendSelection == backendAuto {
			backendInUseValue.SetText(fmt.Sprintf("Auto → %s", strings.ToUpper(backend)))
		} else {
			backendInUseValue.SetText(strings.ToUpper(backend))
		}

		if origin == minerStartOriginUser && cfg.WatchdogEnabled {
			startWatchdogSession(watchdogSettings{
				NoJobTimeout: time.Duration(cfg.WatchdogNoJobTimeoutSec) * time.Second,
				RestartDelay: time.Duration(cfg.WatchdogRestartDelaySec) * time.Second,
				RetryWindow:  time.Duration(cfg.WatchdogRetryWindowMin) * time.Minute,
			})
		}

		go streamLines(stdout, appendMinerLog)
		go streamLines(stderr, appendMinerLog)

		go pollStats(pollCtx, "127.0.0.1", apiPort, cfg.HWMon, func(s Stat) {
			firstStat := waitingForStats.Swap(false)
			prevAccepted := lastAccepted.Swap(s.Accepted)
			hasNewAccept := s.Accepted > prevAccepted
			if s.Difficulty > 0 {
				jobDifficulty.Store(formatDifficulty(s.Difficulty))
			}
			updateLastFoundFromAccept := cfg.Mode != modeRPCLocal
			if hasNewAccept && updateLastFoundFromAccept {
				if block := currentJobBlock.Load(); block > 0 {
					lastFoundBlock.Store(block)
				}
			}
			statCopy := s
			statCopy.PerGPU_KHs = append([]int64(nil), s.PerGPU_KHs...)
			statCopy.PerGPU_Power = append([]float64(nil), s.PerGPU_Power...)
			statCopy.Temps = append([]int(nil), s.Temps...)
			statCopy.Fans = append([]int(nil), s.Fans...)
			lastStatMu.Lock()
			lastStat = &statCopy
			lastStatMu.Unlock()
			hs := fmt.Sprintf("%.2f MH/s", float64(s.TotalKHs)/1000.0)
			fyne.Do(func() {
				if firstStat {
					setStatusText("Running")
					setConnectionBadge("Conn: Live", connLiveColor)
				}
				hashrateValue.Text = hs
				hashrateValue.Refresh()
				hashrateHistory.Add(float64(s.TotalKHs) / 1000.0)
				if avg, ok := hashrateHistory.Average(); ok {
					avgHashrateValue.SetText(fmt.Sprintf("Avg %.2f MH/s", avg))
				} else {
					avgHashrateValue.SetText("Avg —")
				}
				sharesValue.SetText(fmt.Sprintf("Accepted %d | Rejected %d | Invalid %d", s.Accepted, s.Rejected, s.Invalid))
				if hasNewAccept {
					highlightShares()
				}
				poolValue.SetText(s.Pool)
				uptimeValue.SetText(fmt.Sprintf("%d min", s.UptimeMin))
				if block := currentJobBlock.Load(); block > 0 {
					currentBlockValue.SetText(fmt.Sprintf("%d", block))
				} else {
					currentBlockValue.SetText("—")
				}
				if diff, ok := jobDifficulty.Load().(string); ok && strings.TrimSpace(diff) != "" {
					currentDifficultyValue.SetText(diff)
				} else {
					currentDifficultyValue.SetText("—")
				}
				if block := lastFoundBlock.Load(); block > 0 {
					lastFoundBlockValue.SetText(fmt.Sprintf("%d", block))
				} else {
					lastFoundBlockValue.SetText("—")
				}
				updateStatsTable(statCopy)
			})
		}, func(err error) {
			// Only show transient failures in log; API might not be ready yet.
			appendMinerLog(fmt.Sprintf("[api] %v\n", err))
		})

		go func() {
			err := cmd.Wait()
			procMu.Lock()
			minerCmd = nil
			if pollCancel != nil {
				pollCancel()
				pollCancel = nil
			}
			if minerCancel != nil {
				minerCancel()
				minerCancel = nil
			}
			procMu.Unlock()

			fyne.Do(func() { setRunningUI(false) })
			if err != nil && !errors.Is(err, context.Canceled) {
				appendMinerLog(fmt.Sprintf("\n[exit] %v\n", err))
			} else {
				appendMinerLog("\n[exit] miner stopped\n")
			}
		}()
		return nil
	}

	startMinerUser := func() {
		err := startMinerWithOrigin(minerStartOriginUser)
		if err == nil {
			return
		}
		if errors.Is(err, errMinerAlreadyRunning) {
			dialog.ShowInformation(appName, "Miner already running", w)
			return
		}
		dialog.ShowError(err, w)
	}

	stopMinerUser := func() {
		stopMinerWithOrigin(minerStopOriginUser)
	}

	nodeStartBtn = widget.NewButtonWithIcon("Start node", theme.MediaPlayIcon(), func() {
		if err := startNodeAsync(false); err != nil {
			dialog.ShowError(err, w)
		}
	})
	nodeStartBtn.Importance = widget.HighImportance
	nodeStopBtn = widget.NewButtonWithIcon("Stop node", theme.MediaStopIcon(), stopNode)
	nodeStopBtn.Importance = widget.DangerImportance

	startBtn = widget.NewButtonWithIcon("Start mining", theme.MediaPlayIcon(), startMinerUser)
	startBtn.Importance = widget.HighImportance
	stopBtn = widget.NewButtonWithIcon("Stop", theme.MediaStopIcon(), stopMinerUser)
	stopBtn.Importance = widget.DangerImportance

	if ethminerErr != nil {
		startBtn.Disable()
		stopBtn.Disable()
	} else {
		stopBtn.Disable()
	}

	setNodeButtons(false)

	devicesScroll := container.NewVScroll(devicesBox)
	devicesScroll.SetMinSize(fyne.NewSize(0, 240))

	connectionBody := container.NewVBox(
		modeRow,
		modeHint,
		walletRow,
		workerRow,
		poolRow,
		rpcRow,
	)
	connectionPanel := panel("Connection", connectionBody)

	nodeDataDirBrowseBtn := widget.NewButtonWithIcon("Browse", theme.FolderOpenIcon(), func() {
		dialog.ShowFolderOpen(func(listable fyne.ListableURI, err error) {
			if err != nil {
				dialog.ShowError(err, w)
				return
			}
			if listable == nil {
				return
			}
			nodeDataDirEntry.SetText(listable.Path())
		}, w)
	})
	nodeDataDirField := container.NewBorder(nil, nil, nil, nodeDataDirBrowseBtn, nodeDataDirEntry)
	nodeModeRow := formRow("Node mode", nodeModeSelect)
	nodeDataDirRow := formRow("Data directory", nodeDataDirField)
	nodePortsGrid := container.NewGridWithColumns(2,
		fieldLabel("RPC port"), nodeRPCPortEntry,
		fieldLabel("P2P port"), nodeP2PPortEntry,
		fieldLabel("Verbosity"), nodeVerbosityEntry,
	)
	nodeAdvancedBody := container.NewVBox(
		nodePortsGrid,
		formRow("Bootnodes", nodeBootnodesEntry),
	)
	nodeAdvanced := widget.NewAccordion(widget.NewAccordionItem("Advanced", nodeAdvancedBody))
	nodeAdvanced.CloseAll()

	timeSyncLabel := widget.NewLabelWithStyle("Time sync: Unknown", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	timeSyncLabel.Wrapping = fyne.TextWrapOff
	timeSyncBg := canvas.NewRectangle(theme.Color(theme.ColorNameDisabledButton))
	timeSyncBg.StrokeColor = theme.Color(theme.ColorNameSeparator)
	timeSyncBg.StrokeWidth = 1
	timeSyncBg.CornerRadius = theme.Padding() * 2
	timeSyncBadge := container.NewMax(
		timeSyncBg,
		container.NewPadded(container.NewCenter(timeSyncLabel)),
	)
	timeSyncOkColor := theme.Color(theme.ColorNamePrimary)
	timeSyncBadColor := color.NRGBA{R: 0xF8, G: 0x71, B: 0x71, A: 0xFF}
	timeSyncUnknownColor := theme.Color(theme.ColorNameDisabledButton)
	setTimeSyncBadge := func(status timeSyncStatus) {
		if !status.Known {
			timeSyncLabel.SetText("Time sync: Unknown")
			timeSyncBg.FillColor = timeSyncUnknownColor
			timeSyncBg.Refresh()
			return
		}
		if status.Synchronized {
			timeSyncLabel.SetText("Time sync: OK")
			timeSyncBg.FillColor = timeSyncOkColor
			timeSyncBg.Refresh()
			return
		}
		timeSyncLabel.SetText("Time sync: NOT synchronized")
		timeSyncBg.FillColor = timeSyncBadColor
		timeSyncBg.Refresh()
	}
	refreshTimeSync := func(showDialog bool) {
		go func() {
			status := checkSystemTimeSync()
			fyne.Do(func() {
				setTimeSyncBadge(status)
				if !showDialog {
					return
				}
				if status.Known && !status.Synchronized {
					help := "Enable time synchronization (NTP) and try again."
					if runtime.GOOS == "linux" {
						help = "Linux: enable NTP (timedatectl set-ntp true) and verify:\n\n  timedatectl show -p NTPSynchronized --value\n\nExpected output: yes"
					} else if runtime.GOOS == "windows" {
						help = "Windows: enable automatic time sync in Date & time settings and ensure the Windows Time service is running."
					}
					msg := widget.NewLabel("System time is not synchronized. This may affect mining and node operation.\n\n" + help)
					msg.Wrapping = fyne.TextWrapWord
					dialog.ShowCustom("Time sync warning", "OK", msg, w)
					return
				}
				if !status.Known {
					msg := widget.NewLabel("Unable to determine system time synchronization status. Please ensure your system time is synchronized (NTP).")
					msg.Wrapping = fyne.TextWrapWord
					dialog.ShowCustom("Time sync status", "OK", msg, w)
				}
			})
		}()
	}
	timeSyncBtn := widget.NewButtonWithIcon("Check system time sync", theme.ViewRefreshIcon(), func() {
		refreshTimeSync(true)
	})
	timeSyncRow := container.NewHBox(timeSyncBadge, layout.NewSpacer(), timeSyncBtn)

	nodeHint := widget.NewLabel("Tip: For the embedded node, use RPC URL http://127.0.0.1:8545 (or your configured RPC port). For external nodes, disable Run a node and enter your RPC URL.")
	nodeHint.Wrapping = fyne.TextWrapWord
	nodeHint.TextStyle = fyne.TextStyle{Italic: true}

	nodeEtherbaseRow := formRow("Mining address", nodeEtherbaseEntry)
	nodeEtherbaseHint := widget.NewLabel("Used as --miner.etherbase when the mining service is enabled. Leave empty to reuse Wallet from Connection.")
	nodeEtherbaseHint.Wrapping = fyne.TextWrapWord
	nodeEtherbaseHint.TextStyle = fyne.TextStyle{Italic: true}

	watchdogGrid := container.NewGridWithColumns(2,
		fieldLabel("No-job timeout (s)"), watchdogNoJobEntry,
		fieldLabel("Restart delay (s)"), watchdogRestartDelayEntry,
		fieldLabel("Retry window (min)"), watchdogRetryWindowEntry,
	)
	watchdogHint := widget.NewLabel("Restarts the miner if it stops receiving new jobs. Useful for unstable connections or pool issues.")
	watchdogHint.Wrapping = fyne.TextWrapWord
	watchdogHint.TextStyle = fyne.TextStyle{Italic: true}
	watchdogFields := container.NewVBox(watchdogGrid, watchdogHint)
	if !watchdogEnabledCheck.Checked {
		watchdogFields.Hide()
	}
	watchdogEnabledCheck.OnChanged = func(enabled bool) {
		if enabled {
			watchdogFields.Show()
		} else {
			watchdogFields.Hide()
		}
	}

	nodeButtonsRow := container.NewHBox(nodeStartBtn, layout.NewSpacer(), nodeStopBtn)
	nodeResetBtn := widget.NewButtonWithIcon("Reset node data & resync", theme.DeleteIcon(), func() {
		resetNodeDataAndResync(true, true)
	})
	nodeResetBtn.Importance = widget.DangerImportance
	nodeAdvancedBody.Add(widget.NewSeparator())
	nodeAdvancedBody.Add(nodeCleanStartCheck)
	nodeAdvancedBody.Add(nodeResetBtn)

	nodeSettingsBox := container.NewVBox(
		timeSyncRow,
		nodeHint,
		nodeModeRow,
		nodeEtherbaseRow,
		nodeEtherbaseHint,
		nodeDataDirRow,
		nodeAdvanced,
		nodeButtonsRow,
	)
	if !nodeEnabledCheck.Checked {
		nodeSettingsBox.Hide()
	}
	nodeEnabledCheck.OnChanged = func(enabled bool) {
		if enabled {
			nodeSettingsBox.Show()
		} else {
			nodeSettingsBox.Hide()
		}
		procMu.Lock()
		running := nodeCmd != nil && nodeCmd.Process != nil
		procMu.Unlock()
		setNodeButtons(running)
		applyModeUI()
	}

	nodeBody := container.NewVBox(
		nodeEnabledCheck,
		nodeSettingsBox,
	)
	nodePanel := panel("Node", nodeBody)

	watchdogBody := container.NewVBox(
		watchdogEnabledCheck,
		watchdogFields,
	)
	watchdogPanel := panel("Watchdog", watchdogBody)

	hardwareGrid := container.NewGridWithColumns(2,
		fieldLabel("GPU backend"), backendSelect,
		fieldLabel("Display interval (s)"), displayIntervalEntry,
		fieldLabel("Hardware monitoring"), hwmonCheck,
		widget.NewLabel(""), reportHashrateCheck,
	)
	hardwareBody := container.NewVBox(
		hardwareGrid,
		backendHint,
		backendResolvedHint,
		widget.NewSeparator(),
		container.NewHBox(fieldLabel("GPUs"), layout.NewSpacer(), refreshBtn),
		devicesScroll,
	)
	hardwarePanel := panel("Hardware", hardwareBody)

	setupLeft := container.NewVBox(connectionPanel, nodePanel, watchdogPanel)
	setupLeftScroll := container.NewVScroll(setupLeft)
	setupSplit := container.NewHSplit(setupLeftScroll, hardwarePanel)
	setupSplit.Offset = 0.52
	setupTab := container.NewPadded(setupSplit)

	hashrate10mTitle := widget.NewLabelWithStyle("Hashrate (10 min)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	hashrate10mTitle.Wrapping = fyne.TextWrapOff
	hashrate10mHeader := container.NewHBox(widget.NewIcon(theme.HistoryIcon()), hashrate10mTitle, layout.NewSpacer(), avgHashrateValue)

	overviewGrid := container.NewGridWithColumns(4,
		metricTileWithIcon("Backend", theme.ComputerIcon(), backendInUseValue),
		metricTileWithIcon("Uptime", theme.HistoryIcon(), uptimeValue),
		sharesTile,
		metricTileWithIcon("Pool", theme.StorageIcon(), poolValue),
	)
	jobRow := container.New(&centeredTileRowLayout{Columns: 4},
		metricTileWithIcon("Current mining block", iconPickaxeWhite, currentBlockValue),
		metricTileWithIcon("Difficulty", theme.InfoIcon(), currentDifficultyValue),
		metricTileWithIcon("Last found", theme.SearchIcon(), lastFoundBlockValue),
	)
	overviewBody := container.NewVBox(
		fieldLabel("Total hashrate"),
		hashrateValue,
		overviewGrid,
		jobRow,
	)
	overviewPanel := panel("Overview", overviewBody)
	hashratePanel := panelWithHeader(hashrate10mHeader, hashrateHistory.Object())
	statsScroll := container.NewVScroll(statsTable)
	statsScroll.SetMinSize(fyne.NewSize(0, 220))
	statsBody := container.NewVBox(statsHeaderRow, widget.NewSeparator(), statsScroll)
	statsPanel := panel("Per-GPU", statsBody)
	dashboardStack := container.NewVBox(overviewPanel, hashratePanel, statsPanel)
	dashboardTab := container.NewPadded(container.NewVScroll(dashboardStack))

	minerLogLines := func() []string {
		if !minerFollowTailCheck.Checked {
			minerLogSnapshotMu.RLock()
			if minerLogSnapshot != nil {
				out := append([]string(nil), minerLogSnapshot...)
				minerLogSnapshotMu.RUnlock()
				return out
			}
			minerLogSnapshotMu.RUnlock()
		}
		return minerLogBuf.Snapshot()
	}
	nodeLogLines := func() []string {
		if !nodeFollowTailCheck.Checked {
			nodeLogSnapshotMu.RLock()
			if nodeLogSnapshot != nil {
				out := append([]string(nil), nodeLogSnapshot...)
				nodeLogSnapshotMu.RUnlock()
				return out
			}
			nodeLogSnapshotMu.RUnlock()
		}
		return nodeLogBuf.Snapshot()
	}

	minerCopyLogsBtn := widget.NewButtonWithIcon("Copy", theme.ContentCopyIcon(), func() {
		w.Clipboard().SetContent(strings.Join(minerLogLines(), "\n"))
	})
	minerClearLogsBtn := widget.NewButtonWithIcon("Clear", theme.ContentClearIcon(), resetMinerLog)
	minerLogBar := container.NewHBox(minerFollowTailCheck, layout.NewSpacer(), minerCopyLogsBtn, minerClearLogsBtn)
	minerLogPanel := panel("Miner Logs", container.NewBorder(minerLogBar, nil, nil, nil, container.NewPadded(minerLogList)))
	minerLogTab := container.NewPadded(minerLogPanel)

	nodeCopyLogsBtn := widget.NewButtonWithIcon("Copy", theme.ContentCopyIcon(), func() {
		w.Clipboard().SetContent(strings.Join(nodeLogLines(), "\n"))
	})
	nodeClearLogsBtn := widget.NewButtonWithIcon("Clear", theme.ContentClearIcon(), resetNodeLog)
	nodeLogBar := container.NewHBox(nodeFollowTailCheck, layout.NewSpacer(), nodeCopyLogsBtn, nodeClearLogsBtn)
	nodeLogPanel := panel("Node Logs", container.NewBorder(nodeLogBar, nil, nil, nil, container.NewPadded(nodeLogList)))
	nodeLogTab := container.NewPadded(nodeLogPanel)

	minerLogsItem := container.NewTabItemWithIcon("Miner", theme.ComputerIcon(), minerLogTab)
	nodeLogsItem := container.NewTabItemWithIcon("Node", theme.StorageIcon(), nodeLogTab)
	logTabs := container.NewAppTabs(minerLogsItem, nodeLogsItem)
	logTabs.OnSelected = func(item *container.TabItem) {
		minerLogsActive.Store(item == minerLogsItem)
		nodeLogsActive.Store(item == nodeLogsItem)
	}
	minerLogsActive.Store(true)
	nodeLogsActive.Store(false)

	logToolbar := container.NewHBox(wrapLogsCheck, layout.NewSpacer())
	logTab := container.NewPadded(container.NewBorder(logToolbar, nil, nil, nil, logTabs))

	setupItem := container.NewTabItemWithIcon("Setup", theme.SettingsIcon(), setupTab)
	dashboardItem := container.NewTabItemWithIcon("Dashboard", theme.HomeIcon(), dashboardTab)
	logsItem := container.NewTabItemWithIcon("Logs", theme.ListIcon(), logTab)
	tabs := container.NewAppTabs(setupItem, dashboardItem, logsItem)
	logsTabActive.Store(false)
	tabs.OnSelected = func(item *container.TabItem) {
		logsTabActive.Store(item == logsItem)
		if item == logsItem {
			selected := logTabs.Selected()
			minerLogsActive.Store(selected == minerLogsItem)
			nodeLogsActive.Store(selected == nodeLogsItem)
		}
	}

	headerTitle := canvas.NewText(appName, theme.Color(theme.ColorNamePrimary))
	headerTitle.TextStyle = fyne.TextStyle{Bold: true}
	headerTitle.TextSize = theme.TextSize() * 2.1
	headerSubtitle := widget.NewLabel("Modern GUI for ethminer (Olivetumhash)")
	headerSubtitle.Wrapping = fyne.TextWrapWord

	statusPillBg := canvas.NewRectangle(theme.Color(theme.ColorNameButton))
	statusPillBg.StrokeColor = theme.Color(theme.ColorNameSeparator)
	statusPillBg.StrokeWidth = 1
	statusPillBg.CornerRadius = theme.Padding()
	statusPill := container.NewMax(
		statusPillBg,
		container.NewPadded(container.NewCenter(container.NewHBox(statusDotHolder, statusValue))),
	)

	headerLeft := container.NewVBox(headerTitle, headerSubtitle)
	headerTileWidth := float32(180)
	headerTileHeight := headerLeft.MinSize().Height
	headerTileSize := fyne.NewSize(headerTileWidth, headerTileHeight)
	wrapHeaderTile := func(obj fyne.CanvasObject) fyne.CanvasObject {
		return fixedSize(headerTileSize, obj)
	}
	headerRight := container.NewHBox(
		wrapHeaderTile(nodeBadge),
		wrapHeaderTile(connectionBadge),
		wrapHeaderTile(statusPill),
		wrapHeaderTile(startBtn),
		wrapHeaderTile(stopBtn),
	)
	headerRow := container.NewHBox(headerLeft, layout.NewSpacer(), headerRight)

	headerBg := canvas.NewRectangle(theme.Color(theme.ColorNameInputBackground))
	headerBg.StrokeColor = theme.Color(theme.ColorNameSeparator)
	headerBg.StrokeWidth = 1
	header := container.NewMax(headerBg, container.NewPadded(headerRow))

	bg := canvas.NewLinearGradient(
		color.NRGBA{R: 0x0B, G: 0x0F, B: 0x14, A: 0xFF},
		color.NRGBA{R: 0x0F, G: 0x17, B: 0x2A, A: 0xFF},
		145,
	)
	main := container.NewBorder(container.NewVBox(header, widget.NewSeparator()), nil, nil, nil, tabs)
	w.SetContent(container.NewMax(bg, main))
	refreshTimeSync(false)

	if ethminerErr != nil {
		dialog.ShowError(fmt.Errorf("ethminer not found. Place it next to this app or in PATH: %w", ethminerErr), w)
	} else {
		refreshDevices()
	}

	w.SetCloseIntercept(func() {
		procMu.Lock()
		minerRunning := minerCmd != nil && minerCmd.Process != nil
		nodeRunning := nodeCmd != nil && nodeCmd.Process != nil
		procMu.Unlock()
		if !minerRunning && !nodeRunning {
			saveDraftFromUI()
			w.Close()
			return
		}
		message := "Services are running. Stop and quit?"
		if minerRunning && !nodeRunning {
			message = "Mining is running. Stop and quit?"
		} else if !minerRunning && nodeRunning {
			message = "Node is running. Stop and quit?"
		}
		dialog.ShowConfirm(appName, message, func(ok bool) {
			if ok {
				saveDraftFromUI()
				stopMinerUser()
				stopNode()
				time.AfterFunc(500*time.Millisecond, func() {
					fyne.Do(func() { w.Close() })
				})
			}
		}, w)
	})

	if runtime.GOOS == "linux" {
		appendMinerLog("Tip: You can run this as AppImage and launch from desktop.\n")
	}
	w.ShowAndRun()
}

func loadConfig() *Config {
	cfg := &Config{
		Mode:            modeStratum,
		Backend:         backendAuto,
		StratumHost:     defaultStratumHost,
		StratumPort:     defaultStratumPort,
		RPCURL:          defaultRPCURL,
		WalletAddress:   "",
		WorkerName:      "",
		SelectedDevices: nil,
		ReportHashrate:  true,
		DisplayInterval: 10,
		HWMon:           false,

		NodeEnabled:   false,
		NodeMode:      nodeModeSync,
		NodeDataDir:   "",
		NodeRPCPort:   defaultNodeRPCPort,
		NodeP2PPort:   defaultNodeP2PPort,
		NodeBootnodes: defaultNodeBootnodes,
		NodeVerbosity: defaultNodeVerbosity,
		NodeEtherbase: "",

		WatchdogEnabled:         false,
		WatchdogNoJobTimeoutSec: 120,
		WatchdogRestartDelaySec: 10,
		WatchdogRetryWindowMin:  10,
	}
	path, err := configPath()
	if err != nil {
		return cfg
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(b, cfg)
	if cfg.StratumHost == "" {
		cfg.StratumHost = defaultStratumHost
	}
	if cfg.StratumPort == 0 {
		cfg.StratumPort = defaultStratumPort
	}
	if cfg.Mode == "" {
		cfg.Mode = modeStratum
	}
	if cfg.Mode != modeStratum && cfg.Mode != modeRPCLocal && cfg.Mode != modeRPCGateway {
		cfg.Mode = modeStratum
	}
	if cfg.Backend == "" {
		cfg.Backend = backendAuto
	}
	if cfg.Backend != backendAuto && cfg.Backend != backendCUDA && cfg.Backend != backendOpenCL {
		cfg.Backend = backendAuto
	}
	if cfg.RPCURL == "" {
		cfg.RPCURL = defaultRPCURL
	}
	if cfg.DisplayInterval == 0 {
		cfg.DisplayInterval = 10
	}
	if cfg.NodeMode != nodeModeSync && cfg.NodeMode != nodeModeMine {
		cfg.NodeMode = nodeModeSync
	}
	if cfg.NodeDataDir != "" {
		if filepath.Clean(cfg.NodeDataDir) == filepath.Clean(defaultNodeDataDir()) {
			cfg.NodeDataDir = ""
		}
	}
	if cfg.NodeRPCPort == 0 {
		cfg.NodeRPCPort = defaultNodeRPCPort
	}
	if cfg.NodeP2PPort == 0 {
		cfg.NodeP2PPort = defaultNodeP2PPort
	}
	if cfg.NodeBootnodes == "" {
		cfg.NodeBootnodes = defaultNodeBootnodes
	}
	if cfg.NodeVerbosity == 0 {
		cfg.NodeVerbosity = defaultNodeVerbosity
	}
	if cfg.NodeEtherbase != "" {
		if !isHexAddress(cfg.NodeEtherbase) {
			cfg.NodeEtherbase = ""
		} else {
			cfg.NodeEtherbase = strings.ToLower(cfg.NodeEtherbase)
		}
	}
	if cfg.WatchdogNoJobTimeoutSec <= 0 {
		cfg.WatchdogNoJobTimeoutSec = 120
	}
	if cfg.WatchdogRestartDelaySec <= 0 {
		cfg.WatchdogRestartDelaySec = 10
	}
	if cfg.WatchdogRetryWindowMin <= 0 {
		cfg.WatchdogRetryWindowMin = 10
	}
	return cfg
}

func saveConfig(cfg *Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configDirName, configFileName), nil
}

func defaultNodeDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ""
	}
	return filepath.Join(home, ".olivetum", "node")
}

func isHexAddress(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) != 42 || !strings.HasPrefix(s, "0x") {
		return false
	}
	for _, c := range s[2:] {
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			continue
		}
		return false
	}
	return true
}

func normalizeRPCURL(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errors.New("RPC URL is required")
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("invalid RPC URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "getwork" {
		return "", fmt.Errorf("unsupported RPC URL scheme: %q (use http://)", u.Scheme)
	}
	if u.Host == "" {
		return "", errors.New("invalid RPC URL: missing host")
	}
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String(), nil
}

func buildPoolURL(cfg *Config) (string, error) {
	switch cfg.Mode {
	case modeStratum:
		if cfg.StratumHost == "" {
			return "", errors.New("missing stratum host")
		}
		if cfg.StratumPort < 1 || cfg.StratumPort > 65535 {
			return "", errors.New("invalid stratum port")
		}
		if !isHexAddress(cfg.WalletAddress) {
			return "", errors.New("invalid wallet address (expected 0x + 40 hex chars)")
		}
		user := cfg.WalletAddress
		if cfg.WorkerName != "" {
			user = user + "." + cfg.WorkerName
		}
		return fmt.Sprintf("stratum1+tcp://%s@%s:%d", user, cfg.StratumHost, cfg.StratumPort), nil

	case modeRPCLocal:
		return normalizeRPCURL(cfg.RPCURL)

	case modeRPCGateway:
		if !isHexAddress(cfg.WalletAddress) {
			return "", errors.New("invalid wallet address (expected 0x + 40 hex chars)")
		}
		rpcURL, err := normalizeRPCURL(cfg.RPCURL)
		if err != nil {
			return "", err
		}
		u, err := url.Parse(rpcURL)
		if err != nil {
			return "", fmt.Errorf("invalid RPC URL: %w", err)
		}
		if u.Scheme != "http" {
			return "", errors.New("RPC gateway requires http:// RPC URL")
		}
		if u.Path != "" && u.Path != "/" {
			return "", errors.New("RPC gateway requires RPC URL without a path")
		}
		return fmt.Sprintf("solo+http://%s/%s", u.Host, cfg.WalletAddress), nil

	default:
		return "", fmt.Errorf("unknown mining mode: %q", cfg.Mode)
	}
}

func findEthminer() (string, error) {
	names := []string{"ethminer"}
	if runtime.GOOS == "windows" {
		names = []string{"ethminer.exe", "ethminer"}
	}
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		for _, name := range names {
			candidate := filepath.Join(dir, name)
			if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
				return candidate, nil
			}
		}
	}
	for _, name := range names {
		p, err := exec.LookPath(name)
		if err == nil {
			return p, nil
		}
	}
	return "", errors.New("ethminer not found")
}

var deviceLine = regexp.MustCompile(`^\s*(\d+)\s+(\S+)\s+\S+\s+(.+?)\s+(Yes|No)\s+`)
var gpuStatLine = regexp.MustCompile(`\b(?:cu|cl)(\d+)\s+(?:[0-9]+(?:\.[0-9]+)?\s+)?(\d+)C\s+(\d+)%\s+([0-9]+(?:\.[0-9]+)?)W\b`)
var jobBlockLine = regexp.MustCompile(`\bJob:\s+\S+\s+block\s+(\d+)\b`)
var nodeMinedPotentialBlockLine = regexp.MustCompile(`\bMined potential block\b.*\bnumber=([0-9,]+)\b`)
var nodeSealedNewBlockLine = regexp.MustCompile(`\bSuccessfully sealed new block\b.*\bnumber=([0-9,]+)\b`)

func resolveBackend(ethminerPath string, backend string) string {
	if backend != backendAuto {
		return backend
	}
	if ethminerPath == "" {
		return backendOpenCL
	}
	list, _, err := listEthminerDevices(ethminerPath, backendCUDA)
	if err == nil && len(list) > 0 {
		return backendCUDA
	}
	return backendOpenCL
}

func listEthminerDevices(ethminerPath, backend string) ([]Device, string, error) {
	args := []string{"--list-devices"}
	if backend == backendCUDA {
		args = append([]string{"-U"}, args...)
	} else {
		args = append([]string{"-G"}, args...)
	}
	cmd := exec.Command(ethminerPath, args...)
	configureChildProcess(cmd)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.CombinedOutput()
	outStr := string(out)
	if err != nil {
		return nil, outStr, fmt.Errorf("failed to list devices: %w\n%s", err, outStr)
	}
	var res []Device
	sc := bufio.NewScanner(strings.NewReader(outStr))
	for sc.Scan() {
		line := sc.Text()
		m := deviceLine.FindStringSubmatch(line)
		if len(m) == 0 {
			continue
		}
		idx, _ := strconv.Atoi(m[1])
		res = append(res, Device{
			Index: idx,
			PCI:   m[2],
			Name:  strings.TrimSpace(m[3]),
		})
	}
	if err := sc.Err(); err != nil {
		return nil, outStr, err
	}
	return res, outStr, nil
}

func pickFreePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	_, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		return 0, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0, err
	}
	return port, nil
}

func streamLines(r io.Reader, onLine func(string)) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		onLine(sc.Text())
	}
}

type apiResp struct {
	Result json.RawMessage `json:"result"`
	Error  any             `json:"error"`
}

type deviceSensors struct {
	Temp  int
	Fan   int
	Power float64
}

type detailSnapshot struct {
	Sensors    map[int]deviceSensors
	Labels     map[int]string
	Hashes     map[int]int64
	Difficulty float64
}

type statDetail struct {
	Devices []struct {
		Index    int `json:"_index"`
		Hardware struct {
			Name    string    `json:"name"`
			PCI     string    `json:"pci"`
			Sensors []float64 `json:"sensors"`
		} `json:"hardware"`
		Mining struct {
			Hashrate string `json:"hashrate"`
		} `json:"mining"`
	} `json:"devices"`
	Mining struct {
		Difficulty float64 `json:"difficulty"`
	} `json:"mining"`
}

func pollStats(ctx context.Context, host string, port int, detail bool, onStat func(Stat), onErr func(error)) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	detailEvery := 3     // 6s cadence for sensors/hwmon
	difficultyEvery := 5 // 10s cadence for mining info
	detailTick := detailEvery - 1
	difficultyTick := difficultyEvery - 1
	var cachedDetail detailSnapshot

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			st, err := getStat1(host, port)
			if err != nil {
				onErr(err)
				continue
			}
			needSensors := detail || len(st.PerGPU_KHs) == 0
			needDetailCall := false
			if needSensors {
				detailTick++
				if detailTick >= detailEvery || (len(cachedDetail.Sensors) == 0 && len(cachedDetail.Hashes) == 0) {
					detailTick = 0
					needDetailCall = true
				}
			}
			difficultyTick++
			if difficultyTick >= difficultyEvery || cachedDetail.Difficulty == 0 {
				difficultyTick = 0
				needDetailCall = true
			}
			if needDetailCall {
				snapshot, err := getStatDetail(host, port)
				if err != nil {
					onErr(err)
				} else {
					cachedDetail = snapshot
				}
			}
			applyDetail(&st, cachedDetail)
			onStat(st)
		}
	}
}

func getStat1(host string, port int) (Stat, error) {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), 1*time.Second)
	if err != nil {
		return Stat{}, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(1500 * time.Millisecond))

	req := `{"id":1,"jsonrpc":"2.0","method":"miner_getstat1"}`
	if _, err := io.WriteString(conn, req+"\n"); err != nil {
		return Stat{}, err
	}
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return Stat{}, err
	}

	var resp apiResp
	if err := json.Unmarshal(line, &resp); err != nil {
		return Stat{}, err
	}
	if resp.Error != nil {
		return Stat{}, fmt.Errorf("api error: %v", resp.Error)
	}
	var arr []string
	if err := json.Unmarshal(resp.Result, &arr); err != nil {
		return Stat{}, err
	}
	if len(arr) < 9 {
		return Stat{}, fmt.Errorf("unexpected stat format (%d items)", len(arr))
	}

	st := Stat{Version: arr[0]}
	st.UptimeMin, _ = strconv.Atoi(arr[1])

	// "kh;accepted;rejected"
	if parts := strings.Split(arr[2], ";"); len(parts) >= 3 {
		st.TotalKHs, _ = strconv.ParseInt(parts[0], 10, 64)
		st.Accepted, _ = strconv.ParseInt(parts[1], 10, 64)
		st.Rejected, _ = strconv.ParseInt(parts[2], 10, 64)
	}

	// "kh1;kh2;..."
	if parts := strings.Split(arr[3], ";"); len(parts) > 0 && parts[0] != "" {
		for _, p := range parts {
			v, _ := strconv.ParseInt(p, 10, 64)
			st.PerGPU_KHs = append(st.PerGPU_KHs, v)
		}
	}

	// temps/fans pairs
	if parts := strings.Split(arr[6], ";"); len(parts) >= 2 {
		for i := 0; i+1 < len(parts); i += 2 {
			t, _ := strconv.Atoi(parts[i])
			f, _ := strconv.Atoi(parts[i+1])
			st.Temps = append(st.Temps, t)
			st.Fans = append(st.Fans, f)
		}
	}

	st.Pool = arr[7]

	// "ethInvalid;ethSwitches;dcrInvalid;dcrSwitches"
	if parts := strings.Split(arr[8], ";"); len(parts) >= 2 {
		st.Invalid, _ = strconv.ParseInt(parts[0], 10, 64)
		st.PoolSwitches, _ = strconv.ParseInt(parts[1], 10, 64)
	}
	return st, nil
}

func getStatDetail(host string, port int) (detailSnapshot, error) {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), 1*time.Second)
	if err != nil {
		return detailSnapshot{}, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(1500 * time.Millisecond))

	req := `{"id":1,"jsonrpc":"2.0","method":"miner_getstatdetail"}`
	if _, err := io.WriteString(conn, req+"\n"); err != nil {
		return detailSnapshot{}, err
	}
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return detailSnapshot{}, err
	}

	var resp apiResp
	if err := json.Unmarshal(line, &resp); err != nil {
		return detailSnapshot{}, err
	}
	if resp.Error != nil {
		return detailSnapshot{}, fmt.Errorf("api error: %v", resp.Error)
	}
	var detail statDetail
	if err := json.Unmarshal(resp.Result, &detail); err != nil {
		return detailSnapshot{}, err
	}

	snapshot := detailSnapshot{
		Sensors:    make(map[int]deviceSensors),
		Labels:     make(map[int]string),
		Hashes:     make(map[int]int64),
		Difficulty: detail.Mining.Difficulty,
	}
	for _, dev := range detail.Devices {
		temp := 0
		fan := 0
		power := -1.0
		if len(dev.Hardware.Sensors) >= 1 {
			temp = int(dev.Hardware.Sensors[0])
		}
		if len(dev.Hardware.Sensors) >= 2 {
			fan = int(dev.Hardware.Sensors[1])
		}
		if len(dev.Hardware.Sensors) >= 3 {
			power = dev.Hardware.Sensors[2]
		}
		snapshot.Sensors[dev.Index] = deviceSensors{Temp: temp, Fan: fan, Power: power}
		if kh, ok := parseHashrateHex(dev.Mining.Hashrate); ok {
			snapshot.Hashes[dev.Index] = kh
		}

		name := strings.TrimSpace(dev.Hardware.Name)
		if name != "" {
			label := name
			if pci := strings.TrimSpace(dev.Hardware.PCI); pci != "" {
				label = fmt.Sprintf("%s (%s)", name, pci)
			}
			snapshot.Labels[dev.Index] = label
		}
	}
	return snapshot, nil
}

func parseHashrateHex(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	s = strings.TrimPrefix(s, "0x")
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseUint(s, 16, 64)
	if err != nil {
		return 0, false
	}
	return int64(v / 1000), true
}

func formatDifficulty(diff float64) string {
	if diff <= 0 {
		return ""
	}
	suffixes := []string{"", "K", "M", "G", "T", "P", "E"}
	idx := 0
	for diff >= 1000 && idx < len(suffixes)-1 {
		diff /= 1000
		idx++
	}
	return fmt.Sprintf("%.2f %sH", diff, suffixes[idx])
}

func applyDetail(st *Stat, detail detailSnapshot) {
	st.Difficulty = detail.Difficulty
	if len(detail.Sensors) == 0 && len(detail.Hashes) == 0 {
		return
	}
	maxIndex := len(st.PerGPU_KHs)
	if len(st.Temps) > maxIndex {
		maxIndex = len(st.Temps)
	}
	if len(st.Fans) > maxIndex {
		maxIndex = len(st.Fans)
	}
	if len(st.PerGPU_Power) > maxIndex {
		maxIndex = len(st.PerGPU_Power)
	}
	for idx := range detail.Sensors {
		if idx+1 > maxIndex {
			maxIndex = idx + 1
		}
	}
	for idx := range detail.Hashes {
		if idx+1 > maxIndex {
			maxIndex = idx + 1
		}
	}
	if maxIndex == 0 {
		return
	}

	hashes := make([]int64, maxIndex)
	temps := make([]int, maxIndex)
	fans := make([]int, maxIndex)
	power := make([]float64, maxIndex)
	for i := range power {
		power[i] = -1
	}
	copy(hashes, st.PerGPU_KHs)
	copy(temps, st.Temps)
	copy(fans, st.Fans)
	copy(power, st.PerGPU_Power)

	for idx, v := range detail.Hashes {
		if idx < 0 || idx >= maxIndex {
			continue
		}
		if v > 0 {
			hashes[idx] = v
		}
	}
	for idx, s := range detail.Sensors {
		if idx < 0 || idx >= maxIndex {
			continue
		}
		if s.Temp > 0 {
			temps[idx] = s.Temp
		}
		if s.Fan > 0 {
			fans[idx] = s.Fan
		}
		if s.Power >= 0 {
			power[idx] = s.Power
		}
	}

	st.PerGPU_KHs = hashes
	st.Temps = temps
	st.Fans = fans
	st.PerGPU_Power = power
}

var ansiCSI = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

func sanitizeLogLine(s string) string {
	// Strip common terminal control sequences and keep things readable in a GUI.
	if strings.IndexByte(s, '\r') >= 0 {
		s = strings.ReplaceAll(s, "\r", "")
	}
	if strings.IndexByte(s, 0x1b) >= 0 {
		s = ansiCSI.ReplaceAllString(s, "")
	}

	// Fast path: keep common ethminer ASCII logs without extra allocations.
	asciiSafe := true
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b == '\n' || b == '\t' || b == ' ' || (b >= 0x21 && b <= 0x7e) {
			continue
		}
		asciiSafe = false
		break
	}
	if asciiSafe {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		// Keep printable + whitespace we care about.
		if r == '\n' || r == '\t' || r == ' ' || (!unicode.IsControl(r) && r != 0x7f) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

type ringLogs struct {
	mu    sync.RWMutex
	buf   []string
	start int
	size  int
}

func newRingLogs(maxLines int) *ringLogs {
	if maxLines < 1 {
		maxLines = 1
	}
	return &ringLogs{buf: make([]string, maxLines)}
}

func (r *ringLogs) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.buf {
		r.buf[i] = ""
	}
	r.start = 0
	r.size = 0
}

func (r *ringLogs) Append(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.buf) == 0 {
		return
	}
	if r.size < len(r.buf) {
		r.buf[(r.start+r.size)%len(r.buf)] = line
		r.size++
		return
	}
	r.buf[r.start] = line
	r.start = (r.start + 1) % len(r.buf)
}

func (r *ringLogs) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.size
}

func (r *ringLogs) At(i int) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if i < 0 || i >= r.size || len(r.buf) == 0 {
		return ""
	}
	return r.buf[(r.start+i)%len(r.buf)]
}

func (r *ringLogs) Snapshot() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.size == 0 || len(r.buf) == 0 {
		return []string{}
	}
	out := make([]string, r.size)
	for i := 0; i < r.size; i++ {
		out[i] = r.buf[(r.start+i)%len(r.buf)]
	}
	return out
}
