#!/usr/bin/env bash
# ==============================================================================
# agy-guard: Plugin Uninstaller & Cleanup Script
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
echo -e "${BOLD}🛡️  agy-guard Uninstaller${RESET}"
echo -e "------------------------------------------------------------------"

CONFIG_DIR="$HOME/.agy-guard"
PLUGIN_DIR="$HOME/.gemini/config/plugins/agy-guard"
HOOKS_FILE="$HOME/.gemini/config/hooks.json"
LOCAL_BIN="$HOME/.local/bin/agy-guard"

PURGE=false
YES_ALL=false

for arg in "$@"; do
    case "$arg" in
        --purge|-p) PURGE=true ;;
        --yes|-y)   YES_ALL=true ;;
        --help|-h)
            echo "Usage: ./uninstall.sh [--purge] [--yes]"
            echo "  --purge, -p   Also remove ~/.agy-guard (config, audit logs, cache)"
            echo "  --yes, -y     Non-interactive mode (auto-confirm)"
            exit 0
            ;;
    esac
done

prompt_yes_no() {
    local question="$1"
    local default_ans="$2"
    if [ "$YES_ALL" = true ]; then
        echo "$default_ans"
        return
    fi
    local ans=""
    if [ -t 0 ]; then
        read -rp "$question" ans || true
    elif [ -e /dev/tty ]; then
        read -rp "$question" ans </dev/tty || true
    fi
    ans="${ans:-$default_ans}"
    case "$ans" in
        [Yy]* ) echo "y" ;;
        * )     echo "n" ;;
    esac
}

echo -e "\n${BOLD}[1/4] Unregistering hooks from Antigravity CLI...${RESET}"
# 1. Unpatch hooks.json
if command -v agy-guard >/dev/null 2>&1; then
    agy-guard uninstall >/dev/null 2>&1 || true
fi

if [ -f "$HOOKS_FILE" ]; then
    if command -v python3 >/dev/null 2>&1; then
        python3 -c "
import json
p = '$HOOKS_FILE'
try:
    with open(p, 'r') as f:
        d = json.load(f)
    if 'agy-guard' in d:
        del d['agy-guard']
        with open(p, 'w') as f:
            json.dump(d, f, indent=2)
except Exception:
    pass
" 2>/dev/null || true
    fi
    echo -e "${GREEN}✓ Cleaned agy-guard from: $HOOKS_FILE${RESET}"
else
    echo "• hooks.json not found, skipping."
fi

# 2. Remove plugin directory
echo -e "\n${BOLD}[2/4] Removing native plugin directory...${RESET}"
if [ -d "$PLUGIN_DIR" ]; then
    rm -rf "$PLUGIN_DIR"
    echo -e "${GREEN}✓ Removed: $PLUGIN_DIR${RESET}"
else
    echo "• Plugin directory not found, skipping."
fi

# 3. Remove binary
echo -e "\n${BOLD}[3/4] Removing executable binary...${RESET}"
REMOVED_BIN=false
if [ -f "$LOCAL_BIN" ]; then
    rm -f "$LOCAL_BIN"
    echo -e "${GREEN}✓ Removed: $LOCAL_BIN${RESET}"
    REMOVED_BIN=true
fi

# Also check go bin
GOBIN="$(go env GOPATH 2>/dev/null || echo "$HOME/go")/bin/agy-guard"
if [ -f "$GOBIN" ]; then
    rm -f "$GOBIN"
    echo -e "${GREEN}✓ Removed: $GOBIN${RESET}"
    REMOVED_BIN=true
fi

if [ "$REMOVED_BIN" = false ]; then
    echo "• No installed binary found in standard paths."
fi

# 4. Remove config / audit logs if requested
echo -e "\n${BOLD}[4/4] Configuration & Audit Logs cleanup...${RESET}"
if [ -d "$CONFIG_DIR" ]; then
    DO_PURGE="$PURGE"
    if [ "$DO_PURGE" = false ] && [ "$YES_ALL" = false ]; then
        CHOICE=$(prompt_yes_no "Do you want to delete ~/.agy-guard (config.json, audit logs, and cache)? [y/N]: " "n")
        if [ "$CHOICE" = "y" ]; then
            DO_PURGE=true
        fi
    fi

    if [ "$DO_PURGE" = true ]; then
        rm -rf "$CONFIG_DIR"
        echo -e "${GREEN}✓ Removed configuration and audit directory: $CONFIG_DIR${RESET}"
    else
        echo -e "${YELLOW}• Kept configuration and logs in: $CONFIG_DIR${RESET}"
    fi
else
    echo "• ~/.agy-guard directory does not exist."
fi

echo -e "\n${GREEN}${BOLD}==================================================================${RESET}"
echo -e "${GREEN}${BOLD}✓ agy-guard uninstalled successfully!${RESET}"
echo -e "${GREEN}${BOLD}==================================================================${RESET}"
echo "Antigravity CLI ('agy') has been restored to default behavior."
echo ""
