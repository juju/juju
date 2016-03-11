// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore

import (
	"net/http"

	"github.com/juju/errors"
	"gopkg.in/juju/charmrepo.v2-unstable"
)

// JujuMetadata holds Juju-specific information that will be provided
// to the charm store.
type JujuMetadata struct {
	// ModelUUID identifies the Juju model to the charm store.
	ModelUUID string
}

// IsZero indicates whether or not the metadata is the zero value.
func (meta JujuMetadata) IsZero() bool {
	return meta.ModelUUID == ""
}

func (meta JujuMetadata) asAttrs() map[string]string {
	return map[string]string{
		"environment_uuid": meta.ModelUUID,
	}
}

func (meta JujuMetadata) addToHeader(header http.Header) {
	attrs := meta.asAttrs()
	for k, v := range attrs {
		header.Add(charmrepo.JujuMetadataHTTPHeader, k+"="+v)
	}
}

type httpHeaderSetter interface {
	// SetHTTPHeader sets custom HTTP headers that will be sent to thei
	// charm store on each request.
	SetHTTPHeader(header http.Header)
}

// setOnClient sets custom HTTP headers that will be sent to the charm
// store on each request.
func (meta JujuMetadata) setOnClient(client BaseClient) error {
	setter, ok := client.(httpHeaderSetter)
	if !ok {
		return errors.NotValidf("charm store client (missing SetHTTPHeader method)")
	}

	header := make(http.Header)
	meta.addToHeader(header)
	setter.SetHTTPHeader(header)
	return nil
}
