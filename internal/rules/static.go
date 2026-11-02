package rules

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/agy-guard/agy-guard/internal/config"
	"github.com/agy-guard/agy-guard/internal/hook"
)

// EvaluateStatic проверяет команду по статическим правилам (Tier 1).
// Возвращает (verdict, matched). Если matched == false, команду нужно передать LLM-классификатору.
func EvaluateStatic(cmd string, rules config.StaticRules) (hook.OutputVerdict, bool) {
	trimmed := strings.TrimSpace(cmd)
	if trimmed == "" {
		return hook.OutputVerdict{
			Decision: hook.DecisionAllow,
			Reason:   "Empty command allowed",
		}, true
	}

	lowerCmd := strings.ToLower(trimmed)

	// 1. Сначала проверяем жесткий черный список (Hard Deny)
	for _, deny := range rules.DenySubstrings {
		denyTrimmed := strings.TrimSpace(deny)
		if denyTrimmed == "" {
			continue
		}
		if strings.Contains(lowerCmd, strings.ToLower(denyTrimmed)) {
			return hook.OutputVerdict{
				Decision: hook.DecisionDeny,
				Reason:   fmt.Sprintf("Blocked by Tier-1 static rule: command contains restricted pattern %q", denyTrimmed),
			}, true
		}
	}

	// 2. Умная проверка белого списка префиксов (с поддержкой безопасных цепочек &&, ||, ;, |)
	parts, ok := splitCompoundCommand(trimmed)
	if !ok {
		// Присутствуют опасные редиректы (<, >), сабшеллы ($(...), `...`), переводы строк или фоновый запуск (&)
		return hook.OutputVerdict{}, false
	}

	// Проверяем, что КАЖДАЯ подкоманда в цепочке входит в белый список разрешенных префиксов
	for _, part := range parts {
		if !matchesAllowPrefix(part, rules.AllowPrefixes) {
			return hook.OutputVerdict{}, false
		}
	}

	if len(parts) == 1 {
		return hook.OutputVerdict{
			Decision: hook.DecisionAllow,
			Reason:   "Matched Tier-1 safe prefix",
		}, true
	}

	return hook.OutputVerdict{
		Decision: hook.DecisionAllow,
		Reason:   fmt.Sprintf("All %d compound command parts matched Tier-1 safe prefixes", len(parts)),
	}, true
}

// matchesAllowPrefix проверяет, что подкоманда строго совпадает с префиксом или начинается с него и пробела.
func matchesAllowPrefix(subCmd string, allowPrefixes []string) bool {
	lowerSub := strings.ToLower(strings.TrimSpace(subCmd))
	if lowerSub == "" {
		return true
	}

	for _, prefix := range allowPrefixes {
		p := strings.ToLower(strings.TrimSpace(prefix))
		if p == "" {
			continue
		}
		if lowerSub == p || strings.HasPrefix(lowerSub, p+" ") {
			return true
		}
	}

	// Поддержка абсолютных/относительных путей (например, /usr/bin/git status или /bin/ls)
	parts := strings.Fields(lowerSub)
	if len(parts) > 0 {
		cmdBase := filepath.Base(parts[0])
		if cmdBase != parts[0] {
			normSub := cmdBase
			if len(parts) > 1 {
				normSub = cmdBase + " " + strings.Join(parts[1:], " ")
			}
			for _, prefix := range allowPrefixes {
				p := strings.ToLower(strings.TrimSpace(prefix))
				if p == "" {
					continue
				}
				if normSub == p || strings.HasPrefix(normSub, p+" ") {
					return true
				}
			}
		}
	}

	return false
}

// splitCompoundCommand разделяет составную команду по безопасным операторам (&&, ||, ;, |),
// учитывая экранирование кавычек. Возвращает false, если обнаружены редиректы вне кавычек,
// сабшеллы ($(...), `...`), непарные кавычки, переводы строк, одиночный фоновый оператор '&'
// или незавершенные операторы цепочки (например, 'cmd |' или '&& cmd').
func splitCompoundCommand(cmd string) ([]string, bool) {
	// Опасные сабшеллы и переводы строк запрещены безусловно
	if strings.ContainsAny(cmd, "`\n\r") || strings.Contains(cmd, "$(") {
		return nil, false
	}

	var parts []string
	var current strings.Builder
	inSingleQuote := false
	inDoubleQuote := false
	n := len(cmd)
	lastWasOperator := false

	for i := 0; i < n; i++ {
		ch := cmd[i]

		// Одинарные кавычки внутри двойных трактуются как обычный символ
		if ch == '\'' && !inDoubleQuote {
			inSingleQuote = !inSingleQuote
			current.WriteByte(ch)
			continue
		}
		// Двойные кавычки внутри одинарных трактуются как обычный символ
		if ch == '"' && !inSingleQuote {
			inDoubleQuote = !inDoubleQuote
			current.WriteByte(ch)
			continue
		}

		if inSingleQuote || inDoubleQuote {
			current.WriteByte(ch)
			continue
		}

		// Редиректы вне кавычек запрещены для статического Tier-1 автопропуска
		if ch == '<' || ch == '>' {
			return nil, false
		}

		// Разделитель команд ';'
		if ch == ';' {
			part := strings.TrimSpace(current.String())
			if part != "" {
				parts = append(parts, part)
			}
			current.Reset()
			lastWasOperator = false // Завершающая ';' допустима в shell синтаксисе
			continue
		}

		// Логическое '&&'
		if ch == '&' {
			if i+1 < n && cmd[i+1] == '&' {
				i++ // пропускаем второй &
				part := strings.TrimSpace(current.String())
				if part == "" {
					return nil, false // Некорректный синтаксис: оператор в начале ("&& cmd") или подряд
				}
				parts = append(parts, part)
				current.Reset()
				lastWasOperator = true
				continue
			}
			// Одиночный '&' (запуск в фоне) — небезопасно для статического автопропуска
			return nil, false
		}

		// Пайп '|' или логическое '||'
		if ch == '|' {
			if i+1 < n && cmd[i+1] == '|' {
				i++ // пропускаем второй |
			}
			part := strings.TrimSpace(current.String())
			if part == "" {
				return nil, false // Некорректный синтаксис: оператор в начале ("| cmd") или подряд
			}
			parts = append(parts, part)
			current.Reset()
			lastWasOperator = true
			continue
		}

		current.WriteByte(ch)
		if ch != ' ' && ch != '\t' {
			lastWasOperator = false
		}
	}

	if inSingleQuote || inDoubleQuote {
		// Незакрытая кавычка
		return nil, false
	}

	if lastWasOperator {
		// Команда оканчивается незавершенным оператором (&&, ||, |)
		return nil, false
	}

	last := strings.TrimSpace(current.String())
	if last != "" {
		parts = append(parts, last)
	}

	if len(parts) == 0 {
		return nil, false
	}

	return parts, true
}
