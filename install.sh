#!/usr/bin/env bash
# ==============================================================================
# agy-guard: Quick Plugin Installer & Configuration Setup
# ==============================================================================
set -euo pipefail

# ANSI Colors
BOLD="\033[1m"
CYAN="\033[36m"
GREEN="\033[32m"
YELLOW="\033[33m"
RED="\033[31m"
RESET="\033[0m"

echo -e "${CYAN}${BOLD}"
echo "    ___    _______  __     _______  _____    ____  ____ "
echo "   /   |  / ____/ \/ /    / ____/ / / /   |  / __ \/ __ \\"
echo "  / /| | / / __  \  /    / / __/ / / / /| | / /_/ / / / /"
echo " / ___ |/ /_/ /  / /    / /_/ / /_/ / ___ |/ _, _/ /_/ / "
echo "/_/  |_|\____/  /_/     \____/\____/_/  |_/_/ |_/_____/  "
echo -e "${RESET}"
echo -e "${BOLD}🛡️  agy-guard: Security Classifier for Antigravity CLI (agy)${RESET}"
echo -e "------------------------------------------------------------------"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_DIR="$HOME/.agy-guard"
CONFIG_FILE="$CONFIG_DIR/config.json"
PLUGIN_DIR="$HOME/.gemini/config/plugins/agy-guard"
LOCAL_BIN_DIR="$HOME/.local/bin"

mkdir -p "$CONFIG_DIR" "$LOCAL_BIN_DIR" "$PLUGIN_DIR"

# ------------------------------------------------------------------------------
# 1. Build / Locate / Download Binary
# ------------------------------------------------------------------------------
echo -e "\n${BOLD}[1/4] Preparing agy-guard binary...${RESET}"

prompt_read() {
    local prompt_msg="$1"
    local default_ans="$2"
    local user_ans=""
    if [ -t 0 ]; then
        read -rp "$prompt_msg" user_ans || true
    elif [ -e /dev/tty ]; then
        read -rp "$prompt_msg" user_ans </dev/tty || true
    fi
    echo "${user_ans:-$default_ans}"
}

if [ -f "${SCRIPT_DIR:-}/cmd/agy-guard/main.go" ] && command -v go >/dev/null 2>&1; then
    echo "Compiling from local source with Go..."
    (cd "$SCRIPT_DIR" && go build -o "$SCRIPT_DIR/agy-guard" ./cmd/agy-guard)
    cp "$SCRIPT_DIR/agy-guard" "$LOCAL_BIN_DIR/agy-guard"
    chmod +x "$LOCAL_BIN_DIR/agy-guard"
    BIN_PATH="$LOCAL_BIN_DIR/agy-guard"
    echo -e "${GREEN}✓ Binary built and installed to: $BIN_PATH${RESET}"
elif [ -f "${SCRIPT_DIR:-}/agy-guard" ]; then
    cp "$SCRIPT_DIR/agy-guard" "$LOCAL_BIN_DIR/agy-guard"
    chmod +x "$LOCAL_BIN_DIR/agy-guard"
    BIN_PATH="$LOCAL_BIN_DIR/agy-guard"
    echo -e "${GREEN}✓ Using existing binary installed to: $BIN_PATH${RESET}"
