// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package debinterfaces_test

// These tests verify the commands that would be executed, but using a
// dryrun option to the script that is executed.

import (
	"fmt"
	"runtime"
	"time"

	"github.com/juju/juju/network/debinterfaces"
	"github.com/juju/testing"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"
)

type ActivationSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ActivationSuite{})

func (s *ActivationSuite) SetUpSuite(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("skipping ActivationSuite tests on windows")
	}
	s.IsolationSuite.SetUpSuite(c)
}

func (*BridgeSuite) TestActivateNonExistentDevice(c *gc.C) {
	params := debinterfaces.ActivationParams{
		DryRun:           true,
		Clock:            clock.WallClock,
		Devices:          map[string]string{"non-existent": "non-existent"},
		Filename:         "testdata/TestInputSourceStanza/interfaces",
		ReconfigureDelay: 10,
		Timeout:          5 * time.Minute,
	}

	result, err := debinterfaces.BridgeAndActivate(params)
	c.Assert(err, gc.IsNil)
	c.Check(result, gc.IsNil)
}

func (s *BridgeSuite) TestActivateEth0(c *gc.C) {
	filename := "testdata/TestInputSourceStanza/interfaces"

	params := debinterfaces.ActivationParams{
		Clock:            testing.NewClock(time.Now()),
		Devices:          map[string]string{"eth0": "br-eth0", "eth1": "br-eth1"},
		DryRun:           true,
		Filename:         filename,
		ReconfigureDelay: 10,
		Timeout:          5 * time.Minute,
	}
	result, err := debinterfaces.BridgeAndActivate(params)
	c.Assert(err, gc.IsNil)
	c.Check(result, gc.NotNil)
	c.Check(result.Code, gc.Equals, 0)

	expected := fmt.Sprintf(`
write_backup testdata/TestInputSourceStanza/interfaces.backup-%d
write_content testdata/TestInputSourceStanza/interfaces.new
ifdown --interfaces=testdata/TestInputSourceStanza/interfaces eth0 eth1
sleep 10
cp testdata/TestInputSourceStanza/interfaces.new testdata/TestInputSourceStanza/interfaces
ifup --interfaces=testdata/TestInputSourceStanza/interfaces -a
`, params.Clock.Now().Unix())
	c.Check(string(result.Stdout), gc.Equals, expected[1:])
}

func (s *BridgeSuite) TestActivateEth0WithoutBackup(c *gc.C) {
	filename := "testdata/TestInputSourceStanza/interfaces"

	params := debinterfaces.ActivationParams{
		Clock:            testing.NewClock(time.Now()),
		Devices:          map[string]string{"eth0": "br-eth0", "eth1": "br-eth1"},
		DryRun:           true,
		Filename:         filename,
		ReconfigureDelay: 100,
		Timeout:          5 * time.Minute,
	}

	result, err := debinterfaces.BridgeAndActivate(params)
	c.Assert(err, gc.IsNil)
	c.Check(result, gc.NotNil)
	c.Check(result.Code, gc.Equals, 0)

	expected := fmt.Sprintf(`
write_backup testdata/TestInputSourceStanza/interfaces.backup-%d
write_content testdata/TestInputSourceStanza/interfaces.new
ifdown --interfaces=testdata/TestInputSourceStanza/interfaces eth0 eth1
sleep 100
cp testdata/TestInputSourceStanza/interfaces.new testdata/TestInputSourceStanza/interfaces
ifup --interfaces=testdata/TestInputSourceStanza/interfaces -a
`, params.Clock.Now().Unix())
	c.Check(string(result.Stdout), gc.Equals, expected[1:])
}

