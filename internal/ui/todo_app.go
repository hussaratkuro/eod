package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"eod/internal/todo"
)

type todoView int

const (
	tvWall todoView = iota
	tvNote
	tvInput
	tvPalette
	tvMove
	tvSummary
	tvHelp
)

type inputKind int

const (
	inCapture inputKind = iota
	inSubtask
	inEditTask
	inNewNote
	inRenameNote
)

type todoConfirm int

const (
	tcNone todoConfirm = iota
	tcDeleteNote
	tcDeleteTask
)

// flatTask is one visible row of the open note. list/idx point at the slice
// the task really lives in, so rows can be reordered or removed in place.
type flatTask struct {
	task  *todo.Task
	depth int
	list  *[]*todo.Task
	idx   int
	owner *todo.Note
}

// todoSnapshot is the undo unit: the markdown of every note file at one point
// in time. Notes are small, so snapshotting the whole board keeps undo simple
// and correct even for cross-note operations like moving a task.
type todoSnapshot struct {
	label string
	files map[string]string
}

type todoTickMsg time.Time

func todoTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return todoTickMsg(t) })
}

// TodoApp is the sticky-note checklist side of the program.
type TodoApp struct {
	width, height int

	store *todo.Store
	board *todo.Board

	view    todoView
	cards   []*todo.Note
	cardIdx int
	cols    int
	rowTop  int

	note     *todo.Note
	flat     []flatTask
	taskIdx  int
	vp       viewport.Model
	hideDone bool

	input     textinput.Model
	inputKind inputKind
	inputTask *todo.Task
	returnTo  todoView

	matches  []todo.Ref
	matchIdx int

	moveTargets []*todo.Note
	moveIdx     int

	focus      *todo.Task
	focusOwner *todo.Note
	focusStart time.Time
	now        time.Time

	undo    []todoSnapshot
	confirm todoConfirm

	status    string
	statusErr bool

	eodAppend func([]eodGroup) (int, error)
}

func NewTodoApp(store *todo.Store) (*TodoApp, error) {
	now := time.Now()
	board, err := store.Load(now)
	if err != nil {
		return nil, err
	}

	ti := textinput.New()
	ti.Prompt = "› "

	t := &TodoApp{
		store: store,
		board: board,
		now:   now,
		vp:    viewport.New(0, 0),
		input: ti,
	}
	t.rebuildCards()
	return t, nil
}

// Enter re-reads the board from disk, so edits made in an editor (or by the
// EOD side) show up whenever the launcher hands control over.
func (t *TodoApp) Enter() {
	t.now = time.Now()
	if b, err := t.store.Load(t.now); err == nil {
		t.board = b
	}
	t.view = tvWall
	t.note = nil
	t.status = ""
	t.confirm = tcNone
	t.rebuildCards()
}

func (t *TodoApp) Init() tea.Cmd { return nil }

func (t *TodoApp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		t.width, t.height = m.Width, m.Height
		t.resize()
		return t, nil

	case todoTickMsg:
		t.now = time.Time(m)
		if t.focus != nil {
			return t, todoTick()
		}
		return t, nil

	case tea.KeyMsg:
		switch t.view {
		case tvWall:
			return t.updateWall(m)
		case tvNote:
			return t.updateNote(m)
		case tvInput:
			return t.updateInput(m)
		case tvPalette:
			return t.updatePalette(m)
		case tvMove:
			return t.updateMove(m)
		case tvSummary, tvHelp:
			return t.updateTodoAux(m)
		}
	}
	return t, nil
}

func (t *TodoApp) resize() {
	t.cols = max((t.width-1)/(cardWidth+1), 1)
	t.vp.Width = max(t.width-2, 10)
	t.vp.Height = max(t.height-5, 3)
	t.input.Width = max(t.width-6, 10)
}

// ── shared state helpers ───────────────────────────────────────────────────

func (t *TodoApp) setStatus(msg string, isErr bool) {
	t.status = msg
	t.statusErr = isErr
}

func (t *TodoApp) currentCard() *todo.Note {
	if t.cardIdx < 0 || t.cardIdx >= len(t.cards) {
		return nil
	}
	return t.cards[t.cardIdx]
}

