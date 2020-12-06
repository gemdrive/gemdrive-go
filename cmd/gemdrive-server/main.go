package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/GeertJohan/go.rice"
	gemdrive "github.com/gemdrive/gemdrive-go"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
)

func main() {
	port := flag.Int("port", 3838, "Port")
	var dirs arrayFlags
	flag.Var(&dirs, "dir", "Directory to add")
	gemCacheDir := flag.String("meta-dir", "./gemdrive", "Gem directory")
	rclone := flag.String("rclone", "", "Enable rclone proxy")
	flag.Parse()

	var config *gemdrive.Config
	configBytes, err := ioutil.ReadFile("gemdrive_config.json")
	if err != nil {
		log.Fatal(err)
	}

	err = json.Unmarshal(configBytes, &config)
	if err != nil {
		log.Fatal(err)
	}

	multiBackend := gemdrive.NewMultiBackend()

	for _, dir := range dirs {
		dirName := path.Base(dir)
		gemDir := path.Join(*gemCacheDir, dirName)
		fsBackend, err := gemdrive.NewFileSystemBackend(dir, gemDir)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		multiBackend.AddBackend(path.Base(dir), fsBackend)
	}

	if *rclone != "" {
		rcloneBackend := gemdrive.NewRcloneBackend()
		multiBackend.AddBackend(*rclone, rcloneBackend)
	}

	//dir := dirs[0]
	//dirName := path.Base(dir)
	//gemDir := path.Join(*gemCacheDir, dirName)
	//fmt.Println(dir, gemDir)
	//fsBackend, err := gemdrive.NewFileSystemBackend(dir, gemDir)
	auth := gemdrive.NewAuth(*gemCacheDir, config)

	server := NewGemDriveServer(*port, multiBackend, auth)
	//server := NewGemDriveServer(*port, fsBackend, auth)
	server.Run()
}

type GemDriveServer struct {
	port      int
	backend   gemdrive.Backend
	auth      *gemdrive.Auth
	loginHtml []byte
}

func NewGemDriveServer(port int, backend gemdrive.Backend, auth *gemdrive.Auth) *GemDriveServer {
	return &GemDriveServer{
		port:    port,
		backend: backend,
		auth:    auth,
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

func (s *GemDriveServer) Run() {

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

		box, err := rice.FindBox("files")
		if err != nil {
			w.WriteHeader(500)
			io.WriteString(w, err.Error())
			return
		}

		s.loginHtml, err = box.Bytes("login.html")
		if err != nil {
			w.WriteHeader(500)
			io.WriteString(w, err.Error())
			return
		}

		header := w.Header()

		header["Access-Control-Allow-Origin"] = []string{"*"}

		logLine := fmt.Sprintf("%s\t%s", r.Method, r.URL.Path)
		fmt.Println(logLine)

		pathParts := strings.Split(r.URL.Path, "gemdrive/")

		if len(pathParts) == 2 {
			s.handleGemDriveRequest(w, r)
		} else {
			switch r.Method {
			case "GET":
				s.serveItem(w, r)
			case "PUT":
				s.handlePut(w, r)
			case "PATCH":
				s.handlePatch(w, r)
			case "DELETE":
				s.handleDelete(w, r)
			}
		}
	})

	fmt.Println("Running")
	err := http.ListenAndServe(fmt.Sprintf(":%d", s.port), nil)
	if err != nil {
		fmt.Println(err)
	}
}

func (s *GemDriveServer) handlePut(w http.ResponseWriter, r *http.Request) {

	token, _ := extractToken(r)

	query := r.URL.Query()

	if !s.auth.CanWrite(token, r.URL.Path) {
		s.sendLoginPage(w, r)
		return
	}

	backend, ok := s.backend.(gemdrive.WritableBackend)

	if !ok {
		w.WriteHeader(500)
		io.WriteString(w, "Backend does not support writing")
		return
	}

	isDir := strings.HasSuffix(r.URL.Path, "/")

	if isDir {
		recursive := query.Get("recursive") == "true"
		err := backend.MakeDir(r.URL.Path, recursive)
		if err != nil {
			w.WriteHeader(400)
			io.WriteString(w, err.Error())
		}
	} else {
		offset := 0
		truncate := true
		overwrite := query.Get("overwrite") == "true"
		size, err := strconv.Atoi(r.Header.Get("Content-Length"))
		if err != nil {
			w.WriteHeader(400)
			io.WriteString(w, "Invalid content length")
		}

		err = backend.Write(r.URL.Path, r.Body, int64(offset), int64(size), overwrite, truncate)
		if err != nil {
			w.WriteHeader(500)
			io.WriteString(w, err.Error())
		}
	}
}

