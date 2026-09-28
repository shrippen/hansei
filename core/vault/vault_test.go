package vault_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"git.arianw.de/shrippen/hansei/core/testvault"
	"git.arianw.de/shrippen/hansei/core/vault"
)

func open(t *testing.T) *vault.Vault {
	t.Helper()
	c := testvault.New(t)
	v, err := vault.Open(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Scan(); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestScope(t *testing.T) {
	v := open(t)
	for _, n := range v.Notes() {
		if n.Path == "Tagebuch/2026-09-20.md" || n.Path == "Projekte/Harbour Lights.md" {
			t.Errorf("out-of-scope note indexed: %s", n.Path)
		}
	}
	if _, _, err := v.Read("Tagebuch/2026-09-20.md"); !errors.Is(err, vault.ErrOutOfScope) {
		t.Errorf("blocked read: %v", err)
	}
	if err := v.Write("Projekte/x.md", "x"); !errors.Is(err, vault.ErrOutOfScope) {
		t.Errorf("out-of-scope write: %v", err)
	}
	if hits := v.Search("nichtlesen123", nil, 0); len(hits) != 0 {
		t.Errorf("blocked content searchable: %v", hits)
	}
}

func TestBadPaths(t *testing.T) {
	v := open(t)
	for _, p := range []string{"../x.md", "/etc/passwd.md", "IT/../Tagebuch/2026-09-20.md", "IT/.obsidian/x.md", "IT/x.txt", "IT\\x.md"} {
		if _, _, err := v.Read(p); err == nil {
			t.Errorf("%s accepted", p)
		}
	}
}

func TestSymlinkEscape(t *testing.T) {
	v := open(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(v.Root(), "IT", "link")); err != nil {
		t.Skip(err)
	}
	if err := v.Write("IT/link/evil.md", "x"); err == nil {
		t.Error("write through symlink escaped the vault")
	}
}

func TestLinksAndWrite(t *testing.T) {
	v := open(t)
	if !v.LinkExists("Harbour Lights") {
		t.Error("out-of-scope note should resolve as a link")
	}
	if !v.LinkExists("2026-09-20") {
		t.Error("blocked note should count as existing link")
	}
	if v.LinkExists("Gibt es nicht") {
		t.Error("unknown link resolved")
	}
	if bl := v.Backlinks("IT/Geräte/Nebelhorn.md"); len(bl) != 2 {
		t.Errorf("backlinks = %v", bl)
	}

	if err := v.Write("IT/Neu/Notiz.md", "# Neu\n[[Nebelhorn]]\n"); err != nil {
		t.Fatal(err)
	}
	got, exists, err := v.Read("IT/Neu/Notiz.md")
	if err != nil || !exists || got != "# Neu\n[[Nebelhorn]]\n" {
		t.Errorf("read back %q %v %v", got, exists, err)
	}
	if bl := v.Backlinks("IT/Geräte/Nebelhorn.md"); len(bl) != 3 {
		t.Errorf("backlinks after write = %v", bl)
	}
}

func TestFrontmatter(t *testing.T) {
	front, end := vault.Frontmatter("---\nletzte Prüfung: 2026-08-30\ntags:\n  - IT\n---\n# X\n")
	if end != 5 || vault.FrontString(front["letzte Prüfung"]) != "2026-08-30" || vault.FrontString(front["tags"]) != "IT" {
		t.Errorf("front=%v end=%d", front, end)
	}
}

func TestLinksInTables(t *testing.T) {
	got := vault.Links("| [[IT/Dienste/Eredin/Borgmatic\\|Borgmatic]] | [[Plain]] |")
	if len(got) != 2 || got[0] != "IT/Dienste/Eredin/Borgmatic" || got[1] != "Plain" {
		t.Errorf("links = %q", got)
	}
}

func TestRulebookFilesAreEditable(t *testing.T) {
	v := open(t)
	if !v.Allowed("agent.md") {
		t.Error("rulebook note in the root should be editable")
	}
	if v.Allowed("other-root-note.md") {
		t.Error("other root notes must stay out of scope")
	}
}
