// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpstorage

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"time"

	"launchpad.net/juju-core/cert"
	"launchpad.net/juju-core/environs/storage"
)

// storageBackend provides HTTP access to a storage object.
type storageBackend struct {
	backend storage.Storage

	// httpsPort is the port to send to clients
	// if they perform a HEAD request.
	httpsPort int

	// authkey is non-empty if modifying requests
	// require an auth key.
	authkey string
}

// ServeHTTP handles the HTTP requests to the container.
func (s *storageBackend) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "PUT", "DELETE":
		// Don't allow modifying operations if there's an HTTPS backend
		// to handle that, and ensure the user is authorized/authenticated.
		if s.httpsPort != 0 || !s.authorized(req) {
			http.Error(w, "unauthorized access", http.StatusUnauthorized)
			return
		}
	}
	switch req.Method {
	case "GET":
		if strings.HasSuffix(req.URL.Path, "*") {
			s.handleList(w, req)
		} else {
			s.handleGet(w, req)
		}
	case "HEAD":
		s.handleHead(w, req)
	case "PUT":
		s.handlePut(w, req)
	case "DELETE":
		s.handleDelete(w, req)
	default:
		http.Error(w, "method "+req.Method+" is not supported", http.StatusMethodNotAllowed)
	}
}

// authorized checks that either the storage does not require
// authorization, or the user has specified the correct auth key.
func (s *storageBackend) authorized(req *http.Request) bool {
	if s.authkey == "" {
		return true
	}
	return req.URL.Query().Get("authkey") == s.authkey
}

// hostOnly splits a host of the form host, or host:port,
// into its host and port parts, and returns the host part.
func hostOnly(host string) (string, error) {
	hostonly, _, err := net.SplitHostPort(host)
	if err != nil {
		// err may be because of missing :port. Checking
		// the error message is brittle, so let's try
		// again with ":0" tacked on the end.
		var err2 error
		hostonly, _, err = net.SplitHostPort(host + ":0")
		if err2 != nil {
			// something heinous, return the original error
			return "", err
		}
	}
	return hostonly, nil
}

// handleHead returns the HTTPS URL for the specified
// path in the Location header.
func (s *storageBackend) handleHead(w http.ResponseWriter, req *http.Request) {
	if s.httpsPort != 0 {
		host, err := hostOnly(req.Host)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to split host: %v", err), http.StatusBadRequest)
			return
		}
		url := fmt.Sprintf("https://%s:%d%s", host, s.httpsPort, req.URL.Path)
		w.Header().Set("Location", url)
	} else {
		http.Error(w, "method HEAD is not supported", http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleGet returns a storage file to the client.
func (s *storageBackend) handleGet(w http.ResponseWriter, req *http.Request) {
	readcloser, err := s.backend.Get(req.URL.Path[1:])
	if err != nil {
		http.Error(w, fmt.Sprint(err), http.StatusNotFound)
		return
	}
	defer readcloser.Close()
	data, err := ioutil.ReadAll(readcloser)
	if err != nil {
		http.Error(w, fmt.Sprint(err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(data)
}

// handleList returns the file names in the storage to the client.
func (s *storageBackend) handleList(w http.ResponseWriter, req *http.Request) {
	prefix := req.URL.Path
	prefix = prefix[1 : len(prefix)-1] // drop the leading '/' and trailing '*'
	names, err := s.backend.List(prefix)
	if err != nil {
		http.Error(w, fmt.Sprint(err), http.StatusInternalServerError)
		return
	}
	data := []byte(strings.Join(names, "\n"))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(data)
}

// handlePut stores data from the client in the storage.
func (s *storageBackend) handlePut(w http.ResponseWriter, req *http.Request) {
	if req.ContentLength < 0 {
		http.Error(w, "missing or invalid Content-Length header", http.StatusInternalServerError)
		return
	}
	err := s.backend.Put(req.URL.Path[1:], req.Body, req.ContentLength)
	if err != nil {
		http.Error(w, fmt.Sprint(err), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// handleDelete removes a file from the storage.
func (s *storageBackend) handleDelete(w http.ResponseWriter, req *http.Request) {
	if !s.authorized(req) {
		http.Error(w, "unauthorized access", http.StatusUnauthorized)
		return
	}
	err := s.backend.Remove(req.URL.Path[1:])
	if err != nil {
		http.Error(w, fmt.Sprint(err), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// Serve runs a storage server on the given network address, relaying
// requests to the given storage implementation. It returns the network
// listener. This can then be attached to with Client.
func Serve(addr string, stor storage.Storage) (net.Listener, error) {
	return serve(addr, stor, nil, "")
}

// ServeTLS runs a storage server on the given network address, relaying
// requests to the given storage implementation. The server runs a TLS
// listener, and verifies client certificates (if given) against the
// specified CA certificate. A client certificate is only required for
// PUT and DELETE methods.
//
// This method returns the network listener, which can then be attached
// to with ClientTLS.
func ServeTLS(addr string, stor storage.Storage, caCertPEM, caKeyPEM string, hostnames []string, authkey string) (net.Listener, error) {
	expiry := time.Now().UTC().AddDate(10, 0, 0)
	certPEM, keyPEM, err := cert.NewServer(caCertPEM, caKeyPEM, expiry, hostnames)
	if err != nil {
		return nil, err
	}
	serverCert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		return nil, err
	}
	caCerts := x509.NewCertPool()
	if !caCerts.AppendCertsFromPEM([]byte(caCertPEM)) {
		return nil, errors.New("error adding CA certificate to pool")
	}
	config := &tls.Config{
		NextProtos:   []string{"http/1.1"},
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.VerifyClientCertIfGiven,
		ClientCAs:    caCerts,
	}
	return serve(addr, stor, config, authkey)
}

func serve(addr string, stor storage.Storage, tlsConfig *tls.Config, authkey string) (net.Listener, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("cannot start listener: %v", err)
	}
	backend := &storageBackend{backend: stor}
	if tlsConfig != nil {
		tlsBackend := &storageBackend{backend: stor, authkey: authkey}
		tcpAddr := listener.Addr().(*net.TCPAddr)
		tlsListener, err := tls.Listen("tcp", fmt.Sprintf("[%s]:0", tcpAddr.IP), tlsConfig)
		if err != nil {
			listener.Close()
			return nil, fmt.Errorf("cannot start TLS listener: %v", err)
		}
		backend.httpsPort = tlsListener.Addr().(*net.TCPAddr).Port
		goServe(tlsListener, tlsBackend)
	}
	goServe(listener, backend)
	return listener, nil
}

func goServe(listener net.Listener, backend *storageBackend) {
	// Construct a NewServeMux to sanitise request paths.
	mux := http.NewServeMux()
	mux.Handle("/", backend)
	go http.Serve(listener, mux)
}