func (t *TodoApp) current() *flatTask {
	if t.taskIdx < 0 || t.taskIdx >= len(t.flat) {
		return nil
	}
	return &t.flat[t.taskIdx]
}

func (t *TodoApp) rebuildCards() {
	sel := t.currentCard()
	t.cards = nil
	t.cards = append(t.cards, todo.Virtuals(t.board, t.now)...)
	t.cards = append(t.cards, t.board.Notes...)

	if sel != nil {
		for i, c := range t.cards {
			if c == sel || (sel.Virtual && c.Virtual && c.Title == sel.Title) || (!sel.Virtual && c.Slug == sel.Slug) {
				t.cardIdx = i
				break
			}
		}
	}
	if t.cardIdx >= len(t.cards) {
		t.cardIdx = max(len(t.cards)-1, 0)
	}
}

func (t *TodoApp) rebuildFlat() {
	t.flat = nil
	if t.note == nil {
		return
	}

	if t.note.Flat {
		for i := range t.note.Tasks {
			task := t.note.Tasks[i]
			t.flat = append(t.flat, flatTask{
				task:  task,
				list:  &t.note.Tasks,
				idx:   i,
				owner: t.board.OwnerOf(task),
			})
		}
	} else {
		var walk func(list *[]*todo.Task, depth int)
		walk = func(list *[]*todo.Task, depth int) {
			for i := range *list {
				task := (*list)[i]
				if t.hideDone && task.Done {
					continue
				}
				t.flat = append(t.flat, flatTask{task: task, depth: depth, list: list, idx: i, owner: t.note})
				if len(task.Children) > 0 {
					walk(&task.Children, depth+1)
				}
			}
		}
		walk(&t.note.Tasks, 0)
	}

	if t.taskIdx >= len(t.flat) {
		t.taskIdx = max(len(t.flat)-1, 0)
	}
}

// refresh recomputes the virtual cards and the open note's rows after a change.
func (t *TodoApp) refresh() {
	title := ""
	if t.note != nil && t.note.Virtual {
		title = t.note.Title
	}
	t.rebuildCards()

	if title != "" {
		t.note = nil
		for _, c := range t.cards {
			if c.Virtual && c.Title == title {
				t.note = c
			}
		}
		if t.note == nil {
			// The virtual list emptied out from under us.
			t.view = tvWall
		}
	}
	t.rebuildFlat()
}

func (t *TodoApp) selectTask(task *todo.Task) {
	for i := range t.flat {
		if t.flat[i].task == task {
			t.taskIdx = i
			return
		}
	}
}

func (t *TodoApp) save(n *todo.Note) {
	if n == nil || n.Virtual {
		return
	}
	todo.Normalize(n.Tasks)
	if err := t.store.Save(n); err != nil {
		t.setStatus("Save error: "+err.Error(), true)
	}
}

// defaultNote is where a task goes when the user did not name a note.
func (t *TodoApp) defaultNote() *todo.Note {
	if t.note != nil && !t.note.Virtual {
		return t.note
	}
	if c := t.currentCard(); c != nil && !c.Virtual {
		return c
	}
	if len(t.board.Notes) > 0 {
		return t.board.Notes[0]
	}
	return t.createNote("Inbox")
}

func (t *TodoApp) createNote(title string) *todo.Note {
	n := &todo.Note{
		Title: title,
		Slug:  t.board.UniqueSlug(todo.Slugify(title)),
		Color: accentFor(len(t.board.Notes)),
	}
	t.board.Notes = append(t.board.Notes, n)
	t.board.Sort()
	t.save(n)
	return n
}

// ── undo ───────────────────────────────────────────────────────────────────

func (t *TodoApp) pushUndo(label string) {
	snap := todoSnapshot{label: label, files: map[string]string{}}
	for _, n := range t.board.Notes {
		if n.Path == "" {
			continue
		}
		snap.files[n.Path] = todo.SerializeNote(n)
	}
	t.undo = append(t.undo, snap)
	if len(t.undo) > 20 {
		t.undo = t.undo[1:]
	}
}

