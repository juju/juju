// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import "net/http"

// NewHTTPDownloadRequest creates a new HTTP download request
// for the given resource.
//
// Intended for use on the client side.
func NewHTTPDownloadRequest(resourceName string) (*http.Request, error) {
	return http.NewRequest("GET", "/resources/"+resourceName, nil)
}
