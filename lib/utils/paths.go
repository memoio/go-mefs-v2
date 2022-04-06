package utils

import (
	"os"
	"syscall"

	"github.com/memoio/go-mefs-v2/lib/types/store"
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

// GetDiskStatus returns disk usage of path/disk
func GetDiskStatus(path string) (store.DiskStats, error) {
	m := store.DiskStats{
		Path: path,
	}
	fs := syscall.Statfs_t{}
	err := syscall.Statfs(path, &fs)
	if err != nil {
		return m, err
	}

	m.Total = fs.Blocks * uint64(fs.Bsize)
	m.Free = fs.Bfree * uint64(fs.Bsize)

	m.Used = m.Total - m.Free
	return m, nil
}
