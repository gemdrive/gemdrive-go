package gemdrive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/anderspitman/treemess-go"
)

type Server struct {
	tmess      *treemess.TreeMess
	state      string
	runCtx     context.Context
	httpServer HttpServer
	config     *Config
	backend    Backend
	loginHtml  []byte
	db         *GemDriveDatabase
	keyAuth    *KeyAuth
	handler    http.Handler
}

type HttpServer interface {
	ListenAndServe() error
	Shutdown(context.Context) error
}

func NewServer(config *Config, tmess *treemess.TreeMess) (*Server, error) {

	var backend Backend

	if config.CacheDir == "" {
		config.CacheDir = filepath.Join(config.DataDir, "cache")
	}

	if len(config.Dirs) == 1 && config.RcloneDir == "" {
		fsBackend, err := NewFileSystemBackend(config.Dirs[0], config.CacheDir)
		if err != nil {
			return nil, err
		}

		backend = fsBackend
	} else {

		multiBackend := NewMultiBackend()

		for _, dir := range config.Dirs {
			dirName := filepath.Base(dir)
			subCacheDir := filepath.Join(config.CacheDir, dirName)
			fsBackend, err := NewFileSystemBackend(dir, subCacheDir)
			if err != nil {
				return nil, err
			}
			multiBackend.AddBackend(filepath.Base(dir), fsBackend)
		}

		if config.RcloneDir != "" {
			rcloneBackend := NewRcloneBackend()
			multiBackend.AddBackend(config.RcloneDir, rcloneBackend)
		}

		backend = multiBackend
	}

	db, err := NewGemDriveDatabase(config.DataDir)
	if err != nil {
		return nil, err
	}

	masterKey, err := db.GetMasterKey()
	if err != nil {
		fmt.Println("No master key found. Shouldn't be possible")
	}

	fmt.Println("Master key: " + masterKey)

	keyAuth, err := NewKeyAuth(db)
	if err != nil {
		return nil, err
	}

	mux := &http.ServeMux{}

	server := &Server{
		tmess:   tmess,
		state:   "stopped",
		config:  config,
		backend: backend,
		keyAuth: keyAuth,
		db:      db,
		handler: mux,
	}

	tmess.ListenFunc(func(msg treemess.Message) {
		//fmt.Println("gd tmess listen", msg.Channel, msg.Data)
		switch msg.Channel {
		case "start":
			server.start()
		case "stop":
			server.stop()
		case "server-stopped":
			server.runCtx = nil
			server.state = "stopped"
			tmess.Send("state-updated", server.state)
			//case "add-directory":
			//	dir := msg.Data.(string)
			//	dirName := filepath.Base(dir)
			//	subCacheDir := filepath.Join(config.CacheDir, dirName)
			//	fsBackend, err := NewFileSystemBackend(dir, subCacheDir)
			//	if err != nil {
			//		return
			//	}
			//	multiBackend.AddBackend(filepath.Base(dir), fsBackend)

			//	tmess.Send("directory-added", dir)
			//case "remove-directory":
			//	dir := msg.Data.(string)
			//	multiBackend.RemoveBackend(filepath.Base(dir))
			//	tmess.Send("directory-removed", dir)
		}
	})

	s := server

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

		header := w.Header()

		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}

		header["Access-Control-Allow-Origin"] = []string{origin}
		//header["Access-Control-Allow-Credentials"] = []string{"true"}
		header["Access-Control-Allow-Methods"] = []string{"*"}
		header["Access-Control-Allow-Headers"] = []string{"*"}
		if r.Method == "OPTIONS" {
			return
		}

		reqPath := r.URL.Path

		hostname := r.Header.Get("X-Forwarded-Host")
		if hostname == "" {
			hostname = r.Host
		}

		mappedRoot, exists := s.config.DomainMap[hostname]
		if !exists {
			mappedRoot = ""
		}

		logLine := fmt.Sprintf("%s\t%s\t%s", r.Method, hostname, reqPath)
		fmt.Println(logLine)

		ext := path.Ext(reqPath)
		contentType := mime.TypeByExtension(ext)
		header.Set("Content-Type", contentType)

		if hostname == s.config.DashboardDomain {
			io.WriteString(w, "<h1>GemDrive Dashboard</h1>")
			return
		} else if strings.HasPrefix(reqPath, "/gemdrive/") {
			s.handleGemDriveRequest(w, r, reqPath, mappedRoot)
		} else {

			reqPath = mappedRoot + reqPath

			override, exists := s.config.Overrides[reqPath]
			if exists {
				if override.ContentType != "" {
					header.Set("Content-Type", override.ContentType)
				}
			}

			switch r.Method {
			case "HEAD":
				s.handleHead(w, r, reqPath)
			case "GET":
				s.serveItem(w, r, reqPath)
			case "PUT":
				// TODO: return HTTP 409 if already exists
				s.handlePut(w, r, reqPath)
			case "PATCH":
				// TODO: return HTTP 409 if already exists
				s.handlePatch(w, r, reqPath)
			case "DELETE":
				s.handleDelete(w, r, reqPath)
			}
		}
	})

	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.config.Port),
		Handler: mux,
	}

	return server, nil
}

