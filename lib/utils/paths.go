package utils

import (
	"os"

	"github.com/mitchellh/go-homedir"
)

// node repo path defaults
const MemoPathVar = "MEFS_PATH"
const defaultRepoDir = "~/.memo"

// GetRepoPath returns the path of the repo from a potential override
// string, the MEFS_PATH environment variable and a default of ~/.memo.
func GetRepoPath(override string) (string, error) {
	// override is first precedence
	if override != "" {
		return homedir.Expand(override)
	}
	// Environment variable is second precedence
	envRepoDir := os.Getenv(MemoPathVar)
	if envRepoDir != "" {
		return homedir.Expand(envRepoDir)
	}
	// Default is third precedence
	return homedir.Expand(defaultRepoDir)
}

func GetMefsPath() (string, error) {
	return GetRepoPath("")
}
