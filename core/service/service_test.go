package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.arianw.de/shrippen/hansei/core/agent"
	"git.arianw.de/shrippen/hansei/core/batch"
	"git.arianw.de/shrippen/hansei/core/config"
	"git.arianw.de/shrippen/hansei/core/llm"
	"git.arianw.de/shrippen/hansei/core/testvault"
)

const nextcloud = "IT/Dienste/Nebelhorn/Nextcloud.md"

// scriptModel plays a model that fixes the secret and, on feedback, the backup line.
func scriptModel(ctx context.Context, req llm.Request, call llm.CallFunc) (string, error) {
	content, _ := call("read_note", map[string]string{"path": nextcloud})
	if strings.Contains(content, "Elbfeuer-2026!") {
		return "", errLeak
	}
	next := strings.Replace(content, "- Passwort: ⟦GEHEIM_1⟧", "- Passwort: siehe Vaultwarden: Nextcloud Admin", 1)
	if strings.Contains(req.Prompt, "Borg") {
		next = strings.Replace(next, "Restic nach", "Borg nach", 1)
	}
	if out, bad := call("propose_edit", map[string]any{"path": nextcloud, "content": next, "summary": "Secret ersetzt",
		"changes": []map[string]string{{"before": "Passwort: ⟦GEHEIM_1⟧", "after": "siehe Vaultwarden", "reason": "Keine Klartext-Secrets", "rule": "Design.md › Secrets"}}}); bad {
		return "", &scriptErr{out}
	}
	call("finish", map[string]string{"title": "Secrets entfernen", "topic": "Secrets", "summary": "Ein Passwort ersetzt.", "reply": "Erledigt."})
	return "ok", nil
}

type scriptErr struct{ s string }

func (e *scriptErr) Error() string { return e.s }

var errLeak = &scriptErr{"secret reached the model"}

