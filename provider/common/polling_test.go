// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"errors"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/provider/common"
	"launchpad.net/juju-core/utils"
)

type pollingSuite struct {
	originalLongAttempt utils.AttemptStrategy
}

var _ = gc.Suite(&pollingSuite{})

func (s *pollingSuite) SetUpSuite(c *gc.C) {
	s.originalLongAttempt = common.LongAttempt
	// The implementation of AttemptStrategy does not yield at all for a
	// delay that's already expired.  So while this setting must be short
	// to avoid blocking tests, it must also allow enough time to convince
	// AttemptStrategy to sleep.  Otherwise a polling loop would just run
	// uninterrupted and a concurrent goroutine that it was waiting for
	// might never actually get to do its work.
	common.LongAttempt = utils.AttemptStrategy{
		Total: 10 * time.Millisecond,
		Delay: 1 * time.Millisecond,
	}
}

func (s *pollingSuite) TearDownSuite(c *gc.C) {
	common.LongAttempt = s.originalLongAttempt
}

// dnsNameFakeInstance is a fake environs.Instance implementation where
// DNSName returns whatever you tell it to, and WaitDNSName delegates to the
// shared WaitDNSName implementation.  All the other methods are empty stubs.
type dnsNameFakeInstance struct {
	// embed a nil Instance to panic on unimplemented method
	instance.Instance
	name string
	err  error
}

var _ instance.Instance = (*dnsNameFakeInstance)(nil)

func (inst *dnsNameFakeInstance) DNSName() (string, error) {
	return inst.name, inst.err
}

func (inst *dnsNameFakeInstance) WaitDNSName() (string, error) {
	return common.WaitDNSName(inst)
}

func (*dnsNameFakeInstance) Id() instance.Id { return "" }

func (pollingSuite) TestWaitDNSNameReturnsDNSNameIfAvailable(c *gc.C) {
	inst := dnsNameFakeInstance{name: "anansi"}
	name, err := common.WaitDNSName(&inst)
	c.Assert(err, gc.IsNil)
	c.Check(name, gc.Equals, "anansi")
}

func (pollingSuite) TestWaitDNSNamePollsOnErrNoDNSName(c *gc.C) {
	inst := dnsNameFakeInstance{err: instance.ErrNoDNSName}
	_, err := common.WaitDNSName(&inst)
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, ".*timed out trying to get DNS address.*")
}

func (pollingSuite) TestWaitDNSNamePropagatesFailure(c *gc.C) {
	failure := errors.New("deliberate failure")
	inst := dnsNameFakeInstance{err: failure}
	_, err := common.WaitDNSName(&inst)
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.Equals, failure)
}

func (pollingSuite) TestInstanceWaitDNSDelegatesToSharedWaitDNS(c *gc.C) {
	inst := dnsNameFakeInstance{name: "anansi"}
	name, err := inst.WaitDNSName()
	c.Assert(err, gc.IsNil)
	c.Check(name, gc.Equals, "anansi")
}
