// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"crypto/sha3"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
					"version": "4.1-beta2",
					"download": {"url": "/this-bad-url-does-not-matter", "sha3-384": "abc123"}
				}]
			}`)
		}))
		defer server.Close()

		client := newSnapStoreClient(server.URL)
		rev, err := client.resolveChannel(context.Background(), "jujud", "amd64", "4.2/edge")
		c.Assert(err, tc.ErrorIsNil)
		c.Check(rev.Revision, tc.Equals, 42)
		c.Check(rev.Version, tc.Equals, "4.1-beta2")
		c.Check(rev.Sha3, tc.Equals, "abc123")
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
					 "revision": 41, "version": "4.1-beta1",
					 "download": {"url": "u", "sha3-384": "aaa"}},
					{"channel": {"track": "4.2", "risk": "edge", "architecture": "amd64"},
					 "revision": 42, "version": "4.1-beta2",
					 "download": {"url": "u2", "sha3-384": "bbb"}}
				]
			}`)
		}))
		defer server.Close()

		client := newSnapStoreClient(server.URL)
		rev, err := client.resolveRevision(context.Background(), "jujud", "amd64", 42)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(rev.Revision, tc.Equals, 42)
		c.Check(rev.Sha3, tc.Equals, "bbb")
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

func TestStoreClientAcquire(t *testing.T) {
	tc.Run(t, func(c *tc.C) {
		// The downloaded .snap bytes.
		snapBytes := []byte("fake snap content")
		sha := sha384HexBytes(snapBytes)
		shaB64, err := hexToBase64URL(sha)
		c.Assert(err, tc.ErrorIsNil)

		var downloadHits, assertHits int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasPrefix(r.URL.Path, "/api/v1/snaps/download/"):
				downloadHits++
				w.Write(snapBytes)
			case strings.HasPrefix(r.URL.Path, "/v2/assertions/"):
				assertHits++
				assertType := strings.TrimPrefix(r.URL.Path, "/v2/assertions/")
				switch assertType {
				case "snap-revision/" + shaB64:
					fmt.Fprint(w, "type: snap-revision\nsnap-sha3-384: "+shaB64+"\n")
				case "snap-declaration/16/snap-id-1":
					fmt.Fprint(w, "type: snap-declaration\npublisher-id: acct-1\n")
				case "account/acct-1":
					fmt.Fprint(w, "type: account\nsign-key-sha3-384: key-1\n")
				case "account-key/key-1":
					fmt.Fprint(w, "type: account-key\n")
				default:
					http.NotFound(w, r)
				}
			default:
				http.NotFound(w, r)
			}
		}))
		defer server.Close()

		dir := c.MkDir()
		client := newSnapStoreClient(server.URL)
		target := snapStoreRevision{
			Revision:    42,
			Version:     "4.1-beta2",
			DownloadURL: server.URL + "/api/v1/snaps/download/" + sha,
			Sha3:        sha,
			SnapID:      "snap-id-1",
		}
		snapPath, assertPath, err := client.acquire(context.Background(), dir, target)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(downloadHits, tc.Equals, 1)
		c.Check(assertHits, tc.Equals, 4)

		data, err := os.ReadFile(snapPath)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(data, tc.DeepEquals, snapBytes)
		c.Check(filepath.Base(snapPath), tc.Equals, "jujud.snap")
		c.Check(filepath.Base(assertPath), tc.Equals, "jujud.assert")

		assertData, err := os.ReadFile(assertPath)
		c.Assert(err, tc.ErrorIsNil)
		assertText := string(assertData)
		c.Check(strings.Contains(assertText, "type: account-key"), tc.IsTrue)
		c.Check(strings.Contains(assertText, "type: account\n"), tc.IsTrue)
		c.Check(strings.Contains(assertText, "type: snap-declaration"), tc.IsTrue)
		c.Check(strings.Contains(assertText, "type: snap-revision"), tc.IsTrue)
	})
}

func TestStoreClientAcquireDigestMismatch(t *testing.T) {
	tc.Run(t, func(c *tc.C) {
		snapBytes := []byte("fake snap content")
		realSha := sha384HexBytes(snapBytes)
		wrongSha := "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
		wrongShaB64, err := hexToBase64URL(wrongSha)
		c.Assert(err, tc.ErrorIsNil)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasPrefix(r.URL.Path, "/api/v1/snaps/download/"):
				w.Write(snapBytes)
			case strings.HasPrefix(r.URL.Path, "/v2/assertions/"):
				assertType := strings.TrimPrefix(r.URL.Path, "/v2/assertions/")
				switch assertType {
				case "snap-revision/" + wrongShaB64:
					fmt.Fprint(w, "type: snap-revision\n")
				case "snap-declaration/16/snap-id-1":
					fmt.Fprint(w, "type: snap-declaration\npublisher-id: acct-1\n")
				case "account/acct-1":
					fmt.Fprint(w, "type: account\nsign-key-sha3-384: key-1\n")
				case "account-key/key-1":
					fmt.Fprint(w, "type: account-key\n")
				default:
					http.NotFound(w, r)
				}
			default:
				http.NotFound(w, r)
			}
		}))
		defer server.Close()

		dir := c.MkDir()
		client := newSnapStoreClient(server.URL)
		target := snapStoreRevision{
			Revision:    42,
			DownloadURL: server.URL + "/api/v1/snaps/download/" + realSha,
			SnapID:      "snap-id-1",
			// Deliberately wrong digest.
			Sha3: wrongSha,
		}
		_, _, err = client.acquire(context.Background(), dir, target)
		c.Assert(err, tc.ErrorMatches, `.*does not match store revision.*`)
	})
}

func sha384HexBytes(b []byte) string {
	h := sha3.New384()
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}
