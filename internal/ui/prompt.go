/*
MIT License

# Copyright (c) 2025 OcomSoft

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

// Package ui provides bubbletea TUI prompts for destructive migration operations.
package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	yamlpkg "github.com/ocomsoft/morphic/internal/yaml"
)

// PromptScope controls how a prompt response is applied to subsequent operations.
type PromptScope int

// PromptScope constants define how broadly a prompt response is applied.
const (
	ScopeOne  PromptScope = iota // apply to this operation only
	ScopeAll                     // apply to all remaining destructive ops
	ScopeType                    // apply to all remaining ops of the same ChangeType
)

// ParsePromptInput parses a prompt input like "1", "3a", "5t" into a
// PromptResponse and a scope. Used as fallback for non-interactive environments.
func ParsePromptInput(input string) (yamlpkg.PromptResponse, PromptScope) {
	if len(input) == 0 {
		return yamlpkg.PromptGenerate, ScopeOne
	}

	scope := ScopeOne
	last := input[len(input)-1]
	switch last {
	case 'a', 'A':
		scope = ScopeAll
		input = input[:len(input)-1]
	case 't', 'T':
		scope = ScopeType
		input = input[:len(input)-1]
	}

	switch input {
	case "1":
		return yamlpkg.PromptGenerate, scope
	case "2":
		return yamlpkg.PromptReview, scope
	case "3":
		return yamlpkg.PromptOmit, scope
	case "4":
		return yamlpkg.PromptExit, ScopeOne
	case "5":
		return yamlpkg.PromptIgnoreErrors, scope
	default:
		return yamlpkg.PromptGenerate, ScopeOne
	}
}

// promptChoice maps a display label to a PromptResponse.
type promptChoice struct {
	label string
	desc  string
	resp  yamlpkg.PromptResponse
}

var destructiveChoices = []promptChoice{
	{"Generate", "include operation in migration", yamlpkg.PromptGenerate},
	{"Review", "include with // REVIEW comment", yamlpkg.PromptReview},
	{"Omit", "skip SQL; schema state still advances (SchemaOnly)", yamlpkg.PromptOmit},
	{"IgnoreErrors", "include with IgnoreErrors: true", yamlpkg.PromptIgnoreErrors},
	{"Exit", "cancel migration generation", yamlpkg.PromptExit},
}

var scopeLabels = []string{"This only", "All remaining", "All of this type"}

// promptModel is the bubbletea model for the destructive-operation prompt.
type promptModel struct {
	title    string
	choices  []promptChoice
	cursor   int
	scope    PromptScope
	maxScope int // 2 = all three scopes available
	chosen   bool
	quitting bool
}

func newPromptModel(title string, changeType yamlpkg.ChangeType) promptModel {
	return promptModel{
		title:    title,
		choices:  destructiveChoices,
		maxScope: 2,
	}
}

func (m promptModel) Init() tea.Cmd {
	return nil
}

func (m promptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.choices)-1 {
				m.cursor++
			}
		case "tab":
			m.scope = (m.scope + 1) % PromptScope(m.maxScope+1)
		case "shift+tab":
			if m.scope == 0 {
				m.scope = PromptScope(m.maxScope)
			} else {
				m.scope--
			}
		case "enter":
			m.chosen = true
			return m, tea.Quit
		case "q", "esc", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}
	}
	return m, nil
}

var (
	warningStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	scopeStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	hintStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
)

func (m promptModel) View() string {
	var b strings.Builder

	b.WriteString(warningStyle.Render("⚠  "+m.title) + "\n\n")

	for i, c := range m.choices {
		cursor := "  "
		label := fmt.Sprintf("%-14s", c.label)
		desc := dimStyle.Render("— " + c.desc)

		if i == m.cursor {
			cursor = selectedStyle.Render("> ")
			label = selectedStyle.Render(label)
		}
		fmt.Fprintf(&b, "%s%s %s\n", cursor, label, desc)
	}

	// Exit choice doesn't support scope
	if m.choices[m.cursor].resp != yamlpkg.PromptExit {
		b.WriteString("\n  Scope: ")
		for i, label := range scopeLabels {
			if PromptScope(i) == m.scope {
				b.WriteString(scopeStyle.Render("[" + label + "]"))
			} else {
				b.WriteString(dimStyle.Render(" " + label + " "))
			}
			if i < len(scopeLabels)-1 {
				b.WriteString("  ")
			}
		}
		b.WriteString("\n")
	}

	b.WriteString("\n" + hintStyle.Render("↑/↓ select • tab scope • enter confirm • esc cancel") + "\n")

	return b.String()
}

// RunDestructivePrompt shows a bubbletea prompt for a single destructive
// operation and returns the user's chosen response and scope.
func RunDestructivePrompt(title string, changeType yamlpkg.ChangeType) (yamlpkg.PromptResponse, PromptScope, error) {
	m := newPromptModel(title, changeType)
	p := tea.NewProgram(m)
	result, err := p.Run()
	if err != nil {
		return 0, ScopeOne, fmt.Errorf("running prompt: %w", err)
	}

	final := result.(promptModel)
	if final.quitting {
		return yamlpkg.PromptExit, ScopeOne, nil
	}

	resp := final.choices[final.cursor].resp
	scope := final.scope
	if resp == yamlpkg.PromptExit {
		scope = ScopeOne
	}
	return resp, scope, nil
}
