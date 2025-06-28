package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// No replacement, just deleting the old model

func main() {
	initialTree, err := newTree(".", false)
	if err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
	p := tea.NewProgram(initialTree)
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
