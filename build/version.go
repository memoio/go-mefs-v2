package build

var CurrentCommit string
var BuildType int

const (
	BuildDefault = 0
	BuildMainnet = 0x1
	BuildDev     = 0x2
	BuildTest    = 0x3
)

func buildType() string {
	switch BuildType {
	case BuildDefault:
		return ""
	case BuildMainnet:
		return "+mainnet"
	case BuildDev:
		return "+devnet"
	case BuildTest:
		return "+testnet"
	default:
		return "+huh?"
	}
}

// BuildVersion is the local build version, set by build system
const BuildVersion = "0.0.1-dev"

func UserVersion() string {
	return BuildVersion + buildType() + CurrentCommit
}
