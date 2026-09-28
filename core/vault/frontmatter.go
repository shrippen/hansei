package vault

import (
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const frontFence = "---"

// Frontmatter parses the YAML block at the start of a note.
// It returns the fields and the index of the first line after the closing fence
// (0 when the note has no frontmatter or it is not valid YAML).
func Frontmatter(content string) (map[string]any, int) {
	lines := strings.Split(content, "\n")
	if len(lines) < 2 || strings.TrimRight(lines[0], "\r ") != frontFence {
		return nil, 0
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r ") != frontFence {
			continue
		}
		fields := map[string]any{}
		if err := yaml.Unmarshal([]byte(strings.Join(lines[1:i], "\n")), &fields); err != nil {
			return nil, 0
		}
		return fields, i + 1
	}
	return nil, 0
}

// FrontString renders a frontmatter value as text, e.g. a date or a list.
func FrontString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case time.Time:
		return t.Format(time.DateOnly)
	case []any:
		parts := make([]string, 0, len(t))
		for _, x := range t {
			parts = append(parts, FrontString(x))
		}
		return strings.Join(parts, ", ")
	default:
		out, err := yaml.Marshal(t)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
}
