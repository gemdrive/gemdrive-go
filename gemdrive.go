package gemdrive

import (
	"fmt"
	"io"
)

type Item struct {
	Size         int64            `json:"size,omitempty"`
	ModTime      string           `json:"modTime,omitempty"`
	Children     map[string]*Item `json:"children,omitempty"`
	IsExecutable bool             `json:"isExecutable,omitempty"`
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
	AdminEmail string            `json:"admin_email,omitempty"`
	Smtp       *SmtpConfig       `json:"smtp,omitempty"`
	DomainMap  map[string]string `json:"domain_map,omitempty"`
}

type SmtpConfig struct {
	Server   string `json:"server,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Port     int    `json:"port,omitempty"`
	Sender   string `json:"sender,omitempty"`
}
