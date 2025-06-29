package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/pkoukk/tiktoken-go"
)

var (
	dirStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	fileStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	cursorStyle        = lipgloss.NewStyle().Background(lipgloss.Color("237")).Foreground(lipgloss.Color("252"))
	selectedCheck      = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Render("[✓]")
	partialCheck       = lipgloss.NewStyle().Foreground(lipgloss.Color("69")).Render("[-]")
	normalCheck        = "[ ]"
	statusBarContainer = lipgloss.NewStyle().Background(lipgloss.Color("235")).Foreground(lipgloss.Color("252")).Padding(0, 1)
	statusBarStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	helpStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

type selState int

const (
	none selState = iota
	full
	partial
)

type node struct {
	name     string
	path     string
	isDir    bool
	depth    int
	expanded bool
	parent   *node
	children []*node
	state    selState
	isLast   bool
}

func buildNode(path string, name string, depth int, patterns []gitignore.Pattern, ignoreGitignore bool, parentNode *node) (*node, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	isDir := info.IsDir()

	n := &node{
		name:     name,
		path:     path,
		isDir:    isDir,
		depth:    depth,
		expanded: false,
		parent:   parentNode,
		state:    none,
	}

	if !isDir {
		return n, nil
	}

	files, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		fullPath := filepath.Join(path, file.Name())
		relativePath := fullPath
		repoRoot := ""
		if repo, err := git.PlainOpenWithOptions(path, &git.PlainOpenOptions{DetectDotGit: true}); err == nil {
			worktree, err := repo.Worktree()
			if err == nil {
				repoRoot = worktree.Filesystem.Root()
			}
		}
		if repoRoot != "" {
			rel, err := filepath.Rel(repoRoot, fullPath)
			if err == nil {
				relativePath = rel
			}
		}

		if file.IsDir() && file.Name() == ".git" && !ignoreGitignore {
			continue
		}

		if !ignoreGitignore && isIgnored(patterns, relativePath, file.IsDir()) {
			continue
		}

		child, err := buildNode(fullPath, file.Name(), depth+1, patterns, ignoreGitignore, n)
		if err != nil {
			return nil, err
		}
		n.children = append(n.children, child)
	}

	if len(n.children) > 0 {
		n.children[len(n.children)-1].isLast = true
	}

	return n, nil
}

func flattenVisible(n *node) []*node {
	var visible []*node
	visible = append(visible, n)
	if n.expanded {
		for _, child := range n.children {
			visible = append(visible, flattenVisible(child)...)
		}
	}
	return visible
}

type tree struct {
	path            string
	root            *node
	visible         []*node
	indexToNode     map[int]*node
	cursor          int
	selected        map[int]struct{}
	ignoreGitignore bool

	selectedFiles int
	selectedDirs  int
	totalSize     int64
	totalTokens   int

	output            string
	outputFile        string
	previewing        bool
	textInput         textinput.Model
	inputtingFilename bool
	showHelp          bool
	width             int
}

func newTree(path string, ignoreGitignore bool) (*tree, error) {
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

	if !ignoreGitignore && repoRoot != "" {
		gitignorePath := filepath.Join(repoRoot, ".gitignore")
		if data, err := ioutil.ReadFile(gitignorePath); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				patterns = append(patterns, gitignore.ParsePattern(line, nil))
			}
		}
	}

	rootNode, err := buildNode(path, filepath.Base(path), 0, patterns, ignoreGitignore, nil)
	if err != nil {
		return nil, err
	}
	rootNode.expanded = true

	t := &tree{
		path:            path,
		root:            rootNode,
		selected:        make(map[int]struct{}),
		ignoreGitignore: ignoreGitignore,
	}
	t.rebuildVisible()

	ti := textinput.New()
	ti.Placeholder = "digest.txt"
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 20
	t.textInput = ti

	return t, nil
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
	return tea.EnterAltScreen
}

