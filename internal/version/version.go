package version

import (
	"strings"

	"runtime/debug"
)

// Build-time parameters set via -ldflags
var Version = "unknown"

// A user may install pug using `go install github.com/opencode-ai/opencode@latest`.
// without -ldflags, in which case the version above is unset. As a workaround
// we use the embedded build version that *is* set when using `go install` (and
// is only set for `go install` and not for `go build`).
func init() {
	// A version stamped through -ldflags is authoritative and must not be overwritten. This
	// matters now that Go stamps a VCS version into local builds too (when the tree sits on a
	// tag), which would otherwise shadow the release workflow's ldflags.
	if Version != "unknown" && Version != "" {
		Version = strings.TrimPrefix(Version, "v")
		return
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		// < go v1.18
		return
	}
	mainVersion := info.Main.Version
	if mainVersion == "" || mainVersion == "(devel)" {
		// bin not built using `go install`
		return
	}
	// bin built using `go install`
	// Strip the leading `v` and any build-metadata suffix (`+dirty`, `+incompatible`, …) so the
	// printed version is a plain `X.Y.Z`, consistent with the release binaries.
	if idx := strings.IndexAny(mainVersion, "+"); idx >= 0 {
		mainVersion = mainVersion[:idx]
	}
	Version = strings.TrimPrefix(mainVersion, "v")
}
