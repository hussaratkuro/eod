package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/atotto/clipboard"

	"eod/internal/export"
	"eod/internal/model"
	"eod/internal/parser"
	"eod/internal/storage"
)

type viewKind int

const (
	viewMain viewKind = iota
	viewEditor
	viewSummary
	viewSearch
	viewHelp
)

const listWidth = 22

type App struct {
	width, height int
	view          viewKind

	store *storage.Store
	file  *model.EODFile
	files map[string]*model.EODFile

	// dayIdx is the index into file.Days (0=oldest, len-1=newest/today).
	// The list displays days in reverse: today at top, oldest at bottom.
	dayIdx     int
	listScroll int
	detailVP   viewport.Model

	editorTA   textarea.Model
	editorDay  int // actual index into file.Days being edited, -1 = new day
	editorName string
	editorErr  string
	editorUndo []editorSnapshot
	editorRedo []editorSnapshot

	auxVP viewport.Model

	searchTI      textinput.Model
	searchFocus   bool
	searchMatches []int // display indices

	status    string
	statusErr bool

	// confirmDelete is set while the status bar is asking the user to confirm
	// deleting the selected day, so a stray [d] can't wipe an entry.
	confirmDelete bool

	// todoText renders the still-open todos as editor text; the todo side
	// installs it so the EOD editor can pull them in with Ctrl+T.
	todoText func() string
}

func New(store *storage.Store, file *model.EODFile, allFiles []*model.EODFile) *App {
	files := map[string]*model.EODFile{}
	for _, f := range allFiles {
		files[f.Key()] = f
	}
	files[file.Key()] = file

	ta := textarea.New()
	ta.Placeholder = "    - Category:\n        - Task description"
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	// Ctrl+Arrow word navigation
	ta.KeyMap.WordBackward.SetKeys("alt+left", "ctrl+left")
	ta.KeyMap.WordForward.SetKeys("alt+right", "ctrl+right")

	ti := textinput.New()
	ti.Placeholder = "search…"

	a := &App{
		store:    store,
		file:     file,
		files:    files,
		dayIdx:   len(file.Days) - 1,
		detailVP: viewport.New(0, 0),
		auxVP:    viewport.New(0, 0),
		editorTA: ta,
		searchTI: ti,
	}
	if a.dayIdx < 0 {
		a.dayIdx = 0
	}
	return a
}

func (a *App) Init() tea.Cmd { return nil }

// ── display index helpers ─────────────────────────────────────────────────
// The list shows newest day first (display index 0 = file.Days[len-1]).

func (a *App) displayIdx() int {
	return len(a.file.Days) - 1 - a.dayIdx
}

func (a *App) fileIdxFromDisplay(di int) int {
	return len(a.file.Days) - 1 - di
}

// ── Update ─────────────────────────────────────────────────────────────────

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.resizeComponents()
		return a, nil
	case tea.MouseMsg:
		if a.view == viewEditor {
			var cmd tea.Cmd
			a.editorTA, cmd = a.editorTA.Update(msg)
			return a, cmd
		}
	case tea.KeyMsg:
		switch a.view {
		case viewMain:
			return a.updateMain(msg)
		case viewEditor:
			return a.updateEditor(msg)
		case viewSummary, viewHelp:
			return a.updateAux(msg)
		case viewSearch:
			return a.updateSearch(msg)
		}
	default:
		// Ctrl+Shift+Delete arrives as a raw CSI escape sequence that
		// bubbletea does not decode into a named tea.KeyMsg, so it has to be
		// matched here instead of in a normal key.String() switch.
		if a.view == viewEditor && isDeleteAllSequence(msg) {
			before := snapshotEditor(a.editorTA)
			a.clearEditor()
			if a.editorTA.Value() != before.value {
				a.pushUndo(before)
			}
		}
	}
	return a, nil
}

// isDeleteAllSequence reports whether msg is the raw "\x1b[3;6~" CSI
// sequence terminals send for Ctrl+Shift+Delete.
func isDeleteAllSequence(msg tea.Msg) bool {
	v := reflect.ValueOf(msg)
	if v.Kind() != reflect.Slice || v.Type().Elem().Kind() != reflect.Uint8 {
		return false
	}
	if !strings.Contains(v.Type().String(), "CSISequence") {
		return false
	}
	return string(v.Bytes()) == "\x1b[3;6~"
}

