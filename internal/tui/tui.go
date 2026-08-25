package tui

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"gdrive-bisync/internal/appstate"
	"gdrive-bisync/internal/config"
	"gdrive-bisync/internal/utils"
)

const (
	pageOverview = iota
	pageActivity
	pageLogs
	pageTrash
	pageSafety
	pageSystem
	pageHelp
)

var pageNames = []string{"Overview", "Activity", "Logs", "Trash", "Safety", "System", "Help"}

var (
	accent     = lipgloss.Color("26")
	green      = lipgloss.Color("28")
	yellow     = lipgloss.Color("130")
	red        = lipgloss.Color("124")
	muted      = lipgloss.Color("244")
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(accent)
	dimStyle   = lipgloss.NewStyle().Foreground(muted)
	errorStyle = lipgloss.NewStyle().Foreground(red)
	panelStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("250")).Padding(0, 1)
)

// Light-palette-safe colors for log categories, chosen for contrast on
// white/near-white terminal backgrounds.
var categoryPalette = []lipgloss.Color{"26", "28", "130", "127", "18", "94", "168", "30"}

type refreshMsg struct{}

type Model struct {
	paths                                   appstate.Paths
	syncRoot                                string
	cfg                                     *config.Config
	status                                  appstate.Status
	statusError                             error
	events                                  []appstate.Event
	eventsError                             error
	trash                                   []utils.TrashEntry
	trashError                              error
	page, width, height, scroll             int
	privacy, errorsOnly, follow             bool
	filterMode, restoreMode, confirmRestore bool
	filter, restoreID, message              string
}

func NewModel(paths appstate.Paths, syncRoot string, configs ...*config.Config) Model {
	var cfg *config.Config
	if len(configs) > 0 {
		cfg = configs[0]
	}
	model := Model{paths: paths, syncRoot: syncRoot, cfg: cfg, width: 100, height: 30, follow: true}
	model.refresh()
	return model
}

func (model Model) Init() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return refreshMsg{} })
}

func (model Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		model.width, model.height = message.Width, message.Height
	case refreshMsg:
		model.refresh()
		return model, tea.Tick(time.Second, func(time.Time) tea.Msg { return refreshMsg{} })
	case tea.KeyMsg:
		key := message.String()
		if model.filterMode {
			return model.updateInput(key, message, true)
		}
		if model.restoreMode {
			return model.updateInput(key, message, false)
		}
		if model.confirmRestore {
			if key == "y" || key == "enter" {
				entry, err := utils.RestoreTrash(model.syncRoot, model.restoreID)
				if err != nil {
					model.message = "Restore failed: " + err.Error()
				} else {
					model.message = "Restored " + model.displayPath(entry.OriginalPath)
				}
				model.confirmRestore = false
				model.restoreID = ""
				model.refresh()
			}
			if key == "n" || key == "esc" {
				model.confirmRestore = false
			}
			return model, nil
		}
		switch key {
		case "q", "ctrl+c":
			return model, tea.Quit
		case "tab", "right":
			model.page = (model.page + 1) % len(pageNames)
			model.scroll = 0
		case "shift+tab", "left":
			model.page = (model.page + len(pageNames) - 1) % len(pageNames)
			model.scroll = 0
		case "1", "2", "3", "4", "5", "6", "7":
			model.page = int(key[0] - '1')
			model.scroll = 0
		case "up", "k":
			model.scroll++
			model.follow = false
		case "down", "j":
			if model.scroll > 0 {
				model.scroll--
			}
			model.follow = false
		case "g":
			model.scroll = 0
		case "G":
			model.follow = true
			model.scroll = 999999
		case "p":
			paused := !appstate.IsPaused(model.paths.PauseFile)
			model.message = setPauseMessage(model.paths.PauseFile, paused)
		case "s":
			model.message = requestMessage(model.paths.SyncNowFile, "Sync requested")
		case "d":
			model.message = requestMessage(model.paths.DryRunFile, "Dry-run preview requested")
		case "f":
			model.privacy = !model.privacy
		case "e":
			model.errorsOnly = !model.errorsOnly
			model.scroll = 0
		case "/":
			model.filterMode = true
		case "x":
			model.restoreMode = true
			model.restoreID = ""
		case "?":
			model.page = pageHelp
			model.scroll = 0
		case "esc":
			model.filter = ""
			model.errorsOnly = false
		}
		model.refresh()
	}
	return model, nil
}

