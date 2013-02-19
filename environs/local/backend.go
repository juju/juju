package local

import (
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// storageBackend provides HTTP access to a defined path.
type storageBackend struct {
	environName string
	uri         string
	path        string
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
	data, err := ioutil.ReadFile(filepath.Join(s.path, req.URL.Path))
	if err != nil {
		http.Error(w, fmt.Sprintf("404 %v", err), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(data)
}

// handleList returns the names in the storage to the client.
func (s *storageBackend) handleList(w http.ResponseWriter, req *http.Request) {
	fp := filepath.Join(s.path, req.URL.Path)
	dir, prefix := filepath.Split(fp)
	names, err := readDir(dir, prefix, len(s.path)+1)
	if err != nil {
		http.Error(w, fmt.Sprintf("404 %v", err), http.StatusNotFound)
		return
	}
	sort.Strings(names)
	data := []byte(strings.Join(names, "\n"))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(data)
}

// readDir reads the directory and compares the found file
// names with the given prefix.
func readDir(dir, prefix string, start int) ([]string, error) {
	prefix = prefix[:len(prefix)-1]
	names := []string{}
	fis, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, fi := range fis {
		name := fi.Name()
		if fi.IsDir() {
			continue
		}
		if strings.HasPrefix(name, prefix) {
			fullname := filepath.Join(dir[start:], name)
			names = append(names, fullname)
		}
	}
	return names, nil
}

// handlePut stores data from the client in the storage.
func (s *storageBackend) handlePut(w http.ResponseWriter, req *http.Request) {
	fp := filepath.Join(s.path, req.URL.Path)
	dir, _ := filepath.Split(fp)
	err := os.MkdirAll(dir, 0777)
	if err != nil {
		http.Error(w, fmt.Sprintf("403 %v", err), http.StatusForbidden)
		return
	}
	out, err := os.Create(fp)
	defer out.Close()
	if err != nil {
		http.Error(w, fmt.Sprintf("403 %v", err), http.StatusForbidden)
		return
	}
	_, err = io.Copy(out, req.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("403 %v", err), http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// handleDelete removes data from the storage.
func (s *storageBackend) handleDelete(w http.ResponseWriter, req *http.Request) {
	fp := filepath.Join(s.path, req.URL.Path)
	if err := os.Remove(fp); err != nil && !os.IsNotExist(err) {
		http.Error(w, fmt.Sprintf("500 %v", err), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// listen starts an HTTP listener to serve the
// provider storage.
func listen(dataPath, environName, ip string, port int) (net.Listener, error) {
	backend := &storageBackend{
		environName: environName,
		uri:         fmt.Sprintf("/%s/", environName),
		path:        filepath.Join(dataPath, environName),
	}
	if err := os.MkdirAll(backend.path, 0777); err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", ip, port))
	if err != nil {
		return nil, fmt.Errorf("cannot start listener: %v", err)
	}
	mux := http.NewServeMux()
	mux.Handle(backend.uri, http.StripPrefix(backend.uri, backend))

	go http.Serve(listener, mux)

	return listener, nil
}