func (a *App) resizeComponents() {
	detailW := a.width - listWidth - 3
	detailH := max(a.height-4, 1)
	a.detailVP.Width = detailW
	a.detailVP.Height = detailH
	a.auxVP.Width = a.width - 4
	a.auxVP.Height = a.height - 4
	a.editorTA.SetWidth(a.width - 4)
	a.editorTA.SetHeight(a.height - 6)
	a.searchTI.Width = a.width - 20
}

// ── Main view ──────────────────────────────────────────────────────────────

func (a *App) updateMain(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// While a delete confirmation is pending it swallows every key: only an
	// explicit yes deletes, anything else aborts.
	if a.confirmDelete {
		a.confirmDelete = false
		switch msg.String() {
		case "y", "Y":
			a.deleteCurrentDay()
		default:
			a.setStatus("Delete cancelled.", false)
		}
		return a, nil
	}

	switch msg.String() {
	case "ctrl+c":
		return a, tea.Quit
	case "q":
		return a, tea.Quit
	case "esc":
		return a, toLauncher

	// Up = visually up = toward today (higher dayIdx)
	case "up", "k":
		if a.dayIdx < len(a.file.Days)-1 {
			a.dayIdx++
			a.adjustListScroll()
			a.refreshDetail()
		}

	// Down = visually down = toward oldest (lower dayIdx)
	case "down", "j":
		if a.dayIdx > 0 {
			a.dayIdx--
			a.adjustListScroll()
			a.refreshDetail()
		}

	case "home", "g":
		if len(a.file.Days) > 0 {
			a.dayIdx = len(a.file.Days) - 1
			a.adjustListScroll()
			a.refreshDetail()
		}

	case "end", "G":
		if a.dayIdx != 0 {
			a.dayIdx = 0
			a.adjustListScroll()
			a.refreshDetail()
		}

	case "left", "h":
		a.prevMonth()
	case "right", "l":
		a.nextMonth()

	case "n":
		a.openNewDay()
	case "e":
		a.openEditDay()
	case "d":
		a.askDeleteCurrentDay()
	case "v":
		a.toggleVacation()
	case "y":
		a.copyCurrentDay()

	case "s":
		a.openSummary()
	case "/":
		a.openSearch()
	case "x":
		a.exportMonth()
	case "?":
		a.openHelp()

	case "pgup", "ctrl+u":
		a.detailVP.HalfViewUp()
	case "pgdown", "ctrl+d":
		a.detailVP.HalfViewDown()
	}
	return a, nil
}

func (a *App) adjustListScroll() {
	visibleLines := max(a.height-6, 1)
	di := a.displayIdx()
	if di < a.listScroll {
		a.listScroll = di
	}
	if di >= a.listScroll+visibleLines {
		a.listScroll = di - visibleLines + 1
	}
}

func (a *App) prevMonth() {
	y, m := a.file.Year, a.file.Month-1
	if m < 1 {
		m = 12
		y--
	}
	a.switchMonth(y, m)
}

func (a *App) nextMonth() {
	y, m := a.file.Year, a.file.Month+1
	if m > 12 {
		m = 1
		y++
	}
	a.switchMonth(y, m)
}

func (a *App) switchMonth(year, month int) {
	key := fmt.Sprintf("%04d-%02d", year, month)
	if f, ok := a.files[key]; ok {
		a.file = f
	} else {
		f, err := a.store.Load(year, month)
		if err != nil || f == nil {
			f = a.store.NewFile(year, month)
		}
		a.files[key] = f
		a.file = f
	}
	a.dayIdx = max(len(a.file.Days)-1, 0)
	a.listScroll = 0
	a.refreshDetail()
}

// ── Editor ─────────────────────────────────────────────────────────────────

func (a *App) openNewDay() {
	a.editorDay = -1
	a.editorName = ""
	a.editorErr = ""
	a.editorTA.Reset()
	a.editorTA.SetValue("")
	a.editorTA.Focus()
	a.editorUndo = nil
	a.editorRedo = nil
	a.view = viewEditor
}

// clearEditor wipes the content of the day currently being edited. The
// change is only persisted if the user then saves with Ctrl+S; Esc still
// discards it like any other in-progress edit.
func (a *App) clearEditor() {
	a.editorTA.Reset()
	a.editorTA.SetValue("")
	a.setStatus("Editor cleared.", false)
}

