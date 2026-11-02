package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agy-guard/agy-guard/internal/audit"
	"github.com/agy-guard/agy-guard/internal/classifier"
	"github.com/agy-guard/agy-guard/internal/config"
	"github.com/agy-guard/agy-guard/internal/hook"
	"github.com/agy-guard/agy-guard/internal/rules"
)

func main() {
	args := os.Args[1:]

	if len(args) == 0 {
		if isTerminal(os.Stdin) {
			printUsage()
			os.Exit(0)
		}
		runHookMode()
		return
	}

	switch args[0] {
	case "init":
		installAsPlugin := false
		if len(args) > 1 && (args[1] == "--plugin" || args[1] == "-p") {
			installAsPlugin = true
		}
		runInit(installAsPlugin)
	case "install-plugin":
		runInit(true)
	case "uninstall":
		purge := false
		for _, a := range args[1:] {
			if a == "--purge" || a == "-p" {
				purge = true
				break
			}
		}
		runUninstall(purge)
	case "dry-run":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: agy-guard dry-run \"<command>\"")
			os.Exit(1)
		}
		cmdToTest := strings.Join(args[1:], " ")
		runDryRun(cmdToTest)
	case "reset-state", "reset":
		runResetState()
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command %q\n\n", args[0])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`agy-guard: Security Classifier for Antigravity CLI (agy)

