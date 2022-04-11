package utils

import (
	"os"

	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/mitchellh/go-homedir"
	"github.com/shirou/gopsutil/v3/disk"
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

// GetDiskStatus returns disk usage of path/disk
func GetDiskStatus(path string) (store.DiskStats, error) {
	m := store.DiskStats{
		Path: path,
	}
	dus, err := disk.Usage(path)
	if err != nil {
		return m, err
	}

	m.Total = dus.Total
	m.Free = dus.Free

	m.Used = m.Total - m.Free
	return m, nil
}
