package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
)

func main() {
	backend := NewFileSystemBackend()
	server := NewRdriveServer(backend)
	server.Run()
}

type RdriveServer struct {
	backend Backend
}

func NewRdriveServer(backend Backend) *RdriveServer {
	return &RdriveServer{
		backend,
	}
}

func (s *RdriveServer) Run() {

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

		w.Header()["Access-Control-Allow-Origin"] = []string{"*"}

		if strings.HasSuffix(r.URL.Path, "/.gemdrive-ls.json") {
			item, err := s.backend.List(r.URL.Path[:len(r.URL.Path)-len(".gemdrive-ls.json")])
			if err != nil {
				w.WriteHeader(500)
				w.Write([]byte(err.Error()))
			}

			jsonBody, err := json.MarshalIndent(item, "", "  ")
			if err != nil {
				w.WriteHeader(500)
				w.Write([]byte(err.Error()))
			}

			w.Write(jsonBody)
		} else if strings.HasSuffix(r.URL.Path, "/.gemdrive-ls.tsv") {
			item, err := s.backend.List(r.URL.Path[:len(r.URL.Path)-len(".gemdrive-ls.tsv")])
			if err != nil {
				w.WriteHeader(500)
				w.Write([]byte(err.Error()))
			}

			outStr := ""

			if item.Children != nil {
				for name, child := range item.Children {
					line := fmt.Sprintf("%s\t%s\t%d\n", name, "2020-02-02", child.Size)
					outStr = outStr + line
				}
			}

			w.Write([]byte(outStr))
		} else {
			header := w.Header()
			header.Set("Accept-Ranges", "bytes")

			rangeHeader := r.Header.Get("Range")

			var offset int64 = 0
			var copyLength int64 = 0

			var rang *HttpRange
			if rangeHeader != "" {
				var err error
				rang, err = parseRange(rangeHeader)
				if err != nil {
					w.WriteHeader(500)
					w.Write([]byte(err.Error()))
					return
				}

				offset = rang.Start

				if rang.End != MAX_INT64 {
					copyLength = rang.End - rang.Start + 1
				}

			}

			item, reader, err := s.backend.Get(r.Context(), r.URL.Path, offset, copyLength)
			if err != nil {
				w.WriteHeader(404)
				w.Write([]byte("Not found"))
				return
			}

			if rang != nil {
				end := rang.End
				if end == MAX_INT64 {
					end = (item.Size - 1) - rang.Start
				}
				header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", rang.Start, end, item.Size))
				header.Set("Content-Length", fmt.Sprintf("%d", end-rang.Start+1))
				w.WriteHeader(206)
			}

			_, err = io.Copy(w, reader)
			if err != nil {
				fmt.Println(err)
			}
		}
	})

	fmt.Println("Running")
	http.ListenAndServe(":9002", nil)
}

type FileSystemBackend struct {
	rootDir string
}

func NewFileSystemBackend() *FileSystemBackend {
	rootDir := "./"

	return &FileSystemBackend{rootDir}
}

func (fs *FileSystemBackend) List(reqPath string) (*Item, error) {
	p := path.Join(fs.rootDir, reqPath)
	files, err := ioutil.ReadDir(p)
	if err != nil {
		return nil, err
	}

	item := DirToGemDrive(files)

	return item, nil
}

func (fs *FileSystemBackend) Get(ctx context.Context, reqPath string, offset, length int64) (*Item, io.Reader, error) {
	p := path.Join(fs.rootDir, reqPath)

	file, err := os.Open(p)
	if err != nil {
		return nil, nil, err
	}

	file.Seek(offset, 0)

	stat, err := file.Stat()
	if err != nil {
		return nil, nil, err
	}

	reader, writer := io.Pipe()

	copyLength := length
	if length == 0 {
		copyLength = stat.Size() - offset
	}

	go func() {
		<-ctx.Done()
		writer.Close()
	}()

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

func DirToGemDrive(files []os.FileInfo) *Item {

	item := &Item{}

	if len(files) > 0 {
		item.Children = make(map[string]*Item)
	}

	for _, file := range files {
		if file.IsDir() {
			item.Children[file.Name()+"/"] = &Item{
				Size: file.Size(),
			}
		} else {
			item.Children[file.Name()] = &Item{
				Size: file.Size(),
			}
		}
	}

	return item
}

type HttpRange struct {
	Start int64 `json:"start"`
	// Note: if end is 0 it won't be included in the json because of omitempty
	End int64 `json:"end,omitempty"`
}

// TODO: parse byte range specs properly according to
// https://tools.ietf.org/html/rfc7233
const MAX_INT64 int64 = 9223372036854775807

func parseRange(header string) (*HttpRange, error) {

	parts := strings.Split(header, "=")
	if len(parts) != 2 {
		return nil, errors.New("Invalid Range header")
	}

	rangeParts := strings.Split(parts[1], "-")
	if len(rangeParts) != 2 {
		return nil, errors.New("Invalid Range header")
	}

	var start int64 = 0
	if rangeParts[0] != "" {
		var err error
		start, err = strconv.ParseInt(rangeParts[0], 10, 64)
		if err != nil {
			return nil, err
		}
	}

	var end int64 = MAX_INT64
	if rangeParts[1] != "" {
		var err error
		end, err = strconv.ParseInt(rangeParts[1], 10, 64)
		if err != nil {
			return nil, err
		}
	}

	return &HttpRange{
		Start: start,
		End:   end,
	}, nil
}
