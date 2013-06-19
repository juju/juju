// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"encoding/xml"
	"fmt"
	"hash/crc32"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"launchpad.net/goamz/s3"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/version"
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
	for a := shortAttempt.Start(); a.Next(); {
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

func (s *storage) deleteAll() error {
	names, err := s.List("")
	if err != nil {
		return err
	}
	// Remove all the objects in parallel so that we incur less round-trips.
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
		return &errors.NotFoundError{err, ""}
	}
	return err
}

// listBucketResult is the top level XML element of the storage index.
type listBucketResult struct {
	XMLName     xml.Name `xml: "ListBucketResult"`
	Name        string
	Prefix      string
	Marker      string
	MaxKeys     int
	IsTruncated bool
	Contents    []*contents
}

// content describes one entry of the storage index.
type contents struct {
	XMLName      xml.Name `xml: "Contents"`
	Key          string
	LastModified time.Time
	ETag         string
	Size         int
	StorageClass string
}

// HttpStorageReader implements the environs.StorageReader interface
// to access an EC2 storage via HTTP.
type HttpStorageReader struct {
	location string
}

// NewHttpStorageReader creates a storage reader for the HTTP
// access to an EC2 storage like the juju-dist storage.
func NewHttpStorageReader(location string) environs.StorageReader {
	return &HttpStorageReader{location}
}

// Get opens the given storage file and returns a ReadCloser
// that can be used to read its contents.
func (h *HttpStorageReader) Get(name string) (io.ReadCloser, error) {
	locationName, err := h.URL(name)
	if err != nil {
		return nil, err
	}
	resp, err := http.Get(locationName)
	if err != nil && resp.StatusCode == http.StatusNotFound {
		return nil, &errors.NotFoundError{err, ""}
	}
	return resp.Body, nil
}

// List lists all names in the storage with the given prefix.
func (h *HttpStorageReader) List(prefix string) ([]string, error) {
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

// URL returns a URL that can be used to access the given storage file.
func (h *HttpStorageReader) URL(name string) (string, error) {
	if strings.HasSuffix(h.location, "/") {
		return h.location + name, nil
	}
	return h.location + "/" + name, nil
}

// getListBucketResult retrieves the index of the storage,
func (h *HttpStorageReader) getListBucketResult() (*listBucketResult, error) {
	resp, err := http.Get(h.location)
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

// HttpTestStorage acts like an EC2 storage which can be
// accessed by HTTP.
type HttpTestStorage struct {
	location string
	files    map[string][]byte
	listener net.Listener
}

func NewHttpTestStorage(ip string) (*HttpTestStorage, error) {
	var err error
	s := &HttpTestStorage{
		files: make(map[string][]byte),
	}
	s.listener, err = net.Listen("tcp", fmt.Sprintf("%s:%d", ip, 0))
	if err != nil {
		return nil, fmt.Errorf("cannot start test listener: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case "GET":
			if req.URL.Path == "/" {
				s.handleIndex(w, req)
			} else {
				s.handleGet(w, req)
			}
		default:
			http.Error(w, "method "+req.Method+" is not supported", http.StatusMethodNotAllowed)
		}
	})
	s.location = fmt.Sprintf("http://%s:%d/", ip, s.listener.Addr().(*net.TCPAddr).Port)

	go http.Serve(s.listener, mux)

	return s, nil
}

// Stop stops the HTTP test storage.
func (s *HttpTestStorage) Stop() error {
	return s.listener.Close()
}

// Location returns the location that has to be used in the tests.
func (s *HttpTestStorage) Location() string {
	return s.location
}

// PutBinary stores a faked binary in the HTTP test storage.
func (s *HttpTestStorage) PutBinary(v version.Binary) {
	data := v.String()
	name := tools.StorageName(v)
	parts := strings.Split(name, "/")
	if len(parts) > 1 {
		// Also create paths as entries. Needed for
		// the correct contents of the list bucket result.
		path := ""
		for i := 0; i < len(parts)-1; i++ {
			path = path + parts[i] + "/"
			s.files[path] = []byte{}
		}
	}
	s.files[name] = []byte(data)
}

// handleIndex returns the index XML file to the client.
func (s *HttpTestStorage) handleIndex(w http.ResponseWriter, req *http.Request) {
	lbr := &listBucketResult{
		Name:        "juju-dist",
		Prefix:      "",
		Marker:      "",
		MaxKeys:     1000,
		IsTruncated: false,
	}
	names := []string{}
	for name := range s.files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		h := crc32.NewIEEE()
		h.Write([]byte(s.files[name]))
		contents := &contents{
			Key:          name,
			LastModified: time.Now(),
			ETag:         fmt.Sprintf("%x", h.Sum(nil)),
			Size:         len([]byte(s.files[name])),
			StorageClass: "STANDARD",
		}
		lbr.Contents = append(lbr.Contents, contents)
	}
	buf, err := xml.Marshal(lbr)
	if err != nil {
		http.Error(w, fmt.Sprintf("500 %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/xml")
	w.Write(buf)
}

// handleGet returns a storage file to the client.
func (s *HttpTestStorage) handleGet(w http.ResponseWriter, req *http.Request) {
	data, ok := s.files[req.URL.Path[1:]]
	if !ok {
		http.Error(w, "404 file not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(data)
}
