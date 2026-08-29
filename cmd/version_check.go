package cmd

import (
	"strconv"
	"strings"
)

// versionAtLeast compares dotted version strings NUMERICALLY per segment.
// Lexicographic comparison ("1.0.10" < "1.0.9" because '1'<'9') broke every
// release-surface assertion the moment the version rolled past .9 — this
// helper makes the assertions stable for any future 1.0.N / 1.N version.
func versionAtLeast(got, want string) bool {
	g := strings.Split(got, ".")
	w := strings.Split(want, ".")
	for i := 0; i < len(g) || i < len(w); i++ {
		var gv, wv int
		if i < len(g) {
			gv, _ = strconv.Atoi(g[i])
		}
		if i < len(w) {
			wv, _ = strconv.Atoi(w[i])
		}
		if gv != wv {
			return gv > wv
		}
	}
	return true
}
