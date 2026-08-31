package todo

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// DueState buckets a task by how urgent its due date is.
type DueState int

const (
	DueNone DueState = iota
	DueLater
	DueSoon
	DueToday
	DueOverdue
)

// StateOf classifies a task's due date relative to now.
func StateOf(t *Task, now time.Time) DueState {
	if t.Due == nil {
		return DueNone
	}
	today := DateOf(now)
	d := DateOf(*t.Due)
	switch {
	case d.Before(today):
		return DueOverdue
	case d.Equal(today):
		return DueToday
	case d.Before(today.AddDate(0, 0, 4)):
		return DueSoon
	}
	return DueLater
}

// DueLabel renders a due date the way a human reads it.
func DueLabel(t *Task, now time.Time) string {
	if t.Due == nil {
		return ""
	}
	today := DateOf(now)
	d := DateOf(*t.Due)
	days := int(d.Sub(today).Hours() / 24)
	switch {
	case days == 0:
		return "today"
	case days == 1:
		return "tomorrow"
	case days == -1:
		return "1d late"
	case days < 0:
		return fmt.Sprintf("%dd late", -days)
	case days < 7:
		return fmt.Sprintf("%dd", days)
	}
	return d.Format("01-02")
}

// sortTasks orders a virtual list: priority first, then due date, then text.
func sortTasks(tasks []*Task) {
	sort.SliceStable(tasks, func(i, j int) bool {
		a, b := tasks[i], tasks[j]
		pa, pb := a.Priority, b.Priority
		if pa == PrioNone {
			pa = 9
		}
		if pb == PrioNone {
			pb = 9
		}
		if pa != pb {
			return pa < pb
		}
		switch {
		case a.Due != nil && b.Due == nil:
			return true
		case a.Due == nil && b.Due != nil:
			return false
		case a.Due != nil && b.Due != nil && !a.Due.Equal(*b.Due):
			return a.Due.Before(*b.Due)
		}
		return strings.ToLower(a.Text) < strings.ToLower(b.Text)
	})
}

// Virtuals builds the computed cards that aggregate across every note. They
// hold live task pointers, so ticking something off in Today writes through to
// the note that owns it.
func Virtuals(b *Board, now time.Time) []*Note {
	today := DateOf(now)

	collect := func(pred func(*Task) bool) []*Task {
		var out []*Task
		for _, n := range b.Notes {
			n.Walk(func(t *Task) {
				if len(t.Children) > 0 {
					return
				}
				if pred(t) {
					out = append(out, t)
				}
			})
		}
		sortTasks(out)
		return out
	}

	mk := func(title, icon, color string, tasks []*Task) *Note {
		return &Note{
			Title:   title,
			Slug:    "@" + Slugify(title),
			Color:   color,
			Virtual: true,
			Flat:    true,
			Icon:    icon,
			Tasks:   tasks,
		}
	}

	overdue := collect(func(t *Task) bool {
		return !t.Done && t.Due != nil && DateOf(*t.Due).Before(today)
	})
	todayTasks := collect(func(t *Task) bool {
		return !t.Done && t.Due != nil && DateOf(*t.Due).Equal(today)
	})
	week := collect(func(t *Task) bool {
		if t.Done || t.Due == nil {
			return false
		}
		d := DateOf(*t.Due)
		return d.After(today) && !d.After(today.AddDate(0, 0, 7))
	})
	done := collect(func(t *Task) bool { return t.DoneOn(now) })

	var out []*Note
	if len(overdue) > 0 {
		out = append(out, mk("Overdue", "!", "red", overdue))
	}
	out = append(out, mk("Today", "◷", "blue", todayTasks))
	if len(week) > 0 {
		out = append(out, mk("This week", "▤", "mauve", week))
	}
	if len(done) > 0 {
		out = append(out, mk("Done today", "✓", "green", done))
	}
	return out
}

// ── fuzzy search ───────────────────────────────────────────────────────────

// Search ranks every task on the board against a fuzzy query.
func Search(b *Board, query string) []Ref {
	q := strings.ToLower(fold(strings.TrimSpace(query)))
	if q == "" {
		return nil
	}

	type scored struct {
		ref   Ref
		score int
	}
	var hits []scored

	for _, ref := range b.Refs() {
		hay := strings.ToLower(fold(ref.Task.Text + " " + strings.Join(ref.Task.Tags, " ") + " " + ref.Note.Title))
		if s, ok := fuzzyScore(q, hay); ok {
			if ref.Task.Done {
				s -= 30
			}
			hits = append(hits, scored{ref, s})
		}
	}

	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	out := make([]Ref, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.ref)
	}
	return out
}

