// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/xml"
	"fmt"
	"hash/crc32"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/filestorage"
	"launchpad.net/juju-core/environs/httpstorage"
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/environs/tools"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/version"
)

const TestAuthkey = "jabberwocky"

// StartStorageServer starts a new local storage server using a temporary
// directory and returns the listener, a base URL for the server and the
// directory path.
func StartStorageServer(c *gc.C) (listener net.Listener, url, dataDir string) {
	dataDir = c.MkDir()
	embedded, err := filestorage.NewFileStorageWriter(dataDir, filestorage.UseDefaultTmpDir)
	c.Assert(err, gc.IsNil)
	listener, err = httpstorage.Serve("localhost:0", embedded)
	c.Assert(err, gc.IsNil)
	return listener, listener.Addr().String(), dataDir
}

// StartStorageServerTLS starts a new TLS-based local storage server using a
// temporary directory and returns the listener, a base URL for the server and
// the directory path.
func StartStorageServerTLS(c *gc.C) (listener net.Listener, url, dataDir string) {
	dataDir = c.MkDir()
	embedded, err := filestorage.NewFileStorageWriter(dataDir, filestorage.UseDefaultTmpDir)
	c.Assert(err, gc.IsNil)
	hostnames := []string{"127.0.0.1"}
	caCertPEM := []byte(coretesting.CACert)
	caKeyPEM := []byte(coretesting.CAKey)
	listener, err = httpstorage.ServeTLS("127.0.0.1:0", embedded, caCertPEM, caKeyPEM, hostnames, TestAuthkey)
	c.Assert(err, gc.IsNil)
	return listener, fmt.Sprintf("localhost:%d", listener.Addr().(*net.TCPAddr).Port), dataDir
}

// CreateLocalTestStorage returns the listener, which needs to be closed, and
// the storage that is backed by a directory created in the running tests temp
// directory.
func CreateLocalTestStorage(c *gc.C) (closer io.Closer, stor storage.Storage, dataDir string) {
	closer, addr, dataDir := StartStorageServer(c)
	stor = httpstorage.Client(addr)
	return
}

// CreateLocalTestStorageTLS
func CreateLocalTestStorageTLS(c *gc.C) (closer io.Closer, stor storage.Storage, dataDir string) {
	closer, addr, dataDir := StartStorageServerTLS(c)
	pstor, err := httpstorage.ClientTLS(addr, []byte(coretesting.CACert), TestAuthkey)
	c.Assert(err, gc.IsNil)
	err = pstor.Promote()
	c.Assert(err, gc.IsNil)
	stor = pstor
	return
}

func PatchDefaultClientCerts() testbase.Restorer {
	caCerts := x509.NewCertPool()
	caCerts.AppendCertsFromPEM([]byte(coretesting.CACert))
	return testbase.PatchValue(http.DefaultClient, http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: caCerts}},
	})
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

// EC2HTTPTestStorage acts like an EC2 storage which can be
// accessed by HTTP.
type EC2HTTPTestStorage struct {
	location string
	files    map[string][]byte
	listener net.Listener
}

// NewEC2HTTPTestStorage creates a storage server for tests
// with the HTTPStorageReader.
func NewEC2HTTPTestStorage(ip string) (*EC2HTTPTestStorage, error) {
	var err error
	s := &EC2HTTPTestStorage{
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
func (s *EC2HTTPTestStorage) Stop() error {
	return s.listener.Close()
}

// Location returns the location that has to be used in the tests.
func (s *EC2HTTPTestStorage) Location() string {
	return s.location
}

// PutBinary stores a faked binary in the HTTP test storage.
func (s *EC2HTTPTestStorage) PutBinary(v version.Binary) {
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
func (s *EC2HTTPTestStorage) handleIndex(w http.ResponseWriter, req *http.Request) {
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
func (s *EC2HTTPTestStorage) handleGet(w http.ResponseWriter, req *http.Request) {
	data, ok := s.files[req.URL.Path[1:]]
	if !ok {
		http.Error(w, "404 file not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(data)
}
