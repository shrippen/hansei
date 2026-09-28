// Package diff compares note versions line by line and word by word and applies selected hunks.
package diff

// opKind is one step of an edit script.
type opKind int

const (
	opEqual opKind = iota
	opDelete
	opInsert
)

// edit is one step: for opEqual both indexes are set, for opDelete only a, for opInsert only b.
type edit struct {
	kind opKind
	a, b int
}

// myers computes a shortest edit script from a to b (Myers 1986, greedy forward search with trace).
func myers(a, b []string) []edit {
	n, m := len(a), len(b)
	max := n + m
	if max == 0 {
		return nil
	}

	// Trim the common prefix and suffix first; most note edits are local.
	pre := 0
	for pre < n && pre < m && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < n-pre && suf < m-pre && a[n-1-suf] == b[m-1-suf] {
		suf++
	}

	var out []edit
	for i := 0; i < pre; i++ {
		out = append(out, edit{opEqual, i, i})
	}
	for _, e := range core(a[pre:n-suf], b[pre:m-suf]) {
		switch e.kind {
		case opEqual:
			out = append(out, edit{opEqual, e.a + pre, e.b + pre})
		case opDelete:
			out = append(out, edit{opDelete, e.a + pre, -1})
		case opInsert:
			out = append(out, edit{opInsert, -1, e.b + pre})
		}
	}
	for i := 0; i < suf; i++ {
		out = append(out, edit{opEqual, n - suf + i, m - suf + i})
	}
	return out
}

// core is the plain Myers search on the trimmed middle part.
func core(a, b []string) []edit {
	n, m := len(a), len(b)
	if n == 0 {
		out := make([]edit, m)
		for j := range out {
			out[j] = edit{opInsert, -1, j}
		}
		return out
	}
	if m == 0 {
		out := make([]edit, n)
		for i := range out {
			out[i] = edit{opDelete, i, -1}
		}
		return out
	}

	max := n + m
	offset := max
	v := make([]int, 2*max+2)
	var trace [][]int

	// Forward search: v[k] is the furthest x on diagonal k after d edits.
	for d := 0; d <= max; d++ {
		snapshot := make([]int, len(v))
		copy(snapshot, v)
		trace = append(trace, snapshot)
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[offset+k-1] < v[offset+k+1]) {
				x = v[offset+k+1]
			} else {
				x = v[offset+k-1] + 1
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[offset+k] = x
			if x >= n && y >= m {
				return backtrack(trace, a, b, offset, d)
			}
		}
	}
	return nil
}

// backtrack walks the trace from the end to rebuild the edit script.
func backtrack(trace [][]int, a, b []string, offset, dEnd int) []edit {
	x, y := len(a), len(b)
	var rev []edit
	for d := dEnd; d > 0; d-- {
		v := trace[d]
		k := x - y
		var prevK int
		if k == -d || (k != d && v[offset+k-1] < v[offset+k+1]) {
			prevK = k + 1
		} else {
			prevK = k - 1
		}
		prevX := v[offset+prevK]
		prevY := prevX - prevK
		for x > prevX && y > prevY {
			x--
			y--
			rev = append(rev, edit{opEqual, x, y})
		}
		if x == prevX {
			y--
			rev = append(rev, edit{opInsert, -1, y})
		} else {
			x--
			rev = append(rev, edit{opDelete, x, -1})
		}
	}
	for x > 0 && y > 0 {
		x--
		y--
		rev = append(rev, edit{opEqual, x, y})
	}

	out := make([]edit, len(rev))
	for i := range rev {
		out[i] = rev[len(rev)-1-i]
	}
	return out
}