func (t *TodoApp) undoLast() {
	if len(t.undo) == 0 {
		t.setStatus("Nothing to undo.", false)
		return
	}
	s := t.undo[len(t.undo)-1]
	t.undo = t.undo[:len(t.undo)-1]

	if err := t.store.Restore(s.files); err != nil {
		t.setStatus("Undo failed: "+err.Error(), true)
		return
	}
	t.reload()
	t.setStatus("Undone: "+s.label, false)
}

// reload re-reads the board while keeping the open note and selection.
func (t *TodoApp) reload() {
	slug := ""
	virtual := ""
	if t.note != nil {
		if t.note.Virtual {
			virtual = t.note.Title
		} else {
			slug = t.note.Slug
		}
	}

	b, err := t.store.Load(t.now)
	if err != nil {
		t.setStatus("Load error: "+err.Error(), true)
		return
	}
	t.board = b
	t.focus = nil
	t.rebuildCards()

	t.note = nil
	switch {
	case slug != "":
		t.note = t.board.BySlug(slug)
	case virtual != "":
		for _, c := range t.cards {
			if c.Virtual && c.Title == virtual {
				t.note = c
			}
		}
	}
	if t.note == nil && t.view == tvNote {
		t.view = tvWall
	}
	t.rebuildFlat()
}

// ── wall ───────────────────────────────────────────────────────────────────

func (t *TodoApp) updateWall(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if t.confirm != tcNone {
		return t.resolveConfirm(msg)
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return t, tea.Quit
	case "esc":
		return t, toLauncher

	case "left", "h":
		t.moveCard(-1)
	case "right", "l":
		t.moveCard(1)
	case "up", "k":
		t.moveCard(-t.cols)
	case "down", "j":
		t.moveCard(t.cols)
	case "home", "g":
		t.cardIdx = 0
	case "end", "G":
		t.cardIdx = max(len(t.cards)-1, 0)

	case "enter", " ":
		if c := t.currentCard(); c != nil {
			t.note = c
			t.taskIdx = 0
			t.rebuildFlat()
			t.view = tvNote
		}

	case "n":
		t.openInput(inNewNote, "New note title", "")
	case "r":
		if c := t.currentCard(); c != nil && !c.Virtual {
			t.openInput(inRenameNote, "Rename note", c.Title)
		} else {
			t.setStatus("Computed lists cannot be renamed.", true)
		}
	case "c":
		if c := t.currentCard(); c != nil && !c.Virtual {
			t.pushUndo("colour")
			c.Color = nextAccent(c.Color)
			t.save(c)
			t.setStatus("Colour: "+c.Color, false)
		}
	case "P", "p":
		if c := t.currentCard(); c != nil && !c.Virtual {
			t.pushUndo("pin")
			c.Pinned = !c.Pinned
			t.save(c)
			t.board.Sort()
			t.refresh()
			if c.Pinned {
				t.setStatus("Pinned.", false)
			} else {
				t.setStatus("Unpinned.", false)
			}
		}
	case "d":
		if c := t.currentCard(); c != nil && !c.Virtual {
			t.confirm = tcDeleteNote
			t.status = ""
		} else {
			t.setStatus("Computed lists cannot be deleted.", true)
		}

	case "a":
		t.openInput(inCapture, "Add task", "")

	default:
		return t.commonKeys(msg)
	}
	return t, nil
}

func (t *TodoApp) moveCard(delta int) {
	if len(t.cards) == 0 {
		return
	}
	i := t.cardIdx + delta
	if i < 0 {
		i = 0
	}
	if i >= len(t.cards) {
		i = len(t.cards) - 1
	}
	t.cardIdx = i
}

// ── note ───────────────────────────────────────────────────────────────────

