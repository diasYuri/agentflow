// Package version provides shared build metadata for agentflow binaries.
package version

import "fmt"

// These variables are populated at link time via -ldflags.
var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
	BuiltBy = ""
)

// Info holds structured version metadata.
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
	BuiltBy string `json:"built_by,omitempty"`
}

// GetInfo returns the current build metadata.
func GetInfo() Info {
	return Info{
		Version: Version,
		Commit:  Commit,
		Date:    Date,
		BuiltBy: BuiltBy,
	}
}

// String returns a human-readable version string.
func (i Info) String() string {
	if i.BuiltBy != "" {
		return fmt.Sprintf("agentflow version %s (commit %s, built %s by %s)", i.Version, i.Commit, i.Date, i.BuiltBy)
	}
	return fmt.Sprintf("agentflow version %s (commit %s, built %s)", i.Version, i.Commit, i.Date)
}
