package gemdrive

import (
	"errors"
	"fmt"
	"io"
	"strings"
)

type Item struct {
	Size int64 `json:"size"`
	// TODO: json should be mod_time, or some other name
	ModTime  string           `json:"modTime"`
	Children map[string]*Item `json:"children,omitempty"`
}

type Backend interface {
	List(path string) (*Item, error)
	Read(path string, offset, length int64) (*Item, io.ReadCloser, error)
}

type ImageServer interface {
	GetImage(path string, size int) (io.Reader, int64, error)
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

func (b *MultiBackend) List(reqPath string) (*Item, error) {
	if reqPath == "/" {
		rootItem := &Item{
			Children: make(map[string]*Item),
		}

		for name := range b.backends {
			rootItem.Children[name+"/"] = &Item{}
		}

		return rootItem, nil
	}

	backendName, subPath, err := b.parsePath(reqPath)
	if err != nil {
		return nil, &Error{
			HttpCode: 404,
			Message:  "Not found",
		}
	}

	return b.backends[backendName].List(subPath)
}

func (b *MultiBackend) Read(reqPath string, offset, length int64) (*Item, io.ReadCloser, error) {

	backendName, subPath, err := b.parsePath(reqPath)
	if err != nil {
		return nil, nil, &Error{
			HttpCode: 404,
			Message:  "Not found",
		}
	}

	return b.backends[backendName].Read(subPath, offset, length)
}

func (b *MultiBackend) GetImage(reqPath string, size int) (io.Reader, int64, error) {

	backendName, subPath, err := b.parsePath(reqPath)
	if err != nil {
		return nil, 0, &Error{
			HttpCode: 404,
			Message:  "Not found",
		}
	}

	if backend, ok := b.backends[backendName].(ImageServer); ok {
		return backend.GetImage(subPath, size)
	}

	return nil, 0, errors.New("Backend does not support images")
}

func (b *MultiBackend) parsePath(reqPath string) (string, string, error) {
	parts := strings.Split(reqPath, "/")

	if len(parts) < 3 {
		return "", "", errors.New("Invalid path")
	}

	backendName := parts[1]

	if _, exists := b.backends[backendName]; !exists {
		return "", "", errors.New("Backend doesn't exist")
	}

	subPath := "/" + strings.Join(parts[2:], "/")

	return backendName, subPath, nil
}