// fuzzyScore does a subsequence match, rewarding runs and word starts.
func fuzzyScore(needle, haystack string) (int, bool) {
	n := []rune(needle)
	h := []rune(haystack)
	score, ni := 0, 0
	prevMatch := -2

	for hi := 0; hi < len(h) && ni < len(n); hi++ {
		if h[hi] != n[ni] {
			continue
		}
		score += 10
		if hi == prevMatch+1 {
			score += 8
		}
		if hi == 0 || h[hi-1] == ' ' || h[hi-1] == '-' || h[hi-1] == '/' {
			score += 6
		}
		prevMatch = hi
		ni++
	}
	if ni < len(n) {
		return 0, false
	}
	return score - len(h)/4, true
}

// ── stats ──────────────────────────────────────────────────────────────────

// Heat counts completions per day for the last n days, oldest first.
func Heat(b *Board, days int, now time.Time) []int {
	out := make([]int, days)
	today := DateOf(now)
	for _, ref := range b.Refs() {
		t := ref.Task
		if !t.Done || t.DoneAt == nil || len(t.Children) > 0 {
			continue
		}
		diff := int(today.Sub(DateOf(*t.DoneAt)).Hours() / 24)
		if diff < 0 || diff >= days {
			continue
		}
		out[days-1-diff]++
	}
	return out
}

// Streak counts consecutive days with at least one completion, ending today
// (or yesterday, so an unfinished today does not break the run).
func Streak(heat []int) int {
	if len(heat) == 0 {
		return 0
	}
	i := len(heat) - 1
	if heat[i] == 0 {
		i--
	}
	streak := 0
	for ; i >= 0 && heat[i] > 0; i-- {
		streak++
	}
	return streak
}

const heatBlocks = "·▁▂▃▄▅▆▇█"

// HeatStrip renders a heat slice as a sparkline.
func HeatStrip(heat []int) string {
	maxVal := 0
	for _, v := range heat {
		if v > maxVal {
			maxVal = v
		}
	}
	blocks := []rune(heatBlocks)
	var sb strings.Builder
	for _, v := range heat {
		if maxVal == 0 || v == 0 {
			sb.WriteRune(blocks[0])
			continue
		}
		idx := 1 + (v-1)*(len(blocks)-2)/maxVal
		if idx >= len(blocks) {
			idx = len(blocks) - 1
		}
		sb.WriteRune(blocks[idx])
	}
	return sb.String()
}

// Summary is the text of the todo statistics screen.
func Summary(b *Board, now time.Time) string {
	open, done, overdue, dueToday := 0, 0, 0, 0
	tagCount := map[string]int{}
	var tagOrder []string

	for _, ref := range b.Refs() {
		t := ref.Task
		if len(t.Children) > 0 {
			continue
		}
		if t.Done {
			done++
		} else {
			open++
			switch StateOf(t, now) {
			case DueOverdue:
				overdue++
			case DueToday:
				dueToday++
			}
		}
		for _, tag := range t.Tags {
			if _, ok := tagCount[tag]; !ok {
				tagOrder = append(tagOrder, tag)
			}
			tagCount[tag]++
		}
	}

	doneToday := 0
	for _, ref := range b.Refs() {
		if len(ref.Task.Children) == 0 && ref.Task.DoneOn(now) {
			doneToday++
		}
	}

	heat := Heat(b, 30, now)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Todo summary — %s\n", DateOf(now).Format("2006-01-02")))
	sb.WriteString(strings.Repeat("─", 52) + "\n\n")
	sb.WriteString(fmt.Sprintf("  Notes %d    Open %d    Done %d    Done today %d\n", len(b.Notes), open, done, doneToday))
	sb.WriteString(fmt.Sprintf("  Overdue %d    Due today %d\n\n", overdue, dueToday))
	sb.WriteString(fmt.Sprintf("  Streak %d day(s)\n", Streak(heat)))
	sb.WriteString("  Last 30 days  " + HeatStrip(heat) + "\n\n")

	sb.WriteString("  Notes\n")
	for _, n := range b.Notes {
		d, total := n.Counts()
		barLen := 0
		if total > 0 {
			barLen = d * 16 / total
		}
		bar := strings.Repeat("▓", barLen) + strings.Repeat("░", 16-barLen)
		sb.WriteString(fmt.Sprintf("  %-20s %s  %d/%d\n", truncate(n.Title, 20), bar, d, total))
	}

	if len(tagOrder) > 0 {
		sort.SliceStable(tagOrder, func(i, j int) bool { return tagCount[tagOrder[i]] > tagCount[tagOrder[j]] })
		sb.WriteString("\n  Tags\n  ")
		for i, tag := range tagOrder {
			if i == 12 {
				break
			}
			sb.WriteString(fmt.Sprintf("#%s %d   ", tag, tagCount[tag]))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
