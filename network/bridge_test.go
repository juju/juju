// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"os"
	"runtime"
	"time"

	"github.com/juju/juju/network"
	"github.com/juju/testing"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"
)

// A note regarding the use of clock.WallClock in these unit tests.
//
// All the tests pass 0 for a timeout, which means indefinite, and
// therefore no timer/clock is used. There is one test that checks for
// timeout and passes 0.5s as its timeout value. Because of this it's
// not clear why the 'testing clock' would be a better choice.

type BridgeSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&BridgeSuite{})

const echoArgsScript = `
import sys
for arg in sys.argv[1:]: print(arg)
`

func (s *BridgeSuite) SetUpSuite(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("skipping BridgeSuite tests on windows")
	}
	s.IsolationSuite.SetUpSuite(c)
}

func assertCmdResult(c *gc.C, cmd, expected string) {
	result, err := network.RunCommand(cmd, os.Environ(), clock.WallClock, 0)
	c.Assert(err, gc.IsNil)
	c.Assert(result.Code, gc.Equals, 0)
	c.Assert(string(result.Stdout), gc.Equals, expected)
	c.Assert(string(result.Stderr), gc.Equals, "")
}

func (*BridgeSuite) TestBridgeCmdArgumentsNoBridgePrefixAndDryRun(c *gc.C) {
	devices := []network.DeviceToBridge{
		network.DeviceToBridge{
			DeviceName: "ens3",
		},
		network.DeviceToBridge{
			DeviceName: "ens4",
		},
		network.DeviceToBridge{
			DeviceName: "bond0",
		},
	}
	cmd := network.BridgeCmd(devices, "", "/etc/network/interfaces", echoArgsScript, true)
	assertCmdResult(c, cmd, `
--interfaces-to-bridge=ens3 ens4 bond0
--activate
--dry-run
/etc/network/interfaces
`[1:])
}

func (*BridgeSuite) TestBridgeCmdArgumentsNoBondReconfigureDelay(c *gc.C) {
	devices := []network.DeviceToBridge{
		network.DeviceToBridge{
			DeviceName: "ens3",
		},
		network.DeviceToBridge{
			DeviceName: "ens4",
		},
		network.DeviceToBridge{
			DeviceName: "bond0",
		},
	}
	cmd := network.BridgeCmd(devices, "", "/etc/network/interfaces", echoArgsScript, true)
	assertCmdResult(c, cmd, `
--interfaces-to-bridge=ens3 ens4 bond0
--activate
--dry-run
/etc/network/interfaces
`[1:])
}

func (*BridgeSuite) TestBridgeCmdArgumentsWithBridgePrefixAndDryRun(c *gc.C) {
	devices := []network.DeviceToBridge{
		network.DeviceToBridge{
			DeviceName: "ens3",
		},
		network.DeviceToBridge{
			DeviceName: "ens4",
		},
		network.DeviceToBridge{
			DeviceName: "bond0",
		},
	}
	cmd := network.BridgeCmd(devices, "foo-", "/etc/network/interfaces", echoArgsScript, true)
	assertCmdResult(c, cmd, `
--interfaces-to-bridge=ens3 ens4 bond0
--activate
--bridge-prefix=foo-
--dry-run
/etc/network/interfaces
`[1:])
}

func (*BridgeSuite) TestBridgeCmdArgumentsWithBridgePrefixWithoutDryRun(c *gc.C) {
	devices := []network.DeviceToBridge{
		network.DeviceToBridge{
			DeviceName: "ens3",
		},
		network.DeviceToBridge{
			DeviceName: "ens4",
		},
		network.DeviceToBridge{
			DeviceName: "bond0",
		},
	}
	cmd := network.BridgeCmd(devices, "foo-", "/etc/network/interfaces", echoArgsScript, false)
	assertCmdResult(c, cmd, `
--interfaces-to-bridge=ens3 ens4 bond0
--activate
--bridge-prefix=foo-
/etc/network/interfaces
`[1:])
}

