package gemdrive

import (
	"os"
	"os/user"
	"path/filepath"
	"runtime"
)

var homeDir string

func init() {
	usr, err := user.Current()
	if err != nil {
		panic(err)
	}

	homeDir = usr.HomeDir
}

type UserDirs interface {
	GetConfigDir() string
	GetDataDir() string
	GetCacheDir() string
}

func NewUserDirs() UserDirs {
	switch runtime.GOOS {
	case "linux":
		return &XDGUserDirs{}
	case "windows":
		return &WindowsUserDirs{}
	default:
		panic("Unsupported OS")
	}
}

type XDGUserDirs struct {
}

func (ud XDGUserDirs) GetConfigDir() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		dir = filepath.Join(homeDir, ".config")
	}
	return dir
}
func (ud XDGUserDirs) GetDataDir() string {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		dir = filepath.Join(homeDir, ".local", "share")
	}
	return dir
}
func (ud XDGUserDirs) GetCacheDir() string {
	dir := os.Getenv("XDG_CACHE_HOME")
	if dir == "" {
		dir = filepath.Join(homeDir, ".cache")
	}
	return dir
}

type WindowsUserDirs struct {
}

func (ud WindowsUserDirs) GetConfigDir() string {
	return os.Getenv("APPDATA")
}
func (ud WindowsUserDirs) GetDataDir() string {
	return os.Getenv("APPDATA")
}
func (ud WindowsUserDirs) GetCacheDir() string {
	return os.Getenv("LOCALAPPDATA")
}
