// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jtesting "github.com/juju/juju/v2/testing"
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
	c.Check(err, gc.ErrorMatches, fmt.Sprintf("dial tcp %s:.*", regexp.QuoteMeta(addr)))
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