func (a *App) openEditDay() {
	if len(a.file.Days) == 0 {
		a.openNewDay()
		return
	}
	day := a.file.Days[a.dayIdx]
	a.editorDay = a.dayIdx
	a.editorName = day.Name
	a.editorErr = ""
	a.editorTA.Reset()
	a.editorTA.SetValue(parser.SerializeDay(day))
	a.editorTA.Focus()
	a.editorUndo = nil
	a.editorRedo = nil
	a.view = viewEditor
}

// editorSnapshot captures enough of the editor's state to restore it later
// for undo/redo.
type editorSnapshot struct {
	value string
	row   int
	col   int
}

func snapshotEditor(ta textarea.Model) editorSnapshot {
	return editorSnapshot{value: ta.Value(), row: ta.Line(), col: ta.LineInfo().ColumnOffset}
}

// restoreEditor replaces the textarea's content and puts the cursor back
// where the snapshot recorded it.
func restoreEditor(ta *textarea.Model, s editorSnapshot) {
	ta.SetValue(s.value)
	lines := strings.Split(s.value, "\n")
	row := max(0, min(s.row, len(lines)-1))
	for i := 0; i < len(lines)-1-row; i++ {
		ta.CursorUp()
	}
	ta.SetCursor(s.col)
}

// pushUndo records the editor state as it was before an edit, so it can be
// restored later, and drops the redo history since it no longer applies.
func (a *App) pushUndo(before editorSnapshot) {
	a.editorUndo = append(a.editorUndo, before)
	a.editorRedo = nil
}

func (a *App) undoEditor() {
	if len(a.editorUndo) == 0 {
		return
	}
	n := len(a.editorUndo) - 1
	prev := a.editorUndo[n]
	a.editorUndo = a.editorUndo[:n]
	a.editorRedo = append(a.editorRedo, snapshotEditor(a.editorTA))
	restoreEditor(&a.editorTA, prev)
}

func (a *App) redoEditor() {
	if len(a.editorRedo) == 0 {
		return
	}
	n := len(a.editorRedo) - 1
	next := a.editorRedo[n]
	a.editorRedo = a.editorRedo[:n]
	a.editorUndo = append(a.editorUndo, snapshotEditor(a.editorTA))
	restoreEditor(&a.editorTA, next)
}

func (a *App) copyCurrentDay() {
	if len(a.file.Days) == 0 {
		return
	}
	day := a.file.Days[a.dayIdx]
	text := parser.DayHeader(a.file.Month, day) + "\n" + parser.SerializeDay(day)
	if err := clipboard.WriteAll(text); err != nil {
		a.setStatus("Clipboard error: "+err.Error(), true)
	} else {
		a.setStatus("Copied to clipboard.", false)
	}
}

// askDeleteCurrentDay arms the confirmation prompt in the status bar instead
// of deleting straight away.
func (a *App) askDeleteCurrentDay() {
	if len(a.file.Days) == 0 {
		return
	}
	a.status = ""
	a.confirmDelete = true
}

func (a *App) deleteCurrentDay() {
	if len(a.file.Days) == 0 {
		return
	}
	i := a.dayIdx
	a.file.Days = append(a.file.Days[:i], a.file.Days[i+1:]...)
	if a.dayIdx >= len(a.file.Days) && a.dayIdx > 0 {
		a.dayIdx--
	}
	_ = a.store.Save(a.file)
	a.setStatus("Day deleted.", false)
	a.refreshDetail()
}

// pullTodos appends every open todo to the editor, grouped by note, so a day
// entry can start from what is actually outstanding.
func (a *App) pullTodos() {
	if a.todoText == nil {
		return
	}
	text := strings.TrimRight(a.todoText(), "\n")
	if text == "" {
		a.setStatus("No open todos.", false)
		return
	}

	before := snapshotEditor(a.editorTA)
	value := a.editorTA.Value()
	if strings.TrimSpace(value) != "" {
		value = strings.TrimRight(value, "\n") + "\n"
	} else {
		value = ""
	}
	a.editorTA.SetValue(value + text + "\n")
	a.pushUndo(before)
	a.setStatus("Open todos pulled in.", false)
}

// eodGroup is one category worth of items handed over by the todo side.
type eodGroup struct {
	Category string
	Items    []string
}