func (model Model) updateInput(key string, message tea.KeyMsg, filter bool) (tea.Model, tea.Cmd) {
	target := &model.restoreID
	if filter {
		target = &model.filter
	}
	switch key {
	case "esc":
		model.filterMode = false
		model.restoreMode = false
	case "enter":
		if filter {
			model.filterMode = false
			model.scroll = 0
		} else if strings.TrimSpace(*target) != "" {
			model.restoreMode = false
			model.confirmRestore = true
		}
	case "backspace":
		if len(*target) > 0 {
			_, size := utf8.DecodeLastRuneInString(*target)
			*target = (*target)[:len(*target)-size]
		}
	default:
		if len(message.Runes) > 0 {
			*target += string(message.Runes)
		}
	}
	return model, nil
}

func (model Model) View() string {
	width := model.width
	if width < 50 {
		width = 50
	}
	var out strings.Builder
	out.WriteString(model.header(width))
	out.WriteString("\n")
	contentWidth := width - 2
	switch model.page {
	case pageOverview:
		out.WriteString(model.overview(contentWidth))
	case pageActivity:
		out.WriteString(model.eventView(contentWidth, true))
	case pageLogs:
		out.WriteString(model.eventView(contentWidth, false))
	case pageTrash:
		out.WriteString(model.trashView(contentWidth))
	case pageSafety:
		out.WriteString(model.safetyView(contentWidth))
	case pageSystem:
		out.WriteString(model.systemView(contentWidth))
	case pageHelp:
		out.WriteString(model.helpView(contentWidth))
	}
	if model.message != "" {
		out.WriteString("\n" + lipgloss.NewStyle().Foreground(yellow).Render(model.message))
	}
	if model.filterMode {
		out.WriteString("\nFilter: " + model.filter + "█")
	}
	if model.restoreMode {
		out.WriteString("\nTrash ID: " + model.restoreID + "█")
	}
	if model.confirmRestore {
		out.WriteString("\n" + errorStyle.Bold(true).Render("Restore this trash entry? [y] yes  [n] no"))
	}
	footer := dimStyle.Render("1-7 pages · tab navigate · s sync · d dry-run · p pause · f privacy · / filter · ? help · q quit")
	if pad := model.height - (strings.Count(out.String(), "\n") + 1) - 1; pad > 0 {
		out.WriteString(strings.Repeat("\n", pad))
	}
	out.WriteString("\n" + footer)
	return out.String()
}

func (model Model) header(width int) string {
	state, color := strings.ToUpper(model.status.State), green
	if model.statusError != nil {
		state = "OFFLINE"
		color = red
	} else if model.status.State == "error" {
		color = red
	} else if model.status.State == "paused" {
		color = yellow
	}
	tabs := make([]string, len(pageNames))
	plain := make([]string, len(pageNames))
	for i, name := range pageNames {
		plain[i] = fmt.Sprintf("%d %s", i+1, name)
		if i == model.page {
			tabs[i] = lipgloss.NewStyle().Bold(true).Underline(true).Foreground(accent).Render(plain[i])
		} else {
			tabs[i] = dimStyle.Render(plain[i])
		}
	}
	tabLine := strings.Join(tabs, "  ")
	if utf8.RuneCountInString(strings.Join(plain, "  ")) > width {
		tabLine = truncate(strings.Join(plain, "  "), width)
	}
	return titleStyle.Render("gdrive-bisync") + "  " + lipgloss.NewStyle().Bold(true).Foreground(color).Render("● "+state) + "\n" + tabLine
}

func (model Model) overview(width int) string {
	progress := 0
	if model.status.TaskCount > 0 {
		progress = model.status.CompletedTasks * 100 / model.status.TaskCount
	}
	health := fmt.Sprintf("Last sync  %s\nNext sync  %s\nUptime     %s\nWatcher    %s", displayTime(model.status.LastSyncFinished), displayTime(model.status.NextSync), durationSince(model.status.StartedAt), healthWord(model.status.WatcherHealthy))
	sync := fmt.Sprintf("Progress   %s %3d%%\nTasks      %d / %d\n↑ Upload   %-5d ↓ Download %-5d\n− Delete   %-5d + Folders  %-5d", progressBar(progress, 18), progress, model.status.CompletedTasks, model.status.TaskCount, model.status.Uploads, model.status.Downloads, model.status.Deletions, model.status.Folders)
	if width >= 90 {
		return lipgloss.JoinHorizontal(lipgloss.Top, panelStyle.Width(width/2-4).Render("HEALTH\n"+health), "  ", panelStyle.Width(width/2-4).Render("CURRENT SYNC\n"+sync)) + "\n" + panelStyle.Width(width-4).Render("RECENT ACTIVITY\n"+model.eventLines(width-8, true, max(6, model.height-14)))
	}
	return panelStyle.Width(width-4).Render("HEALTH\n"+health) + "\n" + panelStyle.Width(width-4).Render("CURRENT SYNC\n"+sync) + "\n" + panelStyle.Width(width-4).Render("RECENT ACTIVITY\n"+model.eventLines(width-8, true, max(4, model.height-20)))
}

