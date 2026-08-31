package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"eod/internal/todo"
)

const (
	cardWidth = 30 // outer width, border included
	cardBodyH = 7  // text rows inside a card
	cardRowH  = cardBodyH + 2
)

func (t *TodoApp) View() string {
	if t.width == 0 {
		return "Loading…"
	}

	switch t.view {
	case tvSummary:
		return t.viewTodoAux("Todo summary")
	case tvHelp:
		return t.viewTodoAux("Todo — keys")
	case tvPalette:
		return t.viewPalette()
	case tvMove:
		return t.viewMove()
	}

	body := t.viewWallBody()
	if t.view == tvNote || (t.view == tvInput && t.returnTo == tvNote) {
		body = t.viewNoteBody()
	}

	bar := t.renderTodoStatus()
	if t.view == tvInput {
		bar = t.renderInputBar()
	}
	return lipgloss.JoinVertical(lipgloss.Left, body, bar)
}

// ── wall ───────────────────────────────────────────────────────────────────

func (t *TodoApp) viewWallBody() string {
	open := 0
	for _, ref := range t.board.Refs() {
		if !ref.Task.Done && len(ref.Task.Children) == 0 {
			open++
		}
	}

	header := t.renderTodoHeader(
		fmt.Sprintf("TODO  %s", todo.DateOf(t.now).Format("2006-01-02")),
		fmt.Sprintf("%d notes · %d open", len(t.board.Notes), open),
	)

	if len(t.cards) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, header,
			styleMuted.Render("\n  No notes yet. Press [n] to create one."))
	}

	cols := max(t.cols, 1)
	var rows []string
	var row []string
	for i, n := range t.cards {
		row = append(row, t.renderCard(n, i == t.cardIdx))
		if len(row) == cols {
			rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, row...))
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, row...))
	}

	// Scroll whole card rows so the selected card stays on screen.
	bodyH := max(t.height-2, cardRowH)
	visible := max(bodyH/cardRowH, 1)
	selRow := t.cardIdx / cols
	if selRow < t.rowTop {
		t.rowTop = selRow
	}
	if selRow >= t.rowTop+visible {
		t.rowTop = selRow - visible + 1
	}
	if t.rowTop > len(rows)-1 {
		t.rowTop = max(len(rows)-1, 0)
	}
	end := min(t.rowTop+visible, len(rows))

	return lipgloss.JoinVertical(lipgloss.Left,
		append([]string{header}, rows[t.rowTop:end]...)...)
}

func (t *TodoApp) renderCard(n *todo.Note, selected bool) string {
	inner := cardWidth - 4

	title := n.Title
	if n.Icon != "" {
		title = n.Icon + " " + title
	}
	if n.Pinned {
		title = "★ " + title
	}

	done, total := n.Counts()
	counts := fmt.Sprintf("%d/%d", done, total)
	titleW := inner - lipgloss.Width(counts) - 1

	lines := []string{
		styleCardTitle.Foreground(accentColor(n.Color)).Render(padTrunc(title, titleW)) +
			" " + styleMuted.Render(counts),
		t.renderProgress(done, total, inner),
	}

	preview := n.Preview(cardBodyH - 3)
	for _, task := range preview {
		lines = append(lines, t.renderCardLine(task, inner))
	}
	if len(preview) == 0 {
		if total > 0 {
			lines = append(lines, styleSuccess.Render(padTrunc("  all done ✓", inner)))
		} else {
			lines = append(lines, styleMuted.Render(padTrunc("  (empty)", inner)))
		}
	}

	openCount := 0
	n.Walk(func(task *todo.Task) {
		if !task.Done && len(task.Children) == 0 {
			openCount++
		}
	})
	if rest := openCount - len(preview); rest > 0 {
		lines = append(lines, styleMuted.Render(padTrunc(fmt.Sprintf("  +%d more", rest), inner)))
	}

	st := lipgloss.NewStyle().
		Width(cardWidth-2).
		Height(cardBodyH).
		Padding(0, 1).
		MarginRight(1).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(moSurface1))
	if selected {
		st = st.Border(lipgloss.ThickBorder()).BorderForeground(accentColor(n.Color))
	}
	return st.Render(strings.Join(lines, "\n"))
}