// appendToToday files items under today's day entry, creating the day (and the
// month's file) when needed. Items already present are skipped, so pushing
// twice in a day is harmless. It returns how many items were actually added.
func (a *App) appendToToday(groups []eodGroup) (int, error) {
	now := time.Now()
	key := fmt.Sprintf("%04d-%02d", now.Year(), int(now.Month()))

	file, ok := a.files[key]
	if !ok {
		loaded, err := a.store.Load(now.Year(), int(now.Month()))
		if err != nil {
			return 0, err
		}
		if loaded == nil {
			loaded = a.store.NewFile(now.Year(), int(now.Month()))
		}
		file = loaded
		a.files[key] = file
	}

	day := findOrCreateDay(file, now.Day())

	added := 0
	for _, g := range groups {
		cat := findOrCreateCategory(day, g.Category)
		for _, text := range g.Items {
			if hasChild(cat, text) {
				continue
			}
			cat.Children = append(cat.Children, &model.Item{Text: text, Depth: 1})
			added++
		}
	}
	if added == 0 {
		return 0, nil
	}

	if err := a.store.Save(file); err != nil {
		return 0, err
	}
	if a.file == file {
		a.refreshDetail()
	}
	return added, nil
}

func findOrCreateDay(file *model.EODFile, dayNum int) *model.DayEntry {
	at := len(file.Days)
	for i, d := range file.Days {
		if d.Day == dayNum {
			return d
		}
		if d.Day > dayNum {
			at = i
			break
		}
	}
	day := &model.DayEntry{Day: dayNum}
	file.Days = append(file.Days, nil)
	copy(file.Days[at+1:], file.Days[at:])
	file.Days[at] = day
	return day
}

func findOrCreateCategory(day *model.DayEntry, name string) *model.Item {
	for _, it := range day.Items {
		if it.IsCategory && strings.EqualFold(it.Text, name) {
			return it
		}
	}
	cat := &model.Item{Text: name, IsCategory: true}
	day.Items = append(day.Items, cat)
	return cat
}

func hasChild(item *model.Item, text string) bool {
	for _, c := range item.Children {
		if strings.EqualFold(strings.TrimSpace(c.Text), strings.TrimSpace(text)) {
			return true
		}
	}
	return false
}

func (a *App) toggleVacation() {
	if len(a.file.Days) == 0 {
		return
	}
	day := a.file.Days[a.dayIdx]
	day.Vacation = day.Vacation.Next()
	_ = a.store.Save(a.file)
	switch day.Vacation {
	case model.VacationFull:
		a.setStatus("Marked as vacation (full day).", false)
	case model.VacationHalf:
		a.setStatus("Marked as vacation (half day).", false)
	default:
		a.setStatus("Vacation mark removed.", false)
	}
	a.refreshDetail()
}

func (a *App) updateEditor(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg.String() {
	case "esc":
		a.editorTA.Blur()
		a.view = viewMain
		a.refreshDetail()
		return a, nil

	case "ctrl+s":
		return a, a.saveEditor()

	case "ctrl+z":
		// Undo. Terminals don't reliably distinguish Ctrl+Shift+Z from
		// Ctrl+Z (shift is lost in the control-byte encoding), so redo is
		// also reachable below via Ctrl+Y, the conventional alternative.
		a.undoEditor()
		return a, nil

	case "ctrl+shift+z", "ctrl+y":
		a.redoEditor()
		return a, nil

	case "ctrl+t":
		a.pullTodos()
		return a, nil
	}

	// Snapshot before any edit so it can be restored by undoEditor. Only
	// keep the snapshot if the keystroke actually changed the content.
	before := snapshotEditor(a.editorTA)
	defer func() {
		if a.editorTA.Value() != before.value {
			a.pushUndo(before)
		}
	}()

	switch msg.String() {
	case "tab":
		// Insert 4 spaces instead of a tab character
		a.editorTA, cmd = a.editorTA.Update(
			tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("    ")},
		)
		return a, cmd

	case "enter":
		// Auto-indent: match the indentation of the current line
		indent := currentLineIndent(a.editorTA)
		a.editorTA, cmd = a.editorTA.Update(
			tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("\n" + indent)},
		)
		return a, cmd

	case "ctrl+h", "alt+backspace":
		// Ctrl+Backspace / Alt+Backspace → delete word backward
		n := wordBackwardLen(a.editorTA)
		for range n {
			a.editorTA, _ = a.editorTA.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		}
		return a, nil

	case "ctrl+delete", "alt+d":
		// Ctrl+Delete / Alt+D → delete word forward
		n := wordForwardLen(a.editorTA)
		for range n {
			a.editorTA, _ = a.editorTA.Update(tea.KeyMsg{Type: tea.KeyDelete})
		}
		return a, nil

	}

	a.editorTA, cmd = a.editorTA.Update(msg)
	return a, cmd
}

