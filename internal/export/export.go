package export

import (
	"encoding/csv"
	"fmt"
	"strings"

	"eod/internal/model"
)

// Markdown exports an EODFile as a Markdown document.
func Markdown(file *model.EODFile) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# EOD %d.%02d\n\n", file.Year, file.Month))

	for _, day := range file.Days {
		if day.Name != "" {
			sb.WriteString(fmt.Sprintf("## %d.%02d.%02d (%s)\n\n", file.Year, file.Month, day.Day, day.Name))
		} else {
			sb.WriteString(fmt.Sprintf("## %d.%02d.%02d\n\n", file.Year, file.Month, day.Day))
		}
		for _, item := range day.Items {
			writeMarkdownItem(&sb, item, 0)
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func writeMarkdownItem(sb *strings.Builder, item *model.Item, depth int) {
	indent := strings.Repeat("  ", depth)
	if item.IsCategory {
		fmt.Fprintf(sb, "%s- **%s**\n", indent, item.Text)
	} else {
		fmt.Fprintf(sb, "%s- %s\n", indent, item.Text)
	}
	for _, child := range item.Children {
		writeMarkdownItem(sb, child, depth+1)
	}
}

// CSV exports an EODFile as a flat CSV with columns: year, month, day, category, item.
func CSV(file *model.EODFile) string {
	var sb strings.Builder
	w := csv.NewWriter(&sb)

	_ = w.Write([]string{"year", "month", "day", "name", "category", "item"})

	for _, day := range file.Days {
		for _, item := range day.Items {
			writeCSVItem(w, file, day, item, "")
		}
	}

	w.Flush()
	return sb.String()
}

func writeCSVItem(w *csv.Writer, file *model.EODFile, day *model.DayEntry, item *model.Item, parentCat string) {
	cat := parentCat
	if item.IsCategory {
		cat = item.Text
		for _, child := range item.Children {
			writeCSVItem(w, file, day, child, cat)
		}
		return
	}

	_ = w.Write([]string{
		fmt.Sprintf("%d", file.Year),
		fmt.Sprintf("%02d", file.Month),
		fmt.Sprintf("%02d", day.Day),
		day.Name,
		cat,
		item.Text,
	})

	for _, child := range item.Children {
		writeCSVItem(w, file, day, child, cat)
	}
}

type catStat struct {
	days  int
	items int
}

// Summary returns a text summary of project/category distribution across the month.
func Summary(file *model.EODFile) string {
	stats := map[string]*catStat{}
	order := []string{}

	for _, day := range file.Days {
		seen := map[string]bool{}
		for _, item := range day.Items {
			collectCatStats(item, "", stats, &order, seen)
		}
	}

	if len(order) == 0 {
		return "Nincs adat."
	}

	// Find max items for bar scaling
	maxItems := 0
	for _, s := range stats {
		if s.items > maxItems {
			maxItems = s.items
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Summary: %d.%02d\n", file.Year, file.Month))
	sb.WriteString(strings.Repeat("─", 50) + "\n\n")

	for _, cat := range order {
		s := stats[cat]
		barLen := 0
		if maxItems > 0 {
			barLen = s.items * 20 / maxItems
		}
		bar := strings.Repeat("█", barLen) + strings.Repeat("░", 20-barLen)
		dayWord := "day"
		if s.days != 1 {
			dayWord = "days"
		}
		sb.WriteString(fmt.Sprintf("  %-20s %s  %d %s, %d items\n",
			truncate(cat, 20), bar, s.days, dayWord, s.items))
	}

	fullVac, halfVac := 0, 0
	for _, d := range file.Days {
		switch d.Vacation {
		case model.VacationFull:
			fullVac++
		case model.VacationHalf:
			halfVac++
		}
	}
	workDays := len(file.Days) - fullVac - halfVac
	fmt.Fprintf(&sb, "\nTotal: %d work day(s)", workDays)
	if fullVac > 0 {
		fmt.Fprintf(&sb, ", %d vacation ☀", fullVac)
	}
	if halfVac > 0 {
		fmt.Fprintf(&sb, ", %d half-day ◑", halfVac)
	}
	sb.WriteString("\n")

	return sb.String()
}

func collectCatStats(item *model.Item, parent string, stats map[string]*catStat, order *[]string, seen map[string]bool) {
	if item.IsCategory {
		cat := item.Text
		if _, ok := stats[cat]; !ok {
			stats[cat] = &catStat{}
			*order = append(*order, cat)
		}
		if !seen[cat] {
			stats[cat].days++
			seen[cat] = true
		}
		for _, child := range item.Children {
			collectCatStats(child, cat, stats, order, seen)
		}
		return
	}

	// Leaf item — count under parent category
	cat := parent
	if cat == "" {
		cat = "Egyéb"
	}
	if _, ok := stats[cat]; !ok {
		stats[cat] = &catStat{}
		*order = append(*order, cat)
	}
	stats[cat].items++

	for _, child := range item.Children {
		collectCatStats(child, cat, stats, order, seen)
	}
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}
