// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/juju/errors"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
)

// uploadDataStore describes the the portion of Juju's "state"
// needed for handling upload requests.
type uploadDataStore interface {
	uploadStorage

	// ListResources returns the resources for the given service.
	ListResources(service string) ([]resource.Resource, error)
}

// uploadStorage describes the the portion of Juju's "state needed for upload.
type uploadStorage interface {
	// SetResource adds the resource to blob storage and updates the metadata.
	SetResource(serviceID string, res resource.Resource, r io.Reader) error
}

// handleUpload handles a resource upload request.
func handleUpload(username string, st uploadDataStore, req *http.Request) error {
	defer req.Body.Close()

	service, res, data, err := readResource(req, username, st)
	if err != nil {
		return errors.Trace(err)
	}

	if err := st.SetResource(service, res, data); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// readResource extracts the relevant info from the request.
func readResource(req *http.Request, username string, st uploadDataStore) (string, resource.Resource, io.ReadCloser, error) {
	var res resource.Resource

	ctype := req.Header.Get("Content-Type")
	if ctype != "application/octet-stream" {
		return "", res, nil, errors.Errorf("unsupported context type %q", ctype)
	}

	// See HTTPEndpoint in server.go and pattern handling in apiserver/apiserver.go.
	service := req.URL.Query().Get(":service")
	name := req.URL.Query().Get(":resource")

	fingerprint := req.URL.Query().Get("fingerprint")
	size := req.URL.Query().Get("size")

	res, err := getResource(st, service, name)
	if err != nil {
		return "", res, nil, errors.Trace(err)
	}

	res, err = updateResource(res, fingerprint, size, username)
	if err != nil {
		return "", res, nil, errors.Trace(err)
	}

	return service, res, req.Body, nil
}

func updateResource(res resource.Resource, fingerprint, size, username string) (resource.Resource, error) {
	data, err := hex.DecodeString(fingerprint)
	if err != nil {
		return res, errors.Annotate(err, "invalid fingerprint")
	}
	fp, err := charmresource.NewFingerprint(data)
	if err != nil {
		return res, errors.Annotate(err, "invalid fingerprint")
	}

	sizeInt, err := strconv.ParseInt(size, 10, 64)
	if err != nil {
		return res, errors.Trace(err)
		return res, errors.Annotate(err, "invalid size")
	}

	res.Origin = charmresource.OriginUpload
	res.Revision = 0
	res.Fingerprint = fp
	res.Size = sizeInt
	res.Username = username
	res.Timestamp = time.Now().UTC()

	if err := res.Validate(); err != nil {
		return res, errors.Trace(err)
	}
	return res, nil
}
