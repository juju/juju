// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package s3client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type s3ClientSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&s3ClientSuite{})

func (s *s3ClientSuite) TestObjectExists(c *gc.C) {
	url, httpClient, cleanup := s.setupServer(c, func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, gc.Equals, http.MethodHead)
		c.Check(r.URL.Path, gc.Equals, "/bucket/object")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte{})
	})
	defer cleanup()

	client, err := NewS3Client(url, httpClient, AnonymousCredentials{}, loggertesting.WrapCheckLog(c))
	c.Assert(err, jc.ErrorIsNil)

	err = client.ObjectExists(context.Background(), "bucket", "object")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *s3ClientSuite) TestObjectExistsNotFound(c *gc.C) {
	url, httpClient, cleanup := s.setupServer(c, func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, gc.Equals, http.MethodHead)
		c.Check(r.URL.Path, gc.Equals, "/bucket/object")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte{})
	})
	defer cleanup()

	client, err := NewS3Client(url, httpClient, AnonymousCredentials{}, loggertesting.WrapCheckLog(c))
	c.Assert(err, jc.ErrorIsNil)

	err = client.ObjectExists(context.Background(), "bucket", "object")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *s3ClientSuite) TestObjectExistsForbidden(c *gc.C) {
	url, httpClient, cleanup := s.setupServer(c, func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, gc.Equals, http.MethodHead)
		c.Check(r.URL.Path, gc.Equals, "/bucket/object")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte{})
	})
	defer cleanup()

	client, err := NewS3Client(url, httpClient, AnonymousCredentials{}, loggertesting.WrapCheckLog(c))
	c.Assert(err, jc.ErrorIsNil)

	err = client.ObjectExists(context.Background(), "bucket", "object")
	c.Assert(err, jc.ErrorIs, errors.Forbidden)
}

func (s *s3ClientSuite) TestGetObject(c *gc.C) {
	url, httpClient, cleanup := s.setupServer(c, func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, gc.Equals, http.MethodGet)
		c.Check(r.URL.Path, gc.Equals, "/bucket/object")
		w.Header().Set("x-amz-checksum-mode", "sha256")
		w.Header().Set("x-amz-checksum-sha256", "+iyMxPKBdrvu1Lc231aaNMec03I+nsQvlnS01GrGuLg=")
		w.Write([]byte("blob"))
	})
	defer cleanup()

	client, err := NewS3Client(url, httpClient, AnonymousCredentials{}, loggertesting.WrapCheckLog(c))
	c.Assert(err, jc.ErrorIsNil)

	resp, size, hash, err := client.GetObject(context.Background(), "bucket", "object")
	c.Assert(err, jc.ErrorIsNil)

	blob, err := io.ReadAll(resp)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(blob), gc.Equals, "blob")
	c.Check(size, gc.Equals, int64(4))
	c.Check(hash, gc.Equals, "+iyMxPKBdrvu1Lc231aaNMec03I+nsQvlnS01GrGuLg=")
}

func (s *s3ClientSuite) TestPutObject(c *gc.C) {
	hash := "fa2c8cc4f28176bbeed4b736df569a34c79cd3723e9ec42f9674b4d46ac6b8b8"

	url, httpClient, cleanup := s.setupServer(c, func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, gc.Equals, http.MethodPut)
		c.Check(r.URL.Path, gc.Equals, "/bucket/object")

		// Ensure we have the correct object lock headers sent.
		// This just prevents putting an object that doesn't have object lock
		// enabled.
		c.Check(r.Header.Get("x-amz-object-lock-legal-hold"), gc.Equals, "ON")
		c.Check(r.Header.Get("x-amz-object-lock-mode"), gc.Equals, "GOVERNANCE")

		lockRetentionDateString := r.Header.Get("x-amz-object-lock-retain-until-date")
		lockRetentionDate, err := time.Parse(time.RFC3339, lockRetentionDateString)
		c.Assert(err, jc.ErrorIsNil)

		// Ensure the retention date is within 1 hour of the retention lock
		// date, which is currently set to 20 years.
		c.Check(lockRetentionDate.After(time.Now().Add(retentionLockDate-time.Hour)), jc.IsTrue)

		body, err := io.ReadAll(r.Body)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(string(body), jc.Contains, "blob")
	})
	defer cleanup()

	client, err := NewS3Client(url, httpClient, AnonymousCredentials{}, loggertesting.WrapCheckLog(c))
	c.Assert(err, jc.ErrorIsNil)

	err = client.PutObject(context.Background(), "bucket", "object", strings.NewReader("blob"), hash)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *s3ClientSuite) TestDeleteObject(c *gc.C) {
	url, httpClient, cleanup := s.setupServer(c, func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, gc.Equals, http.MethodDelete)
		c.Check(r.URL.Path, gc.Equals, "/bucket/object")

		// Ensure that we can delete an object without confirmation.
		c.Check(r.Header.Get("x-amz-bypass-governance-retention"), gc.Equals, "true")
	})
	defer cleanup()

	client, err := NewS3Client(url, httpClient, AnonymousCredentials{}, loggertesting.WrapCheckLog(c))
	c.Assert(err, jc.ErrorIsNil)

	err = client.DeleteObject(context.Background(), "bucket", "object")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *s3ClientSuite) TestCreateBucket(c *gc.C) {
	url, httpClient, cleanup := s.setupServer(c, func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, gc.Equals, http.MethodPut)
		c.Check(r.URL.Path, gc.Equals, "/bucket")

		// Ensure the bucket is created with object lock enabled.
		c.Check(r.Header.Get("x-amz-bucket-object-lock-enabled"), gc.Equals, "true")
	})
	defer cleanup()

	client, err := NewS3Client(url, httpClient, AnonymousCredentials{}, loggertesting.WrapCheckLog(c))
	c.Assert(err, jc.ErrorIsNil)

	err = client.CreateBucket(context.Background(), "bucket")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *s3ClientSuite) setupServer(c *gc.C, handler http.HandlerFunc) (string, HTTPClient, func()) {
	server := httptest.NewTLSServer(handler)
	return server.URL, server.Client(), func() {
		server.Close()
	}
}
