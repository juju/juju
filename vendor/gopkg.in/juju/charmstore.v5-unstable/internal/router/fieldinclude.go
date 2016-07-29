// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package router // import "gopkg.in/juju/charmstore.v5-unstable/internal/router"

import (
	"encoding/json"
	"net/http"
	"net/url"

	"gopkg.in/errgo.v1"

	"gopkg.in/juju/charmstore.v5-unstable/audit"
)

// A FieldQueryFunc is used to retrieve a metadata document for the given URL,
// selecting only those fields specified in keys of the given selector.
type FieldQueryFunc func(id *ResolvedURL, selector map[string]int, req *http.Request) (interface{}, error)

// FieldUpdater records field changes made by a FieldUpdateFunc.
type FieldUpdater struct {
	fields  map[string]interface{}
	entries []audit.Entry
	search  bool
}

// UpdateField requests that the provided field is updated with
// the given value.
func (u *FieldUpdater) UpdateField(fieldName string, val interface{}, entry *audit.Entry) {
	u.fields[fieldName] = val
	if entry != nil {
		u.entries = append(u.entries, *entry)
	}
}

// UpdateSearch requests that search records are updated.
func (u *FieldUpdater) UpdateSearch() {
	u.search = true
}

// A FieldUpdateFunc is used to update a metadata document for the
// given id. For each field in fields, it should set that field to
// its corresponding value in the metadata document.
type FieldUpdateFunc func(id *ResolvedURL, fields map[string]interface{}, entries []audit.Entry) error

// A FieldUpdateSearchFunc is used to update a search document for the
// given id. For each field in fields, it should set that field to
// its corresponding value in the search document.
type FieldUpdateSearchFunc func(id *ResolvedURL, fields map[string]interface{}) error

// A FieldGetFunc returns some data from the given document. The
// document will have been returned from an earlier call to the
// associated QueryFunc.
type FieldGetFunc func(doc interface{}, id *ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error)

// FieldPutFunc sets using the given FieldUpdater corresponding to fields to be set
// in the metadata document for the given id. The path holds the metadata path
// after the initial prefix has been removed.
type FieldPutFunc func(id *ResolvedURL, path string, val *json.RawMessage, updater *FieldUpdater, req *http.Request) error

// FieldIncludeHandlerParams specifies the parameters for NewFieldIncludeHandler.
type FieldIncludeHandlerParams struct {
	// Key is used to group together similar FieldIncludeHandlers
	// (the same query should be generated for any given key).
	Key interface{}

	// Query is used to retrieve the document from the database for
	// GET requests. The fields passed to the query will be the
	// union of all fields found in all the handlers in the bulk
	// request.
	Query FieldQueryFunc

	// Fields specifies which fields are required by the given handler.
	Fields []string

	// Handle actually returns the data from the document retrieved
	// by Query, for GET requests.
	HandleGet FieldGetFunc

	// HandlePut generates update operations for a PUT
	// operation.
	HandlePut FieldPutFunc

	// Update is used to update the document in the database for
	// PUT requests.
	Update FieldUpdateFunc

	// UpdateSearch is used to update the document in the search
	// database for PUT requests.
	UpdateSearch FieldUpdateSearchFunc
}

// FieldIncludeHandler implements BulkIncludeHandler by
// making a single request with a number of aggregated fields.
type FieldIncludeHandler struct {
	P FieldIncludeHandlerParams
}

// NewFieldIncludeHandler returns a BulkIncludeHandler that will perform
// only a single database query for several requests. See FieldIncludeHandlerParams
// for more detail.
//
// See in ../v4/api.go for an example of its use.
func NewFieldIncludeHandler(p FieldIncludeHandlerParams) *FieldIncludeHandler {
	return &FieldIncludeHandler{p}
}

// Key implements BulkIncludeHandler.Key.
func (h *FieldIncludeHandler) Key() interface{} {
	return h.P.Key
}

// HandlePut implements BulkIncludeHandler.HandlePut.
func (h *FieldIncludeHandler) HandlePut(hs []BulkIncludeHandler, id *ResolvedURL, paths []string, values []*json.RawMessage, req *http.Request) []error {
	updater := &FieldUpdater{
		fields:  make(map[string]interface{}),
		entries: make([]audit.Entry, 0),
	}
	var errs []error
	errCount := 0
	setError := func(i int, err error) {
		if errs == nil {
			errs = make([]error, len(hs))
		}
		if errs[i] == nil {
			errs[i] = err
			errCount++
		}
	}
	for i, h := range hs {
		h := h.(*FieldIncludeHandler)
		if h.P.HandlePut == nil {
			setError(i, errgo.New("PUT not supported"))
			continue
		}
		if err := h.P.HandlePut(id, paths[i], values[i], updater, req); err != nil {
			setError(i, errgo.Mask(err, errgo.Any))
		}
	}
	if errCount == len(hs) {
		// Every HandlePut request has drawn an error,
		// no need to call Update.
		return errs
	}
	if err := h.P.Update(id, updater.fields, updater.entries); err != nil {
		for i := range hs {
			setError(i, err)
		}
	}
	if updater.search {
		if err := h.P.UpdateSearch(id, updater.fields); err != nil {
			for i := range hs {
				setError(i, err)
			}
		}
	}
	return errs
}

// HandleGet implements BulkIncludeHandler.HandleGet.
func (h *FieldIncludeHandler) HandleGet(hs []BulkIncludeHandler, id *ResolvedURL, paths []string, flags url.Values, req *http.Request) ([]interface{}, error) {
	funcs := make([]FieldGetFunc, len(hs))
	selector := make(map[string]int)
	// Extract the handler functions and union all the fields.
	for i, h := range hs {
		h := h.(*FieldIncludeHandler)
		funcs[i] = h.P.HandleGet
		for _, field := range h.P.Fields {
			selector[field] = 1
		}
	}
	// Make the single query.
	doc, err := h.P.Query(id, selector, req)
	if err != nil {
		// Note: preserve error cause from handlers.
		return nil, errgo.Mask(err, errgo.Any)
	}

	// Call all the handlers with the resulting query document.
	results := make([]interface{}, len(hs))
	for i, f := range funcs {
		var err error
		results[i], err = f(doc, id, paths[i], flags, req)
		if err != nil {
			// TODO correlate error with handler (perhaps return
			// an error that identifies the slice position of the handler that
			// failed).
			// Note: preserve error cause from handlers.
			return nil, errgo.Mask(err, errgo.Any)
		}
	}
	return results, nil
}
