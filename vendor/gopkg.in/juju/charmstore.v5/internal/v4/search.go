// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package v4 // import "gopkg.in/juju/charmstore.v5/internal/v4"

import (
	"net/http"

	"gopkg.in/juju/charmstore.v5/internal/v5"
)

const maxConcurrency = 20

// GET search[?text=text][&autocomplete=1][&filter=value…][&limit=limit][&include=meta][&skip=count][&sort=field[+dir]]
// https://github.com/juju/charmstore/blob/v4/docs/API.md#get-search
func (h ReqHandler) serveSearch(_ http.Header, req *http.Request) (interface{}, error) {
	sp, err := v5.ParseSearchParams(req)
	sp.AutoComplete = true
	if err != nil {
		return "", err
	}
	sp.ExpandedMultiSeries = true
	auth, err := h.Authenticate(req)
	if err != nil {
		logger.Infof("authorization failed on search request, granting no privileges: %v", err)
	}
	sp.Admin = auth.Admin
	if auth.Username != "" {
		sp.Groups = append(sp.Groups, auth.Username)
		groups, err := auth.User.Groups()
		if err != nil {
			logger.Infof("cannot get groups for user %q, assuming no groups: %v", auth.Username, err)
		}
		sp.Groups = append(sp.Groups, groups...)
	}
	return h.Search(sp, req)
}
