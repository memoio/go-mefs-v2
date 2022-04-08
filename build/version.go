package build

var CurrentCommit string

// BuildVersion is the local build version, set by build system
const BuildVersion = "2.3.0408-alpha"

func UserVersion() string {
	return BuildVersion + CurrentCommit
}
