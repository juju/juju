// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package router // import "gopkg.in/juju/charmstore.v5-unstable/internal/router"

import (
	"encoding/json"
	"net/http"
	"net/url"

	"gopkg.in/errgo.v1"
)

var _ BulkIncludeHandler = SingleIncludeHandler(nil)

// SingleIncludeHandler implements BulkMetaHander for a non-batching
// metadata retrieval function that can perform a GET only.
type SingleIncludeHandler func(id *ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error)

// Key implements BulkMetadataHander.Key.
func (h SingleIncludeHandler) Key() interface{} {
	// Use a local type so that we are guaranteed that nothing
	// other than SingleIncludeHandler can generate that key.
	type singleMetaHandlerKey struct{}
	return singleMetaHandlerKey(singleMetaHandlerKey{})
}

// HandleGet implements BulkMetadataHander.HandleGet.
func (h SingleIncludeHandler) HandleGet(hs []BulkIncludeHandler, id *ResolvedURL, paths []string, flags url.Values, req *http.Request) ([]interface{}, error) {
	results := make([]interface{}, len(hs))
	for i, h := range hs {
		h := h.(SingleIncludeHandler)
		result, err := h(id, paths[i], flags, req)
		if err != nil {
			// TODO(rog) include index of failed handler.
			return nil, errgo.Mask(err, errgo.Any)
		}
		results[i] = result
	}
	return results, nil
}

var errPutNotImplemented = errgo.New("PUT not implemented")

// HandlePut implements BulkMetadataHander.HandlePut.
func (h SingleIncludeHandler) HandlePut(hs []BulkIncludeHandler, id *ResolvedURL, paths []string, values []*json.RawMessage, req *http.Request) []error {
	errs := make([]error, len(hs))
	for i := range hs {
		errs[i] = errPutNotImplemented
	}
	return errs
}
