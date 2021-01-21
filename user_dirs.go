package gemdrive

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
)

type UserDirs interface {
	GetConfigDir() string
	GetDataDir() string
	GetCacheDir() string
}

func NewUserDirs() (UserDirs, error) {
	switch runtime.GOOS {
	case "linux":
		ud, err := NewXDGUserDirs()
		if err != nil {
			return nil, err
		}

		return ud, nil
	case "windows":
		return &WindowsUserDirs{}, nil
	default:
		return nil, errors.New("Unsupported OS")
	}
}

type XDGUserDirs struct {
	homeDir string
}

func NewXDGUserDirs() (*XDGUserDirs, error) {

	usr, err := user.Current()
	if err != nil {
		return nil, err
	}

	return &XDGUserDirs{usr.HomeDir}, nil
}

func (ud XDGUserDirs) GetConfigDir() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		dir = filepath.Join(ud.homeDir, ".config")
	}
	return dir
}
func (ud XDGUserDirs) GetDataDir() string {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		dir = filepath.Join(ud.homeDir, ".local", "share")
	}
	return dir
}
func (ud XDGUserDirs) GetCacheDir() string {
	dir := os.Getenv("XDG_CACHE_HOME")
	if dir == "" {
		dir = filepath.Join(ud.homeDir, ".cache")
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
