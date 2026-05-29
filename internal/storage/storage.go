package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"

	"eod/internal/model"
	"eod/internal/parser"
)

var fileRe = regexp.MustCompile(`(?i)eod[_-]?(\d{4})[._-](\d{2})\.txt$`)

type Store struct {
	Dir string
}

func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{Dir: dir}, nil
}

// DefaultDir returns the XDG data dir for eod, or falls back to ~/.local/share/eod.
func DefaultDir() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "eod")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "eod")
}

// ListFiles returns all monthly EOD file paths in the store directory, sorted.
func (s *Store) ListFiles() ([]string, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, e := range entries {
		if !e.IsDir() && fileRe.MatchString(e.Name()) {
			paths = append(paths, filepath.Join(s.Dir, e.Name()))
		}
	}
	sort.Strings(paths)
	return paths, nil
}

// LoadAll loads every monthly file found in the store.
func (s *Store) LoadAll() ([]*model.EODFile, error) {
	paths, err := s.ListFiles()
	if err != nil {
		return nil, err
	}
	var files []*model.EODFile
	for _, p := range paths {
		f, err := parser.ParseFile(p)
		if err != nil {
			continue // skip unparseable files
		}
		files = append(files, f)
	}
	return files, nil
}

// Load loads the monthly file for the given year/month. Returns nil, nil if not found.
func (s *Store) Load(year, month int) (*model.EODFile, error) {
	path := s.pathFor(year, month)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, nil
	}
	return parser.ParseFile(path)
}

// Save serializes an EODFile and writes it to the store.
func (s *Store) Save(file *model.EODFile) error {
	path := s.pathFor(file.Year, file.Month)
	file.Path = path
	content := parser.Serialize(file)
	return os.WriteFile(path, []byte(content), 0o644)
}

// NewFile creates a new empty EODFile for the given year/month.
func (s *Store) NewFile(year, month int) *model.EODFile {
	return &model.EODFile{
		Year:  year,
		Month: month,
		Path:  s.pathFor(year, month),
	}
}

// CurrentFile loads (or creates) the file for the current month.
func (s *Store) CurrentFile() (*model.EODFile, error) {
	now := time.Now()
	f, err := s.Load(now.Year(), int(now.Month()))
	if err != nil {
		return nil, err
	}
	if f == nil {
		f = s.NewFile(now.Year(), int(now.Month()))
	}
	return f, nil
}

// Import copies an external txt file into the store, preserving its year/month.
// If dest already exists the existing file is backed up with a .bak suffix.
func (s *Store) Import(srcPath string) (*model.EODFile, error) {
	f, err := parser.ParseFile(srcPath)
	if err != nil {
		return nil, fmt.Errorf("import: %w", err)
	}
	if f.Year == 0 {
		// Try to extract year/month from filename
		if m := fileRe.FindStringSubmatch(filepath.Base(srcPath)); m != nil {
			f.Year, _ = strconv.Atoi(m[1])
			f.Month, _ = strconv.Atoi(m[2])
		}
	}
	if f.Year == 0 {
		return nil, fmt.Errorf("import: could not determine year/month from %s", srcPath)
	}

	dest := s.pathFor(f.Year, f.Month)

	// Back up existing file if present
	if _, err := os.Stat(dest); err == nil {
		_ = os.Rename(dest, dest+".bak")
	}

	src, err := os.Open(srcPath)
	if err != nil {
		return nil, err
	}
	defer src.Close()

	dst, err := os.Create(dest)
	if err != nil {
		return nil, err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return nil, err
	}

	f.Path = dest
	return f, nil
}

// ImportDir imports all EOD txt files from a directory.
func (s *Store) ImportDir(dir string) ([]*model.EODFile, []error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, []error{err}
	}

	var imported []*model.EODFile
	var errs []error

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			continue
		}
		// Accept any .txt file — Import will determine year/month from content or filename.
		if filepath.Ext(name) != ".txt" {
			continue
		}
		f, err := s.Import(filepath.Join(dir, name))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		imported = append(imported, f)
	}
	return imported, errs
}

func (s *Store) pathFor(year, month int) string {
	return filepath.Join(s.Dir, fmt.Sprintf("eod_%04d_%02d.txt", year, month))
}