func (s *BridgeSuite) TestActivateWithNegativeReconfigureDelay(c *gc.C) {
	filename := "testdata/TestInputSourceStanza/interfaces"

	params := debinterfaces.ActivationParams{
		Clock:            testing.NewClock(time.Now()),
		Devices:          map[string]string{"eth0": "br-eth0", "eth1": "br-eth1"},
		DryRun:           true,
		Filename:         filename,
		ReconfigureDelay: -3,
		Timeout:          5 * time.Minute,
	}

	result, err := debinterfaces.BridgeAndActivate(params)
	c.Assert(err, gc.IsNil)
	c.Check(result, gc.NotNil)
	c.Check(result.Code, gc.Equals, 0)

	expected := fmt.Sprintf(`
write_backup testdata/TestInputSourceStanza/interfaces.backup-%d
write_content testdata/TestInputSourceStanza/interfaces.new
ifdown --interfaces=testdata/TestInputSourceStanza/interfaces eth0 eth1
sleep 0
cp testdata/TestInputSourceStanza/interfaces.new testdata/TestInputSourceStanza/interfaces
ifup --interfaces=testdata/TestInputSourceStanza/interfaces -a
`, params.Clock.Now().Unix())
	c.Check(string(result.Stdout), gc.Equals, expected[1:])
}

func (*BridgeSuite) TestActivateWithNoDevicesSpecified(c *gc.C) {
	filename := "testdata/TestInputSourceStanza/interfaces"

	params := debinterfaces.ActivationParams{
		Clock:    clock.WallClock,
		Devices:  map[string]string{},
		DryRun:   true,
		Filename: filename,
	}

	_, err := debinterfaces.BridgeAndActivate(params)
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "no devices specified")
}

func (*BridgeSuite) TestActivateWithParsingError(c *gc.C) {
	filename := "testdata/TestInputSourceStanzaWithErrors/interfaces"

	params := debinterfaces.ActivationParams{
		Clock:    clock.WallClock,
		Devices:  map[string]string{"eth0": "br-eth0"},
		DryRun:   true,
		Filename: filename,
	}

	_, err := debinterfaces.BridgeAndActivate(params)
	c.Assert(err, gc.NotNil)
	c.Assert(err, gc.FitsTypeOf, &debinterfaces.ParseError{})
	parseError := err.(*debinterfaces.ParseError)
	c.Check(parseError, gc.DeepEquals, &debinterfaces.ParseError{
		Filename: "testdata/TestInputSourceStanzaWithErrors/interfaces.d/eth1.cfg",
		Line:     "iface",
		LineNum:  2,
		Message:  "missing device name",
	})
}

func (*BridgeSuite) TestActivateWithTimeout(c *gc.C) {
	filename := "testdata/TestInputSourceStanza/interfaces"

	params := debinterfaces.ActivationParams{
		Clock:    clock.WallClock,
		Devices:  map[string]string{"eth0": "br-eth0", "eth1": "br-eth1"},
		DryRun:   true,
		Filename: filename,
		// magic value causing the bash script to sleep
		ReconfigureDelay: 25694,
		Timeout:          10,
	}

	_, err := debinterfaces.BridgeAndActivate(params)
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "bridge activation error: command cancelled")
}

func (*BridgeSuite) TestActivateFailure(c *gc.C) {
	filename := "testdata/TestInputSourceStanza/interfaces"

	params := debinterfaces.ActivationParams{
		Clock:    clock.WallClock,
		Devices:  map[string]string{"eth0": "br-eth0", "eth1": "br-eth1"},
		DryRun:   true,
		Filename: filename,
		// magic value causing the bash script to fail
		ReconfigureDelay: 25695,
		Timeout:          5 * time.Minute,
	}

	result, err := debinterfaces.BridgeAndActivate(params)
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "bridge activation failed: artificial failure\n")
	c.Check(result.Code, gc.Equals, 1)
}

func (*BridgeSuite) TestActivateFailureShortMessage(c *gc.C) {
	filename := "testdata/TestInputSourceStanza/interfaces"

	params := debinterfaces.ActivationParams{
		Clock:    clock.WallClock,
		Devices:  map[string]string{"eth0": "br-eth0", "eth1": "br-eth1"},
		DryRun:   true,
		Filename: filename,
		// magic value causing the bash script to fail
		ReconfigureDelay: 25696,
		Timeout:          5 * time.Minute,
	}

	result, err := debinterfaces.BridgeAndActivate(params)
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "bridge activation failed, see logs for details")
	c.Check(result.Code, gc.Equals, 1)
}
