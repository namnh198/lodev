package cmd

import (
	"io"

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
