// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"fmt"

	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/ssh"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type collectMetricsSuite struct {
	baseSuite
}

var _ = gc.Suite(&collectMetricsSuite{})

func (s *collectMetricsSuite) TestCollectMetrics(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	machine.SetProviderAddresses(network.NewAddress("10.3.2.1"))

	charm := s.AddTestingCharm(c, "metered")
	owner := s.Factory.MakeUser(c, nil).Tag()
	meteredService, err := s.State.AddService(state.AddServiceArgs{
		Name:  "metered",
		Owner: owner.String(),
		Charm: charm,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.addUnit(c, meteredService)
	s.addUnit(c, meteredService)

	s.mockSSH(c, cmd(charm.URL().String()))

	client := s.APIState.Client()
	collectResults, err := client.CollectMetrics(
		params.CollectMetricsParams{
			Timeout:  testing.LongWait,
			Services: []string{"metered"},
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(collectResults, gc.HasLen, 2)
	c.Assert(collectResults[0].Error, gc.Equals, "")
	c.Assert(collectResults[1].Error, gc.Equals, "")

	batches, err := s.State.MetricBatches()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(batches, gc.HasLen, 2)
	c.Assert(batches[0].CharmURL(), gc.Equals, charm.URL().String())
	c.Assert(batches[0].Metrics(), gc.HasLen, 1)
	c.Assert(batches[0].Metrics()[0].Key, gc.Equals, "pings")
	c.Assert(batches[0].Metrics()[0].Value, gc.Equals, "1")
	c.Assert(batches[1].CharmURL(), gc.Equals, charm.URL().String())
	c.Assert(batches[1].Metrics(), gc.HasLen, 1)
	c.Assert(batches[1].Metrics()[0].Key, gc.Equals, "pings")
	c.Assert(batches[1].Metrics()[0].Value, gc.Equals, "1")
}

func (s *collectMetricsSuite) addUnit(c *gc.C, service *state.Service) *state.Unit {
	chURL, _ := service.CharmURL()
	unit, err := service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.SetCharmURL(chURL)
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToNewMachine()
	c.Assert(err, jc.ErrorIsNil)
	mId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.Machine(mId)
	c.Assert(err, jc.ErrorIsNil)
	machine.SetProviderAddresses(network.NewAddress("10.3.2.1"))
	return unit
}

func (s *collectMetricsSuite) mockSSH(c *gc.C, cmd string) {
	gitjujutesting.PatchExecutable(c, s, "ssh", cmd)
	gitjujutesting.PatchExecutable(c, s, "scp", cmd)
	client, _ := ssh.NewOpenSSHClient()
	s.PatchValue(&ssh.DefaultClient, client)
}

func cmd(charmURL string) string {
	return fmt.Sprintf(`#!/bin/bash
while read line
do
	set -- $line
	if [ "$1" != "juju-collect-metrics" ]; then
		exit 1
	fi
	unitDetails=$(echo $2 | tr '/' ' ')
	unitDetails=( $unitDetails )
	uuid=$(cat /proc/sys/kernel/random/uuid)
	echo "[{\"charmurl\":\"%s\",\"uuid\":\"$uuid\",\"created\":\"2016-01-01T00:00:00Z\",\"metrics\":[{\"Key\":\"pings\",\"Value\":\"1\",\"Time\":\"2016-01-01T00:00:00.00Z\"}],\"unit-tag\":\"unit-${unitDetails[0]}-${unitDetails[1]}\"}]"
done <&0
`, charmURL)
}
