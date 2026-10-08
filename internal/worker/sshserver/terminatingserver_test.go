// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"
	"github.com/prometheus/client_golang/prometheus/testutil"
	ssh "github.com/tailscale/gliderssh"

	"github.com/juju/juju/core/virtualhostname"
	jujutesting "github.com/juju/juju/internal/testing"
)

type terminatingServerSuite struct{}

func TestTerminatingServerSuite(t *testing.T) {
	tc.Run(t, &terminatingServerSuite{})
}

// valuesContext is an ssh.Context that only stores values.
type valuesContext struct {
	ssh.Context
	values map[any]any
}

func (v valuesContext) SetValue(key, value any) { v.values[key] = value }

func (v valuesContext) Value(key any) any { return v.values[key] }

func (s *terminatingServerSuite) TestConnCallbackRecordsTimeToSession(c *tc.C) {
	ctrl := gomock.NewController(c)
	proxyFactory := NewMockProxyFactory(ctrl)
	proxyHandlers := NewMockProxyHandlers(ctrl)

	destination, err := virtualhostname.Parse(testVirtualHostname)
	c.Assert(err, tc.ErrorIsNil)
	proxyFactory.EXPECT().New(destination).Return(proxyHandlers, nil)
	proxyHandlers.EXPECT().DirectTCPIPHandler().Return(rejectDirectTCPIP)
	proxyHandlers.EXPECT().SFTPHandler().Return(rejectSFTP)

	factory := newTerminatingServerFactory(
		proxyFactory,
		stubSSHService{jumpHostKey: testHostKey, virtualHostKey: jujutesting.SSHServerHostKey},
	)
	server, err := factory.New(c.Context(), destination)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(server.ConnCallback, tc.NotNil)

	// HandleConn calls ConnCallback with the connection's context, which
	// the session handlers later pass to ObserveTimeToSession.
	ctx := valuesContext{values: map[any]any{}}
	server.ConnCallback(ctx, nil)

	collector := NewMetricsCollector()
	sessionMetrics{collector: collector, modelType: "machine"}.ObserveTimeToSession(ctx)
	c.Check(testutil.CollectAndCount(collector.timeToSession), tc.Equals, 1)
}
