package model

import "fmt"

type EODFile struct {
	Year  int
	Month int
	Days  []*DayEntry
	Path  string // source file path
}

func (f *EODFile) Key() string {
	return fmt.Sprintf("%04d-%02d", f.Year, f.Month)
}

func (f *EODFile) Title() string {
	return fmt.Sprintf("EOD %d.%02d", f.Year, f.Month)
}

type VacationKind int

const (
	VacationNone VacationKind = iota
	VacationFull
	VacationHalf
)

func (v VacationKind) Next() VacationKind {
	switch v {
	case VacationNone:
		return VacationFull
	case VacationFull:
		return VacationHalf
	default:
		return VacationNone
	}
}

type DayEntry struct {
	Day      int
	Name     string // optional label, e.g. "szabi"
	Vacation VacationKind
	Items    []*Item
}

func (d *DayEntry) Label() string {
	if d.Name != "" {
		return fmt.Sprintf("%02d ( %s )", d.Day, d.Name)
	}
	return fmt.Sprintf("%02d", d.Day)
}

// Categories returns the top-level category names (items where IsCategory=true).
func (d *DayEntry) Categories() []string {
	var cats []string
	for _, it := range d.Items {
		if it.IsCategory {
			cats = append(cats, it.Text)
		}
	}
	return cats
}

// ItemCount returns total number of leaf items (non-category).
func (d *DayEntry) ItemCount() int {
	total := 0
	for _, it := range d.Items {
		total += countLeaves(it)
	}
	return total
}

func countLeaves(it *Item) int {
	if len(it.Children) == 0 {
		return 1
	}
	total := 0
	for _, c := range it.Children {
		total += countLeaves(c)
	}
	return total
}

type Item struct {
	Text       string
	IsCategory bool // ends with ":" in source
	Children   []*Item
	Depth      int // indentation depth (0 = top level within day)
}