// wordBackwardLen returns how many runes to delete to remove the word before
// the cursor. If the cursor sits at the start of a line, it returns 1 so the
// caller's Backspace merges it with the previous line instead of stalling;
// the next call then continues eating the previous line's trailing word.
func wordBackwardLen(ta textarea.Model) int {
	lines := strings.Split(ta.Value(), "\n")
	li := ta.Line()
	if li >= len(lines) {
		return 0
	}
	pos := ta.LineInfo().CharOffset
	if pos == 0 {
		if li == 0 {
			return 0
		}
		return 1
	}
	runes := []rune(lines[li])
	start := pos
	// skip trailing spaces
	for start > 0 && runes[start-1] == ' ' {
		start--
	}
	// skip word characters
	for start > 0 && runes[start-1] != ' ' {
		start--
	}
	return pos - start
}

// wordForwardLen returns how many runes to delete to remove the word after
// the cursor. If the cursor sits at the end of a line, it returns 1 so the
// caller's Delete merges it with the next line instead of stalling; the next
// call then continues eating the next line's leading word.
func wordForwardLen(ta textarea.Model) int {
	lines := strings.Split(ta.Value(), "\n")
	li := ta.Line()
	if li >= len(lines) {
		return 0
	}
	runes := []rune(lines[li])
	pos := ta.LineInfo().CharOffset
	if pos >= len(runes) {
		if li == len(lines)-1 {
			return 0
		}
		return 1
	}
	end := pos
	// skip leading spaces
	for end < len(runes) && runes[end] == ' ' {
		end++
	}
	// skip word characters
	for end < len(runes) && runes[end] != ' ' {
		end++
	}
	return end - pos
}

// currentLineIndent returns the leading spaces of the line the cursor is on.
func currentLineIndent(ta textarea.Model) string {
	lines := strings.Split(ta.Value(), "\n")
	li := ta.Line()
	if li >= len(lines) {
		return ""
	}
	line := lines[li]
	indent := ""
	for _, ch := range line {
		if ch == ' ' {
			indent += " "
		} else {
			break
		}
	}
	return indent
}

func (a *App) saveEditor() tea.Cmd {
	raw := a.editorTA.Value()
	items := parser.ParseDayItems(raw)
	now := time.Now()

	if a.editorDay == -1 {
		dayNum := now.Day()
		if a.file.Year != now.Year() || a.file.Month != int(now.Month()) {
			if len(a.file.Days) > 0 {
				dayNum = a.file.Days[len(a.file.Days)-1].Day + 1
			} else {
				dayNum = 1
			}
		}
		day := &model.DayEntry{
			Day:   dayNum,
			Name:  strings.TrimSpace(a.editorName),
			Items: items,
		}
		inserted := false
		for i, d := range a.file.Days {
			if d.Day == dayNum {
				a.file.Days[i] = day
				a.dayIdx = i
				inserted = true
				break
			}
			if d.Day > dayNum {
				newDays := make([]*model.DayEntry, 0, len(a.file.Days)+1)
				newDays = append(newDays, a.file.Days[:i]...)
				newDays = append(newDays, day)
				newDays = append(newDays, a.file.Days[i:]...)
				a.file.Days = newDays
				a.dayIdx = i
				inserted = true
				break
			}
		}
		if !inserted {
			a.file.Days = append(a.file.Days, day)
			a.dayIdx = len(a.file.Days) - 1
		}
	} else {
		day := a.file.Days[a.editorDay]
		day.Items = items
		day.Name = strings.TrimSpace(a.editorName)
	}

	if err := a.store.Save(a.file); err != nil {
		a.editorErr = err.Error()
		return nil
	}

	a.editorTA.Blur()
	a.view = viewMain
	a.setStatus("Saved.", false)
	a.refreshDetail()
	return nil
}

// ── Summary / Help ─────────────────────────────────────────────────────────

func (a *App) openSummary() {
	a.auxVP.SetContent(export.Summary(a.file))
	a.auxVP.GotoTop()
	a.view = viewSummary
}

func (a *App) openHelp() {
	a.auxVP.SetContent(helpText())
	a.auxVP.GotoTop()
	a.view = viewHelp
}

