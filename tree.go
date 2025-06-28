package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/pkoukk/tiktoken-go"
)

type tree struct {
	path              string
	items             []string
	cursor            int
	selected          map[int]struct{}
	ignoreGitignore   bool
	selectedFiles     int
	selectedDirs      int
	totalSize         int64
	totalTokens       int
	output            string
	outputFile        string
	previewing        bool
	textInput         textinput.Model
	inputtingFilename bool
}

func newTree(path string, ignoreGitignore bool) (*tree, error) {
	files, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	repo, err := git.PlainOpenWithOptions(path, &git.PlainOpenOptions{DetectDotGit: true})
	var patterns []gitignore.Pattern
	var repoRoot string
	if err == nil {
		worktree, err := repo.Worktree()
		if err == nil {
			patterns = worktree.Excludes
			repoRoot = worktree.Filesystem.Root()
		}
	}

	// Manually load patterns from the root .gitignore. The go-git worktree
	// excludes do not include these rules, so we append them here to make sure
	// they are respected by the UI (e.g. to hide go.sum, test artifacts, etc.).
	if !ignoreGitignore && repoRoot != "" {
		gitignorePath := filepath.Join(repoRoot, ".gitignore")
		if data, err := ioutil.ReadFile(gitignorePath); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				// Skip empty lines and comments
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				patterns = append(patterns, gitignore.ParsePattern(line, nil))
			}
		}
	}

	var items []string
	for _, file := range files {
		fullPath := filepath.Join(path, file.Name())
		relativePath := fullPath
		if repoRoot != "" {
			rel, err := filepath.Rel(repoRoot, fullPath)
			if err == nil {
				relativePath = rel
			}
		}

		if !ignoreGitignore && isIgnored(patterns, relativePath, file.IsDir()) {
			continue
		}
		items = append(items, file.Name())
	}

	ti := textinput.New()
	ti.Placeholder = "digest.txt"
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 20

	return &tree{
		path:            path,
		items:           items,
		selected:        make(map[int]struct{}),
		ignoreGitignore: ignoreGitignore,
		textInput:       ti,
	}, nil
}

func isIgnored(patterns []gitignore.Pattern, path string, isDir bool) bool {
	pathComponents := strings.Split(path, string(filepath.Separator))
	for _, p := range patterns {
		if p.Match(pathComponents, isDir) == gitignore.Exclude {
			return true
		}
	}
	return false
}

func (t *tree) Init() tea.Cmd {
	return nil
}

func (t *tree) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	if t.inputtingFilename {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.Type {
			case tea.KeyEnter:
				t.outputFile = t.textInput.Value()
				t.inputtingFilename = false
			case tea.KeyEsc:
				t.inputtingFilename = false
			}
		}
		t.textInput, cmd = t.textInput.Update(msg)
		return t, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return t, tea.Quit
		case "up", "k":
			if t.cursor > 0 {
				t.cursor--
			}
		case "down", "j":
			if t.cursor < len(t.items)-1 {
				t.cursor++
			}
		case " ":
			_, ok := t.selected[t.cursor]
			if ok {
				delete(t.selected, t.cursor)
			} else {
				t.selected[t.cursor] = struct{}{}
			}
			t.updateStats()
		case "i":
			t.ignoreGitignore = !t.ignoreGitignore
			newTree, err := newTree(t.path, t.ignoreGitignore)
			if err != nil {
				// Handle error
				return t, nil
			}
			return newTree, nil
		case "g":
			t.generateOutput()
			return t, tea.Quit
		case "p":
			t.generateOutput()
			t.previewing = true
			return t, nil
		case "o":
			t.inputtingFilename = true
			return t, nil
		case "enter":
			if t.previewing {
				t.previewing = false
				return t, nil
			}
			selectedPath := filepath.Join(t.path, t.items[t.cursor])
			info, err := os.Stat(selectedPath)
			if err != nil {
				// Handle error
				return t, nil
			}
			if info.IsDir() {
				newTree, err := newTree(selectedPath, t.ignoreGitignore)
				if err != nil {
					// Handle error
					return t, nil
				}
				return newTree, nil
			}
		}
	}
	return t, nil
}

func (t *tree) View() string {
	if t.inputtingFilename {
		return fmt.Sprintf(
			"Enter output filename:\n%s\n\n(esc to cancel)",
			t.textInput.View(),
		)
	}

	if t.previewing {
		return t.output
	}

	s := fmt.Sprintf("Current path: %s\n\n", t.path)
	for i, item := range t.items {
		cursor := " "
		if t.cursor == i {
			cursor = ">"
		}

		checked := " "
		if _, ok := t.selected[i]; ok {
			checked = "x"
		}

		s += fmt.Sprintf("%s [%s] %s\n", cursor, checked, item)
	}
	s += fmt.Sprintf("\n[i]gnore .gitignore: %v\n", t.ignoreGitignore)
	s += fmt.Sprintf("Selected: %d files, %d folders | Size: %s | Tokens: %d\n", t.selectedFiles, t.selectedDirs, formatBytes(t.totalSize), t.totalTokens)
	s += fmt.Sprintf("\n[g]enerate | [p]review | [o]utput file: %s | [q]uit\n", t.outputFile)
	return s
}

func (t *tree) generateOutput() {
	var output string
	for i := range t.selected {
		path := filepath.Join(t.path, t.items[i])
		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		if info.IsDir() {
			filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if !info.IsDir() {
					content, err := ioutil.ReadFile(p)
					if err == nil {
						output += fmt.Sprintf("--- %s ---\n\n%s\n\n", p, content)
					}
				}
				return nil
			})
		} else {
			content, err := ioutil.ReadFile(path)
			if err == nil {
				output += fmt.Sprintf("--- %s ---\n\n%s\n\n", path, content)
			}
		}
	}
	t.output = output
}

func (t *tree) updateStats() {
	var selectedFiles, selectedDirs int
	var totalSize int64
	var totalTokens int

	tkn, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		// Handle error
	}

	for i := range t.selected {
		path := filepath.Join(t.path, t.items[i])
		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		if info.IsDir() {
			selectedDirs++
			filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if !info.IsDir() {
					selectedFiles++
					totalSize += info.Size()
					content, err := ioutil.ReadFile(p)
					if err == nil {
						totalTokens += len(tkn.Encode(string(content), nil, nil))
					}
				}
				return nil
			})
		} else {
			selectedFiles++
			totalSize += info.Size()
			content, err := ioutil.ReadFile(path)
			if err == nil {
				totalTokens += len(tkn.Encode(string(content), nil, nil))
			}
		}
	}

	t.selectedFiles = selectedFiles
	t.selectedDirs = selectedDirs
	t.totalSize = totalSize
	t.totalTokens = totalTokens
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}
