package clients

import "regexp"

var nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9 ]+`)

func truncate(s string, n int) string {
	if len(s) < n {
		return s
	}

	return s[:n]
}
