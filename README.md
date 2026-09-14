# eod

A terminal workday companion with two sides:

- **TODO** — a wall of sticky notes, each one a nested checklist with due dates,
  priorities, tags, repeats and a focus timer.
- **EOD** — the end-of-day log: what you did each day, one file per month, with
  markdown/CSV export and a monthly summary.

They are connected: one keystroke turns everything you finished today into
today's EOD entry.

Everything is stored as plain text you can read, edit in any editor, grep and
put in git. No database, no lock-in.

```
╭────────────────────────────╮ ┏━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓ ╭────────────────────────────╮
│ ! Overdue              0/1 │ ┃ ◷ Today                0/2 ┃ │ ▤ This week            0/1 │
│ ░░░░░░░░░░░░░░░░░░░░░░░░░░ │ ┃ ░░░░░░░░░░░░░░░░░░░░░░░░░░ ┃ │ ░░░░░░░░░░░░░░░░░░░░░░░░░░ │
│ · !2 old thing    11d late │ ┃ · !1 ship release    today ┃ │ · write changelog       3d │
╰────────────────────────────╯ ┗━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛ ╰────────────────────────────╯
```

## Install

```sh
go build -o eod .          # or: go install .
```

Go 1.22+. Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea).
The palette follows `~/.cache/hyde/wallbash/shell-colors` at runtime and uses
Catppuccin Mocha as its fallback. Set `TUI_THEME=catppuccin` to force the
fallback.

## Usage

```sh
eod                     # launcher: pick TODO or EOD
eod todo                # straight to the todo board
eod eod                 # straight to the EOD log
eod <directory>         # use a different data directory
eod import <file|dir>   # import existing EOD txt files
eod help
```

Data lives in `$XDG_DATA_HOME/eod` (`~/.local/share/eod`), or wherever
`EOD_DATA_DIR` points:

```
<data>/eod_2026_08.txt        one file per month, the EOD log
<data>/todo/work.md           one file per note, a markdown checklist
<data>/todo/archive/work.md   items archived out of that note
```

## TODO

- **Sticky-note wall.** Every note is a coloured card with a progress bar and a
  peek at what is still open. Cards are laid out in a grid you walk with the
  arrow keys.
- **Computed lists.** *Overdue*, *Today*, *This week* and *Done today* are built
  from every note and appear as cards of their own. Ticking something off there
  writes straight back to the note that owns it.
- **Quick add.** One line, parsed into structure: priority, tags, due date,
  repeat, and which note it belongs to (see the syntax below).
- **Urgency at a glance.** Overdue is red, today is orange, the next few days
  yellow — on the cards and in the lists.
- **Subtasks.** Nest as deep as you like. A parent row shows `[~]` while its
  children are partly done and flips to `[x]` on its own when they are all
  finished.
- **Repeating tasks.** Checking one does not make it vanish: it leaves a dated
  copy behind as the record of this round and re-arms itself for the next date.
- **Focus timer.** Start the clock on a task, stop it when you are done; the
  minutes are stored with the task and travel with it into the EOD entry.
- **Streak and heat strip.** The stats screen shows completions per day for the
  last 30 days plus the current streak.
- **Fuzzy search.** One keystroke, then type; jump straight to any task on any
  note.
- **Undo.** Every destructive action — delete, archive, move, check — can be
  taken back with `u`, and deleting a note asks first.
- **Archive.** Sweep finished items out of a note into `todo/archive/`.

### Quick-add syntax

Type the task, then tack on any of these tokens, in any order:

| token       | meaning                                                                 |
| ----------- | ----------------------------------------------------------------------- |
| `!1 !2 !3`  | priority, 1 is the most urgent                                          |
| `#tag`      | a tag, repeatable                                                       |
| `^when`     | due date                                                                |
| `*spec`     | repeat                                                                  |
| `@note`     | which note it lands on — creates the note if it does not exist yet      |
| `+90m`      | focus time already spent (mostly written by the timer)                  |

`^when` accepts `today`/`ma`, `tomorrow`/`holnap`, a weekday in English or
Hungarian (`mon`…`sun`, `h`, `k`, `sze`, `cs`, `p`, `szo`, `v`), an offset like
`3d` or `2w`, a date like `09-14` or `2026-09-14`, or a bare day number.

`*spec` accepts `daily`, `weekly`, `monthly`, a weekday, or `3d` / `2w`.

```
review PR !1 #dev ^fri *weekly @work
```

A token that does not parse stays in the text, so `C# refactor` keeps its hash.

## EOD

The original half: one entry per day, categories and items, arrow-key
navigation across days and months.

- `n` starts today's entry, `e` edits the selected one, `d` deletes it (after a
  confirmation).
- `v` cycles the day between normal, full-day vacation ☀ and half-day ◑.
- `s` shows the monthly summary: which categories ate the month, work days,
  vacation days.
