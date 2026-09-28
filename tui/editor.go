package tui

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
)

// editExternal opens the current proposal in $VISUAL/$EDITOR; saving creates your own version.
// The temp file holds real secrets, so it lives in the private runtime dir and is removed after.
func (m *Model) editExternal() tea.Cmd {
	if m.file == nil {
		return nil
	}
	id, p := m.batchID, m.path
	fv, err := m.api.File(id, p, m.mode, 3, true)
	if err != nil {
		return func() tea.Msg { return errMsg{err} }
	}

	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	f, err := os.CreateTemp(dir, "hansei-*-"+filepath.Base(p))
	if err != nil {
		return func() tea.Msg { return errMsg{err} }
	}
	name := f.Name()
	_ = f.Chmod(0o600)
	_, err = f.WriteString(fv.Content)
	f.Close()
	if err != nil {
		os.Remove(name)
		return func() tea.Msg { return errMsg{err} }
	}

	editor := firstNonEmpty(os.Getenv("VISUAL"), os.Getenv("EDITOR"))
	if editor == "" {
		editor = "vi"
	}
	cmd := exec.Command("sh", "-c", `exec $0 "$1"`, editor, name)
	return tea.ExecProcess(cmd, func(runErr error) tea.Msg {
		defer os.Remove(name)
		if runErr != nil {
			return errMsg{runErr}
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			return errMsg{err}
		}
		if string(raw) == fv.Content {
			return nil
		}
		if _, err := m.api.EditFile(id, p, string(raw)); err != nil {
			return errMsg{errors.Join(err)}
		}
		return nil
	})
}
