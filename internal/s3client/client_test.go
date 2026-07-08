// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package s3client

import (
	"fmt"
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

	client, err := NewS3Client(url, httpClient, AnonymousCredentials{})
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

	client, err := NewS3Client(url, httpClient, AnonymousCredentials{})
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

	client, err := NewS3Client(url, httpClient, AnonymousCredentials{})
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

	client, err := NewS3Client(url, httpClient, AnonymousCredentials{})
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

	client, err := NewS3Client(url, httpClient, AnonymousCredentials{})
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

	client, err := NewS3Client(url, httpClient, AnonymousCredentials{})
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

	client, err := NewS3Client(url, httpClient, AnonymousCredentials{})
	c.Assert(err, tc.ErrorIsNil)

	err = client.CreateBucket(c.Context(), "bucket")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *s3ClientSuite) TestRegionFromEndpoint(c *tc.C) {
	tests := []struct {
		endpoint string
		expected string
	}{{
		endpoint: "https://s3.amazonaws.com",
		expected: "us-east-1",
	}, {
		endpoint: "https://s3.us-east-1.amazonaws.com",
		expected: "us-east-1",
	}, {
		endpoint: "https://s3.eu-west-1.amazonaws.com",
		expected: "eu-west-1",
	}, {
		endpoint: "https://s3-eu-west-1.amazonaws.com",
		expected: "eu-west-1",
	}, {
		endpoint: "https://mybucket.s3.us-east-2.amazonaws.com",
		expected: "us-east-2",
	}, {
		endpoint: "https://mybucket.s3-eu-west-1.amazonaws.com",
		expected: "eu-west-1",
	}, {
		endpoint: "https://minio.example.com:9000",
		expected: "",
	}, {
		endpoint: "https://10.0.0.1:17070",
		expected: "",
	}, {
		endpoint: "",
		expected: "",
	}}
	for i, test := range tests {
		c.Check(regionFromEndpoint(test.endpoint), tc.Equals, test.expected, tc.Commentf("test %d: %q", i, test.endpoint))
	}
}

func (s *s3ClientSuite) TestResolveRegion(c *tc.C) {
	tests := []struct {
		override    string
		endpoint    string
		expected    string
		expectError bool
	}{{
		override: "eu-central-1",
		endpoint: "https://s3.us-east-1.amazonaws.com",
		expected: "eu-central-1",
	}, {
		override: "",
		endpoint: "https://s3.eu-west-1.amazonaws.com",
		expected: "eu-west-1",
	}, {
		override:    "",
		endpoint:    "https://minio.example.com:9000",
		expectError: true,
	}, {
		override:    "",
		endpoint:    "",
		expectError: true,
	}}
	for i, test := range tests {
		region, err := resolveRegion(test.override, test.endpoint)
		if test.expectError {
			c.Check(err, tc.ErrorMatches, "region could not be derived from endpoint.*",
				tc.Commentf("test %d", i))
		} else {
			c.Check(err, tc.ErrorIsNil, tc.Commentf("test %d", i))
			c.Check(region, tc.Equals, test.expected, tc.Commentf("test %d", i))
		}
	}
}

func (s *s3ClientSuite) TestNewS3ClientRegionOverride(c *tc.C) {
	var authHeader string
	url, httpClient, cleanup := s.setupServer(c, func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte{})
	})
	defer cleanup()

	client, err := NewS3Client(url, httpClient, StaticCredentials{
		Key:    "test-key",
		Secret: "test-secret",
	}, WithRegion("eu-central-1"))
	c.Assert(err, tc.ErrorIsNil)

	_ = client.ObjectExists(c.Context(), "bucket", "object")
	c.Check(strings.Contains(authHeader, "eu-central-1"), tc.IsTrue,
		tc.Commentf("authorization header %q should contain region", authHeader))
}

func (s *s3ClientSuite) TestNewS3ClientRegionFallback(c *tc.C) {
	var authHeader string
	var entries []string
	recorder := loggertesting.RecordLog(func(s string, a ...any) {
		entries = append(entries, fmt.Sprintf(s, a...))
	})
	logger := loggertesting.WrapCheckLog(recorder)

	url, httpClient, cleanup := s.setupServer(c, func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte{})
	})
	defer cleanup()

	client, err := NewS3Client(url, httpClient, StaticCredentials{
		Key:    "test-key",
		Secret: "test-secret",
	}, WithLogger(logger))
	c.Assert(err, tc.ErrorIsNil)

	_ = client.ObjectExists(c.Context(), "bucket", "object")
	c.Check(strings.Contains(authHeader, "juju-unknown-region"), tc.IsTrue,
		tc.Commentf("authorization header %q should contain fallback region", authHeader))

	var foundWarning bool
	for _, msg := range entries {
		if strings.Contains(msg, "WARNING") && strings.Contains(msg, "object-store-s3-region") {
			foundWarning = true
			break
		}
	}
	c.Check(foundWarning, tc.IsTrue,
		tc.Commentf("expected warning about object-store-s3-region, got: %v", entries))
}

func (s *s3ClientSuite) TestNewS3ClientAnonymousNoRegion(c *tc.C) {
	var entries []string
	recorder := loggertesting.RecordLog(func(s string, a ...any) {
		entries = append(entries, fmt.Sprintf(s, a...))
	})
	logger := loggertesting.WrapCheckLog(recorder)

	url, httpClient, cleanup := s.setupServer(c, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte{})
	})
	defer cleanup()

	client, err := NewS3Client(url, httpClient, AnonymousCredentials{},
		WithLogger(logger))
	c.Assert(err, tc.ErrorIsNil)

	err = client.ObjectExists(c.Context(), "bucket", "object")
	c.Assert(err, tc.ErrorIsNil)

	var foundWarning bool
	for _, msg := range entries {
		if strings.Contains(msg, "WARNING") {
			foundWarning = true
			break
		}
	}
	c.Check(foundWarning, tc.IsFalse,
		tc.Commentf("expected no warnings for anonymous creds, got: %v", entries))
}

func (s *s3ClientSuite) setupServer(c *tc.C, handler http.HandlerFunc) (string, HTTPClient, func()) {
	server := httptest.NewTLSServer(handler)
	return server.URL, server.Client(), func() {
		server.Close()
	}
}
