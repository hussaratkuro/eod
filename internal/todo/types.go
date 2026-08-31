package todo

import (
	"sort"
	"strings"
	"time"
)

// Priority levels. 1 is the most urgent; 0 means no priority was set.
const (
	PrioNone = 0
	PrioHigh = 1
	PrioMed  = 2
	PrioLow  = 3
)

// Task is one checklist entry. Tasks nest arbitrarily deep; a task that has
// children acts as a grouping row and takes its progress from its leaves.
type Task struct {
	Text     string
	Done     bool
	Priority int
	Tags     []string
	Due      *time.Time
	DoneAt   *time.Time
	Repeat   string // normalised repeat spec; "" for a one-shot task
	Focus    time.Duration
	Children []*Task
}

// DateOf strips the clock from t, leaving local midnight.
func DateOf(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func (t *Task) Clone() *Task {
	c := *t
	if t.Due != nil {
		d := *t.Due
		c.Due = &d
	}
	if t.DoneAt != nil {
		d := *t.DoneAt
		c.DoneAt = &d
	}
	c.Tags = append([]string(nil), t.Tags...)
	c.Children = nil
	for _, ch := range t.Children {
		c.Children = append(c.Children, ch.Clone())
	}
	return &c
}

// Walk visits t and every descendant, depth first.
func (t *Task) Walk(fn func(*Task)) {
	fn(t)
	for _, c := range t.Children {
		c.Walk(fn)
	}
}

// Counts returns done/total over the leaves of the subtree, so a grouping row
// never inflates a note's progress.
func (t *Task) Counts() (int, int) {
	if len(t.Children) == 0 {
		if t.Done {
			return 1, 1
		}
		return 0, 1
	}
	done, total := 0, 0
	for _, c := range t.Children {
		d, n := c.Counts()
		done += d
		total += n
	}
	return done, total
}

// Partial reports whether some, but not all, leaves below t are done.
func (t *Task) Partial() bool {
	if len(t.Children) == 0 {
		return false
	}
	d, n := t.Counts()
	return d > 0 && d < n
}

// SetDone marks the whole subtree and stamps (or clears) the completion date.
func (t *Task) SetDone(v bool, now time.Time) {
	stamp := DateOf(now)
	t.Walk(func(x *Task) {
		x.Done = v
		if v {
			d := stamp
			x.DoneAt = &d
		} else {
			x.DoneAt = nil
		}
	})
}

func (t *Task) HasTag(tag string) bool {
	for _, g := range t.Tags {
		if strings.EqualFold(g, tag) {
			return true
		}
	}
	return false
}

// DoneOn reports whether the task was completed on the given day.
func (t *Task) DoneOn(day time.Time) bool {
	return t.Done && t.DoneAt != nil && DateOf(*t.DoneAt).Equal(DateOf(day))
}

// Normalize keeps grouping rows in sync with their children: a parent is done
// exactly when every leaf below it is.
func Normalize(tasks []*Task) {
	for _, t := range tasks {
		if len(t.Children) == 0 {
			continue
		}
		Normalize(t.Children)
		d, n := t.Counts()
		t.Done = d == n
		if !t.Done {
			t.DoneAt = nil
		}
	}
}

// Note is one sticky note: a titled checklist stored as a single markdown file.
type Note struct {
	Title  string
	Slug   string
	Color  string
	Pinned bool
	Tasks  []*Task
	Path   string

	// Virtual notes (Today, Overdue, …) are computed views holding pointers to
	// tasks that live in real notes. They are never written to disk, and Flat
	// means their rows are a plain list rather than a tree.
	Virtual bool
	Flat    bool
	Icon    string
}

func (n *Note) Walk(fn func(*Task)) {
	for _, t := range n.Tasks {
		t.Walk(fn)
	}
}

func (n *Note) Counts() (int, int) {
	done, total := 0, 0
	if n.Flat {
		for _, t := range n.Tasks {
			total++
			if t.Done {
				done++
			}
		}
		return done, total
	}
	for _, t := range n.Tasks {
		d, x := t.Counts()
		done += d
		total += x
	}
	return done, total
}

func (n *Note) Progress() float64 {
	d, t := n.Counts()
	if t == 0 {
		return 0
	}
	return float64(d) / float64(t)
}

// Preview returns the first still-open leaves, for the card wall.
func (n *Note) Preview(limit int) []*Task {
	var out []*Task
	if n.Flat {
		for _, t := range n.Tasks {
			if len(out) >= limit {
				break
			}
			out = append(out, t)
		}
		return out
	}
	n.Walk(func(t *Task) {
		if len(out) >= limit || t.Done || len(t.Children) > 0 {
			return
		}
		out = append(out, t)
	})
	return out
}

// Ref pairs a task with the note it lives in.
type Ref struct {
	Note *Note
	Task *Task
}

// Board is every note in the store.
type Board struct {
	Dir   string
	Notes []*Note
}

// Sort puts pinned notes first, then orders by title.
func (b *Board) Sort() {
	sort.SliceStable(b.Notes, func(i, j int) bool {
		a, c := b.Notes[i], b.Notes[j]
		if a.Pinned != c.Pinned {
			return a.Pinned
		}
		return strings.ToLower(a.Title) < strings.ToLower(c.Title)
	})
}

// OwnerOf finds the real note a task pointer belongs to, which is how edits
// made through a virtual list get written back to the right file.
func (b *Board) OwnerOf(task *Task) *Note {
	for _, n := range b.Notes {
		found := false
		n.Walk(func(t *Task) {
			if t == task {
				found = true
			}
		})
		if found {
			return n
		}
	}
	return nil
}

func (b *Board) Refs() []Ref {
	var out []Ref
	for _, n := range b.Notes {
		n.Walk(func(t *Task) { out = append(out, Ref{Note: n, Task: t}) })
	}
	return out
}

func (b *Board) BySlug(slug string) *Note {
	for _, n := range b.Notes {
		if n.Slug == slug {
			return n
		}
	}
	return nil
}

// Match resolves an "@hint" from quick capture: exact slug, then slug prefix,
// then a substring of the title.
func (b *Board) Match(hint string) *Note {
	if hint == "" {
		return nil
	}
	h := strings.ToLower(fold(hint))
	if n := b.BySlug(h); n != nil {
		return n
	}
	for _, n := range b.Notes {
		if strings.HasPrefix(n.Slug, h) {
			return n
		}
	}
	for _, n := range b.Notes {
		if strings.Contains(strings.ToLower(fold(n.Title)), h) {
			return n
		}
	}
	return nil
}

// UniqueSlug returns base, or base-2, base-3… if the board already uses it.
func (b *Board) UniqueSlug(base string) string {
	if base == "" {
		base = "note"
	}
	if b.BySlug(base) == nil {
		return base
	}
	for i := 2; ; i++ {
		s := base + "-" + itoa(i)
		if b.BySlug(s) == nil {
			return s
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
