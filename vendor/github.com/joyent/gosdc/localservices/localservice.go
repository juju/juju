//
// gosdc - Go library to interact with the Joyent CloudAPI
//
// Double testing service
//
// Copyright (c) 2013 Joyent Inc.
//
// Written by Daniele Stroppa <daniele.stroppa@joyent.com>
//

package localservices

import (
	"crypto/rand"
	"fmt"
	"io"
	"net/http"

	"github.com/joyent/gosdc/localservices/hook"
)

// An HttpService provides the HTTP API for a service double.
type HttpService interface {
	SetupHTTP(mux *http.ServeMux)
}

// A ServiceInstance is an Joyent Cloud service.
type ServiceInstance struct {
	hook.TestService
	Scheme      string
	Hostname    string
	UserAccount string
}

// NewUUID generates a random UUID according to RFC 4122
func NewUUID() (string, error) {
	uuid := make([]byte, 16)
	n, err := io.ReadFull(rand.Reader, uuid)
	if n != len(uuid) || err != nil {
		return "", err
	}
	uuid[8] = uuid[8]&^0xc0 | 0x80
	uuid[6] = uuid[6]&^0xf0 | 0x40
	return fmt.Sprintf("%x-%x-%x-%x-%x", uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:]), nil
}