func (t *TodoApp) updateNote(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if t.confirm != tcNone {
		return t.resolveConfirm(msg)
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return t, tea.Quit
	case "esc", "backspace", "left", "h":
		t.view = tvWall
		t.note = nil
		return t, nil

	case "up", "k":
		if t.taskIdx > 0 {
			t.taskIdx--
		}
	case "down", "j":
		if t.taskIdx < len(t.flat)-1 {
			t.taskIdx++
		}
	case "home", "g":
		t.taskIdx = 0
	case "end", "G":
		t.taskIdx = max(len(t.flat)-1, 0)
	case "pgup", "ctrl+u":
		t.taskIdx = max(t.taskIdx-t.vp.Height/2, 0)
	case "pgdown", "ctrl+d":
		t.taskIdx = min(t.taskIdx+t.vp.Height/2, max(len(t.flat)-1, 0))

	case "shift+up", "K":
		t.reorder(-1)
	case "shift+down", "J":
		t.reorder(1)

	case " ", "enter", "x":
		t.toggleCurrent()

	case "a":
		t.openInput(inCapture, "Add task", "")
	case "o":
		if t.requireReal() {
			t.openInput(inSubtask, "Add subtask", "")
		}
	case "e":
		if f := t.current(); f != nil && t.requireReal() {
			t.openInput(inEditTask, "Edit task", todo.TaskInline(f.task))
		}
	case "d":
		if t.current() != nil && t.requireReal() {
			t.confirm = tcDeleteTask
			t.status = ""
		}
	case "m":
		if t.current() != nil {
			t.openMove()
		}
	case "z":
		t.hideDone = !t.hideDone
		t.rebuildFlat()
		if t.hideDone {
			t.setStatus("Done items hidden.", false)
		} else {
			t.setStatus("Done items shown.", false)
		}
	case "A":
		t.archiveDone()

	case "0", "1", "2", "3":
		t.setPriority(int(msg.String()[0] - '0'))
	case "p":
		if f := t.current(); f != nil {
			t.setPriority((f.task.Priority + 1) % 4)
		}
	case "f":
		t.toggleFocus()

	default:
		return t.commonKeys(msg)
	}

	t.syncViewport()
	return t, nil
}

// requireReal guards edits that make no sense on a computed list.
func (t *TodoApp) requireReal() bool {
	if t.note != nil && t.note.Virtual {
		t.setStatus("Open the note itself to edit this item.", true)
		return false
	}
	return true
}

func (t *TodoApp) syncViewport() {
	if t.taskIdx < t.vp.YOffset {
		t.vp.YOffset = t.taskIdx
	}
	if t.taskIdx >= t.vp.YOffset+t.vp.Height {
		t.vp.YOffset = t.taskIdx - t.vp.Height + 1
	}
	if t.vp.YOffset < 0 {
		t.vp.YOffset = 0
	}
}

// commonKeys are the shortcuts that work on both the wall and inside a note.
func (t *TodoApp) commonKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "u":
		t.undoLast()
	case "s":
		t.vp.SetContent(todo.Summary(t.board, t.now))
		t.vp.GotoTop()
		t.view = tvSummary
	case "?":
		t.vp.SetContent(todoHelpText())
		t.vp.GotoTop()
		t.view = tvHelp
	case "/", "ctrl+p":
		t.openPalette()
	case "ctrl+e":
		t.pushEOD()
	}
	return t, nil
}

func (t *TodoApp) toggleCurrent() {
	f := t.current()
	if f == nil {
		return
	}
	owner := f.owner
	if owner == nil {
		owner = t.note
	}
	if owner == nil || owner.Virtual {
		return
	}

	t.pushUndo("toggle")
	task := f.task

	// A repeating task never disappears: it leaves a dated copy behind as the
	// record of this round and re-arms itself for the next occurrence.
	if !task.Done && task.Repeat != "" {
		record := task.Clone()
		record.Repeat = ""
		record.Children = nil
		record.SetDone(true, t.now)

		base := t.now
		if task.Due != nil && task.Due.After(t.now) {
			base = *task.Due
		}
		if next, ok := todo.NextOccurrence(task.Repeat, base); ok {
			task.Due = &next
		}

		if t.note.Flat {
			owner.Tasks = append(owner.Tasks, record)
		} else {
			list := *f.list
			list = append(list, nil)
			copy(list[f.idx+2:], list[f.idx+1:])
			list[f.idx+1] = record
			*f.list = list
		}
		t.save(owner)
		t.refresh()
		t.selectTask(task)
		t.setStatus("Done — repeats "+task.Repeat+", next "+todo.DueLabel(task, t.now)+".", false)
		return
	}

	task.SetDone(!task.Done, t.now)
	t.save(owner)
	t.refresh()
	t.selectTask(task)
	if task.Done {
		t.setStatus("Checked.", false)
	} else {
		t.setStatus("Unchecked.", false)
	}
}

