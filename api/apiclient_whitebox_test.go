// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"context"
	"net"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jtesting "github.com/juju/juju/testing"
	"github.com/juju/testing"
)

type apiclientWhiteboxSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&apiclientWhiteboxSuite{})

func (s *apiclientWhiteboxSuite) TestDialWebsocketMultiTimeout(c *gc.C) {
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
	defer listen.Close()
	addr := listen.Addr().String()
	c.Logf("listening at: %s", addr)
	// Note that we Listen, but we never Accept
	close(started)
	info := &Info{
		Addrs: []string{addr},
	}
	opts := DialOpts{
		DialAddressInterval: 1 * time.Millisecond,
		RetryDelay:          1 * time.Millisecond,
		Timeout:             10 * time.Millisecond,
		DialTimeout:         5 * time.Millisecond,
	}
	_, err = dialAPI(ctx, info, opts)
	c.Assert(err, jc.ErrorIsNil)
}