func (a *App) updateAux(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc", "?":
		a.view = viewMain
		return a, nil
	}
	var cmd tea.Cmd
	a.auxVP, cmd = a.auxVP.Update(msg)
	return a, cmd
}

// ── Search ─────────────────────────────────────────────────────────────────

func (a *App) openSearch() {
	a.searchTI.Reset()
	a.searchTI.Focus()
	a.searchMatches = nil
	a.searchFocus = true
	a.view = viewSearch
}

func (a *App) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.searchTI.Blur()
		a.view = viewMain
		a.refreshDetail()
		return a, nil
	case "enter":
		if len(a.searchMatches) > 0 {
			// searchMatches stores display indices
			di := a.searchMatches[0]
			a.dayIdx = a.fileIdxFromDisplay(di)
			a.searchTI.Blur()
			a.view = viewMain
			a.adjustListScroll()
			a.refreshDetail()
		}
		return a, nil
	}

	var cmd tea.Cmd
	a.searchTI, cmd = a.searchTI.Update(msg)
	a.runSearch()
	return a, cmd
}

func (a *App) runSearch() {
	q := strings.ToLower(a.searchTI.Value())
	a.searchMatches = nil
	if q == "" {
		return
	}
	// Iterate in display order (newest first)
	for di := 0; di < len(a.file.Days); di++ {
		day := a.file.Days[a.fileIdxFromDisplay(di)]
		if strings.Contains(strings.ToLower(dayText(day)), q) {
			a.searchMatches = append(a.searchMatches, di)
		}
	}
}

func dayText(day *model.DayEntry) string {
	var sb strings.Builder
	for _, item := range day.Items {
		itemText(&sb, item)
	}
	return sb.String()
}

func itemText(sb *strings.Builder, item *model.Item) {
	sb.WriteString(item.Text)
	sb.WriteString(" ")
	for _, c := range item.Children {
		itemText(sb, c)
	}
}

// ── Export ─────────────────────────────────────────────────────────────────

func (a *App) exportMonth() {
	dir := a.store.Dir
	mdPath := filepath.Join(dir, fmt.Sprintf("eod_%04d_%02d.md", a.file.Year, a.file.Month))
	csvPath := filepath.Join(dir, fmt.Sprintf("eod_%04d_%02d.csv", a.file.Year, a.file.Month))

	var errs []string
	if err := os.WriteFile(mdPath, []byte(export.Markdown(a.file)), 0o644); err != nil {
		errs = append(errs, err.Error())
	}
	if err := os.WriteFile(csvPath, []byte(export.CSV(a.file)), 0o644); err != nil {
		errs = append(errs, err.Error())
	}

	if len(errs) > 0 {
		a.setStatus("Export error: "+strings.Join(errs, ", "), true)
	} else {
		a.setStatus(fmt.Sprintf("Exported: %s, %s", filepath.Base(mdPath), filepath.Base(csvPath)), false)
	}
}

// ── View ───────────────────────────────────────────────────────────────────

func (a *App) View() string {
	if a.width == 0 {
		return "Loading…"
	}
	switch a.view {
	case viewEditor:
		return a.viewEditor()
	case viewSummary:
		return a.viewAux("Summary")
	case viewHelp:
		return a.viewAux("Help")
	case viewSearch:
		return a.viewSearchScreen()
	}
	return a.viewMain()
}

func (a *App) viewMain() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		a.renderHeader(),
		lipgloss.JoinHorizontal(lipgloss.Top, a.renderDayList(), a.renderDayDetail()),
		a.renderStatusBar(),
	)
}

func (a *App) renderHeader() string {
	title := styleHeader.Render(a.file.Title())
	nav := styleMuted.Render("[←/→] month  [?] help")
	gap := max(a.width-lipgloss.Width(title)-lipgloss.Width(nav)-2, 1)
	return title + strings.Repeat(" ", gap) + nav
}