func (t *TodoApp) setPriority(p int) {
	f := t.current()
	if f == nil {
		return
	}
	owner := f.owner
	if owner == nil || owner.Virtual {
		return
	}
	t.pushUndo("priority")
	f.task.Priority = p
	t.save(owner)
	t.refresh()
	t.selectTask(f.task)
	if p == todo.PrioNone {
		t.setStatus("Priority cleared.", false)
	} else {
		t.setStatus(fmt.Sprintf("Priority !%d.", p), false)
	}
}

func (t *TodoApp) reorder(delta int) {
	f := t.current()
	if f == nil || !t.requireReal() {
		return
	}
	list := *f.list
	j := f.idx + delta
	if j < 0 || j >= len(list) {
		return
	}
	t.pushUndo("reorder")
	list[f.idx], list[j] = list[j], list[f.idx]
	task := list[j]
	t.save(t.note)
	t.rebuildFlat()
	t.selectTask(task)
	t.syncViewport()
}

func (t *TodoApp) deleteTask() {
	f := t.current()
	if f == nil {
		return
	}
	t.pushUndo("delete task")
	list := *f.list
	*f.list = append(list[:f.idx], list[f.idx+1:]...)
	t.save(t.note)
	t.refresh()
	t.setStatus("Task deleted.", false)
}

func (t *TodoApp) deleteNote() {
	c := t.currentCard()
	if c == nil || c.Virtual {
		return
	}
	t.pushUndo("delete note")
	if err := t.store.Delete(c); err != nil {
		t.setStatus("Delete error: "+err.Error(), true)
		return
	}
	for i, n := range t.board.Notes {
		if n == c {
			t.board.Notes = append(t.board.Notes[:i], t.board.Notes[i+1:]...)
			break
		}
	}
	if t.note == c {
		t.note = nil
		t.view = tvWall
	}
	t.refresh()
	t.setStatus("Note deleted — [u] undoes it.", false)
}

func (t *TodoApp) archiveDone() {
	if t.note == nil || !t.requireReal() {
		return
	}
	var moved, keep []*todo.Task
	for _, task := range t.note.Tasks {
		d, n := task.Counts()
		if task.Done && d == n {
			moved = append(moved, task)
			continue
		}
		keep = append(keep, task)
	}
	if len(moved) == 0 {
		t.setStatus("Nothing finished to archive.", false)
		return
	}

	t.pushUndo("archive")
	if err := t.store.ArchiveTasks(t.note, moved, t.now); err != nil {
		t.setStatus("Archive error: "+err.Error(), true)
		return
	}
	t.note.Tasks = keep
	t.save(t.note)
	t.refresh()
	t.setStatus(fmt.Sprintf("Archived %d item(s) to archive/%s.md", len(moved), t.note.Slug), false)
}

func (t *TodoApp) toggleFocus() {
	f := t.current()
	if f == nil {
		return
	}
	owner := f.owner
	if owner == nil || owner.Virtual {
		return
	}

	if t.focus == f.task {
		elapsed := time.Since(t.focusStart)
		t.pushUndo("focus")
		f.task.Focus += elapsed
		t.save(owner)
		t.focus = nil
		t.focusOwner = nil
		t.refresh()
		t.selectTask(f.task)
		t.setStatus("Focus stopped — logged "+todo.FormatDuration(elapsed)+".", false)
		return
	}

	t.focus = f.task
	t.focusOwner = owner
	t.focusStart = time.Now()
	t.setStatus("Focus started.", false)
}

func (t *TodoApp) resolveConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	kind := t.confirm
	t.confirm = tcNone
	switch msg.String() {
	case "y", "Y":
		switch kind {
		case tcDeleteNote:
			t.deleteNote()
		case tcDeleteTask:
			t.deleteTask()
		}
	default:
		t.setStatus("Deletion cancelled.", false)
	}
	return t, nil
}

// ── input prompts ──────────────────────────────────────────────────────────

func (t *TodoApp) openInput(kind inputKind, prompt, value string) {
	t.inputKind = kind
	t.returnTo = t.view
	t.inputTask = nil
	if f := t.current(); f != nil {
		t.inputTask = f.task
	}
	t.input.Reset()
	t.input.Placeholder = prompt
	t.input.SetValue(value)
	t.input.CursorEnd()
	t.input.Focus()
	t.view = tvInput
}

