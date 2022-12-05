package build

var CurrentCommit string

// change when IChainSync api modify
const ApiVersion = 2

// BuildVersion is the local build version, set by build system
const BuildVersion = "2.5.10.1001"

func UserVersion() string {
	return BuildVersion + CurrentCommit
}
