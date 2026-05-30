package app

import "runtime"

const (
	defaultVersion   = "0.0.0-local"
	defaultCommit    = "local"
	defaultBuildDate = "local"
)

type Version struct {
	Version   string
	Commit    string
	BuildDate string
	GoVersion string
}

func DefaultVersion() Version {
	return Version{
		Version:   defaultVersion,
		Commit:    defaultCommit,
		BuildDate: defaultBuildDate,
		GoVersion: runtime.Version(),
	}
}

func (v Version) IsZero() bool {
	return v.Version == "" && v.Commit == "" && v.BuildDate == "" && v.GoVersion == ""
}
