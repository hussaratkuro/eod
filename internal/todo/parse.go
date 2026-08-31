package todo

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	titleRe = regexp.MustCompile(`^#\s+(.+?)\s*$`)
	attrRe  = regexp.MustCompile(`\[([a-zA-Z]+)(?::\s*([^\]]*))?\]\s*$`)
	taskRe  = regexp.MustCompile(`^(\s*)[-*]\s+\[([ xX])\]\s*(.*)$`)

	offsetRe = regexp.MustCompile(`^(\d+)([dwm])$`)
	everyRe  = regexp.MustCompile(`^(\d+)([dw])$`)
)

var weekdays = map[string]time.Weekday{
	"mon": time.Monday, "monday": time.Monday, "h": time.Monday, "hetfo": time.Monday,
	"tue": time.Tuesday, "tuesday": time.Tuesday, "k": time.Tuesday, "kedd": time.Tuesday,
	"wed": time.Wednesday, "wednesday": time.Wednesday, "sze": time.Wednesday, "szerda": time.Wednesday,
	"thu": time.Thursday, "thursday": time.Thursday, "cs": time.Thursday, "csutortok": time.Thursday,
	"fri": time.Friday, "friday": time.Friday, "p": time.Friday, "pentek": time.Friday,
	"sat": time.Saturday, "saturday": time.Saturday, "szo": time.Saturday, "szombat": time.Saturday,
	"sun": time.Sunday, "sunday": time.Sunday, "v": time.Sunday, "vasarnap": time.Sunday,
}

var accentFolds = map[rune]rune{
	'á': 'a', 'é': 'e', 'í': 'i', 'ó': 'o', 'ö': 'o', 'ő': 'o', 'ú': 'u', 'ü': 'u', 'ű': 'u',
	'Á': 'A', 'É': 'E', 'Í': 'I', 'Ó': 'O', 'Ö': 'O', 'Ő': 'O', 'Ú': 'U', 'Ü': 'U', 'Ű': 'U',
}

