package cli

import (
	"fmt"
	"strings"
)

// UnifiedDiff produces a simple readable unified diff between before and after.
func UnifiedDiff(filename string, before, after []byte) string {
	a := splitNormLines(string(before))
	b := splitNormLines(string(after))

	// LCS dynamic programming for clean line diffs.
	m, n := len(a), len(b)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	type op struct {
		kind byte // ' ', '-', '+'
		line string
	}
	var ops []op
	i, j := 0, 0
	for i < m && j < n {
		if a[i] == b[j] {
			ops = append(ops, op{' ', a[i]})
			i++
			j++
		} else if dp[i+1][j] >= dp[i][j+1] {
			ops = append(ops, op{'-', a[i]})
			i++
		} else {
			ops = append(ops, op{'+', b[j]})
			j++
		}
	}
	for ; i < m; i++ {
		ops = append(ops, op{'-', a[i]})
	}
	for ; j < n; j++ {
		ops = append(ops, op{'+', b[j]})
	}

	var out strings.Builder
	fmt.Fprintf(&out, "--- a/%s\n+++ b/%s\n", filename, filename)

	const ctx = 3
	// Mark indices to include (changes + ctx lines around changes).
	keep := make([]bool, len(ops))
	for idx, o := range ops {
		if o.kind != ' ' {
			lo := idx - ctx
			if lo < 0 {
				lo = 0
			}
			hi := idx + ctx
			if hi >= len(ops) {
				hi = len(ops) - 1
			}
			for k := lo; k <= hi; k++ {
				keep[k] = true
			}
		}
	}

	inHunk := false
	for idx, o := range ops {
		if !keep[idx] {
			inHunk = false
			continue
		}
		if !inHunk {
			out.WriteString("@@ ...\n")
			inHunk = true
		}
		out.WriteByte(o.kind)
		out.WriteString(o.line)
		out.WriteByte('\n')
	}

	return out.String()
}

func splitNormLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
