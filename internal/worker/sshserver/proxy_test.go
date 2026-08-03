// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/virtualhostname"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	k8sexec "github.com/juju/juju/internal/provider/kubernetes/exec"
	"github.com/juju/juju/internal/worker/sshserver/handlers/k8s"
	"github.com/juju/juju/internal/worker/sshserver/handlers/machine"
)

type proxySuite struct{}

func TestProxySuite(t *testing.T) {
	tc.Run(t, &proxySuite{})
}

func (s *proxySuite) TestNewSelectsMachineHandlers(c *tc.C) {
	destination, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)

	handlers, err := (proxyFactory{
		logger:    loggertesting.WrapCheckLog(c),
		connector: proxyConnector{},
	}).New(destination)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(handlers, tc.FitsTypeOf, &machine.Handlers{})
}

func (s *proxySuite) TestNewSelectsKubernetesHandlers(c *tc.C) {
	destination, err := virtualhostname.NewInfoContainerTarget("8419cd78-4993-4c3a-928e-c646226beeee", "app/0", "workload")
	c.Assert(err, tc.ErrorIsNil)

	handlers, err := (proxyFactory{
		logger:      loggertesting.WrapCheckLog(c),
		k8sResolver: proxyResolver{},
		getExecutor: func(string) (k8sexec.Executor, error) { return nil, nil },
	}).New(destination)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(handlers, tc.FitsTypeOf, &k8s.Handlers{})
}

type proxyConnector struct {
	machine.SSHConnector
}

type proxyResolver struct {
	k8s.Resolver
}
