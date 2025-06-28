package main

import "os"

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
