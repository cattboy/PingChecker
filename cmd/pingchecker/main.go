package main

import (
	"context"
	"fmt"
	"image/color"
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
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
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

// ConnectionData holds the state of a specific connection
type ConnectionData struct {
	ID       string // IP:Port key
	IP       string
	Port     int
	Type     string    // "Game", "Login", "Other"
	LastPing time.Time // When did we last ping this?
	Latency  int64
	Status   string
	IsActive bool
}

// UIItem is the simplified struct passed to the Fyne List Binding
type UIItem struct {
	Label   string
	Latency string
	Type    string // Used for color coding
}

var (
	currentCancel context.CancelFunc
	// Thread-safe data binding for the list
	listData = binding.NewUntypedList()
)

func main() {
	a := app.New()
	w := a.NewWindow("PingChecker")
	w.Resize(fyne.NewSize(550, 400)) // Fixed reasonable size

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

	// --- Dynamic List Setup ---
	// widget.List is efficient and thread-safe via binding
	connList := widget.NewListWithData(
		listData,
		func() fyne.CanvasObject {
			// Template for a row
			return container.NewHBox(
				canvas.NewText("TYPE", color.White),
				canvas.NewText("0.0.0.0:0000", color.White),
				layout.NewSpacer(),
				widget.NewLabel("000 ms"),
			)
		},
		func(i binding.DataItem, o fyne.CanvasObject) {
			// Bind data to template
			item, _ := i.(binding.Untyped).Get()
			uiData := item.(UIItem)

			box := o.(*fyne.Container)
			txtType := box.Objects[0].(*canvas.Text)
			txtIP := box.Objects[1].(*canvas.Text)
			lblPing := box.Objects[3].(*widget.Label)

			// Color Coding
			var typeColor color.Color
			if uiData.Type == "Game" {
				typeColor = color.RGBA{0, 180, 0, 255} // Green
				txtType.Text = "[GAME]"
			} else if uiData.Type == "Login" {
				typeColor = color.RGBA{0, 100, 200, 255} // Blue
				txtType.Text = "[LOGIN]"
			} else {
				typeColor = theme.ForegroundColor()
				txtType.Text = "[NET]"
			}

			txtType.Color = typeColor
			txtType.Refresh()

			txtIP.Text = uiData.Label
			txtIP.TextStyle = fyne.TextStyle{Bold: true}
			txtIP.Color = typeColor
			txtIP.Refresh()

			lblPing.SetText(uiData.Latency)
		},
	)

	// Legend Button
	btnInfo := widget.NewButtonWithIcon("Legend", theme.InfoIcon(), func() {
		greenText := canvas.NewText("Game Server (Port 7000+)", color.RGBA{0, 180, 0, 255})
		greenText.TextStyle = fyne.TextStyle{Bold: true}
		blueText := canvas.NewText("Login/Web (Port 80/443)", color.RGBA{0, 100, 200, 255})
		blueText.TextStyle = fyne.TextStyle{Bold: true}

		content := container.NewVBox(
			widget.NewLabel("Scan Intervals:"),
			widget.NewLabel("- Game Server: Every 5 seconds"),
			widget.NewLabel("- Login Server: Every 30 seconds"),
			widget.NewSeparator(),
			container.NewHBox(widget.NewIcon(theme.MediaRecordIcon()), greenText),
			container.NewHBox(widget.NewIcon(theme.MediaRecordIcon()), blueText),
		)

		d := dialog.NewCustom("Info", "OK", content, w)
		d.Resize(fyne.NewSize(400, 300))
		d.Show()
	})

	// Layout
	content := container.NewBorder(
		container.NewVBox(
			widget.NewLabel("1. Select Game Process:"),
			processList,
			widget.NewSeparator(),
			widget.NewLabelWithStyle("Active Connections:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		),
		btnInfo,  // Bottom
		nil,      // Left
		nil,      // Right
		connList, // Center (Takes all available space)
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
	keywords := []string{"steam", "riot", "epic", "game", "xbox", "battle.net"}
	for _, k := range keywords {
		if strings.Contains(path, k) {
			return true
		}
	}
	return false
}

func startMonitoring(pid int32) {
	if currentCancel != nil {
		currentCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	currentCancel = cancel

	// Clear list
	listData.Set(nil)

	go monitorLoop(ctx, pid)
}

func monitorLoop(ctx context.Context, pid int32) {
	// We tick every 1 second to be responsive, but individual pings are throttled
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// State Map: Keep track of connections so we know when to ping them next
	stateMap := make(map[string]*ConnectionData)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 1. Get current connections from OS
			currentConns := getRawConnections(pid)
			activeIDs := make(map[string]bool)

			// 2. Update State Map
			for _, c := range currentConns {
				id := fmt.Sprintf("%s:%d", c.IP, c.Port)
				activeIDs[id] = true

				if _, exists := stateMap[id]; !exists {
					// New connection found
					stateMap[id] = &ConnectionData{
						ID:       id,
						IP:       c.IP,
						Port:     c.Port,
						Type:     determineType(c.Port),
						LastPing: time.Time{}, // Never pinged
						Status:   "Waiting...",
					}
				}
			}

			// 3. Remove stale connections
			for id := range stateMap {
				if !activeIDs[id] {
					delete(stateMap, id)
				}
			}

			// 4. Check who needs a Ping
			var wg sync.WaitGroup
			now := time.Now()

			for _, conn := range stateMap {
				interval := getInterval(conn.Type)

				if now.Sub(conn.LastPing) >= interval {
					conn.LastPing = now // Mark as pinged immediately to prevent double scheduling
					wg.Add(1)
					go func(c *ConnectionData) {
						defer wg.Done()
						lat, status, active := doPing(c.IP)
						c.Latency = lat
						c.Status = status
						c.IsActive = active
					}(conn)
				}
			}

			// Wait for this batch to finish so UI doesn't flicker partial updates
			// (Since pings are fast/async, this wait is acceptable for a 1s tick)
			wg.Wait()

			// 5. Update UI Binding
			// We convert our map to a slice for the List
			if ctx.Err() == nil {
				updateBinding(stateMap)
			}
		}
	}
}

func updateBinding(data map[string]*ConnectionData) {
	// Convert map to slice
	var uiList []interface{}

	// Copy to slice
	tempList := make([]*ConnectionData, 0, len(data))
	for _, v := range data {
		tempList = append(tempList, v)
	}

	// Sort: Game > Other > Login
	sort.Slice(tempList, func(i, j int) bool {
		scoreI := getScore(tempList[i].Type)
		scoreJ := getScore(tempList[j].Type)
		return scoreI > scoreJ
	})

	for _, c := range tempList {
		pingText := c.Status
		if c.IsActive {
			pingText = fmt.Sprintf("%d ms (%s)", c.Latency, c.Status)
		}

		uiList = append(uiList, UIItem{
			Label:   fmt.Sprintf("%s:%d", c.IP, c.Port),
			Type:    c.Type,
			Latency: pingText,
		})
	}

	// Thread-safe update
	listData.Set(uiList)
}

func getRawConnections(pid int32) []ConnectionData {
	conns, err := psnet.ConnectionsPid("inet", pid)
	if err != nil {
		return []ConnectionData{}
	}

	unique := make(map[string]bool)
	var results []ConnectionData

	for _, c := range conns {
		ip := c.Raddr.IP
		port := int(c.Raddr.Port)
		if !isPublicIP(ip) {
			continue
		}

		key := fmt.Sprintf("%s:%d", ip, port)
		if unique[key] {
			continue
		}
		unique[key] = true

		results = append(results, ConnectionData{IP: ip, Port: port})
	}
	return results
}

func determineType(port int) string {
	if port >= 7000 {
		return "Game"
	} else if port == 80 || port == 443 || port == 8080 {
		return "Login"
	}
	return "Other"
}

func getInterval(t string) time.Duration {
	if t == "Game" {
		return 5 * time.Second
	} else if t == "Login" {
		return 30 * time.Second
	}
	return 5 * time.Second // Other
}

func getScore(t string) int {
	if t == "Game" {
		return 3
	}
	if t == "Other" {
		return 2
	}
	return 1
}

func doPing(ipAddr string) (int64, string, bool) {
	pinger, err := probing.NewPinger(ipAddr)
	if err != nil {
		return 0, "Error", false
	}
	pinger.Count = 1
	pinger.Timeout = 900 * time.Millisecond
	pinger.SetPrivileged(true)

	err = pinger.Run()
	if err != nil {
		return 0, "Timeout", false
	}

	stats := pinger.Statistics()
	if stats.PacketsRecv > 0 {
		ms := stats.AvgRtt.Milliseconds()
		status := "🔴"
		if ms < 60 {
			status = "🟢"
		} else if ms < 120 {
			status = "🟡"
		}
		return ms, status, true
	}
	return 0, "Packet Loss", false
}

func isPublicIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() {
		return false
	}
	return true
}