func (t *TodoApp) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		t.input.Blur()
		t.view = t.returnTo
		return t, nil
	case "enter":
		t.commitInput(strings.TrimSpace(t.input.Value()))
		t.input.Blur()
		t.view = t.returnTo
		return t, nil
	}
	var cmd tea.Cmd
	t.input, cmd = t.input.Update(msg)
	return t, cmd
}

func (t *TodoApp) commitInput(value string) {
	if value == "" {
		return
	}

	switch t.inputKind {
	case inNewNote:
		t.pushUndo("new note")
		n := t.createNote(value)
		t.refresh()
		for i, c := range t.cards {
			if c == n {
				t.cardIdx = i
			}
		}
		t.setStatus("Note created: "+n.Title, false)

	case inRenameNote:
		c := t.currentCard()
		if c == nil || c.Virtual {
			return
		}
		t.pushUndo("rename note")
		if err := t.store.Rename(t.board, c, value); err != nil {
			t.setStatus("Rename error: "+err.Error(), true)
			return
		}
		t.board.Sort()
		t.refresh()
		t.setStatus("Renamed.", false)

	case inEditTask:
		task := t.inputTask
		if task == nil {
			return
		}
		owner := t.note
		if t.note != nil && t.note.Virtual {
			owner = t.board.OwnerOf(task)
		}
		if owner == nil {
			return
		}
		t.pushUndo("edit task")
		parsed, _ := todo.ParseCapture(value, t.now)
		task.Text = parsed.Text
		task.Priority = parsed.Priority
		task.Tags = parsed.Tags
		task.Due = parsed.Due
		task.Repeat = parsed.Repeat
		if parsed.Focus > 0 {
			task.Focus = parsed.Focus
		}
		t.save(owner)
		t.refresh()
		t.selectTask(task)
		t.setStatus("Task updated.", false)

	case inSubtask:
		parent := t.inputTask
		if parent == nil || t.note == nil || t.note.Virtual {
			return
		}
		t.pushUndo("add subtask")
		task, _ := todo.ParseCapture(value, t.now)
		parent.Children = append(parent.Children, task)
		t.save(t.note)
		t.refresh()
		t.selectTask(task)
		t.setStatus("Subtask added.", false)

	case inCapture:
		task, hint := todo.ParseCapture(value, t.now)
		target := t.board.Match(hint)
		if target == nil {
			if hint != "" {
				t.pushUndo("new note")
				target = t.createNote(hint)
			} else {
				target = t.defaultNote()
			}
		}
		if target == nil {
			t.setStatus("No note to add to.", true)
			return
		}
		t.pushUndo("add task")
		target.Tasks = append(target.Tasks, task)
		t.save(target)
		t.refresh()
		if t.note != nil {
			t.selectTask(task)
		}
		t.setStatus("Added to "+target.Title+".", false)
	}
}

// ── palette ────────────────────────────────────────────────────────────────

func (t *TodoApp) openPalette() {
	t.returnTo = t.view
	t.input.Reset()
	t.input.Placeholder = "search all notes…"
	t.input.Focus()
	t.matches = nil
	t.matchIdx = 0
	t.view = tvPalette
}

func (t *TodoApp) updatePalette(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		t.input.Blur()
		t.view = t.returnTo
		return t, nil
	case "up", "ctrl+k":
		if t.matchIdx > 0 {
			t.matchIdx--
		}
		return t, nil
	case "down", "ctrl+j", "tab":
		if t.matchIdx < len(t.matches)-1 {
			t.matchIdx++
		}
		return t, nil
	case "enter":
		if t.matchIdx < len(t.matches) {
			ref := t.matches[t.matchIdx]
			t.input.Blur()
			t.note = ref.Note
			t.hideDone = false
			t.view = tvNote
			t.rebuildFlat()
			t.selectTask(ref.Task)
			t.syncViewport()
			for i, c := range t.cards {
				if c == ref.Note {
					t.cardIdx = i
				}
			}
		}
		return t, nil
	}

	var cmd tea.Cmd
	t.input, cmd = t.input.Update(msg)
	t.matches = todo.Search(t.board, t.input.Value())
	if t.matchIdx >= len(t.matches) {
		t.matchIdx = 0
	}
	return t, cmd
}

