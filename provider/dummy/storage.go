// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dummy

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"sort"
	"strings"
	"time"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/utils"
)

func (e *environ) Storage() environs.Storage {
	return e.state().storage
}

func (e *environ) PublicStorage() environs.StorageReader {
	return e.state().publicStorage
}

func newStorage(state *environState, path string) *storage {
	return &storage{
		state:    state,
		files:    make(map[string][]byte),
		path:     path,
		poisoned: make(map[string]error),
	}
}

// Poison causes all fetches of the given path to
// return the given error.
func Poison(ss environs.Storage, path string, err error) {
	s := ss.(*storage)
	s.state.mu.Lock()
	s.poisoned[path] = err
	s.state.mu.Unlock()
}

func (s *storage) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != "GET" {
		http.Error(w, "only GET is supported", http.StatusMethodNotAllowed)
		return
	}
	data, err := s.dataWithDelay(req.URL.Path)
	if err != nil {
		http.Error(w, "404 "+err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	// If the write fails, the rest of the tests should pick up the problem.
	// It's more likely because the client has legitimately dropped the
	// connection.
	w.Write(data)
}

func (s *storage) Get(name string) (io.ReadCloser, error) {
	data, err := s.dataWithDelay(name)
	if err != nil {
		return nil, err
	}
	return ioutil.NopCloser(bytes.NewBuffer(data)), nil
}

// dataWithDelay returns the data for the given path,
// waiting for the configured amount of time before
// accessing it.
func (s *storage) dataWithDelay(path string) (data []byte, err error) {
	s.state.mu.Lock()
	delay := s.state.storageDelay
	s.state.mu.Unlock()
	time.Sleep(delay)
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	if err := s.poisoned[path]; err != nil {
		return nil, err
	}
	data, ok := s.files[path]
	if !ok {
		return nil, errors.NotFoundf("file %q not found", path)
	}
	return data, nil
}

func (s *storage) URL(name string) (string, error) {
	return fmt.Sprintf("http://%v%s/%s", s.state.httpListener.Addr(), s.path, name), nil
}

// ConsistencyStrategy is specified in the StorageReader interface.
func (s *storage) DefaultConsistencyStrategy() utils.AttemptStrategy {
	return utils.AttemptStrategy{}
}

// ShouldRetry is specified in the StorageReader interface.
func (s *storage) ShouldRetry(err error) bool {
	return false
}

func (s *storage) Put(name string, r io.Reader, length int64) error {
	// Allow Put to be poisoned as well.
	if err := s.poisoned[name]; err != nil {
		return err
	}

	// We only log Put requests on private storage.
	if strings.HasSuffix(s.path, "/private") {
		s.state.ops <- OpPutFile{s.state.name, name}
	}
	var buf bytes.Buffer
	_, err := io.Copy(&buf, r)
	if err != nil {
		return err
	}
	s.state.mu.Lock()
	s.files[name] = buf.Bytes()
	s.state.mu.Unlock()
	return nil
}

func (s *storage) Remove(name string) error {
	s.state.mu.Lock()
	delete(s.files, name)
	s.state.mu.Unlock()
	return nil
}

func (s *storage) RemoveAll() error {
	s.state.mu.Lock()
	s.files = make(map[string][]byte)
	s.state.mu.Unlock()
	return nil
}

func (s *storage) List(prefix string) ([]string, error) {
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	var names []string
	for name := range s.files {
		if strings.HasPrefix(name, prefix) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}
