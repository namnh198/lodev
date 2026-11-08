package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/term"
)

// Swapped in tests: a select needs a real terminal, line prompts work anywhere.
var (
	useSelect = func(in io.Reader) bool { f, ok := in.(*os.File); return ok && term.IsTerminal(int(f.Fd())) }
	selectFn  = runSelect
)

// selectModel is an arrow-key list: ↑/↓ (or k/j) move, Enter picks, Ctrl+C or Esc aborts.
type selectModel struct {
	title         string
	opts          []string
	cursor        int
	choice        string
	done, aborted bool
}

func newSelectModel(title string, opts []string, def string) selectModel {
	return selectModel{title: title, opts: opts, cursor: max(slices.Index(opts, def), 0)}
}

func (m selectModel) press(key string) selectModel {
	switch key {
	case "up", "k":
		m.cursor = max(m.cursor-1, 0)
	case "down", "j":
		m.cursor = min(m.cursor+1, len(m.opts)-1)
	case "enter":
		m.choice, m.done = m.opts[m.cursor], true
	case "ctrl+c", "esc":
		m.aborted = true
	}
	return m
}

// view is the list while choosing; bubbletea clears it on exit, and runSelect prints the answer.
func (m selectModel) view() string {
	if m.done || m.aborted {
		return ""
	}
	var b strings.Builder
	b.WriteString(m.title + " (↑/↓, enter):\n")
	for i, o := range m.opts {
		if i == m.cursor {
			b.WriteString("> " + o + "\n")
		} else {
			b.WriteString("  " + o + "\n")
		}
	}
	return b.String()
}

type selectTea struct{ m selectModel }

func (s selectTea) Init() tea.Cmd { return nil }

func (s selectTea) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		s.m = s.m.press(k.String())
		if s.m.done || s.m.aborted {
			return s, tea.Quit
		}
	}
	return s, nil
}

func (s selectTea) View() tea.View { return tea.NewView(s.m.view()) }

// runSelect shows the list on the terminal and returns the picked option.
func runSelect(in io.Reader, out io.Writer, title string, opts []string, def string) (string, error) {
	final, err := tea.NewProgram(selectTea{newSelectModel(title, opts, def)}, tea.WithInput(in), tea.WithOutput(out)).Run()
	if err != nil {
		return "", err
	}
	if m := final.(selectTea).m; m.done {
		fmt.Fprintf(out, "%s: %s\n", title, m.choice) // stays on screen like the line prompts
		return m.choice, nil
	}
	return "", errors.New("init aborted")
}
