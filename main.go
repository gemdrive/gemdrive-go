package main

import (
	"./gemdrive"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

func main() {
	fsBackend := NewFileSystemBackend()
	rcloneBackend := NewRcloneBackend()
	multiBackend := gemdrive.NewMultiBackend()
	multiBackend.AddBackend("fs", fsBackend)
	multiBackend.AddBackend("rclone", rcloneBackend)
	server := NewRdriveServer(multiBackend)
	server.Run()
}

type RdriveServer struct {
	backend gemdrive.Backend
}

func NewRdriveServer(backend gemdrive.Backend) *RdriveServer {
	return &RdriveServer{
		backend,
	}
}

func (s *RdriveServer) Run() {

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

		w.Header()["Access-Control-Allow-Origin"] = []string{"*"}

		if strings.HasSuffix(r.URL.Path, "/.gemdrive-ls.json") {
			item, err := s.backend.List(r.URL.Path[:len(r.URL.Path)-len(".gemdrive-ls.json")])
			if e, ok := err.(*gemdrive.Error); ok {
				w.WriteHeader(e.HttpCode)
				w.Write([]byte(e.Message))
				return
			} else if err != nil {
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
			if e, ok := err.(*gemdrive.Error); ok {
				w.WriteHeader(e.HttpCode)
				w.Write([]byte(e.Message))
				return
			} else if err != nil {
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

			item, data, err := s.backend.Read(r.URL.Path, offset, copyLength)
			if readErr, ok := err.(*gemdrive.Error); ok {
				w.WriteHeader(readErr.HttpCode)
				w.Write([]byte(readErr.Message))
				return
			} else if err != nil {
				w.WriteHeader(500)
				w.Write([]byte("Server error"))
				return
			}
			defer data.Close()

			if rang != nil {
				end := rang.End
				if end == MAX_INT64 {
					end = item.Size - 1
				}
				l := end - rang.Start + 1
				fmt.Println("l", l, end, rang.Start)
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
	http.ListenAndServe(":9002", nil)
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
