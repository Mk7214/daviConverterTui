package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
)

type screen int

const (
	screenPicker screen = iota
	screenFormat
	screenConfirm
	screenRunning
	screenDone
	screenError
)

const (
	padding  = 2
	maxWidth = 80
)

type formatItem struct {
	title string
	desc  string
	id    string
}

func (f formatItem) Title() string       { return f.title }
func (f formatItem) Description() string { return f.desc }
func (f formatItem) FilterValue() string { return f.title }

type model struct {
	// screen state
	screen screen
	// fileInput
	filepicker filepicker.Model
	// format input
	formatList list.Model
	// results
	format string // format value
	value  string // input field value
	output string

	progress   progress.Model
	percent    float64
	lastStatus string

	cancelFn context.CancelFunc
	cmd      *exec.Cmd

	err      error
	canceled bool

	progressChan chan tea.Msg
}

type (
	progressMsg     float64
	ffmpegStatusMsg string
	ffmpegErrMsg    error
)

type startedMsg struct {
	cancel context.CancelFunc
	cmd    *exec.Cmd
	ch     chan tea.Msg
}

type clearErrorMsg struct{}

func clearErrorAfter(t time.Duration) tea.Cmd {
	return tea.Tick(t, func(_ time.Time) tea.Msg {
		return clearErrorMsg{}
	})
}

func initialModel() model {
	// filepath input
	fp := filepicker.New()

	fp.AllowedTypes = []string{".mp4", ".mov", ".mxf", ".wav", ".mkv", ".flac"}

	if hd, err := os.UserHomeDir(); err == nil {
		fp.CurrentDirectory = hd
	} else {
		fp.CurrentDirectory = "."
	}
	fp.ShowHidden = false
	fp.AutoHeight = true

	// format input
	items := []list.Item{
		formatItem{title: "H.264 (MP4)", desc: "Smaller files, generally supported", id: "h264"},
		formatItem{title: "Apple ProRes (MOV)", desc: "Edit-friendly, large files (ProRes)", id: "prores"},
		formatItem{title: "DNxHD / DNxHR (MXF)", desc: "Avid-style mezzanine codec", id: "dnxhd"},
		formatItem{title: "WAV 48kHz (Audio only)", desc: "Export audio only as WAV", id: "wav"},
	}

	delegate := list.NewDefaultDelegate()
	ls := list.New(items, delegate, 60, 20)
	ls.Title = "Choose output format (↑/↓ then Enter)"
	ls.Select(0)

	pb := progress.New()
	pb.SetPercent(0)

	return model{
		screen:     screenPicker,
		filepicker: fp,
		formatList: ls,
		progress:   pb,
		percent:    0,
		format:     "h264",
		err:        nil,
		canceled:   false,
	}
}

