# strengthimport

Regenerates `internal/core/strengthlibrary/data/exercises.json` (embedded
into the server binary via `go:embed`) from the reviewer workbook for
VIV-101's Strength movement library.

## Usage

```
go run ./internal/scripts/strengthimport -in <path> [flags]
```

`<path>` is either:

- a single `.xlsx` file, one tab per zone (`Lower body`, `Upper body`,
  `Full body`, `Core`) — e.g. `VIV_Strength_Movement_Groups_Lina.xlsx`
  or `VIV_Strength_Library_v1.xlsx`, or
- a directory of per-tab CSV exports, one file per zone, named after the
  tab (e.g. `Lower body.csv`).

Flags (all optional):

| Flag | Default | Meaning |
|---|---|---|
| `-out` | `internal/core/strengthlibrary/data/exercises.json` | Where the generated catalog is written |
| `-needs-review` | `internal/core/strengthlibrary/data/needs_review.json` | Rows flagged "Doesn't belong here?" |
| `-equipment-map` | `internal/scripts/strengthimport/equipment_mapping.json` | Free-text → controlled equipment tag mapping |

On success it prints a report (exercises per zone, per movement group,
and which groups are thin — fewer than 4 **approved** exercises) and
writes both JSON files. **On any validation failure it writes nothing**
and exits 1, printing every problem it found in one pass (not just the
first one).

After a real run, regenerate and commit `exercises.json` — it's checked
into the repo and embedded at build time, not read from disk at
runtime.

## Workbook layout contract

Each zone's tab holds one table with this header (column order doesn't
matter — columns are matched by header text, not position):

```
Movement group | Exercise | Equipment required | Technical demand | Balance demand | Reviewed by Lina
```

Two optional columns some layouts add:

```
Doesn't belong here? | Comment
```

and two more `VIV_Strength_Library_v1.xlsx` adds, read as informational
fields that never affect approval:

```
Flag | Note
```

The importer finds the real header by scanning every row of a tab for
the one that has both a "Movement group" and an "Exercise" cell — this
is what lets it ignore an unrelated older review table some tabs still
keep in columns A-D, and means the header can be on row 1 or row 5 (or
anywhere else) without a flag to say which.

### Row rules

- A blank `Exercise` cell means a blank row left for additions —
  skipped silently.
- A non-blank `Doesn't belong here?` cell removes the row from the
  catalog entirely; it's written to `needs_review.json` with the
  `Comment` column's text (or, if there's no `Comment` column, the
  `Doesn't belong here?` cell's own text) instead. A human resolves
  these — the importer never moves or drops them on its own.
- `Reviewed by Lina` must be exactly `Yes` (case-insensitive) for a row
  to import as `approved`. Anything else — blank, `No`, a typo — is
  `pending`.
- `Equipment required` is split on comma/semicolon/newline and each
  piece is mapped through `equipment_mapping.json` to one of the
  controlled tags (`none`, `mat`, `dumbbell`, `kettlebell`, `barbell`,
  `bench`, `box`, `rack`, `machine`, `hand_support`). Unmapped text
  fails the import — add it to the mapping file, it's never guessed.
- `Technical demand`/`Balance demand` must be blank or exactly
  `Low`/`Moderate`/`High` (case-insensitive).
- An exercise's ID is `{zone}_{slugified name}` (e.g.
  `lower_goblet_squat`) — **not** the movement group. The same name can
  exist in two zones (two distinct IDs), but the same name twice in one
  zone — even under two different movement groups — collides and fails
  the import; fix the sheet or rename one of them.