func (s *Server) DashboardDomain() string {
	return s.config.DashboardDomain
}

func (s *Server) FsDomain() string {
	return s.config.FsDomain
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

func (s *Server) start() {

	running := s.runCtx != nil
	if running {
		s.tmess.Send("error", "already-running")
		return
	}

	go func() {
		err := s.httpServer.ListenAndServe()
		if err != nil {
			s.tmess.Send("error", err.Error())
		}

		s.tmess.Send("server-stopped", nil)
	}()

	s.runCtx = context.Background()
	s.state = "running"
}

func (s *Server) stop() {

	running := s.runCtx != nil

	if !running {
		s.tmess.Send("error", "not-running")
		return
	}

	err := s.httpServer.Shutdown(s.runCtx)
	if err != nil {
		s.tmess.Send("error", err.Error())
	}
}

func (s *Server) handleHead(w http.ResponseWriter, r *http.Request, reqPath string) {

	token, _ := extractToken(r)

	if token == "" {
		token = "public"
	}

	header := w.Header()

	if !s.keyAuth.CanRead(token, reqPath) {
		s.sendUnauthorized(w, r)
		return
	}

	parentDir := filepath.Dir(reqPath) + "/"

	item, err := s.backend.List(parentDir, 1)
	if e, ok := err.(*Error); ok {
		w.WriteHeader(e.HttpCode)
		w.Write([]byte(e.Message))
		return
	} else if err != nil {
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
		return
	}

	filename := filepath.Base(reqPath)

	child, exists := item.Children[filename]
	if !exists {
		w.WriteHeader(404)
		io.WriteString(w, "Not found")
		return
	}

	modTime, err := time.Parse("2006-01-02T15:04:05Z", child.ModTime)
	if err != nil {
		w.WriteHeader(500)
		io.WriteString(w, "Invalid ModTime")
		return
	}

	header.Set("Last-Modified", modTime.Format(http.TimeFormat))
	header.Set("Content-Length", fmt.Sprintf("%d", child.Size))
}

func (s *Server) handlePut(w http.ResponseWriter, r *http.Request, reqPath string) {

	token, _ := extractToken(r)

	if token == "" {
		token = "public"
	}

	query := r.URL.Query()

	if !s.keyAuth.CanWrite(token, reqPath) {
		s.sendUnauthorized(w, r)
		return
	}

	backend, ok := s.backend.(WritableBackend)

	if !ok {
		w.WriteHeader(500)
		io.WriteString(w, "Backend does not support writing")
		return
	}

	isDir := strings.HasSuffix(reqPath, "/")

	if isDir {
		recursive := query.Get("recursive") == "true"
		err := backend.MakeDir(reqPath, recursive)
		if err != nil {
			w.WriteHeader(400)
			io.WriteString(w, err.Error())
			return
		}
	} else {
		var offset int64 = 0
		truncate := true
		overwrite := query.Get("overwrite") == "true"

		// TODO: consider allowing 0-length files
		if r.ContentLength < 1 {
			w.WriteHeader(400)
			io.WriteString(w, "Invalid write size")
			return
		}

		err := backend.Write(reqPath, r.Body, offset, r.ContentLength, overwrite, truncate)
		if err != nil {
			w.WriteHeader(500)
			io.WriteString(w, err.Error())
			return
		}

		s.setAttrs(w, r, reqPath, backend)
	}
}

func (s *Server) setAttrs(w http.ResponseWriter, r *http.Request, reqPath string, backend WritableBackend) {
	query := r.URL.Query()

	modTimeStr := query.Get("mod-time")
	isExecutable := query.Get("is-executable") == "true"

	if modTimeStr != "" || isExecutable {
		modTime, err := time.Parse("2006-01-02T15:04:05Z", modTimeStr)
		if err != nil {
			w.WriteHeader(500)
			io.WriteString(w, err.Error())
			return
		}

		err = backend.SetAttributes(reqPath, modTime, isExecutable)
		if err != nil {
			w.WriteHeader(500)
			io.WriteString(w, err.Error())
			return
		}
	}
}

func (s *Server) handlePatch(w http.ResponseWriter, r *http.Request, reqPath string) {

	token, _ := extractToken(r)

	if token == "" {
		token = "public"
	}

	query := r.URL.Query()

	if !s.keyAuth.CanWrite(token, reqPath) {
		s.sendUnauthorized(w, r)
		return
	}

	backend, ok := s.backend.(WritableBackend)

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

	err = backend.Write(reqPath, r.Body, int64(offset), int64(size), overwrite, truncate)
	if err != nil {
		w.WriteHeader(500)
		io.WriteString(w, err.Error())
		return
	}

	s.setAttrs(w, r, reqPath, backend)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request, reqPath string) {
	token, _ := extractToken(r)

	if token == "" {
		token = "public"
	}

	query := r.URL.Query()

	if !s.keyAuth.CanWrite(token, reqPath) {
		s.sendUnauthorized(w, r)
		return
	}

	backend, ok := s.backend.(WritableBackend)

	if !ok {
		w.WriteHeader(500)
		io.WriteString(w, "Backend does not support writing")
		return
	}

	recursive := query.Get("recursive") == "true"
	err := backend.Delete(reqPath, recursive)
	if e, ok := err.(*Error); ok {
		w.WriteHeader(e.HttpCode)
		w.Write([]byte(e.Message))
		return
	} else if err != nil {
		w.WriteHeader(500)
		io.WriteString(w, err.Error())
		return
	}
}

func (s *Server) sendUnauthorized(w http.ResponseWriter, r *http.Request) {
	header := w.Header()
	// Need to remove content type in case it's set
	header.Del("Content-Type")
	w.WriteHeader(403)
	io.WriteString(w, "Unauthorized")
}

func (s *Server) handleGemDriveRequest(w http.ResponseWriter, r *http.Request, reqPath, mappedRoot string) {

	token, _ := extractToken(r)

	if token == "" {
		token = "public"
	}

	gemReq := reqPath[len("/gemdrive"):]

	//if gemReq == "authorize" {

	//	s.authorize(w, r)

	//	return
	//}

	if gemReq == "/create-key" {
		s.createKey(w, r)
		return
	}

	if r.Method == "POST" && strings.HasPrefix(gemReq, "/remote-get") {
		s.remoteGet(w, r)
		return
	}

	if r.Method == "PUT" && strings.HasPrefix(gemReq, "/keys/") {

		pathParts := strings.Split(gemReq, "/")

		key := pathParts[len(pathParts)-1]

		s.setKey(w, r, key)
		return
	}

	if strings.HasPrefix(gemReq, "/index/") {

		listFilename := "list.json"
		treeFilename := "tree.json"

		depth := 1
		suffix := ""
		if strings.HasSuffix(gemReq, listFilename) {
			suffix = listFilename
		} else if strings.HasSuffix(gemReq, treeFilename) {

			suffix = treeFilename

			depth = 0

			depthParam := r.URL.Query().Get("depth")
			if depthParam != "" {
				var err error
				depth, err = strconv.Atoi(depthParam)
				if err != nil {
					w.WriteHeader(400)
					w.Write([]byte("Invalid depth param"))
					return
				}
			}
		}

		gemPath := mappedRoot + gemReq[len("/index"):len(gemReq)-len(suffix)]

		if !s.keyAuth.CanRead(token, gemPath) {
			s.sendUnauthorized(w, r)
			return
		}

		item, err := s.backend.List(gemPath, depth)
		if e, ok := err.(*Error); ok {
			w.WriteHeader(e.HttpCode)
			w.Write([]byte(e.Message))
			return
		} else if err != nil {
			w.WriteHeader(500)
			w.Write([]byte(err.Error()))
			return
		}

		jsonBody, err := json.Marshal(item)
		//jsonBody, err := json.MarshalIndent(item, "", "  ")
		if err != nil {
			w.WriteHeader(500)
			w.Write([]byte(err.Error()))
			return
		}

		w.Write(jsonBody)
	} else {
		gemReqParts := strings.Split(gemReq, "/")
		if gemReqParts[1] == "images" {

			sizeStr := gemReqParts[2]

			gemPath := mappedRoot + gemReq[len("/images/")+len(sizeStr):]

			if !s.keyAuth.CanRead(token, gemPath) {
				s.sendUnauthorized(w, r)
				return
			}

			if b, ok := s.backend.(ImageServer); ok {
				size, err := strconv.Atoi(sizeStr)
				if err != nil {
					w.WriteHeader(400)
					w.Write([]byte(err.Error()))
					return
				}

				imagePath := gemPath
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
		} else {
			w.WriteHeader(400)
			io.WriteString(w, "Invalid GemDrive request")
			return
		}
	}
}

func (s *Server) checkNewKeyRequest(w http.ResponseWriter, r *http.Request, parentKey string) (*KeyData, error) {

	bodyJson, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	reqKeyData := &KeyData{}
	err = json.Unmarshal(bodyJson, reqKeyData)
	if err != nil {
		return nil, err
	}

	parentKeyData, err := s.db.GetKeyData(parentKey)
	if err != nil {
		return nil, err
	}

	if !reqKeyData.IsSubsetOf(parentKeyData) {
		return nil, errors.New("You don't have permissions for that")
	}

	return reqKeyData, nil
}

func (s *Server) createKey(w http.ResponseWriter, r *http.Request) {

	parentKey, _ := extractToken(r)

	reqKeyData, err := s.checkNewKeyRequest(w, r, parentKey)
	if err != nil {
		w.WriteHeader(500)
		io.WriteString(w, err.Error())
		return
	}

	newKey, _ := genRandomKey()

	reqKeyData.Parent = parentKey

	s.db.SetKeyData(newKey, reqKeyData)

	io.WriteString(w, newKey)
}

func (s *Server) setKey(w http.ResponseWriter, r *http.Request, key string) {

	fmt.Println("key", key)

	reqKey, _ := extractToken(r)

	masterKey, err := s.db.GetMasterKey()
	if err != nil {
		w.WriteHeader(500)
		io.WriteString(w, err.Error())
		return
	}

	if reqKey != masterKey {
		w.WriteHeader(400)
		io.WriteString(w, "Only the master key can set keys")
		return
	}

	reqKeyData, err := s.checkNewKeyRequest(w, r, reqKey)
	if err != nil {
		w.WriteHeader(500)
		io.WriteString(w, err.Error())
		return
	}

	reqKeyData.Parent = reqKey

	s.db.SetKeyData(key, reqKeyData)
}

func (s *Server) serveItem(w http.ResponseWriter, r *http.Request, reqPath string) {

	token, _ := extractToken(r)

	if token == "" {
		token = "public"
	}

	if !s.keyAuth.CanRead(token, reqPath) {
		s.sendUnauthorized(w, r)
		return
	}

	isDir := strings.HasSuffix(reqPath, "/")

	if isDir {
		s.serveDir(w, r, reqPath)
	} else {
		s.serveFile(w, r, reqPath)
	}
}

func (s *Server) serveDir(w http.ResponseWriter, r *http.Request, reqPath string) {
	// If the directory contains an index.html file, serve that by default.
	// Otherwise reading a directory is an error.
	htmlIndexPath := reqPath + "index.html"
	item, data, err := s.backend.Read(htmlIndexPath, 0, 0)
	if err != nil {
		w.WriteHeader(400)
		io.WriteString(w, "Attempted to read directory")
		return
	}

	header := w.Header()
	header.Set("Content-Length", fmt.Sprintf("%d", item.Size))
	header.Set("Content-Type", "text/html")

	_, err = io.Copy(w, data)
	if err != nil {
		fmt.Println(err)
	}
}

func (s *Server) serveFile(w http.ResponseWriter, r *http.Request, reqPath string) {

	query := r.URL.Query()

	header := w.Header()
	header.Set("Accept-Ranges", "bytes")

	download := query.Get("download") == "true"
	if download {
		header.Set("Content-Disposition", "attachment")
	}

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

	item, data, err := s.backend.Read(reqPath, offset, copyLength)
	if readErr, ok := err.(*Error); ok {
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

	modTime, err := time.Parse("2006-01-02T15:04:05Z", item.ModTime)
	if err != nil {
		w.WriteHeader(500)
		io.WriteString(w, "Invalid ModTime")
		return
	}
	header.Set("Last-Modified", modTime.Format(http.TimeFormat))

	isExecutableHeader := "false"
	if item.IsExecutable {
		isExecutableHeader = "true"
	}
	header.Set("GemDrive-IsExecutable", isExecutableHeader)

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