func (m model) Init() tea.Cmd {
	return m.filepicker.Init()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenPicker:
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "ctrl+c", "q":
				m.canceled = true
				return m, tea.Quit
			}
		case clearErrorMsg:
			m.err = nil
		}

		var cmd tea.Cmd
		m.filepicker, cmd = m.filepicker.Update(msg)

		// Did the user select a file?
		if didSelect, path := m.filepicker.DidSelectFile(msg); didSelect {
			// Get the path of the selected file.
			m.value = path
			m.screen = screenFormat
		}
		// Did the user select a disabled file?
		// This is only necessary to display an error to the user.
		if didSelect, path := m.filepicker.DidSelectDisabledFile(msg); didSelect {
			// Let's clear the selectedFile and display an error.
			m.err = errors.New(path + " is not valid.")
			m.value = ""
			return m, tea.Batch(cmd, clearErrorAfter(2*time.Second))
		}

		return m, cmd

	case screenFormat:
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.Type {
			case tea.KeyEnter:
				it := m.formatList.SelectedItem()
				if it == nil {
					m.format = "h264"
				} else {
					if fi, ok := it.(formatItem); ok {
						m.format = fi.id
					}
				}
				m.output = defaultOutputPath(m.value, m.format)
				m.screen = screenConfirm
				return m, nil

			case tea.KeyEsc:
				m.screen = screenPicker
				return m, nil

			case tea.KeyCtrlC:
				m.canceled = true
				return m, tea.Quit
			}
		}
		var cmd tea.Cmd
		m.formatList, cmd = m.formatList.Update(msg)
		return m, cmd

	case screenConfirm:
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.Type {
			case tea.KeyEnter:
				// start conversion inside TUI
				m.screen = screenRunning
				// ensure progress starts at 0
				m.percent = 0
				m.progress.SetPercent(0)
				return m, startConversionCmd(m.value, m.output, m.format)
			case tea.KeyEsc:
				// go back to format selection
				m.screen = screenFormat
				return m, nil
			case tea.KeyCtrlC:
				m.canceled = true
				return m, tea.Quit
			}
		}
		// nothing else to update in confirm screen
		return m, nil

	case screenRunning:
		// handle progress updates and control keys
		switch msg := msg.(type) {
		case tea.WindowSizeMsg:
			m.progress.Width = msg.Width - padding*2 - 4
			if m.progress.Width > maxWidth {
				m.progress.Width = maxWidth
			}
			return m, nil

		case progress.FrameMsg:
			progressModel, cmd := m.progress.Update(msg)
			m.progress = progressModel.(progress.Model)
			return m, cmd

		case progressMsg:
			m.percent = float64(msg)
			m.progress.SetPercent(m.percent)
			pm, _ := m.progress.Update(nil) // get updated tea.Model and cmd
			if p, ok := pm.(progress.Model); ok {
				m.progress = p
			}
			return m, listen(m.progressChan)

		case ffmpegStatusMsg:
			m.lastStatus = string(msg)
			return m, listen(m.progressChan)

		case ffmpegErrMsg:
			m.err = error(msg)
			m.screen = screenError
			return m, nil

		case startedMsg:
			// store cancel + cmd
			m.cancelFn = msg.cancel
			m.cmd = msg.cmd
			m.progressChan = msg.ch
			// start listening for progress/status messages from the channel
			return m, listen(m.progressChan)

		case tea.KeyMsg:
			// allow cancel during running
			if msg.String() == "ctrl+c" || msg.String() == "esc" {
				// cancel the ffmpeg process
				if m.cancelFn != nil {
					m.cancelFn()
				}
				// kill if still running
				if m.cmd != nil && m.cmd.Process != nil {
					_ = m.cmd.Process.Kill()
				}
				m.canceled = true
				return m, tea.Quit
			}
		}
		// also update internal progress bubble (for animation ticks)
		var cmd tea.Cmd
		pm, cmd := m.progress.Update(msg)     // pm is tea.Model
		if p, ok := pm.(progress.Model); ok { // assert concrete type
			m.progress = p
		}
		return m, cmd

	case screenError:
		// show error until keypress
		switch msg.(type) {
		case tea.KeyMsg:
			// any key -> go back to format screen to retry
			m.err = nil
			m.screen = screenFormat
			return m, nil
		}
		return m, nil

	case screenDone:
		// after done, any key quits
		switch msg.(type) {
		case tea.KeyMsg:
			return m, tea.Quit
		}
		return m, nil
	}

	return m, nil
}

func (m model) View() string {
	switch m.screen {
	case screenPicker:
		if m.canceled {
			return ""
		}
		var s strings.Builder
		s.WriteString("\n  ")
		if m.err != nil {
			s.WriteString(m.filepicker.Styles.DisabledFile.Render(m.err.Error()))
		} else if m.value == "" {
			s.WriteString("Pick a file:")
		} else {
			s.WriteString("Selected file: " + m.filepicker.Styles.Selected.Render(m.value))
		}
		s.WriteString("\n\n" + m.filepicker.View() + "\n")
		return s.String()

	case screenFormat:
		return fmt.Sprintf(
			"Selected input: %s\n\n%s\n\n%s\n",
			m.value,
			m.formatList.View(),
			"(Esc to go back, Enter to confirm selection)",
		)

	case screenConfirm:
		return fmt.Sprintf(
			"Ready to convert:\n\n  input:  %s\n  format: %s\n  output: %s\n\nPress Enter to start conversion, Esc to go back, Ctrl+C to cancel.\n",
			m.value, m.format, m.output,
		)

	case screenRunning:
		// show progress bar + percent + lastStatus
		pad := strings.Repeat(" ", padding)
		return "\n" +
			pad + m.progress.View() + "\n\n"

	case screenError:
		return fmt.Sprintf("Error: %v\n\n(press any key to go back)\n", m.err)

	case screenDone:
		return fmt.Sprintf("Conversion finished!\n\nOutput: %s\n\n(press any key to exit)\n", m.output)

	default:
		return "unknown state"

	}
}

func RunTUI() (model, error) {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return model{}, err
	}
	// p.Run returns the final model interface{}
	if fm, ok := final.(model); ok {
		return fm, nil
	}
	return model{}, fmt.Errorf("unexpected final model type")
}
