// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpclient

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"

	internalhttp "github.com/juju/juju/internal/http"
	"github.com/juju/juju/internal/testhelpers"
)

type trackedWorkerSuite struct {
	baseSuite

	client *internalhttp.Client
	states chan string
}

func TestTrackedWorkerSuite(t *testing.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *testing.T) {
		tc.Run(t, &trackedWorkerSuite{})
	})
}

func (s *trackedWorkerSuite) TestKilled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w, err := NewTrackedWorker(s.client)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CheckKill(c, w)

	w.Kill()
}

func (s *trackedWorkerSuite) TestProxyConfigChanges(c *tc.C) {
	defer s.setupMocks(c).Finish()

	newProxy := func(response string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, err := w.Write([]byte(response))
			c.Check(err, tc.ErrorIsNil)
		}))
	}
	proxyOne := newProxy("proxy-one")
	defer proxyOne.Close()
	proxyTwo := newProxy("proxy-two")
	defer proxyTwo.Close()

	s.PatchEnvironment("NO_PROXY", "")
	s.PatchEnvironment("no_proxy", "")
	s.PatchEnvironment("HTTP_PROXY", proxyOne.URL)
	s.PatchEnvironment("http_proxy", proxyOne.URL)

	w, err := NewTrackedWorker(s.client)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CheckKill(c, w)

	client := w.(*trackedWorker)
	request := func() string {
		req, err := http.NewRequestWithContext(c.Context(), http.MethodGet,
			"http://example.invalid", nil)
		c.Assert(err, tc.ErrorIsNil)

		resp, err := client.Do(req)
		c.Assert(err, tc.ErrorIsNil)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		c.Assert(err, tc.ErrorIsNil)
		return string(body)
	}

	c.Check(request(), tc.Equals, "proxy-one")

	s.PatchEnvironment("HTTP_PROXY", proxyTwo.URL)
	s.PatchEnvironment("http_proxy", proxyTwo.URL)
	c.Check(request(), tc.Equals, "proxy-two")
}

func (s *trackedWorkerSuite) setupMocks(c *tc.C) *gomock.Controller {
	// Ensure we buffer the channel, this is because we might miss the
	// event if we're too quick at starting up.
	s.states = make(chan string, 1)

	ctrl := s.baseSuite.setupMocks(c)

	s.client = internalhttp.NewClient()

	return ctrl
}
