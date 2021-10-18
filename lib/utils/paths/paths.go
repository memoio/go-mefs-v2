package paths

import (
	"os"

	"github.com/mitchellh/go-homedir"
)

// node repo path defaults
const MemoPathVar = "MEMO_PATH"
const defaultRepoDir = "~/.memo"

// GetRepoPath returns the path of the venus repo from a potential override
// string, the MEMO_PATH environment variable and a default of ~/.memo/repo.
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
	mefsPath := "~/.memo"
	if os.Getenv("MEFS_PATH") != "" { //获取环境变量
		mefsPath = os.Getenv("MEFS_PATH")
	}
	mefsPath, err := homedir.Expand(mefsPath)
	if err != nil {
		return "", err
	}
	return mefsPath, nil
}