// renderCardLine is the compact one-line form of a task used on the wall.
func (t *TodoApp) renderCardLine(task *todo.Task, w int) string {
	badge := ""
	if label := todo.DueLabel(task, t.now); label != "" {
		badge = " " + t.dueStyle(task).Render(label)
	}
	prio := ""
	if task.Priority != todo.PrioNone {
		prio = stylePrio[task.Priority].Render(fmt.Sprintf("!%d ", task.Priority))
	}

	textW := w - 2 - lipgloss.Width(badge) - lipgloss.Width(prio)
	text := padTrunc(task.Text, max(textW, 4))
	if task.Done {
		return styleMuted.Render("· ") + prio + styleDone.Render(text) + badge
	}
	return styleMuted.Render("· ") + prio + styleWall.Render(text) + badge
}

func (t *TodoApp) renderProgress(done, total, w int) string {
	if w < 4 {
		return ""
	}
	filled := 0
	if total > 0 {
		filled = done * w / total
	}
	return styleProgress.Render(strings.Repeat("▓", filled)) +
		styleProgressBG.Render(strings.Repeat("░", w-filled))
}

// ── note ───────────────────────────────────────────────────────────────────

func (t *TodoApp) viewNoteBody() string {
	if t.note == nil {
		return t.viewWallBody()
	}

	done, total := t.note.Counts()
	title := t.note.Title
	if t.note.Icon != "" {
		title = t.note.Icon + " " + title
	}
	header := t.renderTodoHeader(
		lipgloss.NewStyle().Bold(true).Foreground(accentColor(t.note.Color)).Render("‹ "+title),
		fmt.Sprintf("%d/%d  %s", done, total, t.renderProgress(done, total, 14)),
	)

	var rows []string
	for i, f := range t.flat {
		rows = append(rows, t.renderTaskRow(f, i == t.taskIdx))
	}
	if len(rows) == 0 {
		msg := "  (empty — press [a] to add a task)"
		if t.note.Virtual {
			msg = "  (nothing here right now)"
		}
		rows = append(rows, styleMuted.Render(msg))
	}

	t.vp.Width = max(t.width-2, 10)
	t.vp.Height = max(t.height-3, 3)
	t.vp.SetContent(strings.Join(rows, "\n"))
	t.syncViewport()

	return lipgloss.JoinVertical(lipgloss.Left, header, t.vp.View())
}

func (t *TodoApp) renderTaskRow(f flatTask, selected bool) string {
	task := f.task

	cursor := "  "
	if selected {
		cursor = styleCursor.Render("▸ ")
	}

	box := "[ ]"
	switch {
	case task.Done:
		box = "[x]"
	case task.Partial():
		box = "[~]"
	}
	boxStyled := styleMuted.Render(box)
	if task.Done {
		boxStyled = styleSuccess.Render(box)
	} else if task.Partial() {
		boxStyled = styleDueSoon.Render(box)
	}

	text := task.Text
	switch {
	case task.Done:
		text = styleDone.Render(text)
	case selected:
		text = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(moText)).Render(text)
	default:
		text = styleWall.Render(text)
	}

	var badges []string
	if task.Priority != todo.PrioNone {
		badges = append(badges, stylePrio[task.Priority].Render(fmt.Sprintf("!%d", task.Priority)))
	}
	for _, tag := range task.Tags {
		badges = append(badges, styleTag.Render("#"+tag))
	}
	if label := todo.DueLabel(task, t.now); label != "" && !task.Done {
		// A late task reads better as "3d late" than "^3d late".
		if todo.StateOf(task, t.now) != todo.DueOverdue {
			label = "^" + label
		}
		badges = append(badges, t.dueStyle(task).Render(label))
	}
	if task.Repeat != "" {
		badges = append(badges, styleRepeat.Render("↻"+task.Repeat))
	}
	if t.focus == task {
		badges = append(badges, styleFocus.Render("⏱ "+clock(time.Since(t.focusStart))))
	} else if task.Focus > 0 {
		badges = append(badges, styleMuted.Render("⏱"+todo.FormatDuration(task.Focus)))
	}
	if t.note.Flat {
		if owner := f.owner; owner != nil {
			badges = append(badges, styleMuted.Render("@"+owner.Title))
		}
	}

	line := cursor + strings.Repeat("  ", f.depth) + boxStyled + " " + text
	if len(badges) > 0 {
		line += "  " + strings.Join(badges, " ")
	}
	return line
}

