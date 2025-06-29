#!/bin/sh
# A script to install the latest version of text-ingests.
# This script is intended to be run via curl:
#   curl -sfL https://raw.githubusercontent.com/your-username/text-ingests/main/install.sh | sh

set -e

# The GitHub repository to fetch from.
REPO="your-username/text-ingests"

# The name of the binary.
BINARY="text-ingests"

# The directory to install the binary to.
INSTALL_DIR="/usr/local/bin"

# Function to determine the operating system.
get_os() {
  case "$(uname -s)" in
    Darwin)
      echo "darwin"
      ;;
    Linux)
      echo "linux"
      ;;
    *)
      echo "Unsupported OS: $(uname -s)" >&2
      exit 1
      ;;
  esac
}

# Function to determine the architecture.
get_arch() {
  case "$(uname -m)" in
    x86_64 | amd64)
      echo "amd64"
      ;;
    arm64 | aarch64)
      echo "arm64"
      ;;
    *)
      echo "Unsupported architecture: $(uname -m)" >&2
      exit 1
      ;;
  esac
}

# Fetch the latest release version from GitHub.
get_latest_release() {
  curl --silent "https://api.github.com/repos/$REPO/releases/latest" | # Get latest release from GitHub API
    grep '"tag_name":' |                                            # Get tag line
    sed -E 's/.*"([^\"]+)\".*/\1/'                                    # Pluck JSON value
}

main() {
  OS=$(get_os)
  ARCH=$(get_arch)
  LATEST_TAG=$(get_latest_release)

  if [ -z "$LATEST_TAG" ]; then
    echo "Error: Could not fetch the latest release tag from GitHub." >&2
    exit 1
  fi

  # Construct the download URL.
  VERSION=$(echo "$LATEST_TAG" | sed 's/v//')
  FILENAME="${BINARY}_${VERSION}_${OS}_${ARCH}.tar.gz"
  DOWNLOAD_URL="https://github.com/$REPO/releases/download/$LATEST_TAG/$FILENAME"

  # Download and extract the binary.
  echo "Downloading $BINARY from $DOWNLOAD_URL..."
  curl -L "$DOWNLOAD_URL" | tar -xz -C /tmp "$BINARY"

  # Install the binary.
  echo "Installing $BINARY to $INSTALL_DIR..."
  if [ -w "$INSTALL_DIR" ]; then
    mv "/tmp/$BINARY" "$INSTALL_DIR/$BINARY"
  else
    sudo mv "/tmp/$BINARY" "$INSTALL_DIR/$BINARY"
  fi

  echo "\n$BINARY was installed successfully to $INSTALL_DIR/$BINARY"
  echo "You can now run '$BINARY' from your terminal."
}

main
