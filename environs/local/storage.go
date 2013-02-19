package local

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"sort"
	"strings"

	"launchpad.net/juju-core/environs"
)

// storage implements the environs.Storage interface.
type storage struct {
	baseURL string
}

// newStorage returns a new local storage.
func newStorage(address string, port int, environName string) *storage {
	return &storage{
		baseURL: fmt.Sprintf("http://%s:%d/%s", address, port, environName),
	}
}

// Get opens the given storage file and returns a ReadCloser
// that can be used to read its contents. It is the caller's
// responsibility to close it after use. If the name does not
// exist, it should return a *NotFoundError.
func (s *storage) Get(name string) (io.ReadCloser, error) {
	url, err := s.URL(name)
	if err != nil {
		return nil, err
	}
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, &environs.NotFoundError{fmt.Errorf("file %q not found", name)}
	}
	return resp.Body, nil
}

// List lists all names in the storage with the given prefix, in
// alphabetical order. The names in the storage are considered
// to be in a flat namespace, so the prefix may include slashes
// and the names returned are the full names for the matching
// entries.
func (s *storage) List(prefix string) ([]string, error) {
	url, err := s.URL(prefix)
	if err != nil {
		return nil, err
	}
	resp, err := http.Get(url + "*")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, errors.New(resp.Status)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if string(body) == "" {
		return nil, nil
	}
	names := strings.Split(string(body), "\n")
	sort.Strings(names)
	return names, nil
}

// URL returns a URL that can be used to access the given storage file.
func (s *storage) URL(name string) (string, error) {
	return fmt.Sprintf("%s/%s", s.baseURL, name), nil
}

// Put reads from r and writes to the given storage file.
// The length must give the total length of the file.
func (s *storage) Put(name string, r io.Reader, length int64) error {
	url, err := s.URL(name)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("PUT", url, r)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != 201 {
		return errors.New(resp.Status)
	}
	return nil
}

// Remove removes the given file from the environment's
// storage. It should not return an error if the file does
// not exist.
func (s *storage) Remove(name string) error {
	url, err := s.URL(name)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return errors.New(resp.Status)
	}
	return nil
}
