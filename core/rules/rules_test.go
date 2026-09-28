package rules

import (
	"strings"
	"testing"
	"time"

	"git.arianw.de/shrippen/hansei/core/testvault"
	"git.arianw.de/shrippen/hansei/core/vault"
)

func TestCheck(t *testing.T) {
	c := testvault.New(t)
	v, err := vault.Open(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Scan(); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	rep := Check(v, c, now)

	count := map[ID]int{}
	for _, f := range rep.Findings {
		count[f.Rule]++
		if strings.Contains(f.Excerpt, "Elbfeuer-2026!") {
			t.Error("secret shown in excerpt")
		}
		if strings.HasPrefix(f.Path, "Tagebuch") || strings.HasPrefix(f.Path, "Projekte") {
			t.Errorf("finding outside scope: %+v", f)
		}
	}
	want := map[ID]int{Secret: 1, Codename: 2, Frontmatter: 1, Review: 1, DeadLink: 2, Compose: 1}
	for id, n := range want {
		if count[id] != n {
			t.Errorf("%s: got %d, want %d (%+v)", id, count[id], n, rep.Findings)
		}
	}
	if rep.Notes != 7 || rep.Clean != 3 {
		t.Errorf("notes %d clean %d", rep.Notes, rep.Clean)
	}
	if rep.Summary[0].Rule != Secret {
		t.Errorf("summary order %+v", rep.Summary)
	}
}

func TestRulebook(t *testing.T) {
	c := testvault.New(t)
	v, _ := vault.Open(c)
	_ = v.Scan()
	book := Rulebook(v, c, []string{"IT/Dienste"})
	if len(book.Files) != 2 || !strings.Contains(book.Text, "Leuchtturm-Namen") || !strings.Contains(book.Text, "Ändere nur") {
		t.Errorf("book = %+v", book)
	}
}

func TestCodenameLinkTargets(t *testing.T) {
	names := map[string]string{"ZeroWRT": ""}
	lines := []string{
		"![Bild](ZeroWRT%20-%20WireGuard.png)",
		"![[ZeroWRT - Firewall.png]]",
		"Der Router ZeroWRT läuft noch.",
	}
	found := codenames("x.md", lines, compileNames(names))
	if len(found) != 1 || found[0].Line != 3 {
		t.Errorf("found %+v", found)
	}
}

func TestOutsideLinks(t *testing.T) {
	got := OutsideLinks("ZeroNAS ![[ZeroNAS.png]] und [x](ZeroNAS.md) ZeroNAS", strings.ToUpper)
	if got != "ZERONAS ![[ZeroNAS.png]] UND [X](ZeroNAS.md) ZERONAS" {
		t.Errorf("got %q", got)
	}
}
