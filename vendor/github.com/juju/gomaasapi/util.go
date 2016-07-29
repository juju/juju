// Copyright 2012-2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package gomaasapi

import (
	"strings"
)

// JoinURLs joins a base URL and a subpath together.
// Regardless of whether baseURL ends in a trailing slash (or even multiple
// trailing slashes), or whether there are any leading slashes at the begining
// of path, the two will always be joined together by a single slash.
func JoinURLs(baseURL, path string) string {
	return strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(path, "/")
}

// EnsureTrailingSlash appends a slash at the end of the given string unless
// there already is one.
// This is used to create the kind of normalized URLs that Django expects.
// (to avoid Django's redirection when an URL does not ends with a slash.)
func EnsureTrailingSlash(URL string) string {
	if strings.HasSuffix(URL, "/") {
		return URL
	}
	return URL + "/"
}