Usage:
  agy-guard                  Run in hook mode (reads PreToolUse JSON from stdin)
  agy-guard init             Initialize ~/.agy-guard/config.json and register hook in ~/.gemini/config/hooks.json
  agy-guard init --plugin    Install as a native Antigravity plugin in ~/.gemini/config/plugins/agy-guard/
  agy-guard install-plugin   Alias for 'agy-guard init --plugin'
  agy-guard uninstall        Unregister hook and remove native Antigravity plugin
  agy-guard uninstall --purge Also delete ~/.agy-guard (config, cache, audit logs)
  agy-guard dry-run "<cmd>"  Evaluate command without running agy and measure evaluation latency
  agy-guard reset            Reset session state and clear Loop Breaker strikes
  agy-guard help             Show this help message`)
}

func isTerminal(f *os.File) bool {
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

func runHookMode() {
	start := time.Now()

	payload, err := hook.ParseStdin(os.Stdin)
	if err != nil {
		hook.EmitAndExit(hook.OutputVerdict{
			Decision: hook.DecisionAsk,
			Reason:   fmt.Sprintf("agy-guard: failed to parse hook stdin: %v", err),
		})
	}

	cfg, err := config.Load()
	if err != nil {
		cfg = config.DefaultConfig()
	}

	sessionID := payload.SessionID
	if sessionID == "" {
		sessionID = payload.ConversationID
	}
	if sessionID == "" {
		sessionID = "default"
	}

	var breaker *classifier.LoopBreaker
	if statePath, err := classifier.DefaultSessionStatePath(); err == nil {
		breaker = classifier.NewLoopBreaker(statePath)
	}

	// Функция безопасного логирования аудита
	logAudit := func(tier, action string, verdict hook.OutputVerdict) {
		if !cfg.EnableAuditLog {
			return
		}
		cwd := payload.Cwd
		if cwd == "" && len(payload.WorkspacePaths) > 0 {
			cwd = payload.WorkspacePaths[0]
		}
		if cwd == "" {
			cwd, _ = os.Getwd()
		}
		_ = audit.Log(audit.Record{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			SessionID: sessionID,
			Command:   action,
			Cwd:       cwd,
			Tier:      tier,
			Decision:  string(verdict.Decision),
			Reason:    verdict.Reason,
			LatencyMS: time.Since(start).Milliseconds(),
		})
	}

	// Быстрый фильтр (0 мс, без LLM) для не-терминальных инструментов (edit_file, write_file, read_file, etc.)
	if verdict, tier, handled := evaluateNonCommandTool(payload, cfg, breaker); handled {
		action := payload.ExtractTargetFile()
		if action == "" {
			action = payload.ToolCall.Name
		}
		if verdict.Decision == hook.DecisionAllow {
			if action != "" {
				verdict.PermissionOverrides = []string{fmt.Sprintf("%s(%s)", payload.ToolCall.Name, action)}
			} else {
				verdict.PermissionOverrides = []string{payload.ToolCall.Name}
			}
		}
		logAudit(tier, action, verdict)
		hook.EmitAndExit(verdict)
	}

	cmd := payload.ExtractCommand()
	if strings.TrimSpace(cmd) == "" {
		hook.EmitAndExit(hook.OutputVerdict{
			Decision:            hook.DecisionAllow,
			Reason:              "No shell command present in tool arguments",
			PermissionOverrides: []string{payload.ToolCall.Name},
		})
	}

	// applyPolicy применяет стоп-директивы для агента, AFK-режим и защиту от бесконечных циклов
	evalPolicy := func(v hook.OutputVerdict) hook.OutputVerdict {
		policed := applyPolicy(v, cmd, sessionID, cfg, breaker)
		if policed.Decision == hook.DecisionAllow {
			policed.PermissionOverrides = []string{fmt.Sprintf("command(%s)", cmd)}
		}
		return policed
	}

	// Tier 1: Zero-latency static matcher
	verdict, matched := rules.EvaluateStatic(cmd, cfg.StaticRules)
	if matched {
		finalVerdict := evalPolicy(verdict)
		logAudit("tier1_static", cmd, finalVerdict)
		hook.EmitAndExit(finalVerdict)
	}

	// Tier 2: User Intent & Transcript Context
	userIntent := strings.TrimSpace(payload.LastUserInput)
	if userIntent == "" && payload.TranscriptPath != "" {
		rawTail := hook.ReadRawTailBytes(payload.TranscriptPath, hook.DefaultMaxTranscriptBytes)
		userIntent = hook.ExtractLatestUserRequest(rawTail)
	}

	if intentVerdict, matched := matchDirectUserIntent(cmd, userIntent); matched {
		finalVerdict := evalPolicy(intentVerdict)
		logAudit("tier2_intent", cmd, finalVerdict)
		hook.EmitAndExit(finalVerdict)
	}

	var transcript string
	if cfg.IncludeTranscriptContext && payload.TranscriptPath != "" {
		transcript = hook.ReadTranscriptTail(payload.TranscriptPath, hook.DefaultMaxTranscriptBytes)
	}

	// Кеш решений классификатора (учитывает и команду, и контекст диалога)
	var cacheInstance *classifier.Cache
	if cfg.EnableCache {
		if cachePath, err := classifier.DefaultCachePath(); err == nil {
			cacheInstance = classifier.NewCache(cachePath, time.Duration(cfg.CacheTTLMinutes)*time.Minute)
			if cachedVerdict, hit := cacheInstance.Get(cfg.Provider, cfg.Model, cmd, transcript); hit {
				if cachedVerdict.Decision == hook.DecisionAllow {
					cachedVerdict.PermissionOverrides = []string{fmt.Sprintf("command(%s)", cmd)}
					logAudit("cache", cmd, cachedVerdict)
					hook.EmitAndExit(cachedVerdict)
				}
			}
		}
	}

	// Tier 3: LLM Classifier
	c, err := classifier.NewClassifier(cfg)
	if err != nil {
		fallbackVerdict := evalPolicy(hook.OutputVerdict{
			Decision: hook.DecisionAsk,
			Reason:   fmt.Sprintf("agy-guard: classifier configuration error: %v", err),
		})
		logAudit("fallback", cmd, fallbackVerdict)
		hook.EmitAndExit(fallbackVerdict)
	}

	verdict, err = c.Evaluate(context.Background(), cmd, transcript)
	if err != nil {
		fallbackVerdict := evalPolicy(hook.OutputVerdict{
			Decision: hook.DecisionAsk,
			Reason:   fmt.Sprintf("agy-guard: classifier evaluation failed: %v", err),
		})
		logAudit("fallback", cmd, fallbackVerdict)
		hook.EmitAndExit(fallbackVerdict)
	}

	// Сохраняем в кеш только allow-вердикты с контекстом диалога
	if cacheInstance != nil && verdict.Decision == hook.DecisionAllow {
		_ = cacheInstance.Put(cfg.Provider, cfg.Model, cmd, verdict, transcript)
	}

	finalVerdict := evalPolicy(verdict)
	tierName := fmt.Sprintf("tier3_%s", cfg.Provider)
	logAudit(tierName, cmd, finalVerdict)
	hook.EmitAndExit(finalVerdict)
}

func runInit(asPlugin bool) {
	fmt.Println("Initializing agy-guard...")

	cfgPath, err := config.SaveDefault()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating config file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[OK] Configuration ready at: %s\n", cfgPath)

	execPath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving executable path: %v\n", err)
		os.Exit(1)
	}
	absExecPath, err := filepath.Abs(execPath)
	if err != nil {
		absExecPath = execPath
	}

	if asPlugin {
		pluginDir, err := config.InstallPlugin(absExecPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error installing plugin: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("[OK] Native Antigravity plugin installed at: %s\n", pluginDir)
		fmt.Printf("[OK] Hook command: %s\n", absExecPath)
	} else {
		hooksPath, err := config.PatchHooksJSON(absExecPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error registering hook in hooks.json: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("[OK] Hook registered at: %s\n", hooksPath)
		fmt.Printf("[OK] Hook command: %s\n", absExecPath)
	}

	if settingsPath, err := config.EnsureSettingsPermissions("", config.DefaultSettingsAllowList()); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to pre-authorize commands in settings.json: %v\n", err)
	} else if settingsPath != "" {
		fmt.Printf("[OK] Pre-authorized standard dev commands in: %s\n", settingsPath)
	}

	fmt.Println("\nSetup complete! Note:")
	fmt.Println("- To use Gemini, add your API key to ~/.agy-guard/config.json")
	fmt.Println("- To use local Ollama, set \"provider\": \"ollama\" in ~/.agy-guard/config.json")
}

func runUninstall(purge bool) {
	fmt.Println("Uninstalling agy-guard...")

	// 1. Remove from ~/.gemini/config/hooks.json
	hooksPath, err := config.UnpatchHooksJSON()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to unpatch hooks.json: %v\n", err)
	} else if hooksPath != "" {
		fmt.Printf("[OK] Removed hook from: %s\n", hooksPath)
	}

	// 2. Remove native plugin directory ~/.gemini/config/plugins/agy-guard
	pluginDir, err := config.RemovePlugin()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to remove plugin directory: %v\n", err)
	} else if pluginDir != "" {
		fmt.Printf("[OK] Removed plugin from: %s\n", pluginDir)
	}

	// 3. Purge config & audit logs if requested
	if purge {
		if home, err := os.UserHomeDir(); err == nil {
			configDir := filepath.Join(home, ".agy-guard")
			if err := os.RemoveAll(configDir); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to remove config dir %s: %v\n", configDir, err)
			} else {
				fmt.Printf("[OK] Purged configuration and logs: %s\n", configDir)
			}
		}
	} else {
		fmt.Println("[INFO] Configuration and audit logs kept in ~/.agy-guard (use 'agy-guard uninstall --purge' to delete)")
	}

	fmt.Println("\nUninstallation complete! Antigravity CLI will no longer route tool events through agy-guard.")
}

func runDryRun(cmd string) {
	fmt.Printf("[dry-run] Command: %s\n", cmd)
	cmdClass := classifier.NormalizeCommandClass(cmd)
	if cmdClass != "" {
		fmt.Printf("[dry-run] Action Class: %s\n", cmdClass)
	}
	start := time.Now()

	cfg, err := config.Load()
	if err != nil {
		cfg = config.DefaultConfig()
	}

	applyDryRunPolicy := func(v hook.OutputVerdict) hook.OutputVerdict {
		return applyPolicy(v, cmd, "dry-run", cfg, nil)
	}

	// Tier 1 Check
	verdict, matched := rules.EvaluateStatic(cmd, cfg.StaticRules)
	if matched {
		elapsed := time.Since(start)
		finalVerdict := applyDryRunPolicy(verdict)
		fmt.Println("[dry-run] Matched Tier: Tier 1 (Static Rules)")
		fmt.Printf("[dry-run] Latency: %v\n", elapsed)
		fmt.Printf("[dry-run] Decision: %s\n", finalVerdict.Decision)
		if finalVerdict.Reason != "" {
			fmt.Printf("[dry-run] Reason: %s\n", finalVerdict.Reason)
		}
		return
	}

	// Cache Check
	var cacheInstance *classifier.Cache
	if cfg.EnableCache {
		if cachePath, err := classifier.DefaultCachePath(); err == nil {
			cacheInstance = classifier.NewCache(cachePath, time.Duration(cfg.CacheTTLMinutes)*time.Minute)
			if cachedVerdict, hit := cacheInstance.Get(cfg.Provider, cfg.Model, cmd); hit {
				if cachedVerdict.Decision == hook.DecisionAllow {
					elapsed := time.Since(start)
					fmt.Println("[dry-run] Matched Tier: Cache (hit)")
					fmt.Printf("[dry-run] Latency: %v\n", elapsed)
					fmt.Printf("[dry-run] Decision: %s\n", cachedVerdict.Decision)
					if cachedVerdict.Reason != "" {
						fmt.Printf("[dry-run] Reason: %s\n", cachedVerdict.Reason)
					}
					return
				}
			}
		}
	}

	// Tier 2/3 Check
	c, err := classifier.NewClassifier(cfg)
	if err != nil {
		elapsed := time.Since(start)
		fmt.Println("[dry-run] Matched Tier: Fallback (Config Error)")
		fmt.Printf("[dry-run] Latency: %v\n", elapsed)
		fmt.Printf("[dry-run] Decision: %s\n", hook.DecisionAsk)
		fmt.Printf("[dry-run] Reason: %v\n", err)
		return
	}

	tierName := fmt.Sprintf("Tier 3 (LLM - %s)", cfg.Provider)
	verdict, err = c.Evaluate(context.Background(), cmd, "")
	elapsed := time.Since(start)
	if err != nil {
		fmt.Println("[dry-run] Matched Tier: Fallback (LLM Error)")
		fmt.Printf("[dry-run] Latency: %v\n", elapsed)
		fmt.Printf("[dry-run] Decision: %s\n", hook.DecisionAsk)
		fmt.Printf("[dry-run] Reason: %v\n", err)
		return
	}

	if cacheInstance != nil && verdict.Decision == hook.DecisionAllow {
		_ = cacheInstance.Put(cfg.Provider, cfg.Model, cmd, verdict)
	}

	finalVerdict := applyDryRunPolicy(verdict)
	fmt.Printf("[dry-run] Matched Tier: %s\n", tierName)
	fmt.Printf("[dry-run] Latency: %v\n", elapsed)
	fmt.Printf("[dry-run] Decision: %s\n", finalVerdict.Decision)
	if finalVerdict.Reason != "" {
		fmt.Printf("[dry-run] Reason: %s\n", finalVerdict.Reason)
	}
}

func runResetState() {
	if statePath, err := classifier.DefaultSessionStatePath(); err == nil {
		_ = os.Remove(statePath)
		fmt.Printf("[OK] Reset session state: %s\n", statePath)
	}
	if cachePath, err := classifier.DefaultCachePath(); err == nil {
		_ = os.Remove(cachePath)
		fmt.Printf("[OK] Cleared classifier cache: %s\n", cachePath)
	}
}

// applyPolicy применяет стоп-директивы для агента, AFK-режим и защиту от бесконечных циклов
func applyPolicy(v hook.OutputVerdict, cmd, sessionID string, cfg *config.Config, breaker *classifier.LoopBreaker) hook.OutputVerdict {
	if v.Decision == hook.DecisionAllow {
		if breaker != nil {
			_ = breaker.ResetCommand(sessionID, cmd)
		}
		return v
	}

	maxRetries := cfg.MaxRetriesBeforeAsk
	if maxRetries <= 0 {
		maxRetries = 2
	}

	// 1. Unattended mode: конвертируем ask в deny со стоп-инструкцией AFK
	if cfg.UnattendedMode && v.Decision == hook.DecisionAsk {
		baseReason := v.Reason
		if baseReason == "" {
			baseReason = "Action requires confirmation"
		}
		v.Decision = hook.DecisionDeny
		v.Reason = hook.FormatDenyReason(baseReason + ". User is AFK. Do not block on confirmation; find a safe alternative or skip this step.")
	}

	// 2. Loop breaker при повторных отказах (Strike 2+)
	if v.Decision == hook.DecisionDeny && breaker != nil {
		escalated, count, cmdClass := breaker.RecordDenyAndCheck(sessionID, cmd, maxRetries)
		if escalated {
			if cfg.UnattendedMode {
				// В AFK-режиме пользователя нет у экрана — эскалация в ask недопустима.
				// Возвращаем жесткий отказ со стоп-директивой прервать цикл.
				return hook.OutputVerdict{
					Decision: hook.DecisionDeny,
					Reason: hook.FormatDenyReason(fmt.Sprintf(
						"Loop Breaker: Agent repeated blocked command (%s) %d times. Operation aborted to prevent infinite loop. User is AFK.",
						cmdClass, count,
					)),
				}
			}
			return hook.OutputVerdict{
				Decision: hook.DecisionAsk,
				Reason:   fmt.Sprintf("Loop Breaker: Agent repeated a blocked command (%s). Escalating to manual confirmation to prevent an infinite loop.", cmdClass),
			}
		}
		v.Reason = hook.FormatDenyReason(v.Reason)
	}

	return v
}

// matchDirectUserIntent мгновенно проверяет явные намерения пользователя из чата (Tier 2 Intent).
// Если пользователь прямо попросил выполнить команду (например, "пушни коммиты" или "сделай релиз"),
// безопасные команды (без деструктивных флагов -f/--force) разрешаются без вызова LLM за 0 мс.
func matchDirectUserIntent(cmd, userIntent string) (hook.OutputVerdict, bool) {
	cleanCmd := strings.TrimSpace(cmd)
	cleanIntent := strings.ToLower(strings.TrimSpace(userIntent))
	if cleanCmd == "" || cleanIntent == "" {
		return hook.OutputVerdict{}, false
	}

	lowerCmd := strings.ToLower(cleanCmd)

	// Защита от опасных флагов: git push --force / git reset --hard никогда не разрешаются автоматически
	if strings.Contains(lowerCmd, "--force") || strings.Contains(lowerCmd, "-f") || strings.Contains(lowerCmd, "--hard") {
		return hook.OutputVerdict{}, false
	}

	// 1. git push
	if strings.HasPrefix(lowerCmd, "git push") {
		pushKeywords := []string{"push", "пуш", "пушни", "запуш", "отправь коммит", "отправь в гит", "push commit"}
		for _, kw := range pushKeywords {
			if strings.Contains(cleanIntent, kw) {
				return hook.OutputVerdict{
					Decision: hook.DecisionAllow,
					Reason:   fmt.Sprintf("User explicitly requested git push in chat (%q)", kw),
				}, true
			}
		}
	}

	// 2. git tag
	if strings.HasPrefix(lowerCmd, "git tag") {
		tagKeywords := []string{"tag", "тег", "релиз", "release"}
		for _, kw := range tagKeywords {
			if strings.Contains(cleanIntent, kw) {
				return hook.OutputVerdict{
					Decision: hook.DecisionAllow,
					Reason:   fmt.Sprintf("User explicitly requested git tag / release in chat (%q)", kw),
				}, true
			}
		}
	}

	// 3. gh release
	if strings.HasPrefix(lowerCmd, "gh release") {
		relKeywords := []string{"release", "релиз", "выпусти"}
		for _, kw := range relKeywords {
			if strings.Contains(cleanIntent, kw) {
				return hook.OutputVerdict{
					Decision: hook.DecisionAllow,
					Reason:   fmt.Sprintf("User explicitly requested GitHub release in chat (%q)", kw),
				}, true
			}
		}
	}

	return hook.OutputVerdict{}, false
}

// isCommandTool возвращает true, если переданное имя инструмента является запуском терминальной команды
func isCommandTool(toolName string) bool {
	lower := strings.ToLower(strings.TrimSpace(toolName))
	return lower == "run_command" || lower == "bash" || lower == "shell" || lower == "terminal" || lower == "exec_command" || lower == "execute_command"
}

// isReadOnlyTool возвращает true, если переданный инструмент является операцией только для чтения
func isReadOnlyTool(toolName string) bool {
	lower := strings.ToLower(strings.TrimSpace(toolName))
	return lower == "read_file" || lower == "view_file" || lower == "read_resource" || lower == "list_resources" || lower == "list_dir" || lower == "list_directory" || lower == "search_web"
}

// evaluateNonCommandTool выполняет быстрый фильтр (0 мс, без LLM) для файлов и других не-терминальных инструментов.
// Возвращает (verdict, auditTier, handled).
func evaluateNonCommandTool(payload *hook.InputPayload, cfg *config.Config, breaker *classifier.LoopBreaker) (hook.OutputVerdict, string, bool) {
	if payload == nil || isCommandTool(payload.ToolCall.Name) {
		return hook.OutputVerdict{}, "", false
	}

	sessionID := payload.SessionID
	if sessionID == "" {
		sessionID = payload.ConversationID
	}
	if sessionID == "" {
		sessionID = "default"
	}

	targetFile := payload.ExtractTargetFile()
	if targetFile == "" {
		// Read-only тулы типа grep/search без указания пути к файлу разрешаем сразу
		return hook.OutputVerdict{
			Decision: hook.DecisionAllow,
			Reason:   "Non-file tool call allowed",
		}, "tool_fast_path", true
	}

	// Проверка 1 (Sensitive Files): защита секретов и конфигурационных файлов
	if hook.IsSensitiveFile(targetFile) {
		v := applyPolicy(hook.OutputVerdict{
			Decision: hook.DecisionAsk,
			Reason:   "Agent is attempting to modify/access sensitive config or secret file: " + targetFile,
		}, targetFile, sessionID, cfg, breaker)
		return v, "file_sensitive", true
	}

	// Чтение нечувствительных файлов разрешено отовсюду (документация, конфиги, библиотеки) за 0 мс
	if isReadOnlyTool(payload.ToolCall.Name) {
		return hook.OutputVerdict{
			Decision: hook.DecisionAllow,
			Reason:   "Read-only file access allowed",
		}, "file_read_fast_path", true
	}

	// Проверка 2 (Границы воркспейса для модифицирующих операций: write_file, edit_file):
	// защита от записи за пределы рабочей директории (workspacePaths / cwd)
	if !hook.IsWithinWorkspaces(payload.WorkspacePaths, payload.Cwd, targetFile) {
		v := applyPolicy(hook.OutputVerdict{
			Decision: hook.DecisionAsk,
			Reason:   "Agent is attempting to modify/access file outside current workspace: " + targetFile,
		}, targetFile, sessionID, cfg, breaker)
		return v, "file_outside_workspace", true
	}

	// Все обычные файлы внутри проекта разрешаем мгновенно за 0 мс без LLM
	return hook.OutputVerdict{
		Decision: hook.DecisionAllow,
		Reason:   "File access within workspace allowed",
	}, "file_fast_path", true
}
