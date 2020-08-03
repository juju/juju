// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"io"
	http "net/http"
	"net/url"
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	path "github.com/juju/juju/charmhub/path"
)

//go:generate go run github.com/golang/mock/mockgen -package charmhub -destination client_mock_test.go github.com/juju/juju/charmhub Transport,RESTClient

func Test(t *testing.T) {
	gc.TestingT(t)
}

func MustParseURL(c *gc.C, path string) *url.URL {
	u, err := url.Parse(path)
	c.Assert(err, jc.ErrorIsNil)
	return u
}

func MustMakePath(c *gc.C, p string) path.Path {
	u := MustParseURL(c, p)
	return path.MakePath(u)
}

type nopCloser struct {
	io.Reader
}

func MakeNopCloser(r io.Reader) nopCloser {
	return nopCloser{
		Reader: r,
	}
}

func (nopCloser) Close() error { return nil }

func MakeContentTypeHeader(name string) http.Header {
	h := make(http.Header)
	h.Set("content-type", name)
	return h
}

func MustNewRequest(c *gc.C, path string) *http.Request {
	req, err := http.NewRequest("GET", path, nil)
	c.Assert(err, jc.ErrorIsNil)

	return req
}
