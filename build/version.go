package build

var CurrentCommit string

// BuildVersion is the local build version, set by build system
const BuildVersion = "2.5.9.1007"

func UserVersion() string {
	return BuildVersion + CurrentCommit
}
