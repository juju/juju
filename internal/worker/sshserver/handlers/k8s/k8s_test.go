// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package k8s

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/virtualhostname"
	"github.com/juju/juju/environs/cloudspec"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	k8sexec "github.com/juju/juju/internal/provider/kubernetes/exec"
	"github.com/juju/juju/internal/worker/sshserver/handlers/common"
)

type k8sSuite struct{}

func TestK8sSuite(t *testing.T) {
	tc.Run(t, &k8sSuite{})
}

func (s *k8sSuite) TestNewHandlers(c *tc.C) {
	destination, err := virtualhostname.NewInfoContainerTarget("8419cd78-4993-4c3a-928e-c646226beeee", "app/0", "workload")
	c.Assert(err, tc.ErrorIsNil)
	resolver := newMockResolver(c)

	handlers, err := NewHandlers(destination, resolver, loggertesting.WrapCheckLog(c), stubExecutor, common.NoopMetrics{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(handlers.destination, tc.Equals, destination)

	_, err = NewHandlers(destination, nil, loggertesting.WrapCheckLog(c), stubExecutor, common.NoopMetrics{})
	c.Check(err, tc.ErrorMatches, "Kubernetes resolver is required")

	_, err = NewHandlers(destination, resolver, nil, stubExecutor, common.NoopMetrics{})
	c.Check(err, tc.ErrorMatches, "logger is required")

	_, err = NewHandlers(destination, resolver, loggertesting.WrapCheckLog(c), nil, common.NoopMetrics{})
	c.Check(err, tc.ErrorMatches, "executor is required")

	machine, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "0")
	c.Assert(err, tc.ErrorIsNil)
	_, err = NewHandlers(machine, resolver, loggertesting.WrapCheckLog(c), stubExecutor, common.NoopMetrics{})
	c.Check(err, tc.ErrorMatches, "destination must be a container target")
}

func stubExecutor(string, cloudspec.CloudSpec) (k8sexec.Executor, error) {
	return nil, nil
}