// ── move ───────────────────────────────────────────────────────────────────

func (t *TodoApp) openMove() {
	t.moveTargets = nil
	for _, n := range t.board.Notes {
		if n != t.note {
			t.moveTargets = append(t.moveTargets, n)
		}
	}
	if len(t.moveTargets) == 0 {
		t.setStatus("No other note to move to.", true)
		return
	}
	t.moveIdx = 0
	t.returnTo = t.view
	t.view = tvMove
}

func (t *TodoApp) updateMove(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "ctrl+c":
		t.view = t.returnTo
	case "up", "k":
		if t.moveIdx > 0 {
			t.moveIdx--
		}
	case "down", "j":
		if t.moveIdx < len(t.moveTargets)-1 {
			t.moveIdx++
		}
	case "enter", " ":
		t.moveCurrentTask(t.moveTargets[t.moveIdx])
		t.view = t.returnTo
	}
	return t, nil
}

func (t *TodoApp) moveCurrentTask(target *todo.Note) {
	f := t.current()
	if f == nil || target == nil {
		return
	}
	source := f.owner
	if source == nil {
		source = t.note
	}
	if source == nil || source.Virtual {
		return
	}

	t.pushUndo("move task")
	task := f.task

	if t.note.Flat {
		removeTask(&source.Tasks, task)
	} else {
		list := *f.list
		*f.list = append(list[:f.idx], list[f.idx+1:]...)
	}
	target.Tasks = append(target.Tasks, task)

	t.save(source)
	t.save(target)
	t.refresh()
	t.setStatus("Moved to "+target.Title+".", false)
}

// removeTask deletes a task pointer from a tree, wherever it sits.
func removeTask(list *[]*todo.Task, task *todo.Task) bool {
	for i, t := range *list {
		if t == task {
			*list = append((*list)[:i], (*list)[i+1:]...)
			return true
		}
		if removeTask(&t.Children, task) {
			return true
		}
	}
	return false
}

// ── aux screens ────────────────────────────────────────────────────────────

func (t *TodoApp) updateTodoAux(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc", "?", "s":
		if t.note != nil {
			t.view = tvNote
		} else {
			t.view = tvWall
		}
		return t, nil
	case "ctrl+c":
		return t, tea.Quit
	}
	var cmd tea.Cmd
	t.vp, cmd = t.vp.Update(msg)
	return t, cmd
}

// ── EOD bridge ─────────────────────────────────────────────────────────────

// pushEOD copies everything finished today into today's EOD entry, one
// category per note.
func (t *TodoApp) pushEOD() {
	if t.eodAppend == nil {
		return
	}

	var groups []eodGroup
	for _, n := range t.board.Notes {
		var items []string
		n.Walk(func(task *todo.Task) {
			if len(task.Children) > 0 || !task.DoneOn(t.now) {
				return
			}
			text := task.Text
			if task.Focus > 0 {
				text += " (" + todo.FormatDuration(task.Focus) + ")"
			}
			items = append(items, text)
		})
		if len(items) > 0 {
			groups = append(groups, eodGroup{Category: n.Title, Items: items})
		}
	}

	if len(groups) == 0 {
		t.setStatus("Nothing was completed today.", false)
		return
	}

	added, err := t.eodAppend(groups)
	if err != nil {
		t.setStatus("EOD push failed: "+err.Error(), true)
		return
	}
	if added == 0 {
		t.setStatus("EOD already has today's items.", false)
		return
	}
	t.setStatus(fmt.Sprintf("Pushed %d item(s) into today's EOD.", added), false)
}

// OpenTasksText renders every open task in the EOD editor's own format, so the
// EOD side can pull it into a day entry.
func (t *TodoApp) OpenTasksText() string {
	var sb strings.Builder
	for _, n := range t.board.Notes {
		var items []string
		n.Walk(func(task *todo.Task) {
			if len(task.Children) == 0 && !task.Done {
				items = append(items, task.Text)
			}
		})
		if len(items) == 0 {
			continue
		}
		sb.WriteString("    - " + n.Title + ":\n")
		for _, item := range items {
			sb.WriteString("        - " + item + "\n")
		}
	}
	return sb.String()
}
