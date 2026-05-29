package parser

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"eod/internal/model"
)

var (
	headerRe = regexp.MustCompile(`^EOD\s+(\d{4})\.(\d{2})\s*$`)
	dayRe    = regexp.MustCompile(`^(\d{2})\.(\d{2})(?:\s*\(\s*([^)]+)\s*\))?(?:\s*\[([^\]]+)\])?\s*:`)
	itemRe   = regexp.MustCompile(`^(\s*)-\s+(.+)$`)
	sepLine  = strings.Repeat("=", 64)
)

func ParseFile(path string) (*model.EODFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	file, err := parseLines(lines)
	if err != nil {
		return nil, err
	}
	file.Path = path
	return file, nil
}

func ParseString(src string, path string) (*model.EODFile, error) {
	lines := strings.Split(src, "\n")
	file, err := parseLines(lines)
	if err != nil {
		return nil, err
	}
	file.Path = path
	return file, nil
}

func parseLines(lines []string) (*model.EODFile, error) {
	file := &model.EODFile{}

	// Find EOD header
	for _, line := range lines {
		if m := headerRe.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			file.Year, _ = strconv.Atoi(m[1])
			file.Month, _ = strconv.Atoi(m[2])
			break
		}
	}

	var currentDay *model.DayEntry

	flushDay := func() {
		if currentDay != nil {
			file.Days = append(file.Days, currentDay)
			currentDay = nil
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "====") {
			// separator — flush current day
			flushDay()
			continue
		}

		if m := dayRe.FindStringSubmatch(trimmed); m != nil {
			flushDay()
			day, _ := strconv.Atoi(m[2])
			currentDay = &model.DayEntry{
				Day:      day,
				Name:     strings.TrimSpace(m[3]),
				Vacation: parseVacationTag(m[4]),
			}
			continue
		}

		if currentDay != nil {
			if m := itemRe.FindStringSubmatch(line); m != nil {
				spaces := len(m[1])
				depth := max(spaces/4-1, 0)
				text := strings.TrimSpace(m[2])
				isCategory := strings.HasSuffix(text, ":")
				if isCategory {
					text = strings.TrimSpace(strings.TrimSuffix(text, ":"))
				}
				item := &model.Item{
					Text:       text,
					IsCategory: isCategory,
					Depth:      depth,
				}
				insertItem(currentDay, item)
			}
		}
	}
	flushDay()

	return file, nil
}

// insertItem adds an item into the day using a stack-based depth match.
func insertItem(day *model.DayEntry, item *model.Item) {
	if item.Depth == 0 || len(day.Items) == 0 {
		day.Items = append(day.Items, item)
		return
	}
	if !tryInsert(day.Items, item) {
		// fallback: attach to root if depth mismatch
		day.Items = append(day.Items, item)
	}
}

func tryInsert(items []*model.Item, item *model.Item) bool {
	if len(items) == 0 {
		return false
	}
	last := items[len(items)-1]
	if item.Depth == last.Depth+1 {
		last.Children = append(last.Children, item)
		return true
	}
	if len(last.Children) > 0 && item.Depth > last.Depth {
		return tryInsert(last.Children, item)
	}
	return false
}

// Serialize writes an EODFile back to the original txt format.
func Serialize(file *model.EODFile) string {
	var sb strings.Builder

	sb.WriteString(sepLine + "\n")
	sb.WriteString(fmt.Sprintf("EOD %d.%02d\n", file.Year, file.Month))
	sb.WriteString(sepLine + "\n")

	for _, day := range file.Days {
		sb.WriteString("\n")
		sb.WriteString(dayHeader(file.Month, day) + "\n")
		for _, item := range day.Items {
			serializeItem(&sb, item, 1)
		}
		sb.WriteString("\n")
		sb.WriteString(sepLine + "\n")
	}

	return sb.String()
}

func parseVacationTag(raw string) model.VacationKind {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "vacation", "szabadság", "szabadsag":
		return model.VacationFull
	case "half-day", "half day", "fél nap", "fel nap":
		return model.VacationHalf
	}
	return model.VacationNone
}

// DayHeader returns the header line for a day entry (e.g. "05.28:" or "05.28 ( szabi ) [vacation]:").
func DayHeader(month int, day *model.DayEntry) string { return dayHeader(month, day) }

func dayHeader(month int, day *model.DayEntry) string {
	s := fmt.Sprintf("%02d.%02d", month, day.Day)
	if day.Name != "" {
		s += fmt.Sprintf(" ( %s )", day.Name)
	}
	switch day.Vacation {
	case model.VacationFull:
		s += " [vacation]"
	case model.VacationHalf:
		s += " [half-day]"
	}
	return s + ":"
}

func serializeItem(sb *strings.Builder, item *model.Item, depth int) {
	indent := strings.Repeat("    ", depth)
	text := item.Text
	if item.IsCategory {
		text += ":"
	}
	fmt.Fprintf(sb, "%s- %s\n", indent, text)
	for _, child := range item.Children {
		serializeItem(sb, child, depth+1)
	}
}

// SerializeDay returns the raw text for a single day (without the day header),
// suitable for editing in a textarea and re-parsing.
func SerializeDay(day *model.DayEntry) string {
	var sb strings.Builder
	for _, item := range day.Items {
		serializeItem(&sb, item, 1)
	}
	return sb.String()
}

// ParseDayItems parses raw indented text (as produced by SerializeDay) back into items.
func ParseDayItems(raw string) []*model.Item {
	lines := strings.Split(raw, "\n")
	day := &model.DayEntry{}
	for _, line := range lines {
		if m := itemRe.FindStringSubmatch(line); m != nil {
			spaces := len(m[1])
			depth := max(spaces/4-1, 0)
			text := strings.TrimSpace(m[2])
			isCategory := strings.HasSuffix(text, ":")
			if isCategory {
				text = strings.TrimSpace(strings.TrimSuffix(text, ":"))
			}
			item := &model.Item{
				Text:       text,
				IsCategory: isCategory,
				Depth:      depth,
			}
			insertItem(day, item)
		}
	}
	return day.Items
}
