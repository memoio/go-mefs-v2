package build

import "fmt"

var CurrentCommit string

// change when IChainSync api modify
const ApiVersion = 2

// BuildVersion is the local build version, set by build system
const BuildVersion = "2.7.0"

func UserVersion() string {
	return BuildVersion + fmt.Sprintf("+api.%d", ApiVersion) + CurrentCommit
}
