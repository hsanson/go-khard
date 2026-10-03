package tui

import "github.com/charmbracelet/lipgloss"

type Styles struct {
	Surface                  lipgloss.Style
	Accent                   lipgloss.Style
	Dim                      lipgloss.Style
	FieldName                lipgloss.Style
	FieldValue               lipgloss.Style
	Selected                 lipgloss.Style
	Error                    lipgloss.Style
	Section                  lipgloss.Style
	DateSelected             lipgloss.Style
	Dialog                   lipgloss.Style
	HelpDialog               lipgloss.Style
	Button                   lipgloss.Style
	PrimaryButton            lipgloss.Style
	DestructiveButton        lipgloss.Style
	FocusedButton            lipgloss.Style
	FocusedDestructiveButton lipgloss.Style
}

func DefaultStyles() Styles {
	return Styles{
		Surface:                  lipgloss.NewStyle(),
		Accent:                   lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true),
		Dim:                      lipgloss.NewStyle().Foreground(lipgloss.Color("241")),
		FieldName:                lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true),
		FieldValue:               lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		Selected:                 lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62")),
		Error:                    lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true),
		Section:                  lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Bold(true),
		DateSelected:             lipgloss.NewStyle().Background(lipgloss.Color("117")).Foreground(lipgloss.Color("232")).Bold(true),
		Dialog:                   lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("39")).Padding(1, 2),
		HelpDialog:               lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("245")).Padding(1, 2),
		Button:                   lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("238")).Bold(true).Padding(0, 1),
		PrimaryButton:            lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62")).Bold(true).Padding(0, 1),
		DestructiveButton:        lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("160")).Bold(true).Padding(0, 1),
		FocusedButton:            lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("117")).Bold(true).Padding(0, 1),
		FocusedDestructiveButton: lipgloss.NewStyle().Foreground(lipgloss.Color("232")).Background(lipgloss.Color("196")).Bold(true).Underline(true),
	}
}
