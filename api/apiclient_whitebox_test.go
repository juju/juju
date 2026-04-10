// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"time"

	proxyutils "github.com/juju/proxy"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jtesting "github.com/juju/juju/testing"
	"github.com/juju/juju/utils/proxy"
)

type apiclientWhiteboxSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&apiclientWhiteboxSuite{})

func (s *apiclientWhiteboxSuite) TestDialWebsocketMultiCancelled(c *gc.C) {
	ctx := context.TODO()
	ctx, cancel := context.WithCancel(ctx)
	started := make(chan struct{})
	go func() {
		select {
		case <-started:
		case <-time.After(jtesting.LongWait):
			c.Fatalf("timed out waiting %s for started", jtesting.LongWait)
		}
		<-time.After(10 * time.Millisecond)
		if cancel != nil {
			c.Logf("cancelling")
			cancel()
		}
	}()
	listen, err := net.Listen("tcp4", ":0")
	c.Assert(err, jc.ErrorIsNil)
	addr := listen.Addr().String()
	c.Logf("listening at: %s", addr)
	// Note that we Listen, but we never Accept
	close(started)
	info := &Info{
		Addrs: []string{addr},
	}
	opts := DialOpts{
		DialAddressInterval: 50 * time.Millisecond,
		RetryDelay:          40 * time.Millisecond,
		Timeout:             100 * time.Millisecond,
		DialTimeout:         100 * time.Millisecond,
	}
	// Close before we connect
	listen.Close()
	_, err = dialAPI(ctx, info, opts)
	c.Check(err, gc.NotNil)
}

func (s *apiclientWhiteboxSuite) TestDialWebsocketMultiClosed(c *gc.C) {
	listen, err := net.Listen("tcp4", ":0")
	c.Assert(err, jc.ErrorIsNil)
	addr := listen.Addr().String()
	c.Logf("listening at: %s", addr)
	// Note that we Listen, but we never Accept
	info := &Info{
		Addrs: []string{addr},
	}
	opts := DialOpts{
		DialAddressInterval: 1 * time.Second,
		RetryDelay:          1 * time.Second,
		Timeout:             2 * time.Second,
		DialTimeout:         3 * time.Second,
	}
	listen.Close()
	_, _, err = DialAPI(info, opts)
	c.Check(err, gc.ErrorMatches, fmt.Sprintf("unable to connect to API: dial tcp %s:.*", regexp.QuoteMeta(addr)))
}

func (s *apiclientWhiteboxSuite) TestProxyForRequestNormalizesWebsocketSchemes(c *gc.C) {
	tests := []struct {
		about    string
		settings proxyutils.Settings
		rawURL   string
		expected string
	}{
		{
			about: "wss uses https proxy",
			settings: proxyutils.Settings{
				Https: "https://proxy.example:8443",
			},
			rawURL:   "wss://controller.example:17070/model/uuid/api",
			expected: "https://proxy.example:8443",
		},
		{
			about: "ws uses http proxy",
			settings: proxyutils.Settings{
				Http: "http://proxy.example:8080",
			},
			rawURL:   "ws://controller.example:17070/model/uuid/api",
			expected: "http://proxy.example:8080",
		},
		{
			about: "wss honours no_proxy",
			settings: proxyutils.Settings{
				Https:   "https://proxy.example:8443",
				NoProxy: "controller.example",
			},
			rawURL:   "wss://controller.example:17070/model/uuid/api",
			expected: "",
		},
	}

	for _, test := range tests {
		c.Logf("test: %s", test.about)
		err := proxy.DefaultConfig.Set(test.settings)
		c.Assert(err, jc.ErrorIsNil)

		target, err := url.Parse(test.rawURL)
		c.Assert(err, jc.ErrorIsNil)

		proxyURL, err := proxyForRequest(&http.Request{URL: target})
		c.Assert(err, jc.ErrorIsNil)
		if test.expected == "" {
			c.Assert(proxyURL, gc.IsNil)
		} else {
			c.Assert(proxyURL, gc.NotNil)
			c.Assert(proxyURL.String(), gc.Equals, test.expected)
		}
	}

	c.Assert(proxy.DefaultConfig.Set(proxyutils.Settings{}), jc.ErrorIsNil)
}
