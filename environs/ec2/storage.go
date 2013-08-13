// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"launchpad.net/goamz/s3"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/utils"
)

func NewStorage(bucket *s3.Bucket) environs.Storage {
	return &storage{bucket: bucket}
}

// storage implements environs.Storage on
// an ec2.bucket.
type storage struct {
	sync.Mutex
	madeBucket bool
	bucket     *s3.Bucket
}

// makeBucket makes the environent's control bucket, the
// place where bootstrap information and deployed charms
// are stored. To avoid two round trips on every PUT operation,
// we do this only once for each environ.
func (s *storage) makeBucket() error {
	s.Lock()
	defer s.Unlock()
	if s.madeBucket {
		return nil
	}
	// PutBucket always return a 200 if we recreate an existing bucket for the
	// original s3.amazonaws.com endpoint. For all other endpoints PutBucket
	// returns 409 with a known subcode.
	if err := s.bucket.PutBucket(s3.Private); err != nil && s3ErrCode(err) != "BucketAlreadyOwnedByYou" {
		return err
	}

	s.madeBucket = true
	return nil
}

func (s *storage) Put(file string, r io.Reader, length int64) error {
	if err := s.makeBucket(); err != nil {
		return fmt.Errorf("cannot make S3 control bucket: %v", err)
	}
	err := s.bucket.PutReader(file, r, length, "binary/octet-stream", s3.Private)
	if err != nil {
		return fmt.Errorf("cannot write file %q to control bucket: %v", file, err)
	}
	return nil
}

func (s *storage) Get(file string) (r io.ReadCloser, err error) {
	for a := s.ConsistencyStrategy().Start(); a.Next(); {
		r, err = s.bucket.GetReader(file)
		if s3ErrorStatusCode(err) != 404 {
			break
		}
	}
	return r, maybeNotFound(err)
}

func (s *storage) URL(name string) (string, error) {
	// 10 years should be good enough.
	return s.bucket.SignedURL(name, time.Now().AddDate(10, 0, 0)), nil
}

var storageAttempt = utils.AttemptStrategy{
	Total: 5 * time.Second,
	Delay: 200 * time.Millisecond,
}

// ConsistencyStrategy is specified in the StorageReader interface.
func (s *storage) ConsistencyStrategy() utils.AttemptStrategy {
	return storageAttempt
}

// s3ErrorStatusCode returns the HTTP status of the S3 request error,
// if it is an error from an S3 operation, or 0 if it was not.
func s3ErrorStatusCode(err error) int {
	if err, _ := err.(*s3.Error); err != nil {
		return err.StatusCode
	}
	return 0
}

// s3ErrCode returns the text status code of the S3 error code.
func s3ErrCode(err error) string {
	if err, ok := err.(*s3.Error); ok {
		return err.Code
	}
	return ""
}

func (s *storage) Remove(file string) error {
	err := s.bucket.Del(file)
	// If we can't delete the object because the bucket doesn't
	// exist, then we don't care.
	if s3ErrorStatusCode(err) == 404 {
		return nil
	}
	return err
}

func (s *storage) List(prefix string) ([]string, error) {
	// TODO cope with more than 1000 objects in the bucket.
	resp, err := s.bucket.List(prefix, "", "", 0)
	if err != nil {
		// If the bucket is not found, it's not an error
		// because it's only created when the first
		// file is put.
		if s3ErrorStatusCode(err) == 404 {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, key := range resp.Contents {
		names = append(names, key.Key)
	}
	return names, nil
}

func (s *storage) RemoveAll() error {
	names, err := s.List("")
	if err != nil {
		return err
	}
	// Remove all the objects in parallel to minimize round-trips.
	// If we're in danger of having hundreds of objects,
	// we'll want to change this to limit the number
	// of concurrent operations.
	var wg sync.WaitGroup
	wg.Add(len(names))
	errc := make(chan error, len(names))
	for _, name := range names {
		name := name
		go func() {
			if err := s.Remove(name); err != nil {
				errc <- err
			}
			wg.Done()
		}()
	}
	wg.Wait()
	select {
	case err := <-errc:
		return fmt.Errorf("cannot delete all provider state: %v", err)
	default:
	}

	s.Lock()
	defer s.Unlock()
	// Even DelBucket fails, it won't harm if we try again - the operation
	// might have succeeded even if we get an error.
	s.madeBucket = false
	err = s.bucket.DelBucket()
	if s3ErrorStatusCode(err) == 404 {
		return nil
	}
	return err
}

func maybeNotFound(err error) error {
	if err != nil && s3ErrorStatusCode(err) == 404 {
		return errors.NewNotFoundError(err, "")
	}
	return err
}

// listBucketResult is the top level XML element of the storage index.
// We only need the contents.
type listBucketResult struct {
	Contents []*contents
}

// contents describes one entry of the storage index.
type contents struct {
	Key string
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
	lbr, err := h.getListBucketResult()
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

// ConsistencyStrategy is specified in the StorageReader interface.
func (s *httpStorageReader) ConsistencyStrategy() utils.AttemptStrategy {
	return storageAttempt
}

// getListBucketResult retrieves the index of the storage,
func (h *httpStorageReader) getListBucketResult() (*listBucketResult, error) {
	resp, err := http.Get(h.url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var lbr listBucketResult
	err = xml.Unmarshal(buf, &lbr)
	if err != nil {
		return nil, err
	}
	return &lbr, nil
}