func (t *tree) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.width = msg.Width
		return t, nil
	}

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
		case "q":
			return t, tea.Quit
		case "?":
			t.showHelp = !t.showHelp
		case "up", "k":
			if t.cursor > 0 {
				t.cursor--
			}
		case "down", "j":
			if t.cursor < len(t.visible)-1 {
				t.cursor++
			}
		case "right", "l":
			n := t.indexToNode[t.cursor]
			if n != nil && n.isDir && !n.expanded {
				n.expanded = true
				current := n
				t.rebuildVisible()
				for i, nd := range t.visible {
					if nd == current {
						t.cursor = i
						break
					}
				}
			}
		case "left", "h":
			n := t.indexToNode[t.cursor]
			if n != nil {
				if n.isDir && n.expanded {
					n.expanded = false
					current := n
					t.rebuildVisible()
					for i, nd := range t.visible {
						if nd == current {
							t.cursor = i
							break
						}
					}
				} else if n.parent != nil {
					parent := n.parent
					parent.expanded = false
					t.rebuildVisible()
					for i, nd := range t.visible {
						if nd == parent {
							t.cursor = i
							break
						}
					}
				}
			}
		case " ":
			n := t.indexToNode[t.cursor]
			if n == nil {
				return t, nil
			}

			newState := full
			if n.state == full || n.state == partial {
				newState = none
			}

			t.toggleSelection(n, newState)
			t.updateStats()
		case "a":
			selectAll := false
			for _, n := range t.visible {
				if n.state == none {
					selectAll = true
					break
				}
			}

			newState := full
			if !selectAll {
				newState = none
			}

			for _, n := range t.visible {
				t.toggleSelection(n, newState)
			}
			t.updateStats()
		case "i":
			t.ignoreGitignore = !t.ignoreGitignore
			newTree, err := newTree(t.path, t.ignoreGitignore)
			if err != nil {
				return t, nil
			}
			return newTree, nil
		case "g":
			t.generateOutput()
			return t, tea.Quit
		case "c":
			t.generateOutput()
			err := clipboard.WriteAll(t.output)
			if err != nil {
				fmt.Printf("Error copying to clipboard: %v\n", err)
			}
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
			selectedNode := t.indexToNode[t.cursor]
			if selectedNode.isDir {
				newTree, err := newTree(selectedNode.path, t.ignoreGitignore)
				if err != nil {
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

	if t.showHelp {
		return `
CodeTree Help:

Navigation:
  ↑/k: Up
  ↓/j: Down
  ←/h: Collapse directory / Go to parent
  →/l: Expand directory

Selection:
  <space>: Toggle selection of current item
  a: Toggle selection of all items

Other:
  i: Toggle ignoring .gitignore files
  g: Generate digest and quit
  c: Generate and copy digest to clipboard and quit
  o: Set output file name
  p: Preview digest
  q: Quit
  ?: Toggle help
`
	}

	s := strings.Builder{}
	for i, n := range t.visible {
		var lineBuilder strings.Builder

		// Checkbox
		check := normalCheck
		if n.state == full {
			check = selectedCheck
		} else if n.state == partial {
			check = partialCheck
		}
		lineBuilder.WriteString(check)
		lineBuilder.WriteString(" ")

		// Tree structure
		ancestors := []*node{}
		p := n.parent
		for p != nil {
			ancestors = append([]*node{p}, ancestors...)
			p = p.parent
		}

		for _, ancestor := range ancestors {
			if ancestor.depth < 0 { // Should not happen, but as a safeguard
				continue
			}
			if ancestor.isLast {
				lineBuilder.WriteString("    ")
			} else {
				lineBuilder.WriteString("│   ")
			}
		}

		if n.depth > 0 {
			if n.isLast {
				lineBuilder.WriteString("└── ")
			} else {
				lineBuilder.WriteString("├── ")
			}
		}

		// Icon and name
		var fileIcon string
		if n.isDir {
			fileIcon = "📁"
		} else {
			switch filepath.Ext(n.name) {
			case ".txt", ".md":
				fileIcon = "📝"
			default:
				fileIcon = "📄"
			}
		}

		expander := "  " // For files
		if n.isDir {
			if n.expanded {
				expander = "▾ "
			} else {
				expander = "▸ "
			}
		}

		lineBuilder.WriteString(expander)
		lineBuilder.WriteString(fileIcon)
		lineBuilder.WriteString(" ")
		lineBuilder.WriteString(n.name)

		// Apply styles
		var lineStyle lipgloss.Style
		if n.isDir {
			lineStyle = dirStyle
		} else {
			lineStyle = fileStyle
		}

		// Render the line
		styledLine := lineStyle.Render(lineBuilder.String())

		if t.cursor == i {
			s.WriteString(cursorStyle.Render(styledLine))
		} else {
			s.WriteString(styledLine)
		}
		s.WriteString("\n")
	}

	// Status bar
	statusText := fmt.Sprintf("Selected: %d files, %d dirs | Size: %s | Tokens: %d", t.selectedFiles, t.selectedDirs, formatBytes(t.totalSize), t.totalTokens)
	var helpText string
	if t.showHelp {
		helpText = "Press ? to close help"
	} else {
		helpText = "[g]enerate | [p]review | [c]opy | [q]uit | [?]help"
	}

	renderedStatus := statusBarStyle.Render(statusText)
	renderedHelp := helpStyle.Render(helpText)

	// Stack the status and help text vertically
	statusBarContent := lipgloss.JoinVertical(lipgloss.Left, renderedStatus, renderedHelp)

	// Render the final bar inside the container, ensuring it spans the full width
	s.WriteString("\n" + statusBarContainer.Width(t.width).Render(statusBarContent))

	return s.String()
}

func (t *tree) generateOutput() {
	var filesToRead []string
	var collectFiles func(*node)
	collectFiles = func(n *node) {
		if n.state == full && !n.isDir {
			filesToRead = append(filesToRead, n.path)
		} else if n.state == partial || (n.state == full && n.isDir) {
			for _, child := range n.children {
				collectFiles(child)
			}
		}
	}
	collectFiles(t.root)

	var b strings.Builder
	for _, path := range filesToRead {
		content, err := ioutil.ReadFile(path)
		if err != nil {
			continue
		}

		relativePath, err := filepath.Rel(t.path, path)
		if err != nil {
			relativePath = path
		}

		b.WriteString(fmt.Sprintf("--- %s ---\n", relativePath))
		b.WriteString(string(content))
		b.WriteString("\n")
	}

	t.output = b.String()

	if t.outputFile != "" {
		ioutil.WriteFile(t.outputFile, []byte(t.output), 0644)
	}
}

func (t *tree) updateStats() {
	t.selectedFiles = 0
	t.selectedDirs = 0
	t.totalSize = 0
	t.totalTokens = 0

	var content strings.Builder

	var countTokens func(*node)
	countTokens = func(n *node) {
		if n.state == none {
			return
		}

		if n.isDir {
			if n.state == full {
				t.selectedDirs++
			}
			for _, child := range n.children {
				countTokens(child)
			}
		} else {
			if n.state == full {
				t.selectedFiles++
				info, err := os.Stat(n.path)
				if err == nil {
					t.totalSize += info.Size()
				}
				fileContent, err := ioutil.ReadFile(n.path)
				if err == nil {
					content.Write(fileContent)
				}
			}
		}
	}

	countTokens(t.root)

	tke, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		return
	}

	t.totalTokens = len(tke.Encode(content.String(), nil, nil))
}

func (t *tree) rebuildVisible() {
	t.visible = flattenVisible(t.root)
	t.indexToNode = make(map[int]*node, len(t.visible))
	for i, n := range t.visible {
		t.indexToNode[i] = n
	}
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

func (t *tree) toggleSelection(n *node, state selState) {
	n.state = state
	if n.isDir {
		for _, child := range n.children {
			t.toggleSelection(child, state)
		}
	}
	if n.parent != nil {
		t.updateParentSelection(n.parent)
	}
}

func (t *tree) updateParentSelection(n *node) {
	if !n.isDir {
		return
	}

	numChildren := len(n.children)
	if numChildren == 0 {
		return
	}

	fullySelected := 0
	partiallySelected := 0
	for _, child := range n.children {
		switch child.state {
		case full:
			fullySelected++
		case partial:
			partiallySelected++
		}
	}

	if fullySelected == numChildren {
		n.state = full
	} else if fullySelected > 0 || partiallySelected > 0 {
		n.state = partial
	} else {
		n.state = none
	}

	if n.parent != nil {
		t.updateParentSelection(n.parent)
	}
}