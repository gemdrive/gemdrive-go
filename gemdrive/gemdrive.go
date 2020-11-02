package gemdrive

import (
	"fmt"
	"io"
        "strings"
)

type Item struct {
	Size     int64            `json:"size"`
        // TODO: json should be mod_time, or some other name
        ModTime string `json:"modTime"`
	Children map[string]*Item `json:"children,omitempty"`
}

type Backend interface {
	List(path string) (*Item, error)
	Read(path string, offset, length int64) (*Item, io.ReadCloser, error)
}

type MultiBackend struct {
        backends map[string]Backend
}

type Error struct {
	HttpCode int
	Message  string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%d: %s", e.HttpCode, e.Message)
}

func NewMultiBackend() *MultiBackend {
        return &MultiBackend{backends: make(map[string]Backend)}
}

func (b *MultiBackend) AddBackend(name string, backend Backend) error {
        b.backends[name] = backend
        return nil
}

func (b * MultiBackend) List(reqPath string) (*Item, error) {
        if reqPath == "/" {
                rootItem := &Item{
                        Children: make(map[string]*Item),
                }

                for name := range b.backends {
                        rootItem.Children[name + "/"] = &Item{}
                }

                return rootItem, nil
        }

        parts := strings.Split(reqPath, "/")
        backendName := parts[1]
        subPath := "/" + strings.Join(parts[2:], "/")
        return b.backends[backendName].List(subPath)
}

func (b *MultiBackend) Read(reqPath string, offset, length int64) (*Item, io.ReadCloser, error) {
        parts := strings.Split(reqPath, "/")
        backendName := parts[1]
        subPath := "/" + strings.Join(parts[2:], "/")
        return b.backends[backendName].Read(subPath, offset, length)
}
