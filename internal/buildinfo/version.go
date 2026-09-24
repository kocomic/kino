package buildinfo

import (
	_ "embed"
	"strings"
)

// rawVersion is the runtime version source used by builds and release checks.
//
//go:embed VERSION
var rawVersion string

var Version = strings.TrimSpace(rawVersion)
