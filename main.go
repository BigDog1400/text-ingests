package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// No replacement, just deleting the old model

func main() {
	// Start with a directory input prompt instead of assuming ".".
	rootModel := newPathPrompt()
	p := tea.NewProgram(rootModel)
	m, err := p.Run()
	if err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}

	if t, ok := m.(*tree); ok {
		if t.outputFile != "" {
			f, err := os.Create(t.outputFile)
			if err != nil {
				fmt.Printf("Alas, there's been an error: %v", err)
				os.Exit(1)
			}
			defer f.Close()
			f.WriteString(t.output)
		} else {
			fmt.Println(t.output)
		}
	}
}
