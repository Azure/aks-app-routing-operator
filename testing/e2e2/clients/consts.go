package clients

import "regexp"

var nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9 ]+`)