func (model Model) eventView(width int, activity bool) string {
	title := "LIVE LOGS"
	if activity {
		title = "RECENT OPERATIONS"
	}
	return panelStyle.Width(width - 4).Render(title + "\n" + model.eventLines(width-8, activity, max(5, model.height-6)))
}

func (model Model) eventLines(width int, activity bool, limit int) string {
	filtered := make([]appstate.Event, 0)
	for _, event := range model.events {
		if model.errorsOnly && event.Level != "ERROR" {
			continue
		}
		if model.filter != "" && !strings.Contains(strings.ToLower(event.Message+" "+event.Category), strings.ToLower(model.filter)) {
			continue
		}
		if activity && !isActivity(event) {
			continue
		}
		filtered = append(filtered, event)
	}
	start := len(filtered) - limit - model.scroll
	if model.follow {
		start = len(filtered) - limit
	}
	if start < 0 {
		start = 0
	}
	end := start + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	var lines []string
	for _, event := range filtered[start:end] {
		path := ""
		if value, ok := event.Fields["path"]; ok {
			path = " " + model.displayPath(fmt.Sprint(value))
		}
		level := event.Level
		switch level {
		case "ERROR":
			level = errorStyle.Render("ERROR")
		case "WARN":
			level = lipgloss.NewStyle().Foreground(yellow).Render("WARN ")
		case "DEBUG", "TRACE":
			level = dimStyle.Render(fmt.Sprintf("%-5s", level))
		case "INFO":
			level = lipgloss.NewStyle().Foreground(green).Render(fmt.Sprintf("%-5s", level))
		default:
			level = dimStyle.Render(fmt.Sprintf("%-5s", level))
		}
		category := lipgloss.NewStyle().Foreground(categoryColor(event.Category)).Render(fmt.Sprintf("%-9s", fallback(event.Category, "·")))
		detail := ""
		if value, ok := event.Fields["error"]; ok {
			detail = " error=" + fmt.Sprint(value)
		}
		rest := truncate(event.Message+path+detail, width-25)
		line := fmt.Sprintf("%s %s %s %s", event.Time.Format("15:04:05"), level, category, rest)
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return dimStyle.Render("No matching events yet.")
	}
	return strings.Join(lines, "\n")
}

func (model Model) trashView(width int) string {
	var lines []string
	for i, e := range model.trash {
		if i >= max(5, model.height-9) {
			break
		}
		lines = append(lines, fmt.Sprintf("%-24s  %-18s  %s", e.ID, e.DeletedAt.Format("2006-01-02 15:04"), model.displayPath(e.OriginalPath)))
	}
	if len(lines) == 0 {
		lines = []string{dimStyle.Render("Trash is empty.")}
	}
	return panelStyle.Width(width - 4).Render(fmt.Sprintf("RECOVERABLE TRASH  %d entries\n%s\n\nx enter a trash ID to restore", len(model.trash), strings.Join(lines, "\n")))
}

func (model Model) safetyView(width int) string {
	if model.cfg == nil {
		return panelStyle.Width(width - 4).Render("SAFETY\nConfiguration unavailable")
	}
	backupCount := countFiles(filepath.Join(model.syncRoot, ".gdrive-bisync-backups"))
	text := fmt.Sprintf("Deletion count limit     %d\nDeletion percentage      %.1f%%\nDatabase backups kept    %d (present: %d)\nDesktop notifications    %v\nNotification cooldown    %s\nSingle-instance lock      %s\nDry-run preview           press d", model.cfg.MaxDeletionsPerSync, model.cfg.MaxDeletionPercent, model.cfg.DatabaseBackupCount, backupCount, model.cfg.DesktopNotifications, time.Duration(model.cfg.NotificationCooldownMs)*time.Millisecond, healthWord(model.status.PID > 0))
	return panelStyle.Width(width - 4).Render("SAFETY & RECOVERY\n" + text)
}