func (*BridgeSuite) TestBridgeCmdArgumentsWithoutBridgePrefixAndWithoutDryRun(c *gc.C) {
	devices := []network.DeviceToBridge{
		network.DeviceToBridge{
			DeviceName: "ens3",
		},
		network.DeviceToBridge{
			DeviceName: "ens4",
		},
		network.DeviceToBridge{
			DeviceName: "bond0",
		},
	}
	cmd := network.BridgeCmd(devices, "", "/etc/network/interfaces", echoArgsScript, false)
	assertCmdResult(c, cmd, `
--interfaces-to-bridge=ens3 ens4 bond0
--activate
/etc/network/interfaces
`[1:])
}

func (*BridgeSuite) TestBridgeCmdArgumentsWithBondReconfigureDelay(c *gc.C) {
	devices := []network.DeviceToBridge{
		network.DeviceToBridge{
			DeviceName:           "ens3",
			BondReconfigureDelay: 0,
		},
		network.DeviceToBridge{
			DeviceName:           "ens4",
			BondReconfigureDelay: 4,
		},
		network.DeviceToBridge{
			DeviceName:           "bond0",
			BondReconfigureDelay: 2,
		},
	}
	cmd := network.BridgeCmd(devices, "", "/etc/network/interfaces", echoArgsScript, false)
	assertCmdResult(c, cmd, `
--interfaces-to-bridge=ens3 ens4 bond0
--activate
--bond-reconfigure-delay=4
/etc/network/interfaces
`[1:])
}

func (*BridgeSuite) TestENIBridgerWithMissingFilenameArgument(c *gc.C) {
	devices := []network.DeviceToBridge{
		network.DeviceToBridge{
			DeviceName: "ens3",
		},
		network.DeviceToBridge{
			DeviceName: "ens4",
		},
		network.DeviceToBridge{
			DeviceName: "bond0",
		},
	}
	bridger := network.NewEtcNetworkInterfacesBridger(os.Environ(), clock.WallClock, 0, "", "", true)
	err := bridger.Bridge(devices)
	c.Assert(err, gc.ErrorMatches, `(?s)bridgescript failed:.*too few arguments\n`)
}

func (*BridgeSuite) TestENIBridgerWithEmptyDeviceNamesArgument(c *gc.C) {
	bridger := network.NewEtcNetworkInterfacesBridger(os.Environ(), clock.WallClock, 0, "", "", true)
	err := bridger.Bridge([]network.DeviceToBridge{})
	c.Assert(err, gc.ErrorMatches, `(?s)bridgescript failed:.*too few arguments\n`)
}

func (*BridgeSuite) TestENIBridgerWithNonExistentFile(c *gc.C) {
	devices := []network.DeviceToBridge{
		network.DeviceToBridge{
			DeviceName: "ens3",
		},
		network.DeviceToBridge{
			DeviceName: "ens4",
		},
		network.DeviceToBridge{
			DeviceName: "bond0",
		},
	}
	bridger := network.NewEtcNetworkInterfacesBridger(os.Environ(), clock.WallClock, 0, "", "testdata/non-existent-file", true)
	err := bridger.Bridge(devices)
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, `(?s).*IOError:.*No such file or directory: 'testdata/non-existent-file'\n`)
}

func (*BridgeSuite) TestENIBridgerWithTimeout(c *gc.C) {
	devices := []network.DeviceToBridge{
		network.DeviceToBridge{
			DeviceName: "ens3",
		},
		network.DeviceToBridge{
			DeviceName: "ens4",
		},
		network.DeviceToBridge{
			DeviceName: "bond0",
		},
	}
	environ := os.Environ()
	environ = append(environ, "ADD_JUJU_BRIDGE_SLEEP_PREAMBLE_FOR_TESTING=10")
	bridger := network.NewEtcNetworkInterfacesBridger(environ, clock.WallClock, 500*time.Millisecond, "", "testdata/non-existent-file", true)
	err := bridger.Bridge(devices)
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, `bridgescript timed out after 500ms`)
}

func (*BridgeSuite) TestENIBridgerWithDryRun(c *gc.C) {
	devices := []network.DeviceToBridge{
		network.DeviceToBridge{
			DeviceName: "ens123",
		},
	}
	bridger := network.NewEtcNetworkInterfacesBridger(os.Environ(), clock.WallClock, 0, "", "testdata/interfaces", true)
	err := bridger.Bridge(devices)
	c.Assert(err, gc.IsNil)
}
