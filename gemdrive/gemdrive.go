package gemdrive

import (
	"fmt"
	"io"
)

type Item struct {
	Size     int64            `json:"size"`
	Children map[string]*Item `json:"children,omitempty"`
}

type Backend interface {
	List(path string) (*Item, error)
	Read(path string, offset, length int64) (*Item, io.ReadCloser, error)
}

type Error struct {
	HttpCode int
	Message  string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%d: %s", e.HttpCode, e.Message)
}
