// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/juju/tc"
)

func TestStoreClientResolveChannel(t *testing.T) {
	tc.Run(t, func(c *tc.C) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c.Check(r.URL.Path, tc.Equals, "/v2/snaps/info/jujud")
			c.Check(r.URL.Query().Get("architecture"), tc.Equals, "amd64")
			fmt.Fprint(w, `{
				"channel-map": [{
					"channel": {"track": "4.2", "risk": "edge", "architecture": "amd64"},
					"revision": 42,
					"version": "4.1-beta2"
				}]
			}`)
		}))
		defer server.Close()

		client := newSnapStoreClient(server.URL)
		rev, err := client.resolveChannel(context.Background(), "jujud", "amd64", "4.2/edge")
		c.Assert(err, tc.ErrorIsNil)
		c.Check(rev.Revision, tc.Equals, 42)
		c.Check(rev.Version, tc.Equals, "4.1-beta2")
	})
}

func TestStoreClientResolveChannelNoMatch(t *testing.T) {
	tc.Run(t, func(c *tc.C) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"channel-map": []}`)
		}))
		defer server.Close()

		client := newSnapStoreClient(server.URL)
		_, err := client.resolveChannel(context.Background(), "jujud", "amd64", "4.2/edge")
		c.Assert(err, tc.ErrorMatches, `.*no controller snap "jujud" revision available for channel.*`)
	})
}

func TestStoreClientResolveRevision(t *testing.T) {
	tc.Run(t, func(c *tc.C) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{
				"channel-map": [
					{"channel": {"track": "4.2", "risk": "edge", "architecture": "amd64"},
					 "revision": 41, "version": "4.1-beta1"},
					{"channel": {"track": "4.2", "risk": "edge", "architecture": "amd64"},
					 "revision": 42, "version": "4.1-beta2"}
				]
			}`)
		}))
		defer server.Close()

		client := newSnapStoreClient(server.URL)
		rev, err := client.resolveRevision(context.Background(), "jujud", "amd64", 42)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(rev.Revision, tc.Equals, 42)
		c.Check(rev.Version, tc.Equals, "4.1-beta2")
	})
}

func TestStoreClientResolveRevisionNoMatch(t *testing.T) {
	tc.Run(t, func(c *tc.C) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"channel-map": []}`)
		}))
		defer server.Close()

		client := newSnapStoreClient(server.URL)
		_, err := client.resolveRevision(context.Background(), "jujud", "amd64", 999)
		c.Assert(err, tc.ErrorMatches, `.*no controller snap "jujud" revision 999 available.*`)
	})
}

func TestStoreClientResolveRevisionFiltersArch(t *testing.T) {
	tc.Run(t, func(c *tc.C) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{
				"channel-map": [
					{"channel": {"track": "4.2", "risk": "edge", "architecture": "arm64"},
					 "revision": 42, "version": "4.1-beta2"},
					{"channel": {"track": "4.2", "risk": "edge", "architecture": "amd64"},
					 "revision": 43, "version": "4.1-beta3"}
				]
			}`)
		}))
		defer server.Close()

		client := newSnapStoreClient(server.URL)

		// Revision 42 only exists on arm64, so resolving it for amd64 must
		// not return the arm64 snap.
		_, err := client.resolveRevision(context.Background(), "jujud", "amd64", 42)
		c.Assert(err, tc.ErrorMatches, `.*no controller snap "jujud" revision 42 available.*`)

		rev, err := client.resolveRevision(context.Background(), "jujud", "amd64", 43)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(rev.Version, tc.Equals, "4.1-beta3")
	})
}

func TestStoreClientServerError(t *testing.T) {
	tc.Run(t, func(c *tc.C) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
		defer server.Close()

		client := newSnapStoreClient(server.URL)
		_, err := client.resolveChannel(context.Background(), "jujud", "amd64", "4.2/edge")
		c.Assert(err, tc.ErrorMatches, `.*snap store returned 500 Internal Server Error.*`)
	})
}

func TestResolveControllerSnapChannel(t *testing.T) {
	tc.Run(t, func(c *tc.C) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c.Check(r.URL.Query().Get("architecture"), tc.Equals, "amd64")
			fmt.Fprint(w, `{
				"channel-map": [{
					"channel": {"track": "4.2", "risk": "edge", "architecture": "amd64"},
					"revision": 42,
					"version": "4.1-beta2"
				}]
			}`)
		}))
		defer server.Close()

		version, revision, err := resolveControllerSnap(
			context.Background(), server.URL, "jujud", "amd64", "4.2/edge", 0,
		)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(version, tc.Equals, "4.1-beta2")
		c.Check(revision, tc.Equals, 42)
	})
}

func TestResolveControllerSnapRevision(t *testing.T) {
	tc.Run(t, func(c *tc.C) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{
				"channel-map": [{
					"channel": {"track": "4.2", "risk": "edge", "architecture": "amd64"},
					"revision": 42,
					"version": "4.1-beta2"
				}]
			}`)
		}))
		defer server.Close()

		version, revision, err := resolveControllerSnap(
			context.Background(), server.URL, "jujud", "amd64", "", 42,
		)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(version, tc.Equals, "4.1-beta2")
		c.Check(revision, tc.Equals, 42)
	})
}
