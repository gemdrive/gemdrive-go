package gemdrive

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/nfnt/resize"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

type FileSystemBackend struct {
	rootDir string
	gemDir  string
}

func NewFileSystemBackend(dirPath, gemDir string) (*FileSystemBackend, error) {
	stat, err := os.Stat(dirPath)
	if os.IsNotExist(err) {
		return nil, err
	} else if !stat.IsDir() {
		return nil, errors.New("Not a directory")
	}

	stat, err = os.Stat(gemDir)
	if os.IsNotExist(err) {
		return nil, err
	} else if !stat.IsDir() {
		return nil, errors.New("Not a directory")
	}

	return &FileSystemBackend{rootDir: dirPath, gemDir: gemDir}, nil
}

func (fs *FileSystemBackend) List(reqPath string) (*Item, error) {
	p := path.Join(fs.rootDir, reqPath)

	files, err := ReadDir(p)
	if err != nil {
		return nil, err
	}

	item := DirToGemDrive(files)

	return item, nil
}

func (fs *FileSystemBackend) Read(reqPath string, offset, length int64) (*Item, io.ReadCloser, error) {
	p := path.Join(fs.rootDir, reqPath)

	file, err := os.Open(p)
	if err != nil {
		return nil, nil, &Error{
			HttpCode: 404,
			Message:  "Not found",
		}
	}

	file.Seek(offset, 0)
	if err != nil {
		return nil, nil, &Error{
			HttpCode: 500,
			Message:  "Error seeking file",
		}
	}

	stat, err := file.Stat()
	if err != nil {
		return nil, nil, &Error{
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

	item := &Item{
		Size: stat.Size(),
	}

	return item, reader, nil
}

func (fs *FileSystemBackend) GetImage(reqPath string, size int) (io.Reader, int64, error) {

	p := path.Join(fs.rootDir, reqPath)
	sizeStr := fmt.Sprintf("%d", size)

	pathParts := strings.Split(reqPath, "/")
	parentDir := strings.Join(pathParts[:len(pathParts)-1], "/")
	filename := pathParts[len(pathParts)-1]

	imgDir := path.Join(fs.gemDir, parentDir, "gemdrive", "images", sizeStr)

	gemPath := path.Join(imgDir, filename)

	_, err := os.Stat(gemPath)
	if os.IsNotExist(err) {

		err := os.MkdirAll(imgDir, 0755)
		if err != nil {
			return nil, 0, err
		}

		file, err := os.Open(p)
		if err != nil {
			return nil, 0, err
		}

		img, err := decodeImage(reqPath, file)
		if err != nil {
			return nil, 0, err
		}
		file.Close()

		bounds := img.Bounds()
		width := bounds.Max.X
		height := bounds.Max.Y

		resizeWidth := uint(size)
		resizeHeight := uint(size)
		if width > height {
			resizeHeight = 0
		} else {
			resizeWidth = 0
		}

		m := resize.Resize(resizeWidth, resizeHeight, img, resize.Lanczos3)

		out, err := os.Create(gemPath)
		if err != nil {
			return nil, 0, err
		}
		defer out.Close()

		err = encodeImage(reqPath, out, m)
		if err != nil {
			return nil, 0, err
		}
	}

	data, err := ioutil.ReadFile(gemPath)
	if err != nil {
		return nil, 0, err
	}

	return bytes.NewReader(data), int64(len(data)), nil

}

func decodeImage(filename string, reader io.Reader) (image.Image, error) {
	ext := strings.ToLower(filepath.Ext(filename))

	switch ext {
	case ".jpg":
		fallthrough
	case ".jpeg":
		return jpeg.Decode(reader)
	case ".png":
		return png.Decode(reader)
	}

	return nil, errors.New("Invalid image file type")
}

func encodeImage(filename string, writer io.Writer, img image.Image) error {
	ext := strings.ToLower(filepath.Ext(filename))

	switch ext {
	case ".jpg":
		fallthrough
	case ".jpeg":
		return jpeg.Encode(writer, img, nil)
	case ".png":
		return png.Encode(writer, img)
	}

	return nil
}

func DirToGemDrive(files []os.FileInfo) *Item {

	item := &Item{}

	if len(files) > 0 {
		item.Children = make(map[string]*Item)
	}

	for _, file := range files {
		var name string
		if file.IsDir() {
			name = file.Name() + "/"
		} else {
			name = file.Name()
		}

		item.Children[name] = &Item{
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