func (a *App) renderDayList() string {
	listH := max(a.height-4, 1)

	title := stylePanelTitle.Render("Days")
	var rows []string

	matchSet := map[int]bool{}
	for _, di := range a.searchMatches {
		matchSet[di] = true
	}

	// Iterate in display order: display index 0 = newest (today) at top
	for di := 0; di < len(a.file.Days); di++ {
		if di < a.listScroll {
			continue
		}
		if len(rows) >= listH-1 {
			break
		}
		day := a.file.Days[a.fileIdxFromDisplay(di)]

		label := fmt.Sprintf("%02d.%02d", a.file.Month, day.Day)
		switch day.Vacation {
		case model.VacationFull:
			label += " " + styleVacation.Render("☀")
		case model.VacationHalf:
			label += " " + styleVacation.Render("◑")
		default:
			if day.Name != "" {
				label += " " + styleMuted.Render("("+day.Name+")")
			}
		}
		if a.view == viewSearch && matchSet[di] {
			label = styleSuccess.Render("* ") + label
		}

		selected := di == a.displayIdx()
		if selected {
			rows = append(rows, styleSelected.Render(pad(label, listWidth-2)))
		} else if day.Vacation != model.VacationNone {
			rows = append(rows, styleVacationRow.Render(pad(label, listWidth-2)))
		} else {
			rows = append(rows, styleNormal.Render(pad(label, listWidth-2)))
		}
	}

	if len(a.file.Days) == 0 {
		rows = append(rows, styleMuted.Render("  (empty)"))
	}
	for len(rows) < listH-1 {
		rows = append(rows, styleNormal.Render(strings.Repeat(" ", listWidth-2)))
	}

	return styleBorder.Width(listWidth).Height(listH).Render(
		title + "\n" + strings.Join(rows, "\n"),
	)
}

func (a *App) renderDayDetail() string {
	detailW := max(a.width-listWidth-3, 10)
	detailH := a.height - 4

	if len(a.file.Days) == 0 {
		return styleBorder.Width(detailW).Height(detailH).Render(
			styleMuted.Render("\n  No days yet. Press [n] to create one."),
		)
	}

	day := a.file.Days[a.dayIdx]
	title := stylePanelTitle.Render(fmt.Sprintf("%d.%02d.%02d", a.file.Year, a.file.Month, day.Day))
	switch day.Vacation {
	case model.VacationFull:
		title += "  " + styleVacation.Render("☀ vacation")
	case model.VacationHalf:
		title += "  " + styleVacation.Render("◑ half-day vacation")
	default:
		if day.Name != "" {
			title += " " + styleMuted.Render("("+day.Name+")")
		}
	}

	a.detailVP.Width = detailW - 2
	a.detailVP.Height = detailH - 2
	a.detailVP.SetContent(a.renderDayItems(day))

	return styleBorder.Width(detailW).Height(detailH).Render(
		title + "\n" + strings.Repeat("─", detailW-2) + "\n" + a.detailVP.View(),
	)
}

func (a *App) renderDayItems(day *model.DayEntry) string {
	if len(day.Items) == 0 {
		switch day.Vacation {
		case model.VacationFull:
			return styleVacation.Render("  ☀ Vacation")
		case model.VacationHalf:
			return styleVacation.Render("  ◑ Half-day vacation")
		}
		return styleMuted.Render("  (empty day — press [e] to edit)")
	}
	var sb strings.Builder
	for _, item := range day.Items {
		renderItem(&sb, item, 0)
	}
	return sb.String()
}

func renderItem(sb *strings.Builder, item *model.Item, depth int) {
	indent := strings.Repeat("  ", depth)
	if item.IsCategory {
		sb.WriteString(indent + styleCategory.Render(styleCategoryIcon+item.Text) + "\n")
	} else {
		sb.WriteString(indent + styleMuted.Render(styleItemIcon) + item.Text + "\n")
	}
	for _, child := range item.Children {
		renderItem(sb, child, depth+1)
	}
}

func (a *App) renderStatusBar() string {
	hints := []string{
		keyHint("n", "new"),
		keyHint("e", "edit"),
		keyHint("d", "delete"),
		keyHint("y", "copy"),
		keyHint("v", "vacation"),
		keyHint("s", "summary"),
		keyHint("/", "search"),
		keyHint("x", "export"),
		keyHint("q", "quit"),
	}
	bar := strings.Join(hints, "  ")

	if a.confirmDelete && len(a.file.Days) > 0 {
		day := a.file.Days[a.dayIdx]
		msg := styleWarn.Render(fmt.Sprintf(
			"Delete %d.%02d.%02d? [y] yes  [any other key] no",
			a.file.Year, a.file.Month, day.Day))
		gap := max(a.width-lipgloss.Width(bar)-lipgloss.Width(msg)-4, 1)
		return styleStatusBar.Render(bar + strings.Repeat(" ", gap) + msg)
	}

	if a.status != "" {
		msg := a.status
		if a.statusErr {
			msg = styleError.Render(msg)
		} else {
			msg = styleSuccess.Render(msg)
		}
		gap := max(a.width-lipgloss.Width(bar)-lipgloss.Width(msg)-4, 1)
		bar = bar + strings.Repeat(" ", gap) + msg
	}
	return styleStatusBar.Render(bar)
}

