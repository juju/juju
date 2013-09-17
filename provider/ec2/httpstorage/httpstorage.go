// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpstorage

import (
	"encoding/xml"
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

var storageAttempt = utils.AttemptStrategy{
	Total: 5 * time.Second,
	Delay: 200 * time.Millisecond,
}

// httpStorageReader implements the environs.StorageReader interface
// to access an EC2 storage via HTTP.

type httpStorageReader struct {
	url string
}

// NewHTTPStorageReader creates a storage reader for the HTTP
// access to an EC2 storage like the juju-dist storage.
func NewHTTPStorageReader(url string) environs.StorageReader {
	return &httpStorageReader{url}
}

// Get implements environs.StorageReader.Get.
func (h *httpStorageReader) Get(name string) (io.ReadCloser, error) {
	nameURL, err := h.URL(name)
	if err != nil {
		return nil, err
	}
	resp, err := http.Get(nameURL)
	if err != nil || resp.StatusCode == http.StatusNotFound {
		return nil, errors.NewNotFoundError(err, "")
	}
	return resp.Body, nil
}

// List implements environs.StorageReader.List.
func (h *httpStorageReader) List(prefix string) ([]string, error) {
	lbr, err := h.getListResult()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, c := range lbr.Contents {
		if strings.HasPrefix(c.Key, prefix) {
			names = append(names, c.Key)
		}
	}
	sort.Strings(names)
	return names, nil
}

// URL implements environs.StorageReader.URL.
func (h *httpStorageReader) URL(name string) (string, error) {
	if strings.HasSuffix(h.url, "/") {
		return h.url + name, nil
	}
	return h.url + "/" + name, nil
}

// DefaultConsistencyStrategy is specified in the StorageReader interface.
func (s *httpStorageReader) DefaultConsistencyStrategy() utils.AttemptStrategy {
	return storageAttempt
}

// ShouldRetry is specified in the StorageReader interface.
func (h *httpStorageReader) ShouldRetry(err error) bool {
	return false
}

// getListResult retrieves the index of the storage,
func (h *httpStorageReader) getListResult() (*listResult, error) {
	resp, err := http.Get(h.url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var lbr listResult
	err = xml.Unmarshal(buf, &lbr)
	if err != nil {
		return nil, err
	}
	return &lbr, nil
}

// listResult is the top level XML element of the storage index.
// We only need the contents.
type listResult struct {
	Contents []*contents
}

// contents describes one entry of the storage index.
type contents struct {
	Key string
}
