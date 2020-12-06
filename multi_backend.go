package gemdrive

import (
	"errors"
	"io"
	"strings"
)

type MultiBackend struct {
	backends map[string]Backend
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

func (b *MultiBackend) MakeDir(reqPath string, recursive bool) error {
	backendName, subPath, err := b.parsePath(reqPath)
	if err != nil {
		return &Error{
			HttpCode: 404,
			Message:  "Not found",
		}
	}

	if backend, ok := b.backends[backendName].(WritableBackend); ok {
		return backend.MakeDir(subPath, recursive)
	}

	return nil
}

func (b *MultiBackend) Write(reqPath string, data io.Reader, offset, length int64, overwrite, truncate bool) error {

	backendName, subPath, err := b.parsePath(reqPath)
	if err != nil {
		return &Error{
			HttpCode: 404,
			Message:  "Not found",
		}
	}

	if backend, ok := b.backends[backendName].(WritableBackend); ok {
		return backend.Write(subPath, data, offset, length, overwrite, truncate)
	}

	return nil
}

func (b *MultiBackend) Delete(reqPath string, recursive bool) error {
	backendName, subPath, err := b.parsePath(reqPath)
	if err != nil {
		return &Error{
			HttpCode: 404,
			Message:  "Not found",
		}
	}

	if backend, ok := b.backends[backendName].(WritableBackend); ok {
		return backend.Delete(subPath, recursive)
	}

	return nil
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
