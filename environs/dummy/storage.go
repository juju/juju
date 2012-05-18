package dummy

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/juju/go/environs"
	"net/http"
	"sort"
	"strings"
)

func (e *environ) Storage() environs.Storage {
	return e.state.storage
}

func (e *environ) PublicStorage() environs.StorageReader {
	return e.state.publicStorage
}

func newStorage(state *environState, path string) *storage {
	return &storage{
		state: state,
		files: make(map[string][]byte),
		path:  path,
	}
}

func (s *storage) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != "GET" {
		http.Error(w, "only GET is supported", http.StatusMethodNotAllowed)
		return
	}
	s.state.mu.Lock()
	data, ok := s.files[req.URL.Path]
	s.state.mu.Unlock()
	if !ok {
		http.NotFound(w, req)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	// If the write fails, the rest of the tests should pick up the problem.
	// It's more likely because the client has legitimately dropped the
	// connection.
	w.Write(data)
}

func (s *storage) URL(name string) (string, error) {
	return fmt.Sprintf("http://%v%s/%s", s.state.httpListener.Addr(), s.path, name), nil
}

func (s *storage) Put(name string, r io.Reader, length int64) error {
	// We only log Put requests on private storage.
	if strings.HasSuffix(s.path, "/private") {
		s.state.ops <- Operation{Kind: OpPutFile, Env: s.state.name}
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

func (s *storage) Get(name string) (io.ReadCloser, error) {
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	data, ok := s.files[name]
	if !ok {
		return nil, &environs.NotFoundError{fmt.Errorf("file %q not found", name)}
	}
	return ioutil.NopCloser(bytes.NewBuffer(data)), nil
}

func (s *storage) Remove(name string) error {
	s.state.mu.Lock()
	delete(s.files, name)
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
