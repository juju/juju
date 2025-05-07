// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package snap_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/juju/tc"

	"github.com/juju/juju/core/snap"
)

var (
	_ = tc.Suite(&SnapSuite{})

	snapProxyResponse = `
type: account-key
authority-id: canonical
revision: 2
public-key-sha3-384: BWDEoaqyr25nF5SNCvEv2v7QnM9QsfCc0PBMYD_i2NGSQ32EF2d4D0hqUel3m8ul
account-id: canonical
name: store
since: 2016-04-01T00:00:00.0Z
body-length: 717
sign-key-sha3-384: -CvQKAwRQ5h3Ffn10FILJoEZUXOv6km9FwA80-Rcj-f-6jadQ89VRswHNiEB9Lxk

DATA...

MORE DATA...

type: account
authority-id: canonical
account-id: 1234567890367OdMqoW9YLp3e0EgakQf
display-name: John Doe
timestamp: 2019-05-10T13:12:32.878905Z
username: jdoe
validation: unproven
sign-key-sha3-384: BWDEoaqyr25nF5SNCvEv2v7QnM9QsfCc0PBMYD_i2NGSQ32EF2d4D0hqUel3m8ul

DATA...

type: store
authority-id: canonical
store: 1234567890STOREIDENTIFIER0123456
operator-id: 0123456789067OdMqoW9YLp3e0EgakQf
timestamp: 2019-08-27T12:20:45.166790Z
url: $store-url
sign-key-sha3-384: BWDEoaqyr25nF5SNCvEv2v7QnM9QsfCc0PBMYD_i2NGSQ32EF2d4D0hqUel3m8ul

DATA...
DATA...

type: store
authority-id: canonical
store: OTHER
operator-id: 0123456789067OdMqoW9YLp3e0EgakQf
timestamp: 2019-08-27T12:20:45.166790Z
url: $other-url/
sign-key-sha3-384: BWDEoaqyr25nF5SNCvEv2v7QnM9QsfCc0PBMYD_i2NGSQ32EF2d4D0hqUel3m8ul

DATA...
DATA...
`
)

type SnapSuite struct {
}

func (s *SnapSuite) TestLookupAssertions(c *tc.C) {
	var (
		srv           *httptest.Server
		assertionsRes string
	)
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(assertionsRes))
	}))
	assertionsRes = strings.Replace(snapProxyResponse, "$store-url", srv.URL, -1)
	defer srv.Close()

	assertions, storeID, err := snap.LookupAssertions(srv.URL)
	c.Assert(err, tc.IsNil)
	c.Assert(assertions, tc.Equals, assertionsRes)
	c.Assert(storeID, tc.Equals, "1234567890STOREIDENTIFIER0123456")
}

func (s *SnapSuite) TestConfigureProxyFromURLWithAmbiguousAssertions(c *tc.C) {
	var (
		srv           *httptest.Server
		assertionsRes string
	)
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(assertionsRes))
	}))
	assertionsRes = strings.Replace(snapProxyResponse, "$store-url", srv.URL, -1)
	assertionsRes = strings.Replace(assertionsRes, "$other-url", srv.URL, -1)
	defer srv.Close()

	// Make sure that we don't leak any credentials in error messages
	srvURLWithPassword := fmt.Sprintf("http://42:secret@%s", srv.Listener.Addr())
	_, _, err := snap.LookupAssertions(srvURLWithPassword)
	expErr := fmt.Sprintf(`assertions response from proxy at "%s" is ambiguous as it contains multiple entries with the same proxy URL but different store ID`, srv.URL)
	c.Assert(err, tc.ErrorMatches, expErr)
}
