// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package mock

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/altoros/gosigma/https"
)

var chID = make(chan int)

func init() {
	go func() {
		for i := 0; ; i++ {
			chID <- i
		}
	}()
}

func genID() int {
	return <-chID
}

var goSigmaID = http.CanonicalHeaderKey("gosigma-id")
var errorNotFound = errors.New("gosigma-id not found")

// GetID returns journal ID from HTTP header
func GetID(h http.Header) (int, error) {
	if v, ok := h[goSigmaID]; ok && len(v) > 0 {
		return strconv.Atoi(v[0])
	}
	return -1, errorNotFound
}

// GetIDFromRequest returns journal ID from HTTP request
func GetIDFromRequest(r *http.Request) int {
	if id, err := GetID(r.Header); err == nil {
		return id
	}
	return genID()
}

// GetIDFromResponse returns journal ID from HTTP response
func GetIDFromResponse(r *https.Response) int {
	if id, err := GetID(r.Header); err == nil {
		return id
	}
	return genID()
}

// SetID adds specified journal ID to HTTP header
func SetID(h http.Header, id int) {
	h.Add(goSigmaID, strconv.Itoa(id))
}
