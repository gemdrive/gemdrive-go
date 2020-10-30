package main

import (
        "encoding/json"
        "fmt"
        "io"
        "io/ioutil"
        "net/http"
        "os"
        "path"
        "strings"
        "strconv"
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
                        item, err := s.backend.List(r.URL.Path[:len(r.URL.Path) - len(".gemdrive-ls.json")])
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
                        item, err := s.backend.List(r.URL.Path[:len(r.URL.Path) - len(".gemdrive-ls.tsv")])
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
                        rangeHeader := r.Header.Get("Range")

                        var offset int64 = 0
                        var copyLength int64 = 0

                        if rangeHeader != "" {
                                r, err := parseRange(rangeHeader)
                                if err != nil {
                                        w.WriteHeader(500)
                                        w.Write([]byte(err.Error()))
                                }

                                offset = r.Start
                                copyLength = r.End - r.Start + 1

                                //w.Header()["Content-Range"] = fmt.Sprintf("%d-%d/%d", r.Start, r.End, 
                                w.WriteHeader(206)
                        }

                        reader, err := s.backend.Get(r.URL.Path, offset, copyLength)
                        if err != nil {
                                w.WriteHeader(500)
                                w.Write([]byte(err.Error()))
                        }

                        io.Copy(w, reader)
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

func (fs *FileSystemBackend) Get(reqPath string, offset, length int64) (io.Reader, error) {
        p := path.Join(fs.rootDir, reqPath)

        file, err := os.Open(p)
        if err != nil {
                return nil, err
        }

        file.Seek(offset, 0)

        reader, writer := io.Pipe()

        copyLength := length
        if copyLength == 0 {
                stat, err := file.Stat()
                if err != nil {
                        return nil, err
                }

                copyLength = stat.Size()
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

        return reader, nil
}

func DirToGemDrive(files []os.FileInfo) *Item {

        item := &Item{}

        if len(files) > 0 {
                item.Children = make(map[string]*Item)
        }

        for _, file := range files {
                if file.IsDir() {
                        item.Children[file.Name() + "/"] = &Item{
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

func parseRange(header string) (*HttpRange, error) {

        // TODO: this is very hacky and brittle
        parts := strings.Split(header, "=")
        rangeParts := strings.Split(parts[1], "-")

        start, err := strconv.Atoi(rangeParts[0])
        if err != nil {
                return nil, err
        }
        end, err := strconv.Atoi(rangeParts[1])
        if err != nil {
                return nil, err
        }

        return &HttpRange {
                Start: int64(start),
                End: int64(end),
        }, nil
}
