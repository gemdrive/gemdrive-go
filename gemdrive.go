package gemdrive

import (
	"fmt"
	"io"
)

type Item struct {
	Size int64 `json:"size"`
	// TODO: json should be mod_time, or some other name
	ModTime  string           `json:"modTime"`
	Children map[string]*Item `json:"children,omitempty"`
}

type Backend interface {
	List(path string, maxDepth int) (*Item, error)
	Read(path string, offset, length int64) (*Item, io.ReadCloser, error)
}

type WritableBackend interface {
	MakeDir(path string, recursive bool) error
	Write(path string, data io.Reader, offset, length int64, overwrite, truncate bool) error
	Delete(path string, recursive bool) error
}

type ImageServer interface {
	GetImage(path string, size int) (io.Reader, int64, error)
}

type Error struct {
	HttpCode int
	Message  string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%d: %s", e.HttpCode, e.Message)
}

type Config struct {
	AdminEmail string
	Smtp       *SmtpConfig
}

type SmtpConfig struct {
	Server   string
	Username string
	Password string
	Port     int
	Sender   string
}
