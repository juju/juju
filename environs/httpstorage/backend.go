// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpstorage

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"

	"launchpad.net/juju-core/environs"
)

// TODO(axw) 2013-09-16 bug #1225916
// Implement authentication for modifying storage.

// storageBackend provides HTTP access to a storage object.
type storageBackend struct {
	backend environs.Storage
}

// ServeHTTP handles the HTTP requests to the container.
func (s *storageBackend) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "GET":
		if strings.HasSuffix(req.URL.Path, "*") {
			s.handleList(w, req)
		} else {
			s.handleGet(w, req)
		}
	case "PUT":
		s.handlePut(w, req)
	case "DELETE":
		s.handleDelete(w, req)
	default:
		http.Error(w, "method "+req.Method+" is not supported", http.StatusMethodNotAllowed)
	}
}

// handleGet returns a storage file to the client.
func (s *storageBackend) handleGet(w http.ResponseWriter, req *http.Request) {
	readcloser, err := s.backend.Get(req.URL.Path[1:])
	if err != nil {
		http.Error(w, fmt.Sprint(err), http.StatusNotFound)
		return
	}
	data, err := ioutil.ReadAll(readcloser)
	if err != nil {
		http.Error(w, fmt.Sprint(err), http.StatusInternalServerError)
		return
	}
	defer readcloser.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(data)
}

// handleList returns the file names in the storage to the client.
func (s *storageBackend) handleList(w http.ResponseWriter, req *http.Request) {
	prefix := req.URL.Path
	prefix = prefix[1 : len(prefix)-1] // drop the leading '/' and trailing '*'
	names, err := s.backend.List(prefix)
	if err != nil {
		http.Error(w, fmt.Sprint(err), http.StatusInternalServerError)
		return
	}
	data := []byte(strings.Join(names, "\n"))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(data)
}

// handlePut stores data from the client in the storage.
func (s *storageBackend) handlePut(w http.ResponseWriter, req *http.Request) {
	if req.ContentLength <= -1 {
		http.Error(w, "missing or invalid Content-Length header", http.StatusInternalServerError)
		return
	}
	err := s.backend.Put(req.URL.Path[1:], req.Body, req.ContentLength)
	if err != nil {
		http.Error(w, fmt.Sprint(err), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// handleDelete removes a file from the storage.
func (s *storageBackend) handleDelete(w http.ResponseWriter, req *http.Request) {
	err := s.backend.Remove(req.URL.Path[1:])
	if err != nil {
		http.Error(w, fmt.Sprint(err), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// Serve runs a storage server on the given network address, relaying
// requests to the given storage implementation. It returns the network
// listener. This can then be attached to with Client.
func Serve(addr string, storage environs.Storage) (net.Listener, error) {
	backend := &storageBackend{backend: storage}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("cannot start listener: %v", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/", backend)
	go http.Serve(listener, mux)
	return listener, nil
}
