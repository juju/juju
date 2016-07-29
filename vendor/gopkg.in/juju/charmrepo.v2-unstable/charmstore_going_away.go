// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrepo // import "gopkg.in/juju/charmrepo.v2-unstable"

// This file may go away once Juju stops using anything here.

import (
	"net/http"

	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
)

// URL returns the root endpoint URL of the charm store.
func (s *CharmStore) URL() string {
	return s.client.ServerURL()
}

// Latest returns the most current revision for each of the identified
// charms. The revision in the provided charm URLs is ignored.
func (s *CharmStore) Latest(curls ...*charm.URL) ([]CharmRevision, error) {
	results, err := s.client.Latest(curls)
	if err != nil {
		return nil, err
	}

	var responses []CharmRevision
	for i, result := range results {
		response := CharmRevision{
			Revision: result.Revision,
			Sha256:   result.Sha256,
			Err:      result.Err,
		}
		if errgo.Cause(result.Err) == params.ErrNotFound {
			curl := curls[i].WithRevision(-1)
			response.Err = CharmNotFound(curl.String())
		}
		responses = append(responses, response)
	}
	return responses, nil
}

// WithTestMode returns a repository Interface where test mode is enabled,
// meaning charm store download stats are not increased when charms are
// retrieved.
func (s *CharmStore) WithTestMode() *CharmStore {
	newRepo := *s
	newRepo.client.DisableStats()
	return &newRepo
}

// JujuMetadataHTTPHeader is the HTTP header name used to send Juju metadata
// attributes to the charm store.
const JujuMetadataHTTPHeader = csclient.JujuMetadataHTTPHeader

// WithJujuAttrs returns a repository Interface with the Juju metadata
// attributes set.
func (s *CharmStore) WithJujuAttrs(attrs map[string]string) *CharmStore {
	newRepo := *s
	header := make(http.Header)
	for k, v := range attrs {
		header.Add(JujuMetadataHTTPHeader, k+"="+v)
	}
	newRepo.client.SetHTTPHeader(header)
	return &newRepo
}
