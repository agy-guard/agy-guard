## What's Changed in v0.3.0

### 🛡️ Dual-Gate Antigravity Sync & Permission Overrides
* **`permissionOverrides` Support:** PreToolUse hook now emits temporary permission grants (`permissionOverrides: ["command(...)"]`) on every `allow` decision, eliminating Antigravity CLI's native confirmation dialogs.
* **Automatic `settings.json` Pre-Authorization:** Implemented `EnsureSettingsPermissions` in `internal/config/settings.go`. Automatically seeds `~/.gemini/antigravity-cli/settings.json` with standard developer commands (`go test`, `go build`, `git status`, `git add`, `git commit`, `ls`, `grep`, `sed`, `python3`, etc.) while preserving user preferences, statusline, and workspace settings.

### 📁 Universal Fast File Filter (0 ms)
* **Read-Only vs Mutating Tool Isolation:** Distinguished read tools (`read_file`, `view_file`) from modifying tools (`write_file`, `edit_file`). Reading non-sensitive files (documentation, system configs, external libraries) is allowed from anywhere in 0–3 ms without triggering workspace boundary violations.
* **Workspace Boundary Enforcement:** File modifications (`write_file`, `edit_file`) outside project workspaces require confirmation, while sensitive files (`.env`, `hooks.json`, `.ssh/id_rsa`) remain strictly guarded.

### ⚡ Tier 2 Direct Intent Matching & Offline Resilience
* **Zero-Network Intent Override:** Direct chat intent matching (`matchDirectUserIntent`) in Tier 2 auto-authorizes safe operations (like `git push` or `git tag` without destructive `--force` flags) when explicitly requested in chat, completely offline in 0 ms.
* **Multi-line Script Parsing:** Enhanced `splitCompoundCommand` to permit newlines within quotes, allowing multi-line Python and script one-liners to execute in Tier 1.
* **Automatic Defaults Merging:** Implemented `MergeDefaults()` in `internal/config/config.go` so existing installations automatically inherit new safe prefixes upon binary update without manual config editing.

### 🛠️ CLI & Hygiene
* **`agy-guard reset` / `agy-guard reset-state`:** Added native CLI subcommand to safely purge session state and Loop Breaker strikes without triggering Antigravity's internal `PolicyGuardian`.
* **Full Linter & Race Compliance:** Clean `golangci-lint run` (0 issues) and `go test -race ./...` across all 6 packages.
* **Streamlined Documentation:** Compact horizontal Mermaid pipeline and clean 3-tier reference table in `README.md`.

**Full Changelog**: https://github.com/agy-guard/agy-guard/compare/v0.2.0...v0.3.0
