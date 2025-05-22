// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package s3client

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"

	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type s3ClientSuite struct {
	testhelpers.IsolationSuite
}

func TestS3ClientSuite(t *testing.T) {
	tc.Run(t, &s3ClientSuite{})
}

func (s *s3ClientSuite) TestObjectExists(c *tc.C) {
	url, httpClient, cleanup := s.setupServer(c, func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, tc.Equals, http.MethodHead)
		c.Check(r.URL.Path, tc.Equals, "/bucket/object")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte{})
	})
	defer cleanup()

	client, err := NewS3Client(url, httpClient, AnonymousCredentials{}, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)

	err = client.ObjectExists(c.Context(), "bucket", "object")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *s3ClientSuite) TestObjectExistsNotFound(c *tc.C) {
	url, httpClient, cleanup := s.setupServer(c, func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, tc.Equals, http.MethodHead)
		c.Check(r.URL.Path, tc.Equals, "/bucket/object")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte{})
	})
	defer cleanup()

	client, err := NewS3Client(url, httpClient, AnonymousCredentials{}, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)

	err = client.ObjectExists(c.Context(), "bucket", "object")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *s3ClientSuite) TestObjectExistsForbidden(c *tc.C) {
	url, httpClient, cleanup := s.setupServer(c, func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, tc.Equals, http.MethodHead)
		c.Check(r.URL.Path, tc.Equals, "/bucket/object")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte{})
	})
	defer cleanup()

	client, err := NewS3Client(url, httpClient, AnonymousCredentials{}, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)

	err = client.ObjectExists(c.Context(), "bucket", "object")
	c.Assert(err, tc.ErrorIs, errors.Forbidden)
}

func (s *s3ClientSuite) TestGetObject(c *tc.C) {
	url, httpClient, cleanup := s.setupServer(c, func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, tc.Equals, http.MethodGet)
		c.Check(r.URL.Path, tc.Equals, "/bucket/object")
		w.Header().Set("x-amz-checksum-mode", "sha256")
		w.Header().Set("x-amz-checksum-sha256", "+iyMxPKBdrvu1Lc231aaNMec03I+nsQvlnS01GrGuLg=")
		w.Write([]byte("blob"))
	})
	defer cleanup()

	client, err := NewS3Client(url, httpClient, AnonymousCredentials{}, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)

	resp, size, hash, err := client.GetObject(c.Context(), "bucket", "object")
	c.Assert(err, tc.ErrorIsNil)

	blob, err := io.ReadAll(resp)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(blob), tc.Equals, "blob")
	c.Check(size, tc.Equals, int64(4))
	c.Check(hash, tc.Equals, "+iyMxPKBdrvu1Lc231aaNMec03I+nsQvlnS01GrGuLg=")
}

func (s *s3ClientSuite) TestPutObject(c *tc.C) {
	hash := "fa2c8cc4f28176bbeed4b736df569a34c79cd3723e9ec42f9674b4d46ac6b8b8"

	url, httpClient, cleanup := s.setupServer(c, func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, tc.Equals, http.MethodPut)
		c.Check(r.URL.Path, tc.Equals, "/bucket/object")

		// Ensure we have the correct object lock headers sent.
		// This just prevents putting an object that doesn't have object lock
		// enabled.
		c.Check(r.Header.Get("x-amz-object-lock-legal-hold"), tc.Equals, "ON")
		c.Check(r.Header.Get("x-amz-object-lock-mode"), tc.Equals, "GOVERNANCE")

		lockRetentionDateString := r.Header.Get("x-amz-object-lock-retain-until-date")
		lockRetentionDate, err := time.Parse(time.RFC3339, lockRetentionDateString)
		c.Assert(err, tc.ErrorIsNil)

		// Ensure the retention date is within 1 hour of the retention lock
		// date, which is currently set to 20 years.
		c.Check(lockRetentionDate.After(time.Now().Add(retentionLockDate-time.Hour)), tc.IsTrue)

		body, err := io.ReadAll(r.Body)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(string(body), tc.Contains, "blob")
	})
	defer cleanup()

	client, err := NewS3Client(url, httpClient, AnonymousCredentials{}, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)

	err = client.PutObject(c.Context(), "bucket", "object", strings.NewReader("blob"), hash)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *s3ClientSuite) TestDeleteObject(c *tc.C) {
	url, httpClient, cleanup := s.setupServer(c, func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, tc.Equals, http.MethodDelete)
		c.Check(r.URL.Path, tc.Equals, "/bucket/object")

		// Ensure that we can delete an object without confirmation.
		c.Check(r.Header.Get("x-amz-bypass-governance-retention"), tc.Equals, "true")
	})
	defer cleanup()

	client, err := NewS3Client(url, httpClient, AnonymousCredentials{}, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)

	err = client.DeleteObject(c.Context(), "bucket", "object")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *s3ClientSuite) TestCreateBucket(c *tc.C) {
	url, httpClient, cleanup := s.setupServer(c, func(w http.ResponseWriter, r *http.Request) {
		c.Check(r.Method, tc.Equals, http.MethodPut)
		c.Check(r.URL.Path, tc.Equals, "/bucket")

		// Ensure the bucket is created with object lock enabled.
		c.Check(r.Header.Get("x-amz-bucket-object-lock-enabled"), tc.Equals, "true")
	})
	defer cleanup()

	client, err := NewS3Client(url, httpClient, AnonymousCredentials{}, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)

	err = client.CreateBucket(c.Context(), "bucket")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *s3ClientSuite) setupServer(c *tc.C, handler http.HandlerFunc) (string, HTTPClient, func()) {
	server := httptest.NewTLSServer(handler)
	return server.URL, server.Client(), func() {
		server.Close()
	}
}
