package main

import "os"

// selState represents selection status for a node.
type selState int

const (
    none selState = iota
    full
    partial
)

// node represents a file or directory in the tree.
// renamed to avoid duplicate definition with tree.go
 type unusedNode struct {
    name     string
    fullPath string
    isDir    bool
    depth    int
    expanded bool
    state    selState
    parent   *node
    children []*node
    info     os.FileInfo
}
