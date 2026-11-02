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
	case "dry-run":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: agy-guard dry-run \"<command>\"")
			os.Exit(1)
		}
		cmdToTest := strings.Join(args[1:], " ")
		runDryRun(cmdToTest)
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
  agy-guard dry-run "<cmd>"  Evaluate command without running agy and measure evaluation latency
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

	cmd := payload.ExtractCommand()
	if strings.TrimSpace(cmd) == "" {
		hook.EmitAndExit(hook.OutputVerdict{
			Decision: hook.DecisionAllow,
			Reason:   "No shell command present in tool arguments",
		})
	}

	cfg, err := config.Load()
	if err != nil {
		cfg = config.DefaultConfig()
	}

	// Функция безопасного логирования аудита
	logAudit := func(tier string, verdict hook.OutputVerdict) {
		if !cfg.EnableAuditLog {
			return
		}
		sessionID := payload.SessionID
		if sessionID == "" {
			sessionID = payload.ConversationID
		}
		cwd := payload.Cwd
		if cwd == "" {
			cwd, _ = os.Getwd()
		}
		_ = audit.Log(audit.Record{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			SessionID: sessionID,
			Command:   cmd,
			Cwd:       cwd,
			Tier:      tier,
			Decision:  string(verdict.Decision),
			Reason:    verdict.Reason,
			LatencyMS: time.Since(start).Milliseconds(),
		})
	}

	// Tier 1: Zero-latency static matcher
	verdict, matched := rules.EvaluateStatic(cmd, cfg.StaticRules)
	if matched {
		logAudit("tier1_static", verdict)
		hook.EmitAndExit(verdict)
	}

	// Кеш решений классификатора (для ускорения повторных запусков в циклах сборки/тестирования)
	var cacheInstance *classifier.Cache
	if cfg.EnableCache {
		if cachePath, err := classifier.DefaultCachePath(); err == nil {
			cacheInstance = classifier.NewCache(cachePath, time.Duration(cfg.CacheTTLMinutes)*time.Minute)
			if cachedVerdict, hit := cacheInstance.Get(cfg.Provider, cfg.Model, cmd); hit {
				logAudit("cache", cachedVerdict)
				hook.EmitAndExit(cachedVerdict)
			}
		}
	}

	// Tier 2: Transcript Context Reader
	var transcript string
	if cfg.IncludeTranscriptContext && payload.TranscriptPath != "" {
		transcript = hook.ReadTranscriptTail(payload.TranscriptPath, hook.DefaultMaxTranscriptBytes)
	}

	// Tier 3: LLM Classifier
	c, err := classifier.NewClassifier(cfg)
	if err != nil {
		fallbackVerdict := hook.OutputVerdict{
			Decision: hook.DecisionAsk,
			Reason:   fmt.Sprintf("agy-guard: classifier configuration error: %v", err),
		}
		logAudit("fallback", fallbackVerdict)
		hook.EmitAndExit(fallbackVerdict)
	}

	verdict, err = c.Evaluate(context.Background(), cmd, transcript)
	if err != nil {
		fallbackVerdict := hook.OutputVerdict{
			Decision: hook.DecisionAsk,
			Reason:   fmt.Sprintf("agy-guard: classifier evaluation failed: %v", err),
		}
		logAudit("fallback", fallbackVerdict)
		hook.EmitAndExit(fallbackVerdict)
	}

	// Сохраняем в кеш при успешной оценке
	if cacheInstance != nil {
		_ = cacheInstance.Put(cfg.Provider, cfg.Model, cmd, verdict)
	}

	tierName := fmt.Sprintf("tier3_%s", cfg.Provider)
	logAudit(tierName, verdict)
	hook.EmitAndExit(verdict)
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

	fmt.Println("\nSetup complete! Note:")
	fmt.Println("- To use Gemini, add your API key to ~/.agy-guard/config.json")
	fmt.Println("- To use local Ollama, set \"provider\": \"ollama\" in ~/.agy-guard/config.json")
}

func runDryRun(cmd string) {
	fmt.Printf("[dry-run] Command: %s\n", cmd)
	start := time.Now()

	cfg, err := config.Load()
	if err != nil {
		cfg = config.DefaultConfig()
	}

	// Tier 1 Check
	verdict, matched := rules.EvaluateStatic(cmd, cfg.StaticRules)
	if matched {
		elapsed := time.Since(start)
		fmt.Println("[dry-run] Matched Tier: Tier 1 (Static Rules)")
		fmt.Printf("[dry-run] Latency: %v\n", elapsed)
		fmt.Printf("[dry-run] Decision: %s\n", verdict.Decision)
		if verdict.Reason != "" {
			fmt.Printf("[dry-run] Reason: %s\n", verdict.Reason)
		}
		return
	}

	// Cache Check
	var cacheInstance *classifier.Cache
	if cfg.EnableCache {
		if cachePath, err := classifier.DefaultCachePath(); err == nil {
			cacheInstance = classifier.NewCache(cachePath, time.Duration(cfg.CacheTTLMinutes)*time.Minute)
			if cachedVerdict, hit := cacheInstance.Get(cfg.Provider, cfg.Model, cmd); hit {
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

	if cacheInstance != nil {
		_ = cacheInstance.Put(cfg.Provider, cfg.Model, cmd, verdict)
	}

	fmt.Printf("[dry-run] Matched Tier: %s\n", tierName)
	fmt.Printf("[dry-run] Latency: %v\n", elapsed)
	fmt.Printf("[dry-run] Decision: %s\n", verdict.Decision)
	if verdict.Reason != "" {
		fmt.Printf("[dry-run] Reason: %s\n", verdict.Reason)
	}
}