- `x` writes `eod_YYYY_MM.md` and `eod_YYYY_MM.csv` next to the data files.
- `y` copies the current day to the clipboard.
- `/` searches inside the month.

## TODO ↔ EOD

- **`Ctrl+E` on the todo side** files everything completed today into today's
  EOD entry — one category per note, focus time in brackets. Running it twice
  does not duplicate anything, and it creates the day (and the month's file) if
  they do not exist yet.
- **`Ctrl+T` in the EOD editor** pulls every still-open todo into the editor,
  grouped by note, as a starting point for the day's entry.

## Keys

### Launcher

| key             | action              |
| --------------- | ------------------- |
| `↑` `↓` `k` `j` | select              |
| `Enter`         | open                |
| `1` `2`         | jump straight to it |
| `q` `Esc`       | quit                |

### Todo — card wall

| key                     | action                        |
| ----------------------- | ----------------------------- |
| `←` `→` `↑` `↓` `h j k l` | move between cards          |
| `Enter` `Space`         | open the note                 |
| `n`                     | new note                      |
| `r`                     | rename note                   |
| `c`                     | cycle the note's colour       |
| `p`                     | pin / unpin                   |
| `d`                     | delete note (asks first)      |
| `a`                     | quick add a task              |
| `Esc`                   | back to the launcher          |

### Todo — inside a note

| key                    | action                              |
| ---------------------- | ----------------------------------- |
| `↑` `↓` `k` `j`        | move between tasks                  |
| `Space` `Enter` `x`    | check / uncheck                     |
| `Shift+↑` `Shift+↓` `K` `J` | reorder                        |
| `a`                    | add task                            |
| `o`                    | add subtask under the cursor        |
| `e`                    | edit task, full inline syntax       |
| `d`                    | delete task (asks first)            |
| `m`                    | move task to another note           |
| `0` `1` `2` `3` / `p`  | set / cycle priority                |
| `f`                    | start / stop the focus timer        |
| `z`                    | hide or show finished items         |
| `A`                    | archive finished items              |
| `←` `h` `Esc`          | back to the wall                    |

### Todo — everywhere

| key              | action                                |
| ---------------- | ------------------------------------- |
| `/`              | fuzzy search every task               |
| `Ctrl+Shift+P`   | fuzzy command palette                 |
| `s`              | stats: streak, heat strip, progress   |
| `u`              | undo the last change                  |
| `Ctrl+E`         | push today's finished tasks to EOD    |
| `?`              | help                                  |
| `q` `Ctrl+C`     | quit                                  |

### EOD — day list

| key                  | action                        |
| -------------------- | ----------------------------- |
| `↑` `↓` `k` `j`      | newer / older day             |
| `←` `→` `h` `l`      | previous / next month         |
| `g` `G` `Home` `End` | newest / oldest day           |
| `PgUp` `PgDn` `Ctrl+U` `Ctrl+D` | scroll the entry   |
| `n`                  | new day                       |
| `e`                  | edit day                      |
| `d`                  | delete day (asks first)       |
| `v`                  | cycle vacation mark           |
| `y`                  | copy day to clipboard         |
| `s`                  | monthly summary               |
| `/`                  | search in the month           |
| `x`                  | export markdown + CSV         |
| `?`                  | help                          |
| `Esc`                | back to the launcher          |
| `q` `Ctrl+C`         | quit                          |

The common `Ctrl+Shift+P` command palette is available from the launcher and
the non-input Todo/EOD screens. `Ctrl+P` is accepted when the terminal does not
distinguish the Shift modifier.

### EOD — editor

| key                          | action                    |
| ---------------------------- | ------------------------- |
| `Ctrl+S`                     | save                      |
| `Esc`                        | cancel                    |
| `Tab`                        | insert 4 spaces           |
| `Enter`                      | new line, keeps the indent |
| `Ctrl+T`                     | pull open todos in        |
| `Ctrl+Z` / `Ctrl+Y`          | undo / redo               |
| `Ctrl+Shift+Delete`          | clear the editor          |
| `Ctrl+←` `Ctrl+→`            | word navigation           |
| `Ctrl+Backspace` `Ctrl+Delete` | delete word back / forward |

## File formats

A todo note, `todo/work.md`:

```markdown
# Work  [color: peach] [pinned]

- [ ] ship release !1 #dev ^2026-09-04 *weekly
- [x] review PR #dev ~2026-08-31
- [ ] groceries
    - [x] milk ~2026-08-31
    - [ ] bread
```

`~date` is when it was completed, `+45m` is logged focus time. Four spaces per
nesting level; two are accepted when you edit by hand.

An EOD month, `eod_2026_08.txt`:

```
================================================================
EOD 2026.08
================================================================

08.31 ( sprint ):
    - Project:
        - what got done
        - and this too

================================================================
```

Both formats survive a round trip: read the file, edit it in the TUI, and what
is written back is the same shape.