func (s *GemDriveServer) handlePatch(w http.ResponseWriter, r *http.Request) {

	token, _ := extractToken(r)

	query := r.URL.Query()

	if !s.auth.CanWrite(token, r.URL.Path) {
		s.sendLoginPage(w, r)
		return
	}

	backend, ok := s.backend.(gemdrive.WritableBackend)

	if !ok {
		w.WriteHeader(500)
		io.WriteString(w, "Backend does not support writing")
		return
	}

	overwrite := true
	truncate := false

	offsetParam := query.Get("offset")

	var offset int
	if offsetParam == "" {
		offset = 0
	} else {

		var err error
		offset, err = strconv.Atoi(query.Get("offset"))
		if err != nil {
			w.WriteHeader(400)
			io.WriteString(w, "Invalid offset")
			return
		}
	}

	size, err := strconv.Atoi(r.Header.Get("Content-Length"))
	if err != nil {
		w.WriteHeader(400)
		io.WriteString(w, "Invalid content length")
		return
	}

	err = backend.Write(r.URL.Path, r.Body, int64(offset), int64(size), overwrite, truncate)
	if err != nil {
		w.WriteHeader(500)
		io.WriteString(w, err.Error())
		return
	}
}

func (s *GemDriveServer) handleDelete(w http.ResponseWriter, r *http.Request) {
	token, _ := extractToken(r)

	query := r.URL.Query()

	if !s.auth.CanWrite(token, r.URL.Path) {
		s.sendLoginPage(w, r)
		return
	}

	backend, ok := s.backend.(gemdrive.WritableBackend)

	if !ok {
		w.WriteHeader(500)
		io.WriteString(w, "Backend does not support writing")
		return
	}

	recursive := query.Get("recursive") == "true"
	err := backend.Delete(r.URL.Path, recursive)
	if err != nil {
		w.WriteHeader(500)
		io.WriteString(w, err.Error())
		return
	}
}

func (s *GemDriveServer) sendLoginPage(w http.ResponseWriter, r *http.Request) {
	header := w.Header()
	header.Set("WWW-Authenticate", "emauth realm=\"Everything\", charset=\"UTF-8\"")
	header.Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(403)
	w.Write(s.loginHtml)
}

func (s *GemDriveServer) handleGemDriveRequest(w http.ResponseWriter, r *http.Request) {

	token, _ := extractToken(r)

	pathParts := strings.Split(r.URL.Path, "gemdrive/")

	gemPath := pathParts[0]
	gemReq := pathParts[1]

	if gemReq == "authorize" {

		s.authorize(w, r)

		return
	}

	if !s.auth.CanRead(token, gemPath) {
		s.sendLoginPage(w, r)
		return
	}

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
}

func (s *GemDriveServer) authorize(w http.ResponseWriter, r *http.Request) {

	query := r.URL.Query()
	id := query.Get("id")
	code := query.Get("code")

	if id != "" && code != "" {
		token, err := s.auth.CompleteAuth(id, code)
		if err != nil {
			w.WriteHeader(400)
			io.WriteString(w, err.Error())
			return
		}

		cookie := &http.Cookie{
			Name:  "access_token",
			Value: token,
			//Secure:   true,
			HttpOnly: true,
			MaxAge:   86400 * 365,
			Path:     "/",
			SameSite: http.SameSiteLaxMode,
		}
		http.SetCookie(w, cookie)

		io.WriteString(w, token)

	} else {
		bodyJson, err := ioutil.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(400)
			io.WriteString(w, err.Error())
			return
		}

		var key gemdrive.Key
		err = json.Unmarshal(bodyJson, &key)
		if err != nil {
			w.WriteHeader(400)
			io.WriteString(w, err.Error())
			return
		}

		authId, err := s.auth.Authorize(key)
		if err != nil {
			w.WriteHeader(400)
			io.WriteString(w, err.Error())
			return
		}

		io.WriteString(w, authId)
	}
}

func (s *GemDriveServer) serveItem(w http.ResponseWriter, r *http.Request) {

	token, _ := extractToken(r)

	header := w.Header()

	if !s.auth.CanRead(token, r.URL.Path) {
		s.sendLoginPage(w, r)
		return
	}

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

// Looks for auth token in cookie, then header, then query string
func extractToken(r *http.Request) (string, error) {
	tokenName := "access_token"

	query := r.URL.Query()

	queryToken := query.Get(tokenName)
	if queryToken != "" {
		return queryToken, nil
	}

	authHeader := r.Header.Get("Authorization")

	if authHeader != "" {
		tokenHeader := strings.Split(authHeader, " ")[1]
		return tokenHeader, nil
	}

	tokenCookie, err := r.Cookie(tokenName)

	if err == nil {
		return tokenCookie.Value, nil
	}

	return "", errors.New("No token found")
}
