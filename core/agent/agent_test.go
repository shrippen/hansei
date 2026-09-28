package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"git.arianw.de/shrippen/hansei/core/llm"
	"git.arianw.de/shrippen/hansei/core/testvault"
	"git.arianw.de/shrippen/hansei/core/vault"
)

// A secret typed into the task must not reach the model.
func TestTaskTextIsRedacted(t *testing.T) {
	c := testvault.New(t)
	v, _ := vault.Open(c)
	_ = v.Scan()
	var seen string
	prov := llm.NewScript("s", func(_ context.Context, req llm.Request, call llm.CallFunc) (string, error) {
		seen = req.Prompt + req.System
		call("ask_user", map[string]string{"question": "?"})
		return "", nil
	})
	_, err := Create(context.Background(), Env{Vault: v, Config: c, Provider: prov, Now: time.Now()}, "Setze Passwort: Geheim-12345 überall ein", nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(seen, "Geheim-12345") {
		t.Error("secret from the task text reached the model")
	}
}

// Notes in local-only folders stay away from external providers.
func TestLocalOnly(t *testing.T) {
	c := testvault.New(t)
	c.LocalOnly = []string{"IT/Anleitungen"}
	v, _ := vault.Open(c)
	_ = v.Scan()
	external := llm.NewScript("ext", nil)
	r := newRun(Env{Vault: v, Config: c, Provider: externalProvider{external}}, nil, nil)
	if _, err := r.readNote(context.Background(), []byte(`{"path":"IT/Anleitungen/Restore.md"}`)); err == nil {
		t.Error("external provider read a local-only note")
	}
	if _, err := r.readNote(context.Background(), []byte(`{"path":"IT/Geräte/Nebelhorn.md"}`)); err != nil {
		t.Errorf("normal note: %v", err)
	}
}

type externalProvider struct{ llm.Provider }

func (externalProvider) Local() bool { return false }

// Notes too large to show in full must not be edited: the model would drop the missing tail.
func TestHugeNoteIsNotEditable(t *testing.T) {
	c := testvault.New(t)
	v, _ := vault.Open(c)
	_ = v.Scan()
	if err := v.Write("IT/Groß.md", strings.Repeat("Zeile mit Text\n", 20000)); err != nil {
		t.Fatal(err)
	}
	r := newRun(Env{Vault: v, Config: c, Provider: llm.NewScript("s", nil)}, nil, nil)
	if _, err := r.readNote(context.Background(), []byte(`{"path":"IT/Groß.md"}`)); err == nil {
		t.Error("huge note was handed to the model for editing")
	}
}
