// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"time"

	"github.com/google/uuid"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/virtualhostname"
	"github.com/juju/juju/internal/worker/sshserver/handlers/k8s"
	"github.com/juju/juju/internal/worker/sshserver/handlers/machine"
)

type proxySuite struct{}

var _ = gc.Suite(&proxySuite{})

type fakeResolver struct {
	k8s.Resolver
}

func (s *proxySuite) TestNewK8sHandler(c *gc.C) {
	proxy := proxyFactory{
		k8sResolver: fakeResolver{},
		logger:      loggo.GetLogger("test"),
	}

	destination, err := virtualhostname.NewInfoContainerTarget(uuid.New().String(), "test/0", "test-container")
	c.Assert(err, gc.IsNil)

	info := ConnectionInfo{
		startTime:   time.Now(),
		destination: destination,
	}

	handlers, err := proxy.New(info)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(handlers, gc.NotNil)
	c.Assert(handlers, gc.FitsTypeOf, &k8s.Handlers{})

	// Verify error path
	proxy.k8sResolver = nil
	_, err = proxy.New(info)
	c.Assert(err, gc.ErrorMatches, "k8s resolver is required not valid")
}

type fakeSSHConnector struct {
	machine.SSHConnector
}

func (s *proxySuite) TestNewMachineHandler(c *gc.C) {
	proxy := proxyFactory{
		connector: fakeSSHConnector{},
		logger:    loggo.GetLogger("test"),
	}

	destination, err := virtualhostname.NewInfoMachineTarget(uuid.New().String(), "0")
	c.Assert(err, gc.IsNil)

	info := ConnectionInfo{
		startTime:   time.Now(),
		destination: destination,
	}

	handlers, err := proxy.New(info)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(handlers, gc.NotNil)
	c.Assert(handlers, gc.FitsTypeOf, &machine.Handlers{})

	// Verify error path
	proxy.connector = nil
	_, err = proxy.New(info)
	c.Assert(err, gc.ErrorMatches, "connector is required not valid")
}
