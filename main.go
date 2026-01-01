package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
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
	defaultStratumHost = "89.117.2.230"
	defaultStratumPort = 8008
	defaultRPCURL      = "http://127.0.0.1:18545"

	modeStratum    = "stratum"
	modeRPCLocal   = "rpc-local"
	modeRPCGateway = "rpc-gateway"

	backendAuto   = "auto"
	backendCUDA   = "cuda"
	backendOpenCL = "opencl"
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
	Temps        []int
	Fans         []int
	Pool         string
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
		"Pool (Stratum)",
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

	reportHashrateCheck := widget.NewCheck("Report hashrate to pool (-R)", nil)
	reportHashrateCheck.SetChecked(cfg.ReportHashrate)

	displayIntervalEntry := widget.NewEntry()
	displayIntervalEntry.SetText(strconv.Itoa(cfg.DisplayInterval))
	displayIntervalEntry.SetPlaceHolder("10")

	statusDot := canvas.NewCircle(theme.Color(theme.ColorNameDisabled))
	statusDot.Resize(fyne.NewSize(10, 10))
	statusDotHolder := container.NewVBox(
		layout.NewSpacer(),
		container.NewGridWrap(fyne.NewSize(10, 10), statusDot),
		layout.NewSpacer(),
	)
	statusValue := widget.NewLabelWithStyle("Stopped", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	statusValue.Wrapping = fyne.TextWrapOff

	hashrateValue := canvas.NewText("—", theme.Color(theme.ColorNameForeground))
	hashrateValue.Alignment = fyne.TextAlignLeading
	hashrateValue.TextStyle = fyne.TextStyle{Bold: true}
	hashrateValue.TextSize = theme.TextSize() * 2.6

	sharesValue := widget.NewLabel("—")
	poolValue := widget.NewLabel("—")
	poolValue.Wrapping = fyne.TextWrapWord
	uptimeValue := widget.NewLabel("—")
	backendInUseValue := widget.NewLabel("—")
	hashrateHistory := newHashrateChart(300) // ~10 minutes at 2s polling
	avgHashrateValue := widget.NewLabelWithStyle("Avg —", fyne.TextAlignTrailing, fyne.TextStyle{Monospace: true})
	avgHashrateValue.Wrapping = fyne.TextWrapOff
	avgHashrateValue.Importance = widget.MediumImportance

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
	var (
		devMu        sync.Mutex
		devices      []Device
		deviceChecks []*widget.Check
	)

	logBuf := newRingLogs(5000)
	followTailCheck := widget.NewCheck("Follow tail", nil)
	followTailCheck.SetChecked(true)

	logList := widget.NewList(
		func() int { return logBuf.Len() },
		func() fyne.CanvasObject {
			l := widget.NewLabel("")
			l.TextStyle = fyne.TextStyle{Monospace: true}
			return l
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			item.(*widget.Label).SetText(logBuf.At(int(id)))
		},
	)

	type logEvent struct {
		text  string
		reset bool
	}
	logEvents := make(chan logEvent, 4096)
	resetLog := func() {
		select {
		case logEvents <- logEvent{reset: true}:
		default:
		}
	}
	appendLog := func(text string) {
		text = sanitizeLogLine(text)
		lines := strings.Split(text, "\n")
		for _, line := range lines {
			select {
			case logEvents <- logEvent{text: line}:
			default:
				// Drop logs if the UI can't keep up.
			}
		}
	}
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()

		dirty := false

		for {
			select {
			case ev := <-logEvents:
				if ev.reset {
					logBuf.Clear()
					dirty = true
					continue
				}
				logBuf.Append(ev.text)
				dirty = true

			case <-ticker.C:
				if !dirty {
					continue
				}
				dirty = false
				fyne.Do(func() {
					logList.Refresh()
					if followTailCheck.Checked {
						logList.ScrollToBottom()
					}
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
			reportHashrateCheck.Enable()
			modeHint.SetText("Stratum: no node required; reward goes to the wallet above.")
		case modeRPCLocal:
			poolRow.Hide()
			workerRow.Hide()
			walletRow.Hide()
			rpcRow.Show()
			reportHashrateCheck.Disable()
			modeHint.SetText("RPC local: mines to node coinbase; wallet/worker ignored.")
		case modeRPCGateway:
			poolRow.Hide()
			workerRow.Hide()
			walletRow.Show()
			rpcRow.Show()
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
		devicesBox.Objects = []fyne.CanvasObject{widget.NewLabel("Detecting GPUs...")}
		devicesBox.Refresh()

		go func() {
			backendSelection := selectedBackend()
			backend := resolveBackend(ethminerPath, backendSelection)
			list, out, err := listEthminerDevices(ethminerPath, backend)
			if err != nil {
				appendLog(fmt.Sprintf("[devices] %v\n", err))
				fyne.Do(func() {
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
					appendLog(fmt.Sprintf("[devices] %s --list-devices output:\n%s\n", backendName, strings.TrimSpace(out)))
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
			devMu.Unlock()

			fyne.Do(func() {
				if backendSelection == backendAuto {
					backendResolvedHint.SetText(fmt.Sprintf("Auto resolved to: %s", strings.ToUpper(backend)))
				} else {
					backendResolvedHint.SetText("")
				}
				devicesBox.Objects = newObjects
				devicesBox.Refresh()
				refreshBtn.Enable()
			})
		}()
	}
	refreshBtn.OnTapped = refreshDevices
	backendSelect.OnChanged = func(_ string) { refreshDevices() }

	var (
		procMu      sync.Mutex
		minerCmd    *exec.Cmd
		minerCtx    context.Context
		minerCancel context.CancelFunc
		apiPort     int
		pollCancel  context.CancelFunc
	)

	var startBtn *widget.Button
	var stopBtn *widget.Button

	setRunningUI := func(running bool) {
		if running {
			statusValue.SetText("Running")
			statusDot.FillColor = theme.Color(theme.ColorNamePrimary)
			statusDot.Refresh()
			if startBtn != nil {
				startBtn.Disable()
			}
			if stopBtn != nil {
				stopBtn.Enable()
			}
		} else {
			statusValue.SetText("Stopped")
			statusDot.FillColor = theme.Color(theme.ColorNameDisabled)
			statusDot.Refresh()
			hashrateValue.Text = "—"
			hashrateValue.Refresh()
			sharesValue.SetText("—")
			poolValue.SetText("—")
			uptimeValue.SetText("—")
			backendInUseValue.SetText("—")
			hashrateHistory.Reset()
			avgHashrateValue.SetText("Avg —")
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
		} else if wallet != "" && !isHexAddress(wallet) {
			return errors.New("invalid wallet address (expected 0x + 40 hex chars)")
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
		cfg.WalletAddress = strings.ToLower(wallet)
		cfg.WorkerName = worker
		cfg.SelectedDevices = selected
		cfg.ReportHashrate = reportHashrateCheck.Checked
		cfg.DisplayInterval = displayIntv
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

		_ = saveConfig(cfg)
	}

	startMiner := func() {
		if ethminerErr != nil {
			dialog.ShowError(fmt.Errorf("ethminer not found: %w", ethminerErr), w)
			return
		}
		if err := saveFromUI(); err != nil {
			dialog.ShowError(err, w)
			return
		}

		procMu.Lock()
		defer procMu.Unlock()
		if minerCmd != nil && minerCmd.Process != nil {
			dialog.ShowInformation(appName, "Miner already running", w)
			return
		}

		port, err := pickFreePort()
		if err != nil {
			dialog.ShowError(err, w)
			return
		}
		apiPort = port

		poolURL, err := buildPoolURL(cfg)
		if err != nil {
			dialog.ShowError(err, w)
			return
		}

		backendSelection := selectedBackend()
		backend := resolveBackend(ethminerPath, backendSelection)
		args := []string{
			"-G",
			"--olivetum",
			"--nocolor",
			"-P", poolURL,
			"--api-bind", fmt.Sprintf("127.0.0.1:-%d", apiPort),
			"--display-interval", strconv.Itoa(cfg.DisplayInterval),
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

		minerCtx, minerCancel = context.WithCancel(context.Background())
		cmd := exec.CommandContext(minerCtx, ethminerPath, args...)
		configureChildProcess(cmd)
		cmd.Env = append(os.Environ(), "LC_ALL=C")

		stdout, _ := cmd.StdoutPipe()
		stderr, _ := cmd.StderrPipe()

		resetLog()
		appendLog(fmt.Sprintf("Starting: %s %s\n\n", ethminerPath, strings.Join(args, " ")))

		if err := cmd.Start(); err != nil {
			minerCancel()
			minerCtx = nil
			minerCancel = nil
			dialog.ShowError(err, w)
			return
		}
		minerCmd = cmd
		setRunningUI(true)
		if backendSelection == backendAuto {
			backendInUseValue.SetText(fmt.Sprintf("Auto → %s", strings.ToUpper(backend)))
		} else {
			backendInUseValue.SetText(strings.ToUpper(backend))
		}

		go streamLines(stdout, appendLog)
		go streamLines(stderr, appendLog)

		ctx, cancel := context.WithCancel(context.Background())
		pollCancel = cancel
		go pollStats(ctx, "127.0.0.1", apiPort, func(s Stat) {
			hs := fmt.Sprintf("%.2f MH/s", float64(s.TotalKHs)/1000.0)
			fyne.Do(func() {
				hashrateValue.Text = hs
				hashrateValue.Refresh()
				hashrateHistory.Add(float64(s.TotalKHs) / 1000.0)
				if avg, ok := hashrateHistory.Average(); ok {
					avgHashrateValue.SetText(fmt.Sprintf("Avg %.2f MH/s", avg))
				} else {
					avgHashrateValue.SetText("Avg —")
				}
				sharesValue.SetText(fmt.Sprintf("Accepted %d | Rejected %d | Invalid %d", s.Accepted, s.Rejected, s.Invalid))
				poolValue.SetText(s.Pool)
				uptimeValue.SetText(fmt.Sprintf("%d min", s.UptimeMin))
			})
		}, func(err error) {
			// Only show transient failures in log; API might not be ready yet.
			appendLog(fmt.Sprintf("[api] %v\n", err))
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
				appendLog(fmt.Sprintf("\n[exit] %v\n", err))
			} else {
				appendLog("\n[exit] miner stopped\n")
			}
		}()
	}

	stopMiner := func() {
		procMu.Lock()
		defer procMu.Unlock()
		if minerCmd == nil || minerCmd.Process == nil {
			return
		}
		appendLog("\nStopping miner...\n")
		cmd := minerCmd
		proc := minerCmd.Process
		_ = proc.Signal(os.Interrupt)
		// Fallback hard kill after a short grace (only if it's still the same process).
		go func(cmd *exec.Cmd, p *os.Process) {
			time.Sleep(5 * time.Second)
			procMu.Lock()
			still := minerCmd == cmd
			procMu.Unlock()
			if still {
				_ = p.Kill()
			}
		}(cmd, proc)
	}

	startBtn = widget.NewButtonWithIcon("Start mining", theme.MediaPlayIcon(), startMiner)
	startBtn.Importance = widget.HighImportance
	stopBtn = widget.NewButtonWithIcon("Stop", theme.MediaStopIcon(), stopMiner)
	stopBtn.Importance = widget.DangerImportance

	if ethminerErr != nil {
		startBtn.Disable()
		stopBtn.Disable()
	} else {
		stopBtn.Disable()
	}

	var advancedOpen bool
	advancedToggleBtn := widget.NewButtonWithIcon("Advanced options", theme.SettingsIcon(), nil)
	advancedToggleBtn.Importance = widget.LowImportance

	quickBody := container.NewVBox(
		modeRow,
		modeHint,
		walletRow,
		workerRow,
		poolRow,
		rpcRow,
		container.NewHBox(layout.NewSpacer(), advancedToggleBtn),
	)
	quickPanel := panel("Quick Start", quickBody)

	devicesScroll := container.NewVScroll(devicesBox)
	devicesScroll.SetMinSize(fyne.NewSize(0, 240))

	advancedGrid := container.NewGridWithColumns(2,
		fieldLabel("GPU backend"), backendSelect,
		fieldLabel("Display interval (s)"), displayIntervalEntry,
		widget.NewLabel(""), reportHashrateCheck,
	)
	advancedBody := container.NewVBox(
		advancedGrid,
		backendHint,
		backendResolvedHint,
		widget.NewSeparator(),
		container.NewHBox(fieldLabel("GPUs"), layout.NewSpacer(), refreshBtn),
		devicesScroll,
	)
	advancedPanel := panel("Advanced", advancedBody)
	advancedPanel.Hide()

	advancedToggleBtn.OnTapped = func() {
		advancedOpen = !advancedOpen
		if advancedOpen {
			advancedPanel.Show()
			advancedToggleBtn.SetText("Hide advanced")
		} else {
			advancedPanel.Hide()
			advancedToggleBtn.SetText("Advanced options")
		}
	}

	hashrate10mTitle := widget.NewLabelWithStyle("Hashrate (10 min)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	hashrate10mTitle.Wrapping = fyne.TextWrapOff
	hashrate10mHeader := container.NewHBox(widget.NewIcon(theme.HistoryIcon()), hashrate10mTitle, layout.NewSpacer(), avgHashrateValue)

	statusBody := container.NewVBox(
		fieldLabel("Total hashrate"),
		hashrateValue,
		container.NewGridWithColumns(4,
			metricTileWithIcon("Backend", theme.ComputerIcon(), backendInUseValue),
			metricTileWithIcon("Uptime", theme.HistoryIcon(), uptimeValue),
			metricTileWithIcon("Shares", theme.ConfirmIcon(), sharesValue),
			metricTileWithIcon("Pool", theme.StorageIcon(), poolValue),
		),
		metricTileWithHeader(hashrate10mHeader, hashrateHistory.Object()),
	)
	statusPanel := panel("Dashboard", statusBody)

	clearLogsBtn := widget.NewButtonWithIcon("Clear", theme.ContentClearIcon(), resetLog)
	logBar := container.NewHBox(followTailCheck, layout.NewSpacer(), clearLogsBtn)
	logPanel := panel("Logs", container.NewBorder(logBar, nil, nil, nil, logList))

	leftStack := container.NewVBox(quickPanel, advancedPanel)
	left := container.NewVScroll(container.NewPadded(leftStack))
	left.SetMinSize(fyne.NewSize(420, 0))

	right := container.NewVSplit(statusPanel, logPanel)
	right.Offset = 0.30
	mainSplit := container.NewHSplit(left, right)
	mainSplit.Offset = 0.40

	headerTitle := canvas.NewText(appName, theme.Color(theme.ColorNamePrimary))
	headerTitle.TextStyle = fyne.TextStyle{Bold: true}
	headerTitle.TextSize = theme.TextSize() * 2.1
	headerSubtitle := widget.NewLabel("Modern GUI for ethminer (Olivetumhash)")
	headerSubtitle.Wrapping = fyne.TextWrapWord

	statusPillBg := canvas.NewRectangle(theme.Color(theme.ColorNameButton))
	statusPillBg.StrokeColor = theme.Color(theme.ColorNameSeparator)
	statusPillBg.StrokeWidth = 1
	statusPill := container.NewMax(
		statusPillBg,
		container.NewPadded(container.NewCenter(container.NewHBox(statusDotHolder, statusValue))),
	)

	headerLeft := container.NewVBox(headerTitle, headerSubtitle)
	headerRight := container.NewGridWithColumns(3, statusPill, startBtn, stopBtn)
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
	main := container.NewBorder(container.NewVBox(header, widget.NewSeparator()), nil, nil, nil, container.NewPadded(mainSplit))
	w.SetContent(container.NewMax(bg, main))

	if ethminerErr != nil {
		dialog.ShowError(fmt.Errorf("ethminer not found. Place it next to this app or in PATH: %w", ethminerErr), w)
	} else {
		refreshDevices()
	}

	w.SetCloseIntercept(func() {
		procMu.Lock()
		running := minerCmd != nil && minerCmd.Process != nil
		procMu.Unlock()
		if !running {
			saveDraftFromUI()
			w.Close()
			return
		}
		dialog.ShowConfirm(appName, "Mining is running. Stop and quit?", func(ok bool) {
			if ok {
				saveDraftFromUI()
				stopMiner()
				time.AfterFunc(500*time.Millisecond, func() {
					fyne.Do(func() { w.Close() })
				})
			}
		}, w)
	})

	if runtime.GOOS == "linux" {
		appendLog("Tip: You can run this as AppImage and launch from desktop.\n")
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

func pollStats(ctx context.Context, host string, port int, onStat func(Stat), onErr func(error)) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

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

var ansiCSI = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

func sanitizeLogLine(s string) string {
	// Strip common terminal control sequences and keep things readable in a GUI.
	s = strings.ReplaceAll(s, "\r", "")
	s = ansiCSI.ReplaceAllString(s, "")

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