func open(t *testing.T) (*Service, *config.Config) {
	t.Helper()
	c := testvault.New(t)
	c.Providers = []config.Provider{{Name: "script", Kind: "script", Model: "script"}}
	c.Provider = "script"
	s, err := Open(c, Options{DataDir: t.TempDir(), Lang: "de", Providers: func(p config.Provider) (llm.Provider, error) {
		return llm.NewScript(p.Name, scriptModel), nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s, c
}

// wait blocks until the batch has no running AI task.
func wait(t *testing.T, s *Service, id string) BatchView {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		v, err := s.Batch(id)
		if err != nil {
			t.Fatal(err)
		}
		if !v.Running && v.Status != string(batch.StatusWorking) && !v.Revising {
			return v
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("AI task did not finish")
	return BatchView{}
}

func TestTaskReviewUndo(t *testing.T) {
	s, c := open(t)
	sum, err := s.Task(TaskInput{Instruction: "Klartext-Secrets entfernen", Scope: []string{"IT/Dienste"}})
	if err != nil {
		t.Fatal(err)
	}
	b := wait(t, s, sum.ID)
	if b.Status != string(batch.StatusReview) || b.Title != "Secrets entfernen" || len(b.Files) != 1 {
		t.Fatalf("batch = %+v", b.BatchSummary)
	}

	fv, err := s.File(b.ID, nextcloud, "split", 3, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(fv.Hunks) != 1 || fv.Hunks[0].Reason != "Keine Klartext-Secrets" {
		t.Fatalf("hunks = %+v", fv.Hunks)
	}
	for _, r := range fv.Rows {
		for _, seg := range append(r.Old, r.New...) {
			if strings.Contains(seg.Text, "Elbfeuer-2026!") {
				t.Fatal("secret visible in the diff")
			}
		}
	}

	res, err := s.Decide(b.ID, nextcloud, fv.Hunks[0].ID, "accepted", "")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Written || !res.Done {
		t.Fatalf("decide result %+v", res)
	}
	raw, _ := os.ReadFile(filepath.Join(c.Vault, nextcloud))
	if !strings.Contains(string(raw), "siehe Vaultwarden: Nextcloud Admin") {
		t.Fatalf("vault not written:\n%s", raw)
	}
	if j := s.Journal(0); len(j) != 1 || j[0].Added != 1 {
		t.Fatalf("journal = %+v", j)
	}

	if _, err := s.UndoBatch(b.ID); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(filepath.Join(c.Vault, nextcloud))
	if !strings.Contains(string(raw), "Elbfeuer-2026!") {
		t.Fatal("undo did not restore the note")
	}
}

func TestFeedbackRevision(t *testing.T) {
	s, _ := open(t)
	sum, _ := s.Task(TaskInput{Instruction: "Secrets"})
	b := wait(t, s, sum.ID)
	if _, err := s.Feedback(FeedbackInput{Batch: b.ID, Scope: "file", Path: nextcloud, Text: "Backup läuft über Borg, nicht Restic."}); err != nil {
		t.Fatal(err)
	}
	b = wait(t, s, b.ID)
	if b.Files[0].Versions != 2 || len(b.Thread) != 3 || b.Thread[1].Text != "Erledigt." ||
		b.Thread[2].Role != batch.AuthorSystem || b.Thread[2].Text != "v2 · Nextcloud.md: 2 Änderungen" {
		t.Fatalf("after feedback: files %+v thread %+v", b.Files, b.Thread)
	}
	cmp, err := s.Compare(b.ID, nextcloud, 1, 2, "split", 3)
	if err != nil || len(cmp.Rows) == 0 {
		t.Fatalf("compare %v %+v", err, cmp)
	}
	fv, _ := s.File(b.ID, nextcloud, "split", 3, false)
	if len(fv.Hunks) != 2 {
		t.Errorf("v2 hunks = %d", len(fv.Hunks))
	}
}

func TestStaleRebaseAndConflict(t *testing.T) {
	s, c := open(t)
	sum, _ := s.Task(TaskInput{Instruction: "Secrets"})
	b := wait(t, s, sum.ID)
	fv, _ := s.File(b.ID, nextcloud, "split", 3, false)

	// Someone edits the note elsewhere, far from the hunk: the change still fits.
	full := filepath.Join(c.Vault, nextcloud)
	raw, _ := os.ReadFile(full)
	os.WriteFile(full, []byte(strings.Replace(string(raw), "# Nextcloud", "# Nextcloud (Studio)", 1)), 0o644)
	fv, _ = s.File(b.ID, nextcloud, "split", 3, false)
	if !fv.Stale {
		t.Error("stale not detected")
	}
	res, err := s.Decide(b.ID, nextcloud, fv.Hunks[0].ID, "accepted", "")
	if err != nil || !res.Written {
		t.Fatalf("rebase write: %v %+v", err, res)
	}
	raw, _ = os.ReadFile(full)
	if !strings.Contains(string(raw), "(Studio)") || !strings.Contains(string(raw), "siehe Vaultwarden") {
		t.Fatalf("rebased note:\n%s", raw)
	}
}

func TestCodenameBatchWithoutAI(t *testing.T) {
	s, _ := open(t)
	sum, err := s.FromFinding("codename", "")
	if err != nil {
		t.Fatal(err)
	}
	if sum.Source != string(batch.SourceRules) || len(sum.Files) != 2 {
		t.Fatalf("codename batch %+v", sum)
	}
	fv, _ := s.File(sum.ID, "IT/Geräte/Nebelhorn.md", "split", 3, false)
	if len(fv.Hunks) != 1 || !strings.Contains(fv.Content, "Früher Nebelhorn") {
		t.Fatalf("file view %+v", fv)
	}
}

func TestFindingsAndHome(t *testing.T) {
	s, _ := open(t)
	f := s.Findings(true)
	if f.Notes != 7 || f.Titles["secret"] != "Klartext-Secrets" {
		t.Fatalf("findings %+v", f.Summary)
	}
	h := s.Home()
	if h.Notes != 7 || len(h.Findings) == 0 {
		t.Fatalf("home %+v", h)
	}
}

func TestImportLegacyQueue(t *testing.T) {
	c := testvault.New(t)
	queue := t.TempDir()
	dir := filepath.Join(queue, "batch-a-secrets", "IT", "Dienste", "Nebelhorn")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "Nextcloud.md.proposed"), []byte("# Nextcloud\n\nneu\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "Nextcloud.md.review.json"), []byte(`{"rel_path":"x","status":"pending","comments":[{"author":"user","text":"siehst du das?","timestamp":"2026-08-30T22:12:55Z"}]}`), 0o644)
	c.ImportQueue = queue
	s, err := Open(c, Options{DataDir: t.TempDir(), Lang: "de"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	list := s.Batches()
	if len(list) != 1 || list[0].Source != "import" || len(list[0].Files) != 1 {
		t.Fatalf("import %+v", list)
	}
	v, _ := s.Batch(list[0].ID)
	if len(v.Thread) != 1 || v.Thread[0].Text != "siehst du das?" {
		t.Fatalf("thread %+v", v.Thread)
	}
	if n, _ := s.Import(); n != 0 {
		t.Error("imported twice")
	}
}

func TestSecondProcessIsBusy(t *testing.T) {
	c := testvault.New(t)
	dir := t.TempDir()
	s, err := Open(c, Options{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := Open(c, Options{DataDir: dir}); err != ErrBusy {
		t.Errorf("want ErrBusy, got %v", err)
	}
}

// A proposal that only differs in the final newline has no changes and must not block the batch.
func TestNoopProposalDoesNotBlock(t *testing.T) {
	s, c := open(t)
	sum, err := s.FromFinding("codename", "")
	if err != nil {
		t.Fatal(err)
	}
	s.store.Lock()
	b, _ := s.store.Get(sum.ID)
	raw, _ := os.ReadFile(filepath.Join(c.Vault, "IT/Anleitungen/Restore.md"))
	s.addProposals(b, []agent.Proposal{{Path: "IT/Anleitungen/Restore.md", Content: strings.TrimSuffix(string(raw), "\n"), Base: string(raw), Exists: true}}, batch.AuthorAI, "")
	s.store.Unlock()
	if _, ok := b.File("IT/Anleitungen/Restore.md"); ok {
		t.Fatal("a proposal without changes was added")
	}
}

func TestFindOnly(t *testing.T) {
	c := testvault.New(t)
	c.Providers = []config.Provider{{Name: "script", Kind: "script", Model: "script"}}
	c.Provider = "script"
	s, err := Open(c, Options{DataDir: t.TempDir(), Lang: "de", Providers: func(p config.Provider) (llm.Provider, error) {
		return llm.NewScript(p.Name, func(_ context.Context, req llm.Request, call llm.CallFunc) (string, error) {
			for _, tool := range req.Tools {
				if tool.Name == "propose_edit" {
					return "", &scriptErr{"find-only run offers propose_edit"}
				}
			}
			call("report_note", map[string]string{"path": nextcloud, "reason": "Klartext-Passwort"})
			call("finish", map[string]string{"title": "Secrets suchen", "topic": "Secrets", "summary": "Eine Notiz."})
			return "", nil
		}), nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	sum, err := s.Task(TaskInput{Instruction: "Wo stehen Passwörter?", FindOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	b := wait(t, s, sum.ID)
	if b.Status != string(batch.StatusReview) || len(b.Found) != 1 || b.Found[0].Path != nextcloud || len(b.Files) != 0 {
		t.Fatalf("find-only batch %+v found %+v", b.BatchSummary, b.Found)
	}
}

func TestJournalDiffAndRuleSection(t *testing.T) {
	s, _ := open(t)
	sum, _ := s.FromFinding("codename", "")
	fv, _ := s.File(sum.ID, sum.Files[0].Path, "split", 3, false)
	if _, err := s.Decide(sum.ID, fv.Path, fv.Hunks[0].ID, "accepted", ""); err != nil {
		t.Fatal(err)
	}
	j := s.Journal(1)
	cmp, err := s.JournalDiff(j[0].ID)
	if err != nil || len(cmp.Rows) == 0 {
		t.Fatalf("journal diff %v %+v", err, cmp)
	}
	sec, err := s.RuleSection("Design.md › Konventionen")
	if err != nil || sec.File != "IT/Design.md" || !strings.Contains(sec.Text, "Leuchtturm-Namen") {
		t.Fatalf("rule section %v %+v", err, sec)
	}
}

func TestFolderTree(t *testing.T) {
	s, _ := open(t)
	var paths []string
	for _, f := range s.Settings().Folders {
		paths = append(paths, f.Path)
		if strings.HasPrefix(f.Path, "Tagebuch/") {
			t.Errorf("listed a folder inside a blocked folder: %s", f.Path)
		}
	}
	joined := strings.Join(paths, ",")
	if !strings.Contains(joined, "IT/Dienste/Nebelhorn") || !strings.Contains(joined, "Tagebuch") {
		t.Errorf("tree = %s", joined)
	}
}
