package main

import (
    "fmt"
    "os"
    "path/filepath"

    "github.com/charmbracelet/bubbles/textinput"
    tea "github.com/charmbracelet/bubbletea"
)

// pathPrompt is the initial Bubble Tea model that asks the user for the
// directory they wish to ingest. Once the user confirms a valid path, it
// replaces itself with a *tree model rooted at that directory.
//
// Separating this into its own model keeps main.go minimal and allows us to
// expand the prompt later (e.g. recent-path history, validation messages).

type pathPrompt struct {
    ti     textinput.Model
    errMsg string
}

func newPathPrompt() *pathPrompt {
    ti := textinput.New()
    ti.Placeholder = "." // default to current directory
    ti.Focus()
    ti.CharLimit = 2048
    ti.Width = 40
    return &pathPrompt{ti: ti}
}

func (p *pathPrompt) Init() tea.Cmd {
    return textinput.Blink
}

func (p *pathPrompt) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        switch msg.Type {
        case tea.KeyCtrlC, tea.KeyEsc:
            return p, tea.Quit
        case tea.KeyEnter:
            // Resolve to absolute path so tree view has a clean root.
            path := p.ti.Value()
            if path == "" {
                path = "."
            }
            absPath, err := filepath.Abs(path)
            if err != nil {
                p.errMsg = fmt.Sprintf("invalid path: %v", err)
                return p, nil
            }
            info, err := os.Stat(absPath)
            if err != nil || !info.IsDir() {
                p.errMsg = fmt.Sprintf("%s is not a directory", absPath)
                return p, nil
            }
            // Create the main tree model. We start with ignoreGitignore = false
            // (respect .gitignore by default).
            t, err := newTree(absPath, false)
            if err != nil {
                p.errMsg = err.Error()
                return p, nil
            }
            return t, nil
        }
    }
    var cmd tea.Cmd
    p.ti, cmd = p.ti.Update(msg)
    return p, cmd
}

func (p *pathPrompt) View() string {
    prompt := "Enter directory to ingest (Enter to confirm):\n" + p.ti.View()
    if p.errMsg != "" {
        prompt += "\n\n" + p.errMsg
    }
    return prompt
}
