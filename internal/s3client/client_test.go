// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package s3client

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujutesting "github.com/juju/juju/testing"
)

type s3ClientSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&s3ClientSuite{})

func (s *s3ClientSuite) SetUpTest(c *gc.C) {

}

func (s *s3ClientSuite) TestGetObject(c *gc.C) {
	httpClient, cleanup := s.setupServer(c, func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, gc.Equals, http.MethodGet)
		c.Check(r.URL.Path, gc.Equals, "/bucket/object")
		w.Write([]byte("blob"))
	})
	defer cleanup()

	client, err := NewS3Client(httpClient, AnonymousCredentials{}, jujutesting.NewCheckLogger(c))
	c.Assert(err, jc.ErrorIsNil)

	resp, err := client.GetObject(context.Background(), "bucket", "object")
	c.Assert(err, jc.ErrorIsNil)

	blob, err := io.ReadAll(resp)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(blob), gc.Equals, "blob")
}

func (s *s3ClientSuite) TestPutObject(c *gc.C) {
	hash := "fa2c8cc4f28176bbeed4b736df569a34c79cd3723e9ec42f9674b4d46ac6b8b8"

	httpClient, cleanup := s.setupServer(c, func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, gc.Equals, http.MethodPut)
		c.Check(r.URL.Path, gc.Equals, "/bucket/object")

		body, err := io.ReadAll(r.Body)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(string(body), gc.Equals, "blob")

		hasher := sha256.New()
		_, err = hasher.Write(body)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(hex.EncodeToString(hasher.Sum(nil)), gc.Equals, hash)
	})
	defer cleanup()

	client, err := NewS3Client(httpClient, AnonymousCredentials{}, jujutesting.NewCheckLogger(c))
	c.Assert(err, jc.ErrorIsNil)

	err = client.PutObject(context.Background(), "bucket", "object", strings.NewReader("blob"), hash)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *s3ClientSuite) TestDeleteObject(c *gc.C) {
	httpClient, cleanup := s.setupServer(c, func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, gc.Equals, http.MethodDelete)
		c.Check(r.URL.Path, gc.Equals, "/bucket/object")
	})
	defer cleanup()

	client, err := NewS3Client(httpClient, AnonymousCredentials{}, jujutesting.NewCheckLogger(c))
	c.Assert(err, jc.ErrorIsNil)

	err = client.DeleteObject(context.Background(), "bucket", "object")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *s3ClientSuite) setupServer(c *gc.C, handler http.HandlerFunc) (*httpClient, func()) {
	server := httptest.NewTLSServer(handler)
	return &httpClient{
			client:  server.Client(),
			baseURL: server.URL,
		}, func() {
			server.Close()
		}
}

type httpClient struct {
	client  *http.Client
	baseURL string
}

func (c httpClient) Do(req *http.Request) (*http.Response, error) {
	return c.client.Do(req)
}

func (c httpClient) BaseURL() string {
	return c.baseURL
}
