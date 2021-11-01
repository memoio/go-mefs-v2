package build

var CurrentCommit string

// BuildVersion is the local build version, set by build system
const BuildVersion = "0.2.0-dev"

func UserVersion() string {
	return BuildVersion + CurrentCommit
}
