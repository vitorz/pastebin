package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Model struct {
	cursor   int
	Selected int
	Items    []fmt.Stringer
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.Selected = m.cursor
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(m.Items)-1 {
				m.cursor++
			}

		case "enter":
			m.Selected = m.cursor
			// On enter, quit and return
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m Model) View() string {
	var b strings.Builder
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).Inline(true).
		Render("Select items with arrow keys and space. Press enter to finish.")
	b.WriteString(title)
	b.WriteString("\n\n")

	for i, item := range m.Items {
		line := item.String()

		if m.cursor == i {
			line = lipgloss.NewStyle().
				Bold(true).Foreground(lipgloss.Color("7")).
				Background(lipgloss.Color("196")).Inline(true).Render(line)
			b.WriteString(line)
			b.WriteString("\n")
		} else {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	return b.String()
}
