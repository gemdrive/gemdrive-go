package main

import (
	"./gemdrive"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"time"
)

type FileSystemBackend struct {
	rootDir string
}

func NewFileSystemBackend(dirPath string) (*FileSystemBackend, error) {
	stat, err := os.Stat(dirPath)
	if os.IsNotExist(err) {
		return nil, err
	} else if !stat.IsDir() {
		return nil, errors.New("Not a directory")
	}

	return &FileSystemBackend{rootDir: dirPath}, nil
}

func (fs *FileSystemBackend) List(reqPath string) (*gemdrive.Item, error) {
	p := path.Join(fs.rootDir, reqPath)

	files, err := ReadDir(p)
	if err != nil {
		return nil, err
	}

	item := DirToGemDrive(files)

	return item, nil
}

func (fs *FileSystemBackend) Read(reqPath string, offset, length int64) (*gemdrive.Item, io.ReadCloser, error) {
	p := path.Join(fs.rootDir, reqPath)

	file, err := os.Open(p)
	if err != nil {
		return nil, nil, &gemdrive.Error{
			HttpCode: 404,
			Message:  "Not found",
		}
	}

	file.Seek(offset, 0)
	if err != nil {
		return nil, nil, &gemdrive.Error{
			HttpCode: 500,
			Message:  "Error seeking file",
		}
	}

	stat, err := file.Stat()
	if err != nil {
		return nil, nil, &gemdrive.Error{
			HttpCode: 500,
			Message:  "Error stat'ing file",
		}
	}

	reader, writer := io.Pipe()

	copyLength := length
	if length == 0 {
		copyLength = stat.Size() - offset
	}

	go func() {
		defer file.Close()
		defer writer.Close()

		n, err := io.CopyN(writer, file, copyLength)
		if err != nil {
			fmt.Println(err.Error())
		}

		if n != copyLength {
			fmt.Println("n != copyLength", n, copyLength)
		}
	}()

	item := &gemdrive.Item{
		Size: stat.Size(),
	}

	return item, reader, nil
}

func DirToGemDrive(files []os.FileInfo) *gemdrive.Item {

	item := &gemdrive.Item{}

	if len(files) > 0 {
		item.Children = make(map[string]*gemdrive.Item)
	}

	for _, file := range files {
		var name string
		if file.IsDir() {
			name = file.Name() + "/"
		} else {
			name = file.Name()
		}

		item.Children[name] = &gemdrive.Item{
			Size:    file.Size(),
			ModTime: file.ModTime().Format(time.RFC3339),
		}
	}

	return item
}

// Like ioutil.ReadDir but follows symlinks
func ReadDir(dirPath string) ([]os.FileInfo, error) {

	dir, err := os.Open(dirPath)
	if err != nil {
		return nil, err
	}
	defer dir.Close()

	names, err := dir.Readdirnames(0)
	if err != nil {
		return nil, err
	}

	files := []os.FileInfo{}

	for _, name := range names {
		filePath := path.Join(dirPath, name)
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			return nil, err
		}

		files = append(files, fileInfo)
	}

	return files, nil
}
