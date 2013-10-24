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

	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/utils"
)

// IsSameStorage returns whether the storage instances are the same.
// Both storages must have been created through the dummy provider.
func IsSameStorage(s1, s2 storage.Storage) bool {
	localS1, localS2 := s1.(*dummyStorage), s2.(*dummyStorage)
	return localS1.env.name == localS2.env.name
}

func (e *environ) Storage() storage.Storage {
	return &dummyStorage{env: e}
}

// storageServer holds the storage for an environState.
type storageServer struct {
	path     string // path prefix in http space.
	state    *environState
	files    map[string][]byte
	poisoned map[string]error
}

func newStorageServer(state *environState, path string) *storageServer {
	return &storageServer{
		state:    state,
		files:    make(map[string][]byte),
		path:     path,
		poisoned: make(map[string]error),
	}
}

// Poison causes all fetches of the given path to
// return the given error.
func Poison(ss storage.Storage, path string, poisonErr error) {
	s := ss.(*dummyStorage)
	srv, err := s.server()
	if err != nil {
		panic("cannot poison destroyed storage")
	}
	srv.state.mu.Lock()
	srv.poisoned[path] = poisonErr
	srv.state.mu.Unlock()
}

func (s *storageServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
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

// dataWithDelay returns the data for the given path,
// waiting for the configured amount of time before
// accessing it.
func (s *storageServer) dataWithDelay(path string) (data []byte, err error) {
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

func (s *storageServer) Put(name string, r io.Reader, length int64) error {
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

func (s *storageServer) Get(name string) (io.ReadCloser, error) {
	data, err := s.dataWithDelay(name)
	if err != nil {
		return nil, err
	}
	return ioutil.NopCloser(bytes.NewBuffer(data)), nil
}

func (s *storageServer) URL(name string) (string, error) {
	return fmt.Sprintf("http://%v%s/%s", s.state.httpListener.Addr(), s.path, name), nil
}

func (s *storageServer) Remove(name string) error {
	s.state.mu.Lock()
	delete(s.files, name)
	s.state.mu.Unlock()
	return nil
}

func (s *storageServer) DefaultConsistencyStrategy() utils.AttemptStrategy {
	return utils.AttemptStrategy{}
}

// ShouldRetry is specified in the StorageReader interface.
func (s *storageServer) ShouldRetry(err error) bool {
	return false
}

func (s *storageServer) RemoveAll() error {
	s.state.mu.Lock()
	s.files = make(map[string][]byte)
	s.state.mu.Unlock()
	return nil
}

func (s *storageServer) List(prefix string) ([]string, error) {
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

// dummyStorage implements the client side of the Storage interface.
type dummyStorage struct {
	env *environ
}

// server returns the server side of the given storage.
func (s *dummyStorage) server() (*storageServer, error) {
	st, err := s.env.state()
	if err != nil {
		return nil, err
	}
	return st.storage, nil
}

func (s *dummyStorage) Get(name string) (io.ReadCloser, error) {
	srv, err := s.server()
	if err != nil {
		return nil, err
	}
	return srv.Get(name)
}

func (s *dummyStorage) URL(name string) (string, error) {
	srv, err := s.server()
	if err != nil {
		return "", err
	}
	return srv.URL(name)
}

func (s *dummyStorage) DefaultConsistencyStrategy() utils.AttemptStrategy {
	return utils.AttemptStrategy{}
}

// ShouldRetry is specified in the StorageReader interface.
func (s *dummyStorage) ShouldRetry(err error) bool {
	return false
}

func (s *dummyStorage) Put(name string, r io.Reader, length int64) error {
	srv, err := s.server()
	if err != nil {
		return err
	}
	return srv.Put(name, r, length)
}

func (s *dummyStorage) Remove(name string) error {
	srv, err := s.server()
	if err != nil {
		return err
	}
	return srv.Remove(name)
}

func (s *dummyStorage) RemoveAll() error {
	srv, err := s.server()
	if err != nil {
		return err
	}
	return srv.RemoveAll()
}

func (s *dummyStorage) List(prefix string) ([]string, error) {
	srv, err := s.server()
	if err != nil {
		return nil, err
	}
	return srv.List(prefix)
}
