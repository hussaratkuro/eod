package todo

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Store is the on-disk board: one markdown file per note under <data>/todo,
// plus an archive/ subdirectory for completed items that were rotated out.
type Store struct {
	Dir     string
	Archive string
}

func NewStore(dataDir string) (*Store, error) {
	dir := filepath.Join(dataDir, "todo")
	arc := filepath.Join(dir, "archive")
	if err := os.MkdirAll(arc, 0o755); err != nil {
		return nil, err
	}
	return &Store{Dir: dir, Archive: arc}, nil
}

// Load reads every note file. On a first run it seeds an empty Inbox so the
// wall is never a blank screen.
func (s *Store) Load(now time.Time) (*Board, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return nil, err
	}

	b := &Board{Dir: s.Dir}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		path := filepath.Join(s.Dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		slug := strings.TrimSuffix(name, filepath.Ext(name))
		n := ParseNote(string(data), path, slug, now)
		n.Slug = slug
		b.Notes = append(b.Notes, n)
	}

	if len(b.Notes) == 0 {
		inbox := &Note{Title: "Inbox", Slug: "inbox", Color: "lavender", Pinned: true}
		inbox.Path = filepath.Join(s.Dir, "inbox.md")
		if err := s.Save(inbox); err == nil {
			b.Notes = append(b.Notes, inbox)
		}
	}

	b.Sort()
	return b, nil
}

func (s *Store) Save(n *Note) error {
	if n == nil || n.Virtual {
		return nil
	}
	if n.Slug == "" {
		n.Slug = Slugify(n.Title)
	}
	if n.Path == "" {
		n.Path = filepath.Join(s.Dir, n.Slug+".md")
	}
	return os.WriteFile(n.Path, []byte(SerializeNote(n)), 0o644)
}

func (s *Store) SaveAll(b *Board) error {
	for _, n := range b.Notes {
		if err := s.Save(n); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Delete(n *Note) error {
	if n == nil || n.Virtual || n.Path == "" {
		return nil
	}
	if err := os.Remove(n.Path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Rename retitles a note and moves its file to the matching slug.
func (s *Store) Rename(b *Board, n *Note, title string) error {
	old := n.Path
	n.Title = title
	slug := b.UniqueSlug(Slugify(title))
	if slug != n.Slug {
		n.Slug = slug
		n.Path = filepath.Join(s.Dir, slug+".md")
	}
	if err := s.Save(n); err != nil {
		return err
	}
	if old != "" && old != n.Path {
		_ = os.Remove(old)
	}
	return nil
}

// ArchiveTasks appends tasks to the note's archive file, dated.
func (s *Store) ArchiveTasks(n *Note, tasks []*Task, now time.Time) error {
	if len(tasks) == 0 {
		return nil
	}
	path := filepath.Join(s.Archive, n.Slug+".md")

	var sb strings.Builder
	if _, err := os.Stat(path); os.IsNotExist(err) {
		sb.WriteString("# " + n.Title + " — archive\n")
	}
	sb.WriteString("\n## " + DateOf(now).Format("2006-01-02") + "\n")
	for _, t := range tasks {
		WriteTask(&sb, t, 0)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(sb.String())
	return err
}

// Restore rewrites the store from an undo snapshot: every file in files is
// written back, and note files the snapshot does not know about are removed.
func (s *Store) Restore(files map[string]string) error {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
			continue
		}
		path := filepath.Join(s.Dir, e.Name())
		if _, keep := files[path]; !keep {
			_ = os.Remove(path)
		}
	}
	for path, src := range files {
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// Slugify turns a note title into a safe, accent-free file name.
func Slugify(title string) string {
	var sb strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(fold(title)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			sb.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && sb.Len() > 0 {
				sb.WriteRune('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(sb.String(), "-")
	if out == "" {
		out = "note"
	}
	return out
}
