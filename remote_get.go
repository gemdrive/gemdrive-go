package gemdrive

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
)

func (s *Server) remoteGet(w http.ResponseWriter, r *http.Request) {
	key, _ := extractToken(r)

	if key == "" {
		key = "public"
	}

	bodyJson, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(400)
		io.WriteString(w, err.Error())
		return
	}

	reqData := &RemoteGetRequest{}
	err = json.Unmarshal(bodyJson, reqData)
	if err != nil {
		w.WriteHeader(400)
		io.WriteString(w, err.Error())
		return
	}

	if reqData.Source == "" {
		w.WriteHeader(400)
		io.WriteString(w, "remote-get: Missing source "+reqData.Source)
		return
	}

	if reqData.Destination == "" {
		w.WriteHeader(400)
		io.WriteString(w, "remote-get: Missing destination "+reqData.Destination)
		return
	}

	if !s.keyAuth.CanWrite(key, reqData.Destination) {
		w.WriteHeader(403)
		io.WriteString(w, "remote-get: You don't have permission to write to"+reqData.Destination)
		return
	}

	backend, ok := s.backend.(WritableBackend)
	if !ok {
		w.WriteHeader(500)
		io.WriteString(w, "remote-get: Backend does not support writing")
		return
	}

	resp, err := http.Get(reqData.Source)
	if err != nil {
		w.WriteHeader(500)
		io.WriteString(w, "remote-get: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		w.WriteHeader(500)
		io.WriteString(w, fmt.Sprintf("remote-get: Failed with status %d", resp.StatusCode))
		return
	}

	err = backend.Write(reqData.Destination, resp.Body, reqData.DestinationOffset, resp.ContentLength, reqData.Overwrite, reqData.Truncate)
	if err != nil {
		w.WriteHeader(500)
		io.WriteString(w, err.Error())
		return
	}
}
