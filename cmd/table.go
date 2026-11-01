package cmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
)

var (
	cellStyle   = lipgloss.NewStyle().Padding(0, 1)
	headerStyle = cellStyle.Bold(true)
)

// renderTable draws a bordered table; a cell may span several lines with "\n".
func renderTable(headers []string, rows [][]string) string {
	return table.New().
		Border(lipgloss.RoundedBorder()).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, _ int) lipgloss.Style {
			if row == table.HeaderRow {
				return headerStyle
			}
			return cellStyle
		}).
		Render()
}

// printTable writes the table to w, dropping colors w cannot show (a pipe, NO_COLOR).
func printTable(w io.Writer, headers []string, rows [][]string) {
	lipgloss.Fprintln(w, renderTable(headers, rows))
}

// shortPath shows paths under $HOME as ~/….
func shortPath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(p, home+string(filepath.Separator)); ok {
		return "~/" + rest
	}
	return p
}
