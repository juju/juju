// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpstorage

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/utils"
)

var logger = loggo.GetLogger("juju.environs.httpstorage")

// storage implements the storage.Storage interface.
type localStorage struct {
	addr   string
	client *http.Client

	authkey           string
	httpsBaseURL      string
	httpsBaseURLError error
	httpsBaseURLOnce  sync.Once
}

// Client returns a storage object that will talk to the
// storage server at the given network address (see Serve)
func Client(addr string) storage.Storage {
	return &localStorage{
		addr:   addr,
		client: utils.GetValidatingHTTPClient(),
	}
}

// ClientTLS returns a storage object that will talk to the
// storage server at the given network address (see Serve),
// using TLS. The client is given an authentication key,
// which the server will verify for Put and Remove* operations.
func ClientTLS(addr string, caCertPEM string, authkey string) (storage.Storage, error) {
	logger.Debugf("using https storage at %q", addr)
	caCerts := x509.NewCertPool()
	if !caCerts.AppendCertsFromPEM([]byte(caCertPEM)) {
		return nil, errors.New("error adding CA certificate to pool")
	}
	return &localStorage{
		addr:    addr,
		authkey: authkey,
		client: &http.Client{
			Transport: utils.NewHttpTLSTransport(&tls.Config{RootCAs: caCerts}),
		},
	}, nil
}

func (s *localStorage) getHTTPSBaseURL() (string, error) {
	url, _ := s.URL("") // never fails
	resp, err := s.client.Head(url)
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Could not access file storage: %v %s", url, resp.Status)
	}
	httpsURL, err := resp.Location()
	if err != nil {
		return "", err
	}
	return httpsURL.String(), nil
}

// Get opens the given storage file and returns a ReadCloser
// that can be used to read its contents. It is the caller's
// responsibility to close it after use. If the name does not
// exist, it should return a *NotFoundError.
func (s *localStorage) Get(name string) (io.ReadCloser, error) {
	logger.Debugf("getting %q from storage", name)
	url, err := s.URL(name)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Get(url)
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
func (s *localStorage) List(prefix string) ([]string, error) {
	url, err := s.URL(prefix)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Get(url + "*")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		// If the path is not found, it's not an error
		// because it's only created when the first
		// file is put.
		if resp.StatusCode == http.StatusNotFound {
			return []string{}, nil
		}
		return nil, fmt.Errorf("%s", resp.Status)
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

// URL returns a URL that can be used to access the given storage file.
func (s *localStorage) URL(name string) (string, error) {
	return fmt.Sprintf("http://%s/%s", s.addr, name), nil
}

// modURL returns a URL that can be used to modify the given storage file.
func (s *localStorage) modURL(name string) (string, error) {
	if s.authkey == "" {
		return s.URL(name)
	}
	s.httpsBaseURLOnce.Do(func() {
		s.httpsBaseURL, s.httpsBaseURLError = s.getHTTPSBaseURL()
	})
	if s.httpsBaseURLError != nil {
		return "", s.httpsBaseURLError
	}
	v := url.Values{}
	v.Set("authkey", s.authkey)
	return fmt.Sprintf("%s%s?%s", s.httpsBaseURL, name, v.Encode()), nil
}

// DefaultConsistencyStrategy is specified in the StorageReader interface.
func (s *localStorage) DefaultConsistencyStrategy() utils.AttemptStrategy {
	return utils.AttemptStrategy{}
}

// ShouldRetry is specified in the StorageReader interface.
func (s *localStorage) ShouldRetry(err error) bool {
	return false
}

// Put reads from r and writes to the given storage file.
// The length must be set to the total length of the file.
func (s *localStorage) Put(name string, r io.Reader, length int64) error {
	logger.Debugf("putting %q (len %d) to storage", name, length)
	url, err := s.modURL(name)
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
	req.ContentLength = length
	resp, err := s.client.Do(req)
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
func (s *localStorage) Remove(name string) error {
	url, err := s.modURL(name)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%d %s", resp.StatusCode, resp.Status)
	}
	return nil
}

func (s *localStorage) RemoveAll() error {
	return storage.RemoveAll(s)
}
