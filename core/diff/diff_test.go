package diff

import (
	"strings"
	"testing"
)

const oldNote = `---
tags: [IT]
---
# Nextcloud

## Zugang
Admin-Login über die Weboberfläche.
- Passwort: geheim123
- Port: 8080

## Backup
Restic nach B2.
`

const newNote = `---
tags: [IT]
Backup via: Borg
---
# Nextcloud

## Zugang
Admin-Login über die Weboberfläche.
- Passwort: siehe Vaultwarden: Nextcloud Admin

## Backup
Borg auf [[Borg Backup Server]].
`

func TestHunks(t *testing.T) {
	r := Compare(oldNote, newNote)
	if got := len(r.Hunks); got != 3 {
		t.Fatalf("want 3 hunks, got %d", got)
	}
	if r.Hunks[0].Heading != FrontmatterHeading {
		t.Errorf("hunk 0 heading %q", r.Hunks[0].Heading)
	}
	if r.Hunks[1].Heading != "## Zugang" || len(r.Hunks[1].OldLines) != 2 || len(r.Hunks[1].NewLines) != 1 {
		t.Errorf("hunk 1 = %+v", r.Hunks[1])
	}
	if r.Hunks[2].Heading != "## Backup" {
		t.Errorf("hunk 2 heading %q", r.Hunks[2].Heading)
	}
}

func TestApplyAllAndNone(t *testing.T) {
	r := Compare(oldNote, newNote)
	if got := r.Apply(func(Hunk) bool { return true }); got != newNote {
		t.Errorf("apply all:\n%s", got)
	}
	if got := r.Apply(func(Hunk) bool { return false }); got != oldNote {
		t.Errorf("apply none:\n%s", got)
	}
}

func TestApplySome(t *testing.T) {
	r := Compare(oldNote, newNote)
	got := r.Apply(func(h Hunk) bool { return h.Index == 2 })
	if !strings.Contains(got, "Borg auf") || strings.Contains(got, "Backup via") || !strings.Contains(got, "geheim123") {
		t.Errorf("apply hunk 2 only:\n%s", got)
	}
}

func TestStableIDs(t *testing.T) {
	a := Compare(oldNote, newNote)
	// A revision that only changes the backup hunk keeps the other hunk IDs.
	revised := strings.Replace(newNote, "Borg auf [[Borg Backup Server]].", "Borg auf [[Borg Backup Server]]. Zeitplan: compose.yaml", 1)
	b := Compare(oldNote, revised)
	if a.Hunks[0].ID != b.Hunks[0].ID || a.Hunks[1].ID != b.Hunks[1].ID {
		t.Error("unchanged hunks changed their IDs")
	}
	if a.Hunks[2].ID == b.Hunks[2].ID {
		t.Error("changed hunk kept its ID")
	}
}

func TestSplitRowsAlignDeletions(t *testing.T) {
	r := Compare(oldNote, newNote)
	rows := r.Rows(ModeSplit, 1)
	var change, del int
	for _, row := range rows {
		switch row.Kind {
		case RowChange:
			change++
			if row.OldNo == 0 || row.NewNo == 0 {
				t.Error("change row misses a side")
			}
		case RowDelete:
			del++
			if row.New != nil || row.NewNo != 0 {
				t.Error("delete row must have an empty right side (filler)")
			}
		}
	}
	if change != 2 || del != 1 {
		t.Errorf("want 2 change rows and 1 delete row, got %d/%d", change, del)
	}
}

func TestGapRows(t *testing.T) {
	long := strings.Repeat("x\n", 50)
	r := Compare(long+"a\n", long+"b\n")
	rows := r.Rows(ModeSplit, 3)
	if rows[0].Kind != RowGap || rows[0].Gap != 47 {
		t.Errorf("first row = %+v", rows[0])
	}
	if all := r.Rows(ModeSplit, AllContext); len(all) != 52 {
		t.Errorf("all context rows = %d", len(all))
	}
}

func TestWords(t *testing.T) {
	o, n := Words("Backup über Restic nach B2.", "Backup über Borg nach B2.")
	if len(o) != 3 || o[1].Text != "Restic" || !o[1].Changed {
		t.Errorf("old segs %+v", o)
	}
	if len(n) != 3 || n[1].Text != "Borg" {
		t.Errorf("new segs %+v", n)
	}
}

func TestRebase(t *testing.T) {
	r := Compare(oldNote, newNote)
	current := strings.Replace(oldNote, "# Nextcloud\n", "# Nextcloud\n\nNeu eingefügter Absatz.\n", 1)
	got, err := r.Rebase(current, func(h Hunk) bool { return h.Index == 2 })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Neu eingefügter Absatz.") || !strings.Contains(got, "Borg auf") {
		t.Errorf("rebase:\n%s", got)
	}

	conflict := strings.Replace(oldNote, "Restic nach B2.", "Restic nach S3.", 1)
	if _, err := r.Rebase(conflict, func(h Hunk) bool { return h.Index == 2 }); err != ErrConflict {
		t.Errorf("want conflict, got %v", err)
	}
}

func TestNewFile(t *testing.T) {
	r := Compare("", "# Neu\nText\n")
	if len(r.Hunks) != 1 {
		t.Fatalf("hunks %d", len(r.Hunks))
	}
	if got := r.Apply(func(Hunk) bool { return true }); got != "# Neu\nText\n" {
		t.Errorf("got %q", got)
	}
}

func TestMyersRandomRoundTrip(t *testing.T) {
	cases := [][2]string{
		{"a b c a b b a", "c b a b a c"},
		{"", "x y"},
		{"x y", ""},
		{"same", "same"},
	}
	for _, c := range cases {
		a, b := strings.Fields(c[0]), strings.Fields(c[1])
		var gotA, gotB []string
		for _, e := range myers(a, b) {
			switch e.kind {
			case opEqual:
				gotA, gotB = append(gotA, a[e.a]), append(gotB, b[e.b])
			case opDelete:
				gotA = append(gotA, a[e.a])
			case opInsert:
				gotB = append(gotB, b[e.b])
			}
		}
		if strings.Join(gotA, " ") != strings.Join(a, " ") || strings.Join(gotB, " ") != strings.Join(b, " ") {
			t.Errorf("round trip failed for %q", c)
		}
	}
}

func TestWordsCalm(t *testing.T) {
	o, n := Words("Port: 8080.", "Port: 8081!")
	for _, s := range append(o, n...) {
		if s.Changed && (s.Text == "." || s.Text == "!") {
			t.Errorf("punctuation highlighted: %+v %+v", o, n)
		}
	}
	o, _ = Words("ganz anders", "völlig neu")
	if len(o) != 1 || o[0].Changed {
		t.Errorf("unrelated lines should not get word highlights: %+v", o)
	}
}
