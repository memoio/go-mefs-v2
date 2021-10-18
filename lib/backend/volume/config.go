package volume

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"time"

	"github.com/chrislusf/seaweedfs/weed/storage/needle"
	"github.com/chrislusf/seaweedfs/weed/storage/types"
	"github.com/chrislusf/seaweedfs/weed/util"

	"github.com/memoio/go-mefs-v2/lib/utils"
)

const (
	collection      = "memo-volume"
	initialVolumeId = needle.VolumeId(0)

	needleIDKey     = "needleIDKey"
	volumeIDKey     = "volumeIDKey"
	volumeConfigKey = "volumeConfigKey"

	defaultVolumeCount = 1024
	maxVolumeSize      = 1 * 1024 * 1024

	gcInterval = 60 * time.Minute
	gcRation   = 0.3 // 30% files are deletes
)

var defaultMinSpace = util.MinFreeSpace{Type: util.AsPercent, Percent: float32(5)}

type Config struct {
	MaxVolumeSize   uint64
	IdxFolder       string
	Dirnames        []string
	MaxVolumeCounts []int
	MinFreeSpaces   []util.MinFreeSpace
	DiskType        []types.DiskType
}

func DefaultConfig(idx string) *Config {
	err := os.Mkdir(idx, 0755)
	if err != nil && !os.IsExist(err) {
		return nil
	}
	return &Config{
		MaxVolumeSize:   maxVolumeSize,
		IdxFolder:       idx, // ssd dir is better?
		Dirnames:        make([]string, 0, 1),
		MaxVolumeCounts: make([]int, 0, 1),
		MinFreeSpaces:   make([]util.MinFreeSpace, 0, 1),
		DiskType:        make([]types.DiskType, 0, 1),
	}
}

func (c *Config) AddPath(path string) error {
	err := os.Mkdir(path, 0755)
	if err != nil && !os.IsExist(err) {
		return err
	}

	for _, dpath := range c.Dirnames {
		if dpath == path {
			return ErrDirExist
		}
	}
	c.Dirnames = append(c.Dirnames, path)
	c.MaxVolumeCounts = append(c.MaxVolumeCounts, defaultVolumeCount)
	c.MinFreeSpaces = append(c.MinFreeSpaces, defaultMinSpace)
	c.DiskType = append(c.DiskType, types.HardDriveType)

	return nil
}

type DirConfig struct {
	IdxPath    string
	VolumePath []string
}

func (c *Config) serialize() ([]byte, error) {
	dc := DirConfig{
		IdxPath:    c.IdxFolder,
		VolumePath: c.Dirnames,
	}

	return json.MarshalIndent(&dc, "", "  ")
}

func deserialize(data []byte) (*Config, error) {
	var dc DirConfig
	err := json.Unmarshal(data, &dc)
	if err != nil {
		return nil, err
	}

	logger.Info("config: ", dc)

	c := DefaultConfig(dc.IdxPath)
	for _, dir := range dc.VolumePath {
		err := c.AddPath(dir)
		if err != nil {
			return nil, err
		}
	}

	return c, nil
}

func StoreConfig(c *Config) error {
	base, err := utils.GetMefsPath()
	if err != nil {
		return err
	}

	data, err := c.serialize()
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(base+"/cfg-volume", data, 0644)
	if err != nil {
		return err
	}

	return nil
}

func GetConfig() (*Config, error) {
	base, err := utils.GetMefsPath()
	if err != nil {
		return nil, err
	}

	data, err := ioutil.ReadFile(base + "/cfg-volume")
	if err != nil {
		return nil, err
	}

	return deserialize(data)
}
