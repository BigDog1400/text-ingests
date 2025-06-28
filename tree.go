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

		// Always omit the .git directory unless we explicitly want all hidden files
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
	items           []string
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

	// Build full directory tree using node structure
		rootNode, err := buildNode(path, filepath.Base(path), 0, patterns, ignoreGitignore, nil)
	if err != nil {
		return nil, err
	}
	rootNode.expanded = true

	visible := flattenVisible(rootNode)
	items := make([]string, len(visible))
	indexMap := make(map[int]*node, len(visible))
	for i, n := range visible {
		indent := strings.Repeat("  ", n.depth)
		prefix := ""
		if n.isDir {
			if n.expanded {
				prefix = "▾ "
			} else {
				prefix = "▸ "
			}
		} else {
			prefix = "  "
		}
		items[i] = indent + prefix + n.name
		indexMap[i] = n
	}

	ti := textinput.New()
	ti.Placeholder = "digest.txt"
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 20

	return &tree{
		path:            path,
		root:            rootNode,
		visible:         visible,
		items:           items,
		indexToNode:     indexMap,
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
				case "q":
			return t, tea.Quit
		case "?":
			t.showHelp = !t.showHelp
		case "up", "k":
			if t.cursor > 0 {
				t.cursor--
			}
		case "down", "j":
			if t.cursor < len(t.items)-1 {
				t.cursor++
			}
		case "right", "l":
			n := t.indexToNode[t.cursor]
			if n != nil && n.isDir && !n.expanded {
				n.expanded = true
				current := n
				t.rebuildVisible()
				// keep cursor on same node
				for i, nd := range t.indexToNode {
					if nd == current {
						t.cursor = i
						break
					}
				}
			}
		case "left":
			n := t.indexToNode[t.cursor]
			if n != nil {
				if n.isDir && n.expanded {
					n.expanded = false
					current := n
					t.rebuildVisible()
					for i, nd := range t.indexToNode {
						if nd == current {
							t.cursor = i
							break
						}
					}
				} else if n.parent != nil {
					parent := n.parent
					parent.expanded = false
					t.rebuildVisible()
					for i, nd := range t.indexToNode {
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

			// Determine the new state: if currently full or partial, deselect; otherwise, select.
			newState := full
			if n.state == full || n.state == partial {
				newState = none
			}

			t.toggleSelection(n, newState)
			t.updateStats()
		case "a":
			// Determine if we should select all or deselect all
			selectAll := false
			for _, n := range t.visible {
				if n.state != none {
					selectAll = true // If any node is selected, we'll deselect all
					break
				}
			}

			newState := full
			if selectAll {
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
  a: Toggle selection of all visible items

Actions:
  g: Generate digest and quit
  p: Preview digest
  o: Output digest to file (prompts for filename)
  i: Toggle ignoring .gitignore files

Other:
  q/esc: Quit
  ?/h: Toggle help (this screen)
`
	}


	s := fmt.Sprintf("Current path: %s\n\n", t.path)
	for i, item := range t.items {
		cursor := " "
		if t.cursor == i {
			cursor = ">"
		}

		checked := " "
		if n := t.indexToNode[i]; n != nil {
			switch n.state {
			case full:
				checked = "x"
			case partial:
				checked = "-"
			case none:
				checked = " "
			}
		}

		s += fmt.Sprintf("%s [%s] %s\n", cursor, checked, item)
	}
	s += fmt.Sprintf("\n[i]gnore .gitignore: %v\n", t.ignoreGitignore)
	s += fmt.Sprintf("Selected: %d files, %d folders | Size: %s | Tokens: %d\n", t.selectedFiles, t.selectedDirs, formatBytes(t.totalSize), t.totalTokens)
	s += fmt.Sprintf("\n[g]enerate | [p]review | [o]utput file: %s | [q]uit | [?] help\n", t.outputFile)
	return s
}

func (t *tree) generateOutput() {
	var output string
	for i := range t.selected {
		n := t.indexToNode[i]
		if n == nil {
			continue
		}
		path := n.path
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

	// Iterate through all visible nodes, not just selected map
	for _, n := range t.visible {
		if n.state == full || n.state == partial {
			if n.isDir {
				selectedDirs++
				// For directories, we need to walk its children to count files and size/tokens
				filepath.Walk(n.path, func(p string, info os.FileInfo, err error) error {
					if err != nil {
						return nil // Skip errors for now
					}
					if !info.IsDir() {
						// Check if this file is actually selected (if its parent is partial)
						// This is a simplification; a more robust solution would track individual file selections
						// For now, if a directory is full or partial, we count all its files.
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
				info, err := os.Stat(n.path)
				if err != nil {
					continue
				}
				totalSize += info.Size()
				content, err := ioutil.ReadFile(n.path)
				if err == nil {
					totalTokens += len(tkn.Encode(string(content), nil, nil))
				}
			}
		}
	}

	t.selectedFiles = selectedFiles
	t.selectedDirs = selectedDirs
	t.totalSize = totalSize
	t.totalTokens = totalTokens
}

// rebuildVisible regenerates the visible slice, items list, and index map
// after expanding/collapsing directories.
func (t *tree) rebuildVisible() {
	t.visible = flattenVisible(t.root)
	t.items = make([]string, len(t.visible))
	t.indexToNode = make(map[int]*node, len(t.visible))
	for i, n := range t.visible {
		indent := strings.Repeat("  ", n.depth)
		prefix := "  "
		if n.isDir {
			if n.expanded {
				prefix = "▾ "
			} else {
				prefix = "▸ "
			}
		}
		t.items[i] = indent + prefix + n.name
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

	// Recursively apply to children
	if n.isDir {
		for _, child := range n.children {
			t.toggleSelection(child, state)
		}
	}

	// Update parent's state
	if n.parent != nil {
		t.updateParentSelection(n.parent)
	}
}

func (t *tree) updateParentSelection(n *node) {
	if !n.isDir {
		return // Only directories have selection states based on children
	}

	allFull := true
	allNone := true
	for _, child := range n.children {
		if child.state != full {
			allFull = false
		}
		if child.state != none {
			allNone = false
		}
	}

	if allFull {
		n.state = full
	} else if allNone {
		n.state = none
	} else {
		n.state = partial
	}

	// Propagate up the tree
	if n.parent != nil {
		t.updateParentSelection(n.parent)
	}
}
