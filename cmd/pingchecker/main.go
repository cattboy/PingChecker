package main

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	probing "github.com/prometheus-community/pro-bing"

	psnet "github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// --- Custom Widget Definition ---

type AutoRefreshSelect struct {
	widget.Select
}

func NewAutoRefreshSelect(changed func(string)) *AutoRefreshSelect {
	s := &AutoRefreshSelect{}
	s.OnChanged = changed
	s.Options = []string{"Click to load processes..."}
	s.ExtendBaseWidget(s)
	return s
}

func (s *AutoRefreshSelect) Tapped(e *fyne.PointEvent) {
	s.Options = getProcessList()
	s.Refresh()
	s.Select.Tapped(e)
}

// --- End Custom Widget ---

type MonitorState struct {
	TargetPID int32
	TargetIP  string
	mu        sync.RWMutex
	cancel    context.CancelFunc
}

var (
	state = &MonitorState{}

	// Data Bindings
	pingDisplay      = binding.NewString()
	displayIP        = binding.NewString()
	displayPortLabel = binding.NewString()
	displayPortVal   = binding.NewString()
)

const pingPrefix = "2. Real-time Latency: "

func main() {
	a := app.New()
	w := a.NewWindow("PingChecker")
	w.Resize(fyne.NewSize(500, 250))

	// UI Elements
	lblStatus := widget.NewLabelWithData(pingDisplay)
	lblStatus.TextStyle = fyne.TextStyle{Bold: true}

	// Target Row
	lblIP := widget.NewLabelWithData(displayIP)

	lblPortLabel := widget.NewLabelWithData(displayPortLabel)
	lblPortLabel.TextStyle = fyne.TextStyle{Bold: true}

	lblPortVal := widget.NewLabelWithData(displayPortVal)

	targetRow := container.NewHBox(lblIP, lblPortLabel, lblPortVal)

	// Initial State
	pingDisplay.Set(pingPrefix + "Waiting for selection...")
	displayIP.Set("Target: None")
	displayPortLabel.Set("")
	displayPortVal.Set("")

	// Process Selector
	processList := NewAutoRefreshSelect(func(selected string) {
		var pid int32
		var name string
		_, err := fmt.Sscanf(selected, "%s (%d)", &name, &pid)
		if err != nil {
			parts := strings.Split(selected, " (")
			if len(parts) > 1 {
				fmt.Sscanf(parts[len(parts)-1], "%d)", &pid)
			}
		}
		startMonitoring(pid)
	})

	// UPDATED: Info Button with Custom Dialog for Bold Text
	btnInfo := widget.NewButtonWithIcon("More Info", theme.InfoIcon(), func() {
		// Define the text with Markdown syntax
		mdText := "**If Port 7000+:**\nGame Server. This is the TRUE ping.\n\n" +
			"**If Port 443/80:**\nLogin/Lobby Server. Stable, but not 'Gameplay' ping.\n\n" +
			"Wait up to 30 seconds for the program to sync and find new port.\n\n" +
			"Note: If **Port 7000+** shows 'Timeout', games blocking pings RIP."

		// Create a RichText widget to render the Markdown
		content := widget.NewRichTextFromMarkdown(mdText)

		// Create a custom dialog containing the RichText
		d := dialog.NewCustom("Port Guide", "OK", content, w)

		// Resize it slightly so text wraps nicely
		d.Resize(fyne.NewSize(400, 300))
		d.Show()
	})

	// Layout
	content := container.NewVBox(
		widget.NewLabel("1. Select Game Process:"),
		processList,
		widget.NewSeparator(),
		lblStatus,
		targetRow,
		widget.NewSeparator(),
		btnInfo,
	)

	w.SetContent(content)
	w.ShowAndRun()
}

func getProcessList() []string {
	procs, err := process.Processes()
	if err != nil {
		return []string{"Error fetching processes"}
	}

	var likelyGames []string
	var otherApps []string

	for _, p := range procs {
		name, err := p.Name()
		if err != nil || name == "" {
			continue
		}

		exePath, err := p.Exe()
		if err != nil {
			continue
		}

		lowerPath := strings.ToLower(exePath)
		if strings.Contains(lowerPath, `c:\windows`) {
			continue
		}

		entry := fmt.Sprintf("%s (%d)", name, p.Pid)

		if isLikelyGame(lowerPath) {
			likelyGames = append(likelyGames, entry)
		} else {
			otherApps = append(otherApps, entry)
		}
	}

	sort.Strings(likelyGames)
	sort.Strings(otherApps)

	return append(likelyGames, otherApps...)
}