// fold strips Hungarian accents so "hétfő" and "hetfo" mean the same thing.
func fold(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if f, ok := accentFolds[r]; ok {
			sb.WriteRune(f)
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// ── note files ─────────────────────────────────────────────────────────────

// ParseNote reads one markdown note file. Anything that is not a task line or
// the title is ignored, so hand-written prose in the file survives untouched
// only in the sense that it is dropped on the next save — the format stays
// deliberately simple.
func ParseNote(src, path, fallbackTitle string, now time.Time) *Note {
	n := &Note{Title: fallbackTitle, Path: path, Color: "lavender"}
	var stack []*Task

	for _, line := range strings.Split(src, "\n") {
		if m := titleRe.FindStringSubmatch(line); m != nil && n.Title == fallbackTitle {
			title, attrs := splitAttrs(m[1])
			if title != "" {
				n.Title = title
			}
			if c, ok := attrs["color"]; ok && c != "" {
				n.Color = c
			}
			if _, ok := attrs["pinned"]; ok {
				n.Pinned = true
			}
			continue
		}

		m := taskRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		task := parseTokens(m[3], now)
		task.Done = m[2] != " "

		depth := indentDepth(m[1])
		if depth == 0 || len(stack) == 0 {
			n.Tasks = append(n.Tasks, task)
			stack = []*Task{task}
			continue
		}
		if depth > len(stack) {
			depth = len(stack)
		}
		parent := stack[depth-1]
		parent.Children = append(parent.Children, task)
		stack = append(stack[:depth], task)
	}

	Normalize(n.Tasks)
	return n
}

// splitAttrs peels trailing "[key: value]" markers off a title.
func splitAttrs(s string) (string, map[string]string) {
	attrs := map[string]string{}
	s = strings.TrimSpace(s)
	for {
		loc := attrRe.FindStringSubmatchIndex(s)
		if loc == nil {
			break
		}
		key := strings.ToLower(s[loc[2]:loc[3]])
		val := ""
		if loc[4] >= 0 {
			val = strings.TrimSpace(s[loc[4]:loc[5]])
		}
		attrs[key] = val
		s = strings.TrimSpace(s[:loc[0]])
	}
	return s, attrs
}

// indentDepth maps leading spaces to a nesting level, accepting both 2- and
// 4-space indentation from hand-edited files.
func indentDepth(indent string) int {
	n := len(strings.ReplaceAll(indent, "\t", "    "))
	switch {
	case n >= 4:
		return n / 4
	case n >= 2:
		return 1
	}
	return 0
}

// SerializeNote renders a note back to its markdown file form.
func SerializeNote(n *Note) string {
	var sb strings.Builder
	head := "# " + n.Title
	if n.Color != "" {
		head += "  [color: " + n.Color + "]"
	}
	if n.Pinned {
		head += " [pinned]"
	}
	sb.WriteString(head + "\n\n")
	for _, t := range n.Tasks {
		WriteTask(&sb, t, 0)
	}
	return sb.String()
}

// WriteTask writes one task and its subtree as markdown checklist lines.
func WriteTask(sb *strings.Builder, t *Task, depth int) {
	sb.WriteString(TaskLine(t, depth) + "\n")
	for _, c := range t.Children {
		WriteTask(sb, c, depth+1)
	}
}

func TaskLine(t *Task, depth int) string {
	box := "[ ]"
	if t.Done {
		box = "[x]"
	}
	return strings.Repeat("    ", depth) + "- " + box + " " + TaskInline(t)
}

// TaskInline renders the task text plus its metadata tokens — the same syntax
// quick capture accepts, so it can be fed straight back into the editor.
func TaskInline(t *Task) string {
	parts := []string{}
	if t.Text != "" {
		parts = append(parts, t.Text)
	}
	if t.Priority != PrioNone {
		parts = append(parts, fmt.Sprintf("!%d", t.Priority))
	}
	for _, tag := range t.Tags {
		parts = append(parts, "#"+tag)
	}
	if t.Due != nil {
		parts = append(parts, "^"+t.Due.Format("2006-01-02"))
	}
	if t.Repeat != "" {
		parts = append(parts, "*"+t.Repeat)
	}
	if t.Focus > 0 {
		parts = append(parts, "+"+FormatDuration(t.Focus))
	}
	if t.DoneAt != nil {
		parts = append(parts, "~"+t.DoneAt.Format("2006-01-02"))
	}
	return strings.Join(parts, " ")
}

// ── inline token syntax ────────────────────────────────────────────────────

// parseTokens splits a raw line into text plus metadata: !priority, #tag,
// ^due, *repeat, +focus and ~completion-date. A token that does not parse is
// left in the text, so "C# refactor" keeps its hash.
func parseTokens(raw string, now time.Time) *Task {
	t := &Task{}
	var words []string

	for _, f := range strings.Fields(raw) {
		if len(f) > 1 {
			switch f[0] {
			case '!':
				if n, err := strconv.Atoi(f[1:]); err == nil && n >= PrioHigh && n <= PrioLow {
					t.Priority = n
					continue
				}
			case '#':
				t.Tags = append(t.Tags, f[1:])
				continue
			case '^':
				if d, ok := ParseDateExpr(f[1:], now); ok {
					t.Due = &d
					continue
				}
			case '~':
				if d, ok := ParseDate(f[1:]); ok {
					t.DoneAt = &d
					continue
				}
			case '*':
				if spec, ok := NormalizeRepeat(f[1:]); ok {
					t.Repeat = spec
					continue
				}
			case '+':
				if d, err := time.ParseDuration(f[1:]); err == nil && d > 0 {
					t.Focus = d
					continue
				}
			}
		}
		words = append(words, f)
	}

	t.Text = strings.Join(words, " ")
	return t
}

// ParseCapture parses a quick-capture line. It returns the task plus the
// "@note" hint (empty when the line did not name a note).
func ParseCapture(line string, now time.Time) (*Task, string) {
	hint := ""
	var kept []string
	for _, f := range strings.Fields(line) {
		if len(f) > 1 && f[0] == '@' {
			hint = f[1:]
			continue
		}
		kept = append(kept, f)
	}
	return parseTokens(strings.Join(kept, " "), now), hint
}

// ── dates ──────────────────────────────────────────────────────────────────

var dateLayouts = []string{"2006-01-02", "2006.01.02", "2006/01/02"}
var monthDayLayouts = []string{"01-02", "01.02", "01/02"}

func ParseDate(s string) (time.Time, bool) {
	for _, l := range dateLayouts {
		if d, err := time.ParseInLocation(l, s, time.Local); err == nil {
			return d, true
		}
	}
	return time.Time{}, false
}

// ParseDateExpr understands both absolute dates and the shorthand people
// actually type: today/ma, tomorrow/holnap, weekday names in English or
// Hungarian, "3d", "2w", "09-14" or a bare day number.
func ParseDateExpr(s string, now time.Time) (time.Time, bool) {
	s = strings.ToLower(fold(strings.TrimSpace(s)))
	if s == "" {
		return time.Time{}, false
	}
	today := DateOf(now)

	switch s {
	case "ma", "today", "t":
		return today, true
	case "holnap", "tomorrow", "tom":
		return today.AddDate(0, 0, 1), true
	case "tegnap", "yesterday":
		return today.AddDate(0, 0, -1), true
	case "hetvege", "weekend":
		return nextWeekday(today, time.Saturday), true
	}

	if wd, ok := weekdays[s]; ok {
		return nextWeekday(today, wd), true
	}

	if m := offsetRe.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		switch m[2] {
		case "d":
			return today.AddDate(0, 0, n), true
		case "w":
			return today.AddDate(0, 0, 7*n), true
		case "m":
			return today.AddDate(0, n, 0), true
		}
	}

	if d, ok := ParseDate(s); ok {
		return d, true
	}

	for _, l := range monthDayLayouts {
		if d, err := time.ParseInLocation(l, s, time.Local); err == nil {
			r := time.Date(today.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.Local)
			if r.Before(today) {
				r = r.AddDate(1, 0, 0)
			}
			return r, true
		}
	}

	if n, err := strconv.Atoi(s); err == nil && n >= 1 && n <= 31 {
		r := time.Date(today.Year(), today.Month(), n, 0, 0, 0, 0, time.Local)
		if r.Before(today) {
			r = r.AddDate(0, 1, 0)
		}
		return r, true
	}

	return time.Time{}, false
}

// nextWeekday returns the next given weekday, counting today as a hit.
func nextWeekday(from time.Time, wd time.Weekday) time.Time {
	d := from
	for i := 0; i < 7; i++ {
		if d.Weekday() == wd {
			return d
		}
		d = d.AddDate(0, 0, 1)
	}
	return from
}

// ── repeats ────────────────────────────────────────────────────────────────

// NormalizeRepeat canonicalises a repeat spec: daily / weekly / monthly, a
// three-letter weekday, or an "every N days/weeks" form like 3d.
func NormalizeRepeat(s string) (string, bool) {
	s = strings.ToLower(fold(strings.TrimSpace(s)))
	switch s {
	case "daily", "napi", "nap", "everyday", "every-day":
		return "daily", true
	case "weekly", "heti", "het":
		return "weekly", true
	case "monthly", "havi", "honap":
		return "monthly", true
	}
	if wd, ok := weekdays[s]; ok {
		return strings.ToLower(wd.String()[:3]), true
	}
	if m := everyRe.FindStringSubmatch(s); m != nil {
		return m[1] + m[2], true
	}
	return "", false
}

// NextOccurrence returns the next due date for a repeating task.
func NextOccurrence(spec string, from time.Time) (time.Time, bool) {
	base := DateOf(from)
	switch spec {
	case "daily":
		return base.AddDate(0, 0, 1), true
	case "weekly":
		return base.AddDate(0, 0, 7), true
	case "monthly":
		return base.AddDate(0, 1, 0), true
	}
	if wd, ok := weekdays[spec]; ok {
		return nextWeekday(base.AddDate(0, 0, 1), wd), true
	}
	if m := everyRe.FindStringSubmatch(spec); m != nil {
		n, _ := strconv.Atoi(m[1])
		if m[2] == "w" {
			n *= 7
		}
		if n < 1 {
			n = 1
		}
		return base.AddDate(0, 0, n), true
	}
	return time.Time{}, false
}

// FormatDuration renders a focus total compactly: 45m, 1h30m, 2h.
func FormatDuration(d time.Duration) string {
	mins := int(d.Minutes())
	if mins < 60 {
		return fmt.Sprintf("%dm", mins)
	}
	h, m := mins/60, mins%60
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh%dm", h, m)
}
