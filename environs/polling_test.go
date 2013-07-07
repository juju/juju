// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"errors"
	"time"

	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/utils"
)

type pollingSuite struct {
	originalLongAttempt utils.AttemptStrategy
}

var _ = Suite(&pollingSuite{})

func (s *pollingSuite) SetUpSuite(c *C) {
	s.originalLongAttempt = environs.LongAttempt
	// The implementation of AttemptStrategy does not yield at all for a
	// delay that's already expired.  So while this setting must be short
	// to avoid blocking tests, it must also allow enough time to convince
	// AttemptStrategy to sleep.  Otherwise a polling loop would just run
	// uninterrupted and a concurrent goroutine that it was waiting for
	// might never actually get to do its work.
	environs.LongAttempt = utils.AttemptStrategy{
		Total: 10 * time.Millisecond,
		Delay: 1 * time.Millisecond,
	}
}

func (s *pollingSuite) TearDownSuite(c *C) {
	environs.LongAttempt = s.originalLongAttempt
}

// dnsNameFakeInstance is a fake environs.Instance implementation where
// DNSName returns whatever you tell it to, and WaitDNSName delegates to the
// shared WaitDNSName implementation.  All the other methods are empty stubs.
type dnsNameFakeInstance struct {
	name string
	err  error
}

var _ instance.Instance = (*dnsNameFakeInstance)(nil)

func (inst *dnsNameFakeInstance) DNSName() (string, error) {
	return inst.name, inst.err
}

func (inst *dnsNameFakeInstance) WaitDNSName() (string, error) {
	return environs.WaitDNSName(inst)
}

func (*dnsNameFakeInstance) Id() instance.Id                          { return "" }
func (*dnsNameFakeInstance) OpenPorts(string, []instance.Port) error  { return nil }
func (*dnsNameFakeInstance) ClosePorts(string, []instance.Port) error { return nil }
func (*dnsNameFakeInstance) Ports(string) ([]instance.Port, error)    { return nil, nil }

func (pollingSuite) TestWaitDNSNameReturnsDNSNameIfAvailable(c *C) {
	inst := dnsNameFakeInstance{name: "anansi"}
	name, err := environs.WaitDNSName(&inst)
	c.Assert(err, IsNil)
	c.Check(name, Equals, "anansi")
}

func (pollingSuite) TestWaitDNSNamePollsOnErrNoDNSName(c *C) {
	inst := dnsNameFakeInstance{err: instance.ErrNoDNSName}
	_, err := environs.WaitDNSName(&inst)
	c.Assert(err, NotNil)
	c.Check(err, ErrorMatches, ".*timed out trying to get DNS address.*")
}

func (pollingSuite) TestWaitDNSNamePropagatesFailure(c *C) {
	failure := errors.New("deliberate failure")
	inst := dnsNameFakeInstance{err: failure}
	_, err := environs.WaitDNSName(&inst)
	c.Assert(err, NotNil)
	c.Check(err, Equals, failure)
}

func (pollingSuite) TestInstanceWaitDNSDelegatesToSharedWaitDNS(c *C) {
	inst := dnsNameFakeInstance{name: "anansi"}
	name, err := inst.WaitDNSName()
	c.Assert(err, IsNil)
	c.Check(name, Equals, "anansi")
}
