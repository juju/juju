// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpstorage

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"sort"
	"strings"
	"time"

	"launchpad.net/juju-core/cert"
	"launchpad.net/juju-core/environs/storage"
	coreerrors "launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/utils"
)

// storage implements the storage.Storage interface.
type localStorage struct {
	baseURL string
	client  *http.Client
}

// Client returns a storage object that will talk to the
// storage server at the given network address (see Serve)
func Client(addr string) storage.Storage {
	return &localStorage{
		baseURL: fmt.Sprintf("http://%s/", addr),
		client:  http.DefaultClient,
	}
}

// ClientTLS returns a storage object that will talk to the
// storage server at the given network address (see Serve),
// using TLS. The client will generate a client certificate,
// signed with the given CA certificate/key, to authenticate.
func ClientTLS(addr string, caCertPEM, caKeyPEM []byte) (storage.Storage, error) {
	caCerts := x509.NewCertPool()
	if !caCerts.AppendCertsFromPEM(caCertPEM) {
		return nil, errors.New("error adding CA certificate to pool")
	}
	expiry := time.Now().UTC().AddDate(10, 0, 0)
	clientCertPEM, clientKeyPEM, err := cert.NewClient(caCertPEM, caKeyPEM, expiry)
	clientCert, err := tls.X509KeyPair(clientCertPEM, clientKeyPEM)
	if err != nil {
		return nil, err
	}
	return &localStorage{
		baseURL: fmt.Sprintf("https://%s/", addr),
		client: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					Certificates: []tls.Certificate{clientCert},
					RootCAs:      caCerts,
				},
			},
		},
	}, nil
}

// Get opens the given storage file and returns a ReadCloser
// that can be used to read its contents. It is the caller's
// responsibility to close it after use. If the name does not
// exist, it should return a *NotFoundError.
func (s *localStorage) Get(name string) (io.ReadCloser, error) {
	url, err := s.URL(name)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Get(url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, coreerrors.NotFoundf("file %q", name)
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

// URL returns an URL that can be used to access the given storage file.
func (s *localStorage) URL(name string) (string, error) {
	return s.baseURL + name, nil
}

// ConsistencyStrategy is specified in the StorageReader interface.
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
	url, err := s.URL(name)
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
