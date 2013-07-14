// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package localstorage

import (
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/errors"
	"net/http"
	"sort"
	"strings"
)

// storage implements the environs.Storage interface.
type storage struct {
	baseURL string
}

// Client returns a storage object that will talk to the storage server
// at the given network address (see Serve)
func Client(addr string) environs.Storage {
	return &storage{
		baseURL: fmt.Sprintf("http://%s/", addr),
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
	if resp.StatusCode != http.StatusOK {
		return nil, errors.NotFoundf("file %q", name)
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
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%d %s", resp.StatusCode, resp.Status)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, nil
	}
	names := strings.Split(string(body), "\n")
	sort.Strings(names)
	return names, nil
}

// URL returns an URL that can be used to access the given storage file.
func (s *storage) URL(name string) (string, error) {
	return s.baseURL + name, nil
}

// Put reads from r and writes to the given storage file.
// The length must be set to the total length of the file.
func (s *storage) Put(name string, r io.Reader, length int64) error {
	url, err := s.URL(name)
	if err != nil {
		return err
	}
	// Here we wrap up the reader.  For some freaky unexplainable reason, the
	// http library will call Close on the reader if it has a Close method
	// available.  Since we sometimes reuse the reader, especially when
	// putting tools, we don't want Close called.  So we wrap the reader in a
	// struct so the Close method is not exposed.
	justReader := struct{ io.Reader }{r}
	req, err := http.NewRequest("PUT", url, justReader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != 201 {
		return fmt.Errorf("%d %s", resp.StatusCode, resp.Status)
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
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%d %s", resp.StatusCode, resp.Status)
	}
	return nil
}

func (s *storage) RemoveAll() error {
	return environs.RemoveAll(s)
}
