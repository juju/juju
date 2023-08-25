// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package path

import (
	"net/url"
	"path"
	"strings"

	"github.com/juju/errors"
)

// Path defines a absolute path for calling requests to the server.
type Path struct {
	base *url.URL
}

// MakePath creates a URL for queries to a server.
func MakePath(base *url.URL) Path {
	return Path{
		base: base,
	}
}

// Join will sum path names onto a base URL and ensure it constructs a URL
// that is valid.
// Example:
//   - http://baseurl/name0/name1/
func (u Path) Join(names ...string) (Path, error) {
	baseURL := u.String()
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}

	namedPath := path.Join(names...)
	path, err := url.Parse(baseURL + namedPath)
	if err != nil {
		return Path{}, errors.Trace(err)
	}
	return MakePath(path), nil
}

// Query adds additional query parameters to the Path.
// Example:
//   - http://baseurl/name0/name1?q=value
func (u Path) Query(key string, value string) (Path, error) {
	// If value is empty, nothing to change and return back the original
	// path.
	if strings.TrimSpace(value) == "" {
		return u, nil
	}

	baseQuery, err := url.ParseQuery(u.base.RawQuery)
	if err != nil {
		return Path{}, errors.Trace(err)
	}

	baseQuery.Add(key, value)
	newURL, err := url.Parse(u.base.String())
	if err != nil {
		return Path{}, errors.Trace(err)
	}

	newURL.RawQuery = baseQuery.Encode()

	return MakePath(newURL), nil
}

// String returns a stringified version of the Path.
// Under the hood this calls the url.URL#String method.
func (u Path) String() string {
	return u.base.String()
}