func (model Model) systemView(width int) string {
	db := filepath.Join(model.syncRoot, ".gdrive-bisync.db")
	text := fmt.Sprintf("PID                 %d\nState               %s\nStarted             %s\nLocal inventory     %d\nRemote inventory    %d\nDatabase size       %s\nEvent journal       %d / %d\nWatcher             %s\nNotifications       %s\nLast error          %s", model.status.PID, model.status.State, displayTime(model.status.StartedAt), model.status.LocalItems, model.status.RemoteItems, fileSize(db), len(model.events), appstate.MaxRuntimeEvents, healthWord(model.status.WatcherHealthy), healthWord(model.status.Notifications), fallback(model.status.LastError, "none"))
	return panelStyle.Width(width - 4).Render("SYSTEM DIAGNOSTICS\n" + text)
}

func (model Model) helpView(width int) string {
	text := "1-7          switch page\nTab / arrows navigate pages\nj/k           scroll\ng / G         top / follow newest\n/             filter logs and activity\ne             errors only\nf             hide or show paths\np             pause or resume daemon\ns             request sync now\nd             request dry-run preview\nx             restore trash by ID\nEsc           clear filter or cancel\nq             close TUI (daemon keeps running)"
	return panelStyle.Width(width - 4).Render("KEYBOARD HELP\n" + text + "\n\nNo file manager is included; use your operating system file explorer.")
}

func (model *Model) refresh() {
	model.status, model.statusError = appstate.ReadStatus(model.paths.StatusFile)
	model.events, model.eventsError = appstate.ReadEvents(model.paths.EventsFile, appstate.MaxRuntimeEvents)
	model.trash, model.trashError = utils.ListTrash(model.syncRoot)
}
func (model Model) displayPath(path string) string {
	if model.privacy {
		return "[path hidden]"
	}
	return path
}
func setPauseMessage(path string, paused bool) string {
	if err := appstate.SetPaused(path, paused); err != nil {
		return "Control failed: " + err.Error()
	}
	if paused {
		return "Sync paused"
	}
	return "Sync resumed"
}
func requestMessage(path, message string) string {
	if err := appstate.Request(path); err != nil {
		return "Request failed: " + err.Error()
	}
	return message
}
func displayTime(value time.Time) string {
	if value.IsZero() {
		return "never"
	}
	return value.Format("2006-01-02 15:04:05")
}
func durationSince(value time.Time) string {
	if value.IsZero() {
		return "unknown"
	}
	return time.Since(value).Round(time.Second).String()
}
func healthWord(ok bool) string {
	if ok {
		return "healthy"
	}
	return "unavailable"
}
func progressBar(percent, width int) string {
	filled := percent * width / 100
	if filled > width {
		filled = width
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}
func truncate(value string, width int) string {
	if width < 4 {
		return value
	}
	r := []rune(value)
	if len(r) <= width {
		return value
	}
	return string(r[:width-1]) + "…"
}
func fallback(value, otherwise string) string {
	if value == "" {
		return otherwise
	}
	return value
}
func countFiles(path string) int {
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0
	}
	return len(entries)
}
func fileSize(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return "unavailable"
	}
	size := float64(info.Size())
	units := []string{"B", "KiB", "MiB", "GiB"}
	i := 0
	for size >= 1024 && i < len(units)-1 {
		size /= 1024
		i++
	}
	return fmt.Sprintf("%.1f %s", size, units[i])
}
func isActivity(event appstate.Event) bool {
	text := strings.ToLower(event.Message)
	for _, word := range []string{"upload", "download", "trash", "restore", "folder", "sync", "retry", "failed"} {
		if strings.Contains(text, word) {
			return true
		}
	}
	return false
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func categoryColor(category string) lipgloss.Color {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(strings.ToUpper(category)))
	return categoryPalette[hash.Sum32()%uint32(len(categoryPalette))]
}

func RunTerminal(paths appstate.Paths, syncRoot string, cfg ...*config.Config) error {
	_, err := tea.NewProgram(NewModel(paths, syncRoot, cfg...), tea.WithAltScreen()).Run()
	return err
}