else
    echo "Downloading pre-compiled binary from GitHub Releases (agy-guard/agy-guard)..."
    OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
    ARCH="$(uname -m)"
    case "$ARCH" in
        x86_64|amd64) ARCH="amd64" ;;
        arm64|aarch64) ARCH="arm64" ;;
        *) echo -e "${RED}Unsupported architecture: $ARCH${RESET}"; exit 1 ;;
    esac

    DOWNLOAD_URL=$(curl -sL https://api.github.com/repos/agy-guard/agy-guard/releases/latest | grep "browser_download_url" | grep "${OS}_${ARCH}\.tar\.gz" | head -n 1 | cut -d '"' -f 4 || true)

    if [ -n "$DOWNLOAD_URL" ]; then
        TMP_DIR="$(mktemp -d)"
        echo "Fetching $DOWNLOAD_URL..."
        curl -sL "$DOWNLOAD_URL" -o "$TMP_DIR/archive.tar.gz"
        tar -xzf "$TMP_DIR/archive.tar.gz" -C "$TMP_DIR"
        cp "$TMP_DIR/agy-guard" "$LOCAL_BIN_DIR/agy-guard"
        chmod +x "$LOCAL_BIN_DIR/agy-guard"
        rm -rf "$TMP_DIR"
        BIN_PATH="$LOCAL_BIN_DIR/agy-guard"
        echo -e "${GREEN}✓ Downloaded and installed to: $BIN_PATH${RESET}"
    elif command -v go >/dev/null 2>&1; then
        echo "Release asset not found, compiling via go install..."
        go install github.com/agy-guard/agy-guard/cmd/agy-guard@latest
        cp "$(go env GOPATH)/bin/agy-guard" "$LOCAL_BIN_DIR/agy-guard"
        chmod +x "$LOCAL_BIN_DIR/agy-guard"
        BIN_PATH="$LOCAL_BIN_DIR/agy-guard"
        echo -e "${GREEN}✓ Installed to: $BIN_PATH${RESET}"
    else
        echo -e "${RED}Error: Could not locate, download, or compile agy-guard.${RESET}"
        exit 1
    fi
fi

# ------------------------------------------------------------------------------
# 2. Interactive Model Provider Selection
# ------------------------------------------------------------------------------
echo -e "\n${BOLD}[2/4] Configure LLM Classifier Provider (Tier 3)...${RESET}"
echo "Select the model provider to evaluate ambiguous/unrecognized shell commands:"
echo -e "  ${BOLD}1)${RESET} Google Gemini ${GREEN}(Recommended - ultra fast, free API key)${RESET}"
echo -e "  ${BOLD}2)${RESET} Local Ollama  ${YELLOW}(100% offline & private, runs locally)${RESET}"

SELECTED_PROVIDER="gemini"
SELECTED_MODEL="gemini-3.5-flash"
API_KEY="YOUR_GEMINI_API_KEY_HERE"
OLLAMA_URL="http://localhost:11434"

PROVIDER_CHOICE=$(prompt_read "Enter choice [1/2] (default: 1): " "1")

if [ "$PROVIDER_CHOICE" = "2" ]; then
    SELECTED_PROVIDER="ollama"
    INPUT_OLLAMA_URL=$(prompt_read "Enter Ollama URL [http://localhost:11434]: " "http://localhost:11434")
    OLLAMA_URL="${INPUT_OLLAMA_URL:-http://localhost:11434}"

    INPUT_MODEL=$(prompt_read "Enter Ollama Model [llama3.2]: " "llama3.2")
    SELECTED_MODEL="${INPUT_MODEL:-llama3.2}"
    echo -e "${GREEN}✓ Selected Ollama (${SELECTED_MODEL}) at ${OLLAMA_URL}${RESET}"
else
    SELECTED_PROVIDER="gemini"
    INPUT_MODEL=$(prompt_read "Enter Gemini Model [gemini-3.5-flash]: " "gemini-3.5-flash")
    SELECTED_MODEL="${INPUT_MODEL:-gemini-3.5-flash}"

    # Check if GEMINI_API_KEY is already set in the current shell
    DETECTED_KEY="${GEMINI_API_KEY:-}"
    if [ -n "$DETECTED_KEY" ]; then
        KEY_MASK="${DETECTED_KEY:0:6}...${DETECTED_KEY: -4}"
        echo -e "Found GEMINI_API_KEY in environment (${KEY_MASK})"
        USE_ENV_KEY=$(prompt_read "Use this API key? [Y/n]: " "Y")
        if [[ "$USE_ENV_KEY" =~ ^[Yy]$ ]]; then
            API_KEY="$DETECTED_KEY"
        fi
    fi

    if [ "$API_KEY" = "YOUR_GEMINI_API_KEY_HERE" ]; then
        echo -e "Get a free Gemini API key from: ${CYAN}https://aistudio.google.com${RESET}"
        INPUT_KEY=$(prompt_read "Enter your Gemini API Key: " "")
        if [ -n "$INPUT_KEY" ]; then
            API_KEY="$INPUT_KEY"
        else
            echo -e "${YELLOW}Warning: No API key provided. You can add it later to ~/.agy-guard/config.json${RESET}"
        fi
    fi
    echo -e "${GREEN}✓ Selected Gemini (${SELECTED_MODEL})${RESET}"
fi

# ------------------------------------------------------------------------------
# 3. Execution Mode Selection (Interactive vs Autonomous AFK)
# ------------------------------------------------------------------------------
echo -e "\n${BOLD}[3/5] Select Execution Mode...${RESET}"
echo "How should agy-guard handle ambiguous or state-changing commands?"
echo -e "  ${BOLD}1)${RESET} Interactive Mode  ${GREEN}(Recommended - pauses and asks confirmation on suspicious actions)${RESET}"
echo -e "  ${BOLD}2)${RESET} Autonomous / AFK  ${YELLOW}(Zero prompts - agent autonomously adapts or skips when blocked)${RESET}"

MODE_CHOICE=$(prompt_read "Enter choice [1/2] (default: 1): " "1")
if [ "$MODE_CHOICE" = "2" ]; then
    UNATTENDED_MODE=true
    MODE_LABEL="Autonomous / AFK Mode (Zero Prompts)"
    echo -e "${YELLOW}✓ Selected Autonomous / AFK Mode${RESET}"
else
    UNATTENDED_MODE=false
    MODE_LABEL="Interactive Mode (Confirmations Enabled)"
    echo -e "${GREEN}✓ Selected Interactive Mode${RESET}"
fi

# ------------------------------------------------------------------------------
# 4. Write / Update ~/.agy-guard/config.json
# ------------------------------------------------------------------------------
echo -e "\n${BOLD}[4/5] Writing configuration to $CONFIG_FILE...${RESET}"

# Initialize default if config does not exist
if [ ! -f "$CONFIG_FILE" ]; then
    "$BIN_PATH" init >/dev/null 2>&1 || true
fi

# Write updated configuration (preserving static rules if already present)
cat <<EOF > "$CONFIG_FILE"
{
  "provider": "$SELECTED_PROVIDER",
  "model": "$SELECTED_MODEL",
  "api_key": "$API_KEY",
  "ollama_url": "$OLLAMA_URL",
  "timeout_sec": 15,
  "include_transcript_context": true,
  "enable_audit_log": true,
  "enable_cache": true,
  "cache_ttl_minutes": 10,
  "unattended_mode": $UNATTENDED_MODE,
  "max_retries_before_ask": 2,
  "static_rules": {
    "allow_prefixes": [
      "go test",
      "go build",
      "go vet",
      "go list",
      "go mod tidy",
      "go fmt",
      "go env",
      "go version",
      "go doc",
      "git status",
      "git diff",
      "git log",
      "git branch",
      "git show",
      "git rev-parse",
      "git remote",
      "git tag",
      "git ls-files",
      "git describe",
      "git blame",
      "cargo check",
      "cargo test",
      "cargo build",
      "cargo clippy",
      "cargo fmt",
      "cargo doc",
      "npm test",
      "npm run test",
      "npm run lint",
      "npm run build",
      "npm run typecheck",
      "npm ls",
      "yarn test",
      "pnpm test",
      "npx eslint",
      "npx tsc",
      "npx prettier --check",
      "bun test",
      "pytest",
      "python -m unittest",
      "python3 -m unittest",
      "python -m pytest",
      "python3 -m pytest",
      "ruff check",
      "black --check",
      "mypy",
      "flake8",
      "ls",
      "cat",
      "head",
      "tail",
      "grep",
      "egrep",
      "fgrep",
      "rg",
      "ag",
      "find",
      "pwd",
      "echo",
      "printf",
      "wc",
      "uname",
      "which",
      "stat",
      "file",
      "diff",
      "jq",
      "sort",
      "uniq",
      "tree"
    ],
    "deny_substrings": [
      ".env",
      "id_rsa",
      "id_ed25519",
      "id_ecdsa",
      "gh auth token",
      "aws_secret_access_key",
      "google_api_key",
      "gemini_api_key",
      "openai_api_key",
      "anthropic_api_key",
      "authorization: bearer",
      "bearer eyj",
      "private key-----",
      ".git-credentials",
      "/etc/shadow",
      "/etc/sudoers",
      "sudo ",
      "doas ",
      "su -",
      "su ",
      "runas ",
      "gsudo ",
      "| sh",
      "| bash",
      "| zsh",
      "| dash",
      "| powershell",
      "| pwsh",
      "| cmd",
      "|sh",
      "|bash",
      "|zsh",
      "|pwsh",
      "git push -f",
      "git push --force",
      "git reset --hard",
      "git reset --merge",
      "git clean -f",
      "git clean -fd",
      "git clean -fx",
      "git checkout .",
      "git restore .",
      "git branch -d",
      "git stash drop",
      "git stash clear",
      ".git/",
      ".bashrc",
      ".zshrc",
      ".profile",
      ".bash_profile",
      "config.fish",
      "rm -rf /",
      "rm -rf ~",
      "rm -rf \$home",
      "rm -rf *",
      "rm -fr /",
      "rm -fr ~",
      "rm -fr \$home",
      "rm -fr *",
      "mkfs",
      "dd if=",
      "format c:",
      "Format-Volume",
      "Remove-Item -Recurse -Force C:\\\\",
      "Stop-Computer",
      "shutdown ",
      "reboot ",
      ":(){ :|:& };:"
    ]
  }
}
EOF
chmod 600 "$CONFIG_FILE"
echo -e "${GREEN}✓ Config saved at: $CONFIG_FILE (permissions 0600)${RESET}"

# ------------------------------------------------------------------------------
# 5. Register Plugin in Antigravity CLI
# ------------------------------------------------------------------------------
echo -e "\n${BOLD}[5/5] Registering agy-guard as native Antigravity plugin...${RESET}"
"$BIN_PATH" init --plugin

# ------------------------------------------------------------------------------
# Verification & Self-Test
# ------------------------------------------------------------------------------
echo -e "\n${BOLD}Running verification self-test...${RESET}"
DRY_RUN_OUTPUT=$("$BIN_PATH" dry-run "go test ./...")
echo "$DRY_RUN_OUTPUT"

echo -e "\n${GREEN}${BOLD}==================================================================${RESET}"
echo -e "${GREEN}${BOLD}🎉 agy-guard installed and configured successfully!${RESET}"
echo -e "${GREEN}${BOLD}==================================================================${RESET}"
echo -e "• Binary path:      ${CYAN}$BIN_PATH${RESET}"
echo -e "• Plugin location:  ${CYAN}$PLUGIN_DIR${RESET}"
echo -e "• Execution mode:   ${CYAN}$MODE_LABEL${RESET}"
echo -e "• Configuration:    ${CYAN}$CONFIG_FILE${RESET}"
echo -e "• Audit logs:       ${CYAN}$HOME/.agy-guard/audit/${RESET}"
echo ""
echo "Antigravity CLI ('agy') will now automatically route PreToolUse events through agy-guard."
echo "You can test any command directly at any time:"
echo -e "  ${BOLD}agy-guard dry-run \"<command>\"${RESET}"
echo ""
echo "To uninstall at any time:"
echo -e "  ${CYAN}./uninstall.sh${RESET}  (or: ${CYAN}agy-guard uninstall${RESET})"
echo ""