func (a *App) viewEditor() string {
	var title string
	if a.editorDay == -1 {
		now := time.Now()
		dayNum := now.Day()
		if a.file.Year != now.Year() || a.file.Month != int(now.Month()) {
			if len(a.file.Days) > 0 {
				dayNum = a.file.Days[len(a.file.Days)-1].Day + 1
			} else {
				dayNum = 1
			}
		}
		title = styleEditorTitle.Render(fmt.Sprintf("New day: %d.%02d.%02d", a.file.Year, a.file.Month, dayNum))
	} else {
		day := a.file.Days[a.editorDay]
		title = styleEditorTitle.Render(fmt.Sprintf("Edit: %d.%02d.%02d", a.file.Year, a.file.Month, day.Day))
	}

	hint := styleHelp.Render("Ctrl+S: save  Esc: cancel  Tab: 4 spaces  Enter: auto-indent  Ctrl+T: pull open todos  Ctrl+Shift+Delete: clear all  Ctrl+Z: undo  Ctrl+Y: redo")

	errLine := ""
	if a.editorErr != "" {
		errLine = "\n" + styleError.Render(a.editorErr)
	}

	return lipgloss.JoinVertical(lipgloss.Left, title, hint, errLine, a.editorTA.View())
}

func (a *App) viewAux(title string) string {
	hdr := styleHeader.Render(title) + "  " + styleMuted.Render("[q/Esc] back")
	return lipgloss.JoinVertical(lipgloss.Left,
		hdr,
		strings.Repeat("─", a.width),
		a.auxVP.View(),
	)
}

func (a *App) viewSearchScreen() string {
	hdr := styleHeader.Render("Search") + "  " + styleMuted.Render("[Esc] back  [Enter] jump")
	matches := ""
	if q := a.searchTI.Value(); q != "" {
		if len(a.searchMatches) == 0 {
			matches = styleError.Render("  No results")
		} else {
			matches = styleSuccess.Render(fmt.Sprintf("  %d result(s)", len(a.searchMatches)))
		}
	}
	panels := lipgloss.JoinHorizontal(lipgloss.Top, a.renderDayList(), a.renderDayDetail())
	return lipgloss.JoinVertical(lipgloss.Left,
		hdr,
		a.searchTI.View()+matches,
		panels,
	)
}

// ── Misc ───────────────────────────────────────────────────────────────────

func (a *App) refreshDetail() {
	if len(a.file.Days) == 0 {
		a.detailVP.SetContent(styleMuted.Render("(empty)"))
		return
	}
	if a.dayIdx >= len(a.file.Days) {
		a.dayIdx = len(a.file.Days) - 1
	}
	a.detailVP.SetContent(a.renderDayItems(a.file.Days[a.dayIdx]))
	a.detailVP.GotoTop()
}

func (a *App) setStatus(msg string, isErr bool) {
	a.status = msg
	a.statusErr = isErr
}

func pad(s string, n int) string {
	w := utf8.RuneCountInString(s)
	if w >= n {
		return s
	}
	return s + strings.Repeat(" ", n-w)
}

func helpText() string {
	return `
  Navigation
  ──────────────────────────────────────
  k / ↑       newer day (up in list)
  j / ↓       older day (down in list)
  h / ←       previous month
  l / →       next month
  PgUp/Ctrl+U scroll detail up
  PgDn/Ctrl+D scroll detail down

  Editing
  ──────────────────────────────────────
  n           new day (today's date)
  e           edit current day
  d           delete current day (asks [y] to confirm)
  v           toggle vacation (☀)

  Editor keys
  ──────────────────────────────────────
  Tab         insert 4 spaces
  Enter       new line with auto-indent
  Ctrl+S      save
  Ctrl+T      pull open todos in
  Esc         cancel

  Format (same as the txt file):
    - Category:
        - Task description
        - Another task

  Other
  ──────────────────────────────────────
	Ctrl+Shift+P fuzzy command palette
  s           monthly summary
  /           search in month
  x           export (md + csv)
  ?           this help
  Esc         back to the launcher
  q / Ctrl+C  quit
`
}