func (t *TodoApp) dueStyle(task *todo.Task) lipgloss.Style {
	switch todo.StateOf(task, t.now) {
	case todo.DueOverdue:
		return styleDueOverdue
	case todo.DueToday:
		return styleDueToday
	case todo.DueSoon:
		return styleDueSoon
	}
	return styleDueLater
}

// ── overlays ───────────────────────────────────────────────────────────────

func (t *TodoApp) viewPalette() string {
	header := t.renderTodoHeader("Search", "[↑/↓] pick  [Enter] jump  [Esc] back")

	var rows []string
	limit := max(t.height-5, 3)
	for i, ref := range t.matches {
		if i >= limit {
			rows = append(rows, styleMuted.Render(fmt.Sprintf("  … %d more", len(t.matches)-limit)))
			break
		}
		cursor := "  "
		if i == t.matchIdx {
			cursor = styleCursor.Render("▸ ")
		}
		box := styleMuted.Render("[ ]")
		text := styleWall.Render(ref.Task.Text)
		if ref.Task.Done {
			box = styleSuccess.Render("[x]")
			text = styleDone.Render(ref.Task.Text)
		}
		note := styleMuted.Render(padTrunc(ref.Note.Title, 14) + " ")
		badge := ""
		if label := todo.DueLabel(ref.Task, t.now); label != "" {
			badge = "  " + t.dueStyle(ref.Task).Render(label)
		}
		rows = append(rows, cursor+note+box+" "+text+badge)
	}
	if len(t.matches) == 0 && t.input.Value() != "" {
		rows = append(rows, styleError.Render("  No matches"))
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		"  "+t.input.View(),
		"",
		strings.Join(rows, "\n"),
	)
}

func (t *TodoApp) viewMove() string {
	var rows []string
	rows = append(rows, styleHeader.Render("Move task to…"), "")
	for i, n := range t.moveTargets {
		line := padTrunc(n.Title, 26)
		if i == t.moveIdx {
			rows = append(rows, styleMenuSelected.Render(line))
		} else {
			rows = append(rows, styleMenuItem.Foreground(accentColor(n.Color)).Render(line))
		}
	}
	rows = append(rows, "", styleMuted.Render("  [↑/↓] pick   [Enter] move   [Esc] cancel"))

	box := styleMenuBox.Render(strings.Join(rows, "\n"))
	return lipgloss.Place(t.width, t.height, lipgloss.Center, lipgloss.Center, box)
}

func (t *TodoApp) viewTodoAux(title string) string {
	hdr := styleHeader.Render(title) + "  " + styleMuted.Render("[q/Esc] back")
	t.vp.Width = max(t.width-2, 10)
	t.vp.Height = max(t.height-3, 3)
	return lipgloss.JoinVertical(lipgloss.Left,
		hdr,
		strings.Repeat("─", t.width),
		t.vp.View(),
	)
}

// ── chrome ─────────────────────────────────────────────────────────────────

func (t *TodoApp) renderTodoHeader(left, right string) string {
	l := styleHeader.Render(left)
	r := styleMuted.Render(right)
	gap := max(t.width-lipgloss.Width(l)-lipgloss.Width(r)-2, 1)
	return l + strings.Repeat(" ", gap) + r
}

func (t *TodoApp) renderInputBar() string {
	label := styleKey.Render(t.input.Placeholder)
	return styleStatusBar.Render(label + "  " + t.input.View())
}

