// Package version contains build information injected at compile time.
package version

// These values are overridden with -ldflags "-X ..." by GoReleaser/CI.
var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

// Info returns a human readable build description.
func Info() string {
	return Version + " (" + Commit + ", " + BuildDate + ")"
}
