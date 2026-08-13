package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"gdrive-bisync/internal/appstate"
	"gdrive-bisync/internal/utils"
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

type refreshMsg struct{}

type Model struct {
	paths       appstate.Paths
	syncRoot    string
	status      appstate.Status
	statusError error
	trash       []utils.TrashEntry
	trashError  error
	restoreMode bool
	restoreID   string
	message     string
}

func NewModel(paths appstate.Paths, syncRoot string) Model {
	model := Model{paths: paths, syncRoot: syncRoot}
	model.refresh()
	return model
}

func (model Model) Init() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return refreshMsg{} })
}

func (model Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case refreshMsg:
		model.refresh()
		return model, tea.Tick(time.Second, func(time.Time) tea.Msg { return refreshMsg{} })
	case tea.KeyMsg:
		if model.restoreMode {
			switch message.String() {
			case "esc":
				model.restoreMode = false
				model.restoreID = ""
			case "enter":
				entry, err := utils.RestoreTrash(model.syncRoot, strings.TrimSpace(model.restoreID))
				if err != nil {
					model.message = "Restore failed: " + err.Error()
				} else {
					model.message = "Restored " + entry.OriginalPath
				}
				model.restoreMode = false
				model.restoreID = ""
				model.refresh()
			case "backspace":
				if len(model.restoreID) > 0 {
					model.restoreID = model.restoreID[:len(model.restoreID)-1]
				}
			default:
				if len(message.Runes) > 0 {
					model.restoreID += string(message.Runes)
				}
			}
			return model, nil
		}

		switch message.String() {
		case "p":
			model.message = setPauseMessage(model.paths.PauseFile, true)
		case "r":
			model.message = setPauseMessage(model.paths.PauseFile, false)
		case "x":
			model.restoreMode = true
			model.restoreID = ""
		case "q", "ctrl+c":
			return model, tea.Quit
		}
		model.refresh()
	}
	return model, nil
}

func (model Model) View() string {
	var output strings.Builder
	output.WriteString(titleStyle.Render("gdrive-bisync manager"))
	output.WriteString("\n\n")
	if model.statusError == nil {
		fmt.Fprintf(&output, "State: %-12s PID: %-8d Paused: %v\n", model.status.State, model.status.PID, appstate.IsPaused(model.paths.PauseFile))
		fmt.Fprintf(&output, "Last sync: %s   Planned tasks: %d\n", displayTime(model.status.LastSyncFinished), model.status.TaskCount)
		if model.status.LastError != "" {
			output.WriteString(errorStyle.Render("Last error: " + model.status.LastError))
			output.WriteString("\n")
		}
	} else {
		output.WriteString(dimStyle.Render("State: stopped or status unavailable"))
		output.WriteString("\n")
	}
	if model.trashError != nil {
		output.WriteString(errorStyle.Render("Trash: " + model.trashError.Error()))
		output.WriteString("\n")
	} else {
		fmt.Fprintf(&output, "\nRecoverable trash: %d\n", len(model.trash))
		for index, entry := range model.trash {
			if index == 10 {
				output.WriteString(dimStyle.Render("  …"))
				output.WriteString("\n")
				break
			}
			fmt.Fprintf(&output, "  %s  %s\n", entry.ID, entry.OriginalPath)
		}
	}
	if model.message != "" {
		output.WriteString("\n" + model.message + "\n")
	}
	if model.restoreMode {
		output.WriteString("\nTrash ID to restore: " + model.restoreID + "█\n")
		output.WriteString(dimStyle.Render("Enter restore · Esc cancel"))
	} else {
		output.WriteString("\n" + dimStyle.Render("p pause · r resume · x restore · q quit"))
	}
	return output.String()
}

func (model *Model) refresh() {
	model.status, model.statusError = appstate.ReadStatus(model.paths.StatusFile)
	model.trash, model.trashError = utils.ListTrash(model.syncRoot)
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

func displayTime(value time.Time) string {
	if value.IsZero() {
		return "never"
	}
	return value.Format("2006-01-02 15:04:05")
}

func RunTerminal(paths appstate.Paths, syncRoot string) error {
	_, err := tea.NewProgram(NewModel(paths, syncRoot), tea.WithAltScreen()).Run()
	return err
}
