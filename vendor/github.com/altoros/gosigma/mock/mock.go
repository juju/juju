// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package mock

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/altoros/gosigma/https"
)

//
// Implementation of CloudSigma server mock object for testing purposes.
//
//	Username: test@example.com
//	Password: test
//

const serverBase = "/api/2.0/"

const (
	// TestUser contains account name for log into mock server
	TestUser = "test@example.com"
	// TestPassword contains password for log into mock server
	TestPassword = "test"
)

var pServer *httptest.Server

// Start mock server for testing CloudSigma endpoint communication.
// If server is already started, this function does nothing.
func Start() {
	if IsStarted() {
		return
	}

	mux := http.NewServeMux()

	mux.HandleFunc(makeHandler("capabilities", capsHandler))
	mux.HandleFunc(makeHandler("drives", Drives.handleRequest))
	mux.HandleFunc(makeHandler("libdrives", LibDrives.handleRequest))
	mux.HandleFunc(makeHandler("servers", serversHandler))
	mux.HandleFunc(makeHandler("jobs", Jobs.handleRequest))

	pServer = httptest.NewUnstartedServer(mux)
	pServer.StartTLS()
}

type handlerType func(http.ResponseWriter, *http.Request)

func makeHandler(name string, f handlerType) (string, handlerType) {
	url := serverBase + name + "/"
	handler := func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		rec := httptest.NewRecorder()

		if isValidAuth(r) {
			f(rec, r)
		} else {
			rec.WriteHeader(401)
			rec.Write([]byte("401 Unauthorized\n"))
		}

		recordJournal(name, r, rec)

		hdr := w.Header()
		for k, v := range rec.HeaderMap {
			hdr[k] = v
		}

		w.WriteHeader(rec.Code)
		w.Write(rec.Body.Bytes())
	}
	return url, handler
}

func isValidAuth(r *http.Request) bool {
	s := strings.SplitN(r.Header.Get("Authorization"), " ", 2)
	if len(s) < 2 {
		return false
	}
	switch s[0] {
	case "Basic":
		return isValidBasicAuth(s[1])
	case "Digest":
		return isValidDigestAuth(s[1])
	}

	return false
}

func isValidBasicAuth(auth string) bool {
	b, err := base64.StdEncoding.DecodeString(auth)
	if err != nil {
		return false
	}
	pair := strings.SplitN(string(b), ":", 2)
	if len(pair) != 2 {
		return false
	}
	if pair[0] != TestUser {
		return false
	}
	if pair[1] != TestPassword {
		return false
	}
	return true
}

func isValidDigestAuth(auth string) bool {
	return false
}

// IsStarted checks the mock server is running
func IsStarted() bool {
	if pServer == nil {
		return false
	}
	return true
}

// Stop mock server.
// Panic if server is not started.
func Stop() {
	pServer.CloseClientConnections()
	pServer.Close()
	pServer = nil
}

// Reset mock server
func Reset() {
	Jobs.Reset()
	Drives.Reset()
	LibDrives.Reset()
	ResetServers()
}

// Endpoint of mock server, represented as string in form
// 'https://host:port/api/{version}/{section}'. Panic if server is not started.
func Endpoint(section string) string {
	return pServer.URL + serverBase + section
}

// GetAuth performs Get request to the given section of mock server with authentication
func GetAuth(section, username, password string) (*https.Response, error) {
	client := https.NewAuthClient(username, password, nil)
	url := Endpoint(section)
	return client.Get(url, nil)
}

// Get performs Get request to the given section of mock server with default authentication
func Get(section string) (*https.Response, error) {
	return GetAuth(section, TestUser, TestPassword)
}
