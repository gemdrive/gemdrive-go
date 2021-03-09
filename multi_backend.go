package gemdrive

import (
	"errors"
	"io"
	"strings"
	"sync"
)

type MultiBackend struct {
	backends map[string]Backend
	mut      *sync.Mutex
}

func NewMultiBackend() *MultiBackend {
	return &MultiBackend{
		backends: make(map[string]Backend),
		mut:      &sync.Mutex{},
	}
}

func (b *MultiBackend) AddBackend(name string, backend Backend) error {

	b.mut.Lock()
	defer b.mut.Unlock()

	b.backends[name] = backend
	return nil
}

func (b *MultiBackend) RemoveBackend(name string) error {

	b.mut.Lock()
	defer b.mut.Unlock()

	delete(b.backends, name)

	return nil
}

func (b *MultiBackend) List(reqPath string, depth int) (*Item, error) {

	b.mut.Lock()
	backends := make(map[string]Backend)
	for k, v := range b.backends {
		backends[k] = v
	}
	b.mut.Unlock()

	if reqPath == "/" {
		rootItem := &Item{
			Children: make(map[string]*Item),
		}

		for name := range backends {
			rootItem.Children[name+"/"] = &Item{}
		}

		if depth == 0 || depth > 1 {
			for name, backend := range backends {
				child, err := backend.List("/", depth-1)
				if err != nil {
					return nil, err
				}

				rootItem.Children[name+"/"] = child
			}
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

	return backends[backendName].List(subPath, depth)
}

func (b *MultiBackend) Read(reqPath string, offset, length int64) (*Item, io.ReadCloser, error) {

	backendName, subPath, err := b.parsePath(reqPath)
	if err != nil {
		return nil, nil, &Error{
			HttpCode: 404,
			Message:  "Not found",
		}
	}

	b.mut.Lock()
	backend := b.backends[backendName]
	b.mut.Unlock()

	return backend.Read(subPath, offset, length)
}

func (b *MultiBackend) MakeDir(reqPath string, recursive bool) error {
	backendName, subPath, err := b.parsePath(reqPath)
	if err != nil {
		return &Error{
			HttpCode: 404,
			Message:  "Not found",
		}
	}

	b.mut.Lock()
	backend := b.backends[backendName]
	b.mut.Unlock()

	if backend, ok := backend.(WritableBackend); ok {
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

	b.mut.Lock()
	backend := b.backends[backendName]
	b.mut.Unlock()

	if backend, ok := backend.(WritableBackend); ok {
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

	b.mut.Lock()
	backend := b.backends[backendName]
	b.mut.Unlock()

	if backend, ok := backend.(WritableBackend); ok {
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

	b.mut.Lock()
	backend := b.backends[backendName]
	b.mut.Unlock()

	if backend, ok := backend.(ImageServer); ok {
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

	b.mut.Lock()
	_, exists := b.backends[backendName]
	b.mut.Unlock()

	if !exists {
		return "", "", errors.New("Backend doesn't exist")
	}

	subPath := "/" + strings.Join(parts[2:], "/")

	return backendName, subPath, nil
}
