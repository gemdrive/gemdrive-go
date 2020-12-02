package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	gemdrive "github.com/gemdrive/gemdrive-go"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
)

func main() {
	port := flag.Int("port", 3838, "Port")
	var dirs arrayFlags
	var gemDirs arrayFlags
	flag.Var(&dirs, "dir", "Directory to add")
	flag.Var(&gemDirs, "gemdir", "Gem Directory to add")
	rclone := flag.String("rclone", "", "Enable rclone proxy")
	flag.Parse()

	multiBackend := gemdrive.NewMultiBackend()

	for i, dir := range dirs {
		fsBackend, err := NewFileSystemBackend(dir, gemDirs[i])
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		multiBackend.AddBackend(path.Base(dir), fsBackend)
	}

	if *rclone != "" {
		rcloneBackend := NewRcloneBackend()
		multiBackend.AddBackend(*rclone, rcloneBackend)
	}

	server := NewRdriveServer(*port, multiBackend)
	server.Run()
}

type RdriveServer struct {
	port    int
	backend gemdrive.Backend
}

func NewRdriveServer(port int, backend gemdrive.Backend) *RdriveServer {
	return &RdriveServer{
		port,
		backend,
	}
}

// Taken from https://stackoverflow.com/a/28323276/943814
type arrayFlags []string

func (i *arrayFlags) String() string {
	return "my string representation"
}

func (i *arrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}

func (s *RdriveServer) Run() {

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

		w.Header()["Access-Control-Allow-Origin"] = []string{"*"}

		fmt.Println(r.URL.Path)

		pathParts := strings.Split(r.URL.Path, "gemdrive/")

		if len(pathParts) == 2 {

			gemPath := pathParts[0]
			gemReq := pathParts[1]

			if gemReq == "meta.json" {
				item, err := s.backend.List(gemPath)
				if e, ok := err.(*gemdrive.Error); ok {
					w.WriteHeader(e.HttpCode)
					w.Write([]byte(e.Message))
					return
				} else if err != nil {
					w.WriteHeader(500)
					w.Write([]byte(err.Error()))
					return
				}

				jsonBody, err := json.MarshalIndent(item, "", "  ")
				if err != nil {
					w.WriteHeader(500)
					w.Write([]byte(err.Error()))
					return
				}

				w.Write(jsonBody)
			} else {
				gemReqParts := strings.Split(gemReq, "/")
				if gemReqParts[0] == "images" {

					if b, ok := s.backend.(gemdrive.ImageServer); ok {
						size, err := strconv.Atoi(gemReqParts[1])
						if err != nil {
							w.WriteHeader(400)
							w.Write([]byte(err.Error()))
							return
						}

						filename := gemReqParts[2]
						imagePath := path.Join(gemPath, filename)
						img, _, err := b.GetImage(imagePath, size)
						if err != nil {
							w.WriteHeader(500)
							w.Write([]byte(err.Error()))
							return
						}

						_, err = io.Copy(w, img)
						if err != nil {
							fmt.Println(err)
						}
					}
				}
			}
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

			item, data, err := s.backend.Read(r.URL.Path, offset, copyLength)
			if readErr, ok := err.(*gemdrive.Error); ok {
				w.WriteHeader(readErr.HttpCode)
				w.Write([]byte(readErr.Message))
				return
			} else if err != nil {
				w.WriteHeader(500)
				w.Write([]byte(err.Error()))
				return
			}
			defer data.Close()

			if rang != nil {
				end := rang.End
				if end == MAX_INT64 {
					end = item.Size - 1
				}
				l := end - rang.Start + 1
				header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", rang.Start, end, item.Size))
				header.Set("Content-Length", fmt.Sprintf("%d", l))
				w.WriteHeader(206)
			} else {
				header.Set("Content-Length", fmt.Sprintf("%d", item.Size))
			}

			_, err = io.Copy(w, data)
			if err != nil {
				fmt.Println(err)
			}
		}
	})

	fmt.Println("Running")
	err := http.ListenAndServe(fmt.Sprintf(":%d", s.port), nil)
	if err != nil {
		fmt.Println(err)
	}
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
