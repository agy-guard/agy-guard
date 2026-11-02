# agy-guard 🛡️

**Fast, zero-dependency command security guard for [Antigravity CLI (`agy`)](https://github.com/google-deepmind).**

`agy-guard` intercepts `PreToolUse` hooks in Antigravity CLI to prevent AI agents from accidentally executing destructive commands, leaking credentials, or running unauthorized scripts.

---

## ⚡ What it Does

When Antigravity CLI (`agy`) prepares to run a terminal command, `agy-guard` intercepts it:

* ⚡ **Instant Pass (<1 ms):** Safe developer commands (`git status`, `go test`, `npm test`, `cargo build`, `ls`, `grep`) and safe compound chains (`git status && ls`) pass instantly with zero network calls.
* 🚫 **Instant Block:** Dangerous patterns (`rm -rf /`, `git push -f`, `git reset --hard`, `sudo`, `.env` access, token leaks) are blocked immediately.
* 🤖 **Smart LLM Check (~500 ms):** Ambiguous or unfamiliar commands are evaluated by a fast, lightweight LLM (**Gemini 3.5 Flash Lite** or local **Ollama**).
* 💾 **Smart Cache:** Repeated commands during build/test loops are cached for 10 minutes so they don't trigger extra LLM calls.
* 🛡️ **Fail-Safe Guarantee:** `agy-guard` **never crashes your CLI session**. If an API drops or times out, it safely falls back to asking you for confirmation (`"ask"`) instead of failing.

---

## 🚀 Quick Start

### 1-Line Interactive Installer

Run this in your terminal to automatically download, install the latest binary, configure your AI provider (Gemini or Ollama), and register the Antigravity plugin:

**macOS & Linux:**
```bash
curl -fsSL https://raw.githubusercontent.com/agy-guard/agy-guard/main/install.sh | bash
```

**Windows (PowerShell):**
```powershell
irm https://raw.githubusercontent.com/agy-guard/agy-guard/main/install.ps1 | iex
```

*(Or if you have cloned the repository locally, simply execute `./install.sh` or `.\install.ps1`)*

---

## 💡 Gemini Models & Quotas (Good to Know)

### Why we use `gemini-3.5-flash-lite` by default
* **1,500 requests/day** on Google AI Studio Free Tier (plenty for daily development).
* **~500–600 ms latency** (fast enough to not slow down the agent).
* **Accurate security reasoning**: easily distinguishes between harmless scripts and risky commands.

> [!WARNING]
> **Avoid `gemini-3.8-flash` on the Free Tier:**
> Google restricts `gemini-3.8-flash` to only **20 requests per day** on the Free Tier, and its servers frequently hit `503 Service Unavailable` spikes during peak hours.

### What about a Google AI Pro / Google One subscription?
Google does not issue Developer API keys from consumer subscriptions (Google One AI Premium / Gemini Advanced). Those subscriptions only apply to Google's consumer products (web chat, Docs, and Antigravity CLI's own OAuth login).

For `agy-guard`, simply grab a free Developer API key at [aistudio.google.com](https://aistudio.google.com/) or switch to **local Ollama** (100% free, private, and offline).

---

## 🔍 Testing Any Command (`dry-run`)

You can test how `agy-guard` evaluates any shell command directly from your terminal:

```bash
# Safe command (passes instantly)
agy-guard dry-run "git status && ls -la"

# Dangerous command (blocked immediately)
agy-guard dry-run "rm -rf /tmp/data"

# Ambiguous command (evaluated by LLM)
agy-guard dry-run "python3 -c 'import sys; print(sys.version)'"
```

Example output:
```text
[dry-run] Command: python3 -c 'import sys; print(sys.version)'
[dry-run] Matched Tier: Tier 3 (LLM - gemini)
[dry-run] Latency: 619ms
[dry-run] Decision: allow
[dry-run] Reason: Reads the Python version which is a harmless, read-only inspection command.
```

---

## ⚙️ Configuration

Config path: `~/.agy-guard/config.json`

```json
{
  "provider": "gemini",
  "model": "gemini-3.5-flash-lite",
  "api_key": "YOUR_GEMINI_API_KEY",
  "ollama_url": "http://localhost:11434",
  "timeout_sec": 8,
  "enable_cache": true,
  "enable_audit_log": true,
  "static_rules": {
    "allow_prefixes": [
      "go test", "go build", "git status", "git diff", "ls", "grep"
    ],
    "deny_substrings": [
      ".env", "id_rsa", "rm -rf /", "git push -f", "sudo"
    ]
  }
}
```

### Using Local Ollama (100% Offline)
Set `"provider": "ollama"` and `"model": "llama3.2"` (or any local model). No API key needed.

---

## 📝 Audit Logs

Every evaluated command is recorded to `~/.agy-guard/audit/<session_id>.jsonl` with timestamps, latency, and the rationale for the verdict.

To inspect live decisions:
```bash
tail -f ~/.agy-guard/audit/*.jsonl | jq .
```

---

## 🧪 Development & Tests

```bash
# Run test suite
go test -v -race ./...

# Vet code
go vet ./...
```

---

## 📄 License

MIT License.
