// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"os"
	"runtime"
	"time"

	"github.com/juju/testing"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
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
	s.IsolationSuite.SetUpSuite(c)
	if runtime.GOOS == "windows" {
		c.Skip("skipping BridgeSuite tests on windows")
	}
}

func assertCmdResult(c *gc.C, cmd, expected string) {
	result, err := network.RunCommand(cmd, os.Environ(), clock.WallClock, 0)
	c.Assert(err, gc.IsNil)
	c.Assert(result.Code, gc.Equals, 0)
	c.Assert(string(result.Stdout), gc.Equals, expected)
	c.Assert(string(result.Stderr), gc.Equals, "")
}

func assertBridgeCmd(c *gc.C, devices []network.DeviceToBridge, bridgePrefix, filename, script string, dryRun bool, reconfigureDelay int, expected string) {
	for _, python := range network.PythonInterpreters() {
		cmd := network.BridgeCmd(devices, python, bridgePrefix, filename, script, dryRun, reconfigureDelay)
		assertCmdResult(c, cmd, expected)
	}
}

func assertENIBridgerError(c *gc.C, devices []network.DeviceToBridge, environ []string, timeout time.Duration, clock clock.Clock, bridgePrefix, filename string, dryRun bool, reconfigureDelay int, expected string) {
	for _, python := range network.PythonInterpreters() {
		bridger := network.NewEtcNetworkInterfacesBridger(python, environ, clock, timeout, bridgePrefix, filename, dryRun)
		err := bridger.Bridge(devices, reconfigureDelay)
		c.Assert(err, gc.NotNil)
		c.Assert(err, gc.ErrorMatches, expected)
	}
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
	dryRun := true
	assertBridgeCmd(c, devices, "", "/etc/network/interfaces", echoArgsScript, dryRun, 0, `
--interfaces-to-bridge=ens3 ens4 bond0
--activate
--dry-run
--reconfigure-delay=0
/etc/network/interfaces
`[1:])
}

func (*BridgeSuite) TestBridgeCmdArgumentsNoNetBondReconfigureDelay(c *gc.C) {
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
	assertBridgeCmd(c, devices, "", "/etc/network/interfaces", echoArgsScript, true, 0, `
--interfaces-to-bridge=ens3 ens4 bond0
--activate
--dry-run
--reconfigure-delay=0
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
	assertBridgeCmd(c, devices, "foo-", "/etc/network/interfaces", echoArgsScript, true, 0, `
--interfaces-to-bridge=ens3 ens4 bond0
--activate
--bridge-prefix=foo-
--dry-run
--reconfigure-delay=0
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
	assertBridgeCmd(c, devices, "foo-", "/etc/network/interfaces", echoArgsScript, false, 0, `
--interfaces-to-bridge=ens3 ens4 bond0
--activate
--bridge-prefix=foo-
--reconfigure-delay=0
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
	assertBridgeCmd(c, devices, "", "/etc/network/interfaces", echoArgsScript, false, 0, `
--interfaces-to-bridge=ens3 ens4 bond0
--activate
--reconfigure-delay=0
/etc/network/interfaces
`[1:])
}

func (*BridgeSuite) TestBridgeCmdArgumentsWithNetBondReconfigureDelay(c *gc.C) {
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
	assertBridgeCmd(c, devices, "", "/etc/network/interfaces", echoArgsScript, false, 4, `
--interfaces-to-bridge=ens3 ens4 bond0
--activate
--reconfigure-delay=4
/etc/network/interfaces
`[1:])
}

func (*BridgeSuite) TestENIBridgerWithMissingFilenameArgument(c *gc.C) {
	devices := []network.DeviceToBridge{}
	expected := `(?s)bridgescript failed:.*(too few arguments|the following arguments are required: filename)\n`
	assertENIBridgerError(c, devices, os.Environ(), 0, clock.WallClock, "br-", "", true, 0, expected)
}

func (*BridgeSuite) TestENIBridgerWithEmptyDeviceNamesArgument(c *gc.C) {
	devices := []network.DeviceToBridge{}
	expected := `(?s)bridgescript failed:.*(too few arguments|no interfaces specified)\n`
	assertENIBridgerError(c, devices, os.Environ(), 0, clock.WallClock, "br-", "testdata/non-existent-filename", true, 0, expected)
}

func (*BridgeSuite) TestENIBridgerWithNonExistentFile(c *gc.C) {
	devices := []network.DeviceToBridge{
		network.DeviceToBridge{
			DeviceName: "ens3",
		},
	}
	expected := `(?s).*(IOError|FileNotFoundError):.*No such file or directory: 'testdata/non-existent-file'\n`
	assertENIBridgerError(c, devices, os.Environ(), 0, clock.WallClock, "br-", "testdata/non-existent-file", true, 0, expected)
}

func (*BridgeSuite) TestENIBridgerWithTimeout(c *gc.C) {
	devices := []network.DeviceToBridge{
		network.DeviceToBridge{
			DeviceName: "ens3",
		},
	}
	environ := os.Environ()
	environ = append(environ, "ADD_JUJU_BRIDGE_SLEEP_PREAMBLE_FOR_TESTING=10")
	expected := "bridgescript timed out after 500ms"
	assertENIBridgerError(c, devices, environ, 500*time.Millisecond, clock.WallClock, "br-", "testdata/non-existent-file", true, 0, expected)
}

func (*BridgeSuite) TestENIBridgerWithDryRun(c *gc.C) {
	devices := []network.DeviceToBridge{
		network.DeviceToBridge{
			DeviceName: "ens123",
		},
	}
	for _, python := range network.PythonInterpreters() {
		bridger := network.NewEtcNetworkInterfacesBridger(python, os.Environ(), clock.WallClock, 0, "", "testdata/interfaces", true)
		err := bridger.Bridge(devices, 0)
		c.Assert(err, gc.IsNil)
	}
}
