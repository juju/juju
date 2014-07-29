// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"fmt"
	"net/http"
	"net/url"
)

// NewAPIRequest returns a new HTTP request that may be used to make the
// backup API call.
func NewAPIRequest(URL *url.URL, uuid, tag, pw string) (*http.Request, error) {
	URL.Path = fmt.Sprintf("/environment/%s/backup", uuid)
	req, err := http.NewRequest("POST", URL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(tag, pw)
	return req, nil
}
