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
	return &storage{
		env: e,
		public: false,
	}
}

func (e *environ) PublicStorage() environs.StorageReader {
	return &storage{
		env: e,
		public: true,
	}
}

// dummystorage holds the storage for an environState.
// There are two instances for each environState
// instance, one for public files and one for private.
type dummystorage struct {
	path     string // path prefix in http space.
	state    *environState
	files    map[string][]byte
	poisoned map[string]error
}

func newStorage(state *environState, path string) *dummystorage {
	return &dummystorage{
		state:    state,
		files:    make(map[string][]byte),
		path:     path,
		poisoned: make(map[string]error),
	}
}

// Poison causes all fetches of the given path to
// return the given error.
func Poison(ss environs.Storage, path string, err error) {
	s := ss.(*dummystorage)
	srv, err := s.server()
	if err != nil {
		panic("cannot poison destroyed storage")
	}
	srv.state.mu.Lock()
	srv.poisoned[path] = err
	srv.state.mu.Unlock()
}

func (s *dummystorage) ServeHTTP(w http.ResponseWriter, req *http.Request) {
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
func (s *dummystorage) dataWithDelay(path string) (data []byte, err error) {
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

func (s *dummystorage) Put(name string, r io.Reader, length int64) error {
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

func (s *dummystorage) Get(name string) (io.ReadCloser, error) {
	data, err := s.dataWithDelay(name)
	if err != nil {
		return nil, err
	}
	return ioutil.NopCloser(bytes.NewBuffer(data)), nil
}

func (s *dummystorage) URL(name string) (string, error) {
	return fmt.Sprintf("http://%v%s/%s", s.state.httpListener.Addr(), s.path, name), nil
}

func (s *dummystorage) Remove(name string) error {
	s.state.mu.Lock()
	delete(s.files, name)
	s.state.mu.Unlock()
	return nil
}

func (s *dummystorage) ConsistencyStrategy() utils.AttemptStrategy {
	return utils.AttemptStrategy{}
}

// ShouldRetry is specified in the StorageReader interface.
func (s *dummystorage) ShouldRetry(err error) bool {
	return false
}

func (s *dummystorage) RemoveAll() error {
	s.state.mu.Lock()
	s.files = make(map[string][]byte)
	s.state.mu.Unlock()
	return nil
}

func (s *dummystorage) List(prefix string) ([]string, error) {
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

// storage implements the client side of the Storage interface.
type storage struct {
	env *environ
	public bool
}

// server returns the server side of the given storage.
func (s *storage) server() (*dummystorage, error) {
	st, err := s.env.state()
	if err != nil {
		return nil, err
	}
	if s.public {
		return st.publicStorage, nil
	}
	return st.storage, nil
}

func (s *storage) Get(name string) (io.ReadCloser, error) {
	srv, err := s.server()
	if err != nil {
		return nil, err
	}
	return srv.Get(name)
}

func (s *storage) URL(name string) (string, error) {
	srv, err := s.server()
	if err != nil {
		return "", err
	}
	return srv.URL(name)
}

func (s *storage) ConsistencyStrategy() utils.AttemptStrategy {
	return utils.AttemptStrategy{}
}

func (s *storage) Put(name string, r io.Reader, length int64) error {
	srv, err := s.server()
	if err != nil {
		return err
	}
	return srv.Put(name, r, length)
}

func (s *storage) Remove(name string) error {
	srv, err := s.server()
	if err != nil {
		return err
	}
	return srv.Remove(name)
}

func (s *storage) RemoveAll() error {
	srv, err := s.server()
	if err != nil {
		return err
	}
	return srv.RemoveAll()
}

func (s *storage) List(prefix string) ([]string, error) {
	srv, err := s.server()
	if err != nil {
		return nil, err
	}
	return srv.List(prefix)
}
