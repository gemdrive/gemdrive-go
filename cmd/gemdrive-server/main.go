package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
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

	auth := gemdrive.NewAuth(*gemCacheDir, config)

	server := NewGemDriveServer(*port, multiBackend, auth)
	server.Run()
}

type GemDriveServer struct {
	port    int
	backend gemdrive.Backend
	auth    *gemdrive.Auth
}

func NewGemDriveServer(port int, backend gemdrive.Backend, auth *gemdrive.Auth) *GemDriveServer {
	return &GemDriveServer{
		port,
		backend,
		auth,
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

		header := w.Header()

		header["Access-Control-Allow-Origin"] = []string{"*"}

		fmt.Println(r.URL.Path)

		pathParts := strings.Split(r.URL.Path, "gemdrive/")

		if len(pathParts) == 2 {
			s.handleGemDriveRequest(w, r)
		} else {
			switch r.Method {
			case "GET":
				s.serveItem(w, r)
			case "PUT":
				s.handlePut(w, r)
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

	//token, _ := extractToken(r)
	//header := w.Header()

	//if !s.auth.CanWrite(token, r.URL.Path) {
	//	header.Set("WWW-Authenticate", "emauth realm=\"Everything\", charset=\"UTF-8\"")
	//	w.WriteHeader(403)
	//	io.WriteString(w, "Unauthorized")
	//	return
	//}

	//isDir := strings.HasSuffix(r.URL.Path, "/")

	//if isDir {
	//}
}

const LoginPageHtml = `
<form method="POST" action="/gemdrive/authorize">
  Email: <input type="email" name="email">
  <input type="hidden" name="perm" value="read">
  <input type="hidden" name="path" value="/">
  <input type="submit" value="Submit">
</form>
`

const LoginConfirmTemplate = `
<form method="POST" action="/gemdrive/authorize">
  Code: <input type="text" name="code">
  <input type="hidden" name="id" value="%s">
  <input type="submit" value="Submit">
</form>
`

func (s *GemDriveServer) handleGemDriveRequest(w http.ResponseWriter, r *http.Request) {

	token, _ := extractToken(r)

	header := w.Header()

	pathParts := strings.Split(r.URL.Path, "gemdrive/")

	gemPath := pathParts[0]
	gemReq := pathParts[1]

	if gemReq == "authorize" && r.Method == "POST" {

		s.authorize(w, r)

		return
	}

	if !s.auth.CanRead(token, gemPath) {
		header.Set("WWW-Authenticate", "emauth realm=\"Everything\", charset=\"UTF-8\"")
		w.WriteHeader(403)
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, LoginPageHtml)
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

	contentType := r.Header.Get("Content-Type")

	if contentType == "application/x-www-form-urlencoded" {
		r.ParseForm()

		id := r.Form.Get("id")
		code := r.Form.Get("code")

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
			}
			http.SetCookie(w, cookie)

			http.Redirect(w, r, "/", 303)
		} else {

			key := gemdrive.Key{
				IdType: "email",
				Id:     r.Form.Get("email"),
				Path:   r.Form.Get("path"),
				Perm:   r.Form.Get("perm"),
			}

			authId, err := s.auth.Authorize(key)
			if err != nil {
				w.WriteHeader(400)
				io.WriteString(w, err.Error())
				return
			}

			html := fmt.Sprintf(LoginConfirmTemplate, authId)

			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			io.WriteString(w, html)
		}
	} else {

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
}

func (s *GemDriveServer) serveItem(w http.ResponseWriter, r *http.Request) {

	token, _ := extractToken(r)

	header := w.Header()

	if !s.auth.CanRead(token, r.URL.Path) {
		header.Set("WWW-Authenticate", "emauth realm=\"Everything\", charset=\"UTF-8\"")
		header.Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(403)
		io.WriteString(w, LoginPageHtml)
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
