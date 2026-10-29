package tui

import "github.com/charmbracelet/lipgloss"

type Styles struct {
	Container       lipgloss.Style
	MainPanel       lipgloss.Style
	FocusedPanel    lipgloss.Style
	PanelTitle      lipgloss.Style
	Accent          lipgloss.Style
	Subtle          lipgloss.Style
	Error           lipgloss.Style
	Success         lipgloss.Style
	Value           lipgloss.Style
	Disabled        lipgloss.Style
	SelectedRow     lipgloss.Style
	Focus           lipgloss.Style
	Overlay         lipgloss.Style
	HelpOverlay     lipgloss.Style
	PrimaryButton   lipgloss.Style
	SecondaryButton lipgloss.Style
	FocusedButton   lipgloss.Style
	DangerButton    lipgloss.Style
	FocusedDanger   lipgloss.Style
}

func DefaultStyles() Styles {
	return Styles{
		Container:       lipgloss.NewStyle().Padding(1, 1),
		MainPanel:       lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1),
		FocusedPanel:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("117")).Padding(0, 1),
		PanelTitle:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("153")),
		Accent:          lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true),
		Subtle:          lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		Error:           lipgloss.NewStyle().Foreground(lipgloss.Color("210")).Bold(true),
		Success:         lipgloss.NewStyle().Foreground(lipgloss.Color("65")).Bold(true),
		Value:           lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		Disabled:        lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		SelectedRow:     lipgloss.NewStyle().Background(lipgloss.Color("238")).Foreground(lipgloss.Color("230")).Bold(true),
		Focus:           lipgloss.NewStyle().Background(lipgloss.Color("117")).Foreground(lipgloss.Color("232")).Bold(true),
		Overlay:         lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2),
		HelpOverlay:     lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("245")).Padding(1, 2),
		PrimaryButton:   lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("238")).Bold(true).Padding(0, 1),
		SecondaryButton: lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("238")).Bold(true).Padding(0, 1),
		FocusedButton:   lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("117")).Bold(true).Padding(0, 1),
		DangerButton:    lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Background(lipgloss.Color("238")).Bold(true).Padding(0, 1),
		FocusedDanger:   lipgloss.NewStyle().Foreground(lipgloss.Color("233")).Background(lipgloss.Color("196")).Bold(true).Padding(0, 1),
	}
}
