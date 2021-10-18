package utils

import (
	"os"

	"github.com/mitchellh/go-homedir"
)

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