func isLikelyGame(path string) bool {
	keywords := []string{
		"steam", "steamapps", "common", "riot games",
		"epic games", "ubisoft", "battle.net", "xboxgames", "game",
	}
	for _, k := range keywords {
		if strings.Contains(path, k) {
			return true
		}
	}
	return false
}

func startMonitoring(pid int32) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.cancel != nil {
		state.cancel()
	}

	ctx, cancel := context.WithCancel(context.Background())
	state.cancel = cancel
	state.TargetPID = pid
	state.TargetIP = ""

	// Reset UI
	displayIP.Set(fmt.Sprintf("Target PID: %d (Scanning...)", pid))
	displayPortLabel.Set("")
	displayPortVal.Set("")
	pingDisplay.Set(pingPrefix + "Scanning...")

	go monitorLoop(ctx, pid)
}

func monitorLoop(ctx context.Context, pid int32) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	lastScanTime := time.Time{}
	scanInterval := 1 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if time.Since(lastScanTime) >= scanInterval {
				lastScanTime = time.Now()
				currentIP, currentPort := resolveGameIP(pid)

				if currentIP != "" {
					scanInterval = 30 * time.Second
					state.mu.Lock()
					if state.TargetIP != currentIP {
						state.TargetIP = currentIP
						if ctx.Err() == nil {
							displayIP.Set(fmt.Sprintf("Target: %s", currentIP))
							displayPortLabel.Set("Port:")
							displayPortVal.Set(fmt.Sprintf("%d", currentPort))
						}
					}
					state.mu.Unlock()
				} else {
					scanInterval = 1 * time.Second
					state.mu.RLock()
					if state.TargetIP == "" && ctx.Err() == nil {
						pingDisplay.Set(pingPrefix + "No connection found")
						displayIP.Set("Target: None")
						displayPortLabel.Set("")
						displayPortVal.Set("")
					}
					state.mu.RUnlock()
				}
			}

			state.mu.RLock()
			target := state.TargetIP
			state.mu.RUnlock()

			if target != "" {
				success := doPing(ctx, target)
				if !success {
					scanInterval = 1 * time.Second
					lastScanTime = time.Time{}
				}
			}
		}
	}
}

func resolveGameIP(pid int32) (string, int) {
	conns, err := psnet.ConnectionsPid("inet", pid)
	if err != nil {
		return "", 0
	}

	var bestIP string
	var bestPort int
	var bestScore int = -1

	for _, c := range conns {
		score := 0
		ip := c.Raddr.IP
		port := int(c.Raddr.Port)

		if !isPublicIP(ip) {
			continue
		}

		if c.Type == 2 {
			score += 100
		}
		if port != 80 && port != 443 && port != 8080 {
			score += 50
		}

		if score > bestScore {
			bestScore = score
			bestIP = ip
			bestPort = port
		}
	}
	return bestIP, bestPort
}

func doPing(ctx context.Context, ipAddr string) bool {
	pinger, err := probing.NewPinger(ipAddr)
	if err != nil {
		if ctx.Err() == nil {
			pingDisplay.Set(pingPrefix + "Error")
		}
		return false
	}

	pinger.Count = 1
	pinger.Timeout = 900 * time.Millisecond
	pinger.SetPrivileged(true)

	err = pinger.Run()

	if ctx.Err() != nil {
		return false
	}

	if err != nil {
		pingDisplay.Set(pingPrefix + "Timeout")
		return false
	}

	stats := pinger.Statistics()
	if stats.PacketsRecv > 0 {
		latency := stats.AvgRtt
		ms := latency.Milliseconds()

		var status string
		if ms < 80 {
			status = "🟢 Excellent"
		} else if ms < 140 {
			status = "🟡 Good"
		} else {
			status = "🔴 Lag"
		}

		pingDisplay.Set(fmt.Sprintf("%s%d ms (%s)", pingPrefix, ms, status))
		return true
	} else {
		pingDisplay.Set(pingPrefix + "Packet Loss")
		return false
	}
}

func isPublicIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() {
		return false
	}
	return true
}
