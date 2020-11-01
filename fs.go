package main

import (
	"./gemdrive"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
)

type FileSystemBackend struct {
	rootDir string
}

func NewFileSystemBackend() *FileSystemBackend {
	rootDir := "./"

	return &FileSystemBackend{rootDir}
}

func (fs *FileSystemBackend) List(reqPath string) (*gemdrive.Item, error) {
	p := path.Join(fs.rootDir, reqPath)
	files, err := ioutil.ReadDir(p)
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
		if file.IsDir() {
			item.Children[file.Name()+"/"] = &gemdrive.Item{
				Size: file.Size(),
			}
		} else {
			item.Children[file.Name()] = &gemdrive.Item{
				Size: file.Size(),
			}
		}
	}

	return item
}