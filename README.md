# Text Ingests

A command-line tool for interactively selecting files and directories from a tree view, aggregating their content, and copying it to the clipboard. It's designed to help developers quickly gather context from multiple files for use with LLMs or other tools.

![Screenshot](https://github.com/BigDog1400/text-ingests/blob/master/screenshot.png) <!-- Replace with an actual screenshot URL -->

## Features

- **Interactive Tree View:** Navigate your file system with a clean, interactive tree UI.
- **Color-Coded and Iconic:** Styled with `lipgloss` for a modern look, with icons for directories and different file types.
- **Recursive Selection:** Select or deselect individual files or entire directories with a single key press.
- **Gitignore Aware:** Automatically respects `.gitignore` rules to hide irrelevant files.
- **Content Aggregation:** Concatenates the content of all selected files.
- **Clipboard Integration:** Copies the aggregated content directly to your clipboard.
- **Token Counting:** Provides an estimated token count for the selected content (using `tiktoken-go`).
- **Live Preview:** Preview the generated output before copying.

## Installation

### One-Liner (macOS & Linux)

This is the simplest way to install. Open your terminal and run the following command:

```bash
curl -sfL https://raw.githubusercontent.com/BigDog1400/text-ingests/master/install.sh | sh
```

This script will automatically download the correct binary for your system and move it to `/usr/local/bin` so you can run it from anywhere.

### From GitHub Releases (Manual)

### Using Pre-compiled Binaries (Recommended)

This is the easiest method and does not require you to have Go installed.

1.  Go to the [**Releases**](https://github.com/BigDog1400/text-ingests/releases) page of this repository.
2.  Find the latest release and download the archive that matches your operating system and architecture (e.g., `text-ingests_1.0.0_darwin_arm64.tar.gz` for an Apple Silicon Mac).
3.  Extract the archive. You will find the `text-ingests` executable inside.
4.  Move the `text-ingests` executable to a directory in your system's `PATH`. For example:

    -   **macOS / Linux:**
        ```bash
        sudo mv ./text-ingests /usr/local/bin/
        ```
    -   **Windows:** Create a folder (e.g., `C:\Program Files\text-ingests`) and add it to your system's `PATH` environment variable.

Now you can run `text-ingests` from anywhere in your terminal.

### From Source

If you have Go installed, you can build from source using the following command:

```bash
go install github.com/BigDog1400/text-ingests@latest
```

This will compile the source code and install the `text-ingests` binary in your Go bin directory.

## Usage

Run the tool from your terminal:

```bash
text-ingests
```

By default, it will open the tree view in the current directory. You can also specify a path:

```bash
text-ingests /path/to/your/project
```

### Keybindings

| Key(s)        | Action                                      |
|---------------|---------------------------------------------|
| `up`/`k`      | Move cursor up                              |
| `down`/`j`    | Move cursor down                            |
| `left`/`h`    | Collapse directory or move to parent        |
| `right`/`l`   | Expand directory                            |
| `space`       | Toggle selection for the current item       |
| `a`           | Toggle selection for all visible items      |
| `g`           | Generate output and copy to clipboard       |
| `p`           | Preview the generated output                |
| `c`           | Copy the last generated output to clipboard |
| `i`           | Toggle whether `.gitignore` is respected    |
| `?`           | Toggle help view in the status bar          |
| `q` / `ctrl+c`| Quit the application                        |

## Development

To run the project locally:

1.  Clone the repository:
    ```bash
    git clone https://github.com/BigDog1400/text-ingests.git
    cd text-ingests
    ```
2.  Run the application:
    ```bash
    go run .
    ```
