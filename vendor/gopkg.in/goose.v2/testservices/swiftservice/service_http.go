// Swift double testing service - HTTP API implementation

package swiftservice

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// verbatim real Swift responses
const (
	notFoundResponse = `404 Not Found

The resource could not be found.


`
	createdResponse = `201 Created




`
	acceptedResponse = `202 Accepted

The request is accepted for processing.


`
)

// handleContainers processes HTTP requests for container management.
func (s *Swift) handleContainers(container string, w http.ResponseWriter, r *http.Request) {
	var err error
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	exists := s.HasContainer(container)
	if !exists && r.Method != "PUT" {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(notFoundResponse))
		return
	}
	switch r.Method {
	case "GET":
		urlParams, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}
		params := make(map[string]string, len(urlParams))
		for k := range urlParams {
			params[k] = urlParams.Get(k)
		}
		contents, err := s.ListContainer(container, params)
		var objdata []byte
		if err == nil {
			objdata, err = json.Marshal(contents)
		}
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
		} else {
			w.WriteHeader(http.StatusOK)
			w.Header().Set("Content-Type", "application/json; charset=UF-8")
			w.Write([]byte(objdata))
		}
	case "DELETE":
		if err = s.RemoveContainer(container); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
		} else {
			w.Header().Set("Content-Length", "0")
			w.WriteHeader(http.StatusNoContent)
		}
	case "HEAD":
		urlParams, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}
		params := make(map[string]string, len(urlParams))
		for k := range urlParams {
			params[k] = urlParams.Get(k)
		}
		_, err = s.ListContainer(container, params)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
		} else {
			w.WriteHeader(http.StatusOK)
			w.Header().Set("Content-Type", "application/json; charset=UF-8")
		}
	case "PUT":
		if exists {
			w.WriteHeader(http.StatusAccepted)
			w.Write([]byte(acceptedResponse))
		} else {
			if err = s.AddContainer(container); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(err.Error()))
			} else {
				w.WriteHeader(http.StatusCreated)
				w.Write([]byte(createdResponse))
			}
		}
	case "POST":
		// [sodre]: we don't implement changing ACLs, so this always succeeds.
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(createdResponse))
	default:
		panic("not implemented request type: " + r.Method)
	}
}

// handleObjects processes HTTP requests for object management.
func (s *Swift) handleObjects(container, object string, w http.ResponseWriter, r *http.Request) {
	var err error
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	if exists := s.HasContainer(container); !exists {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(notFoundResponse))
		return
	}
	objdata, err := s.GetObject(container, object)
	if err != nil && r.Method != "PUT" {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(notFoundResponse))
		return
	}
	exists := err == nil
	switch r.Method {
	case "GET":
		// Note: even though the real Swift service does not
		// quote the Etag header value, we need to quote it here
		// because http.ServeContent requires the quotes, and
		// this means that we can have a naive client that puts
		// the exact Etag value in (for example) an If-Match
		// header and it will work with both the real Swift and
		// the local mock server. Note also that the HTTP
		// standard does require the Etag value to be quoted;
		// see https://tools.ietf.org/html/rfc7232#section-2.3
		// TODO maintain modification time.
		w.Header().Set("Etag", fmt.Sprintf(`"%x"`, md5.Sum(objdata)))
		http.ServeContent(w, r, object, time.Now(), bytes.NewReader(objdata))
	case "DELETE":
		if err = s.RemoveObject(container, object); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
		} else {
			w.Header().Set("Content-Length", "0")
			w.WriteHeader(http.StatusNoContent)
		}
	case "HEAD":
		w.Header().Set("Content-Length", strconv.Itoa(len(objdata)))
		w.Header().Set("Etag", fmt.Sprintf(`"%x"`, md5.Sum(objdata)))
		w.WriteHeader(http.StatusOK)
	case "PUT":
		bodydata, err := ioutil.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}
		if exists {
			err = s.RemoveObject(container, object)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(err.Error()))
			}
		}
		if err = s.AddObject(container, object, bodydata); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
		} else {
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(createdResponse))
		}
	default:
		panic("not implemented request type: " + r.Method)
	}
}

// ServeHTTP is the main entry point in the HTTP request processing.
func (s *Swift) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// TODO(wallyworld) - 2013-02-11 bug=1121682
	// we need to support container ACLs so we can have pubic containers.
	// For public containers, the token is not required to access the files. For now, if the request
	// does not provide a token, we will let it through and assume a public container is being accessed.
	token := r.Header.Get("X-Auth-Token")
	_, err := s.IdentityService.FindUser(token)
	if err != nil && s.FallbackIdentityService != nil {
		_, err = s.FallbackIdentityService.FindUser(token)
	}
	if token != "" && err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	path := strings.TrimRight(r.URL.Path, "/")
	path = strings.Trim(path, "/")
	parts := strings.SplitN(path, "/", 4)
	if len(parts) > 2 {
		parts = parts[2:]
		if len(parts) == 1 {
			container := parts[0]
			s.handleContainers(container, w, r)
		} else if len(parts) == 2 {
			container := parts[0]
			object := parts[1]
			s.handleObjects(container, object, w, r)
		}
	} else {
		panic("not implemented request: " + r.URL.Path)
	}
}

// setupHTTP attaches all the needed handlers to provide the HTTP API.
func (s *Swift) SetupHTTP(mux *http.ServeMux) {
	mux.Handle("/", s)
}

func (s *Swift) Stop() {
	// noop
}