func (t *TodoApp) renderTodoStatus() string {
	var hints []string
	if t.view == tvNote {
		hints = []string{
			keyHint("space", "check"),
			keyHint("a", "add"),
			keyHint("o", "sub"),
			keyHint("e", "edit"),
			keyHint("d", "del"),
			keyHint("m", "move"),
			keyHint("f", "focus"),
			keyHint("z", "hide done"),
			keyHint("A", "archive"),
			keyHint("u", "undo"),
			keyHint("esc", "back"),
		}
	} else {
		hints = []string{
			keyHint("n", "new"),
			keyHint("a", "add"),
			keyHint("enter", "open"),
			keyHint("c", "colour"),
			keyHint("p", "pin"),
			keyHint("r", "rename"),
			keyHint("d", "delete"),
			keyHint("/", "search"),
			keyHint("s", "stats"),
			keyHint("^e", "→eod"),
			keyHint("?", "help"),
			keyHint("esc", "menu"),
		}
	}
	bar := strings.Join(hints, "  ")

	msg := ""
	switch {
	case t.confirm == tcDeleteNote:
		name := ""
		if c := t.currentCard(); c != nil {
			name = c.Title
		}
		msg = styleWarn.Render(fmt.Sprintf("Delete note %q and its file? [y] yes  [any other key] no", name))
	case t.confirm == tcDeleteTask:
		name := ""
		if f := t.current(); f != nil {
			name = f.task.Text
		}
		msg = styleWarn.Render(fmt.Sprintf("Delete %q? [y] yes  [any other key] no", truncRunes(name, 30)))
	case t.focus != nil:
		focusMsg := styleFocus.Render("⏱ " + clock(time.Since(t.focusStart)) + " " + truncRunes(t.focus.Text, 20))
		if t.status != "" {
			focusMsg += "  " + styleMuted.Render(t.status)
		}
		msg = focusMsg
	case t.status != "":
		if t.statusErr {
			msg = styleError.Render(t.status)
		} else {
			msg = styleSuccess.Render(t.status)
		}
	}

	if msg == "" {
		return styleStatusBar.Render(clipANSI(bar, t.width-2))
	}
	room := t.width - lipgloss.Width(msg) - 4
	bar = clipANSI(bar, room)
	gap := max(t.width-lipgloss.Width(bar)-lipgloss.Width(msg)-4, 1)
	return styleStatusBar.Render(bar + strings.Repeat(" ", gap) + msg)
}

// ── helpers ────────────────────────────────────────────────────────────────

// padTrunc fits plain text to exactly w columns.
func padTrunc(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return pad(truncRunes(s, w), w)
}

func truncRunes(s string, w int) string {
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return string(r[:w-1]) + "…"
}

// clipANSI drops whole styled segments from the end of a hint bar until it
// fits, so the escape sequences are never cut in half.
func clipANSI(bar string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(bar) <= w {
		return bar
	}
	parts := strings.Split(bar, "  ")
	for len(parts) > 1 {
		parts = parts[:len(parts)-1]
		joined := strings.Join(parts, "  ")
		if lipgloss.Width(joined) <= w {
			return joined
		}
	}
	return ""
}

func clock(d time.Duration) string {
	total := int(d.Seconds())
	return fmt.Sprintf("%02d:%02d", total/60, total%60)
}

func todoHelpText() string {
	return `
  Card wall
  ──────────────────────────────────────
  ←/→/↑/↓  h j k l   move between cards
  Enter / Space      open the note
  n                  new note
  r                  rename note
  c                  cycle note colour
  p                  pin / unpin
  d                  delete note (asks first)
  a                  quick add a task

  Inside a note
  ──────────────────────────────────────
  ↑/↓  k j           move between tasks
  Space / Enter / x  check / uncheck
  Shift+↑/↓  K J     reorder
  a                  add task
  o                  add subtask under cursor
  e                  edit task (full inline syntax)
  d                  delete task (asks first)
  m                  move task to another note
  0 1 2 3 / p        set / cycle priority
  f                  start / stop the focus timer
  z                  hide or show finished items
  A                  archive finished items
  ←/h/Esc            back to the wall

  Everywhere
  ──────────────────────────────────────
  / or Ctrl+P        fuzzy search every task
  s                  stats: streak, heat, progress
  u                  undo the last change
  Ctrl+E             push today's finished tasks to EOD
  ?                  this help
  Esc (on the wall)  back to the launcher
  q                  quit

  Quick-add syntax
  ──────────────────────────────────────
  write a task and tack on any of:

    !1 !2 !3     priority (1 = most urgent)
    #tag         tag, repeatable
    ^today       due date — today/ma, tomorrow/holnap,
                 mon…sun or h/k/sze/cs/p/szo/v,
                 3d, 2w, 09-14, 2026-09-14, or 14
    *daily       repeat — daily, weekly, monthly,
                 a weekday, or 3d / 2w
    @notename    put it on that note (creates it if new)

  example:
    review PR !1 #dev ^fri *weekly @work

  A repeating task never vanishes: checking it leaves a
  dated copy behind and re-arms itself for the next date.

  Computed lists
  ──────────────────────────────────────
  Overdue, Today, This week and Done today are built from
  every note. Checking something there writes straight back
  to the note that owns it; editing structure happens in
  the note itself.
`
}
