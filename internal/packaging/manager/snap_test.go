// Copyright 2019 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package manager_test

import (
	"os"
	"os/exec"
	"strings"

	"github.com/juju/proxy"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/packaging/commands"
	"github.com/juju/juju/internal/packaging/manager"
)

var (
	_ = gc.Suite(&SnapSuite{})

	snapProxyResponse = `
type: account-key
authority-id: canonical
revision: 2
public-key-sha3-384: BWDEoaqyr25nF5SNCvEv2v7QnM9QsfCc0PBMYD_i2NGSQ32EF2d4D0hqUel3m8ul
account-id: canonical
name: store
since: 2016-04-01T00:00:00.0Z
body-length: 717
sign-key-sha3-384: -CvQKAwRQ5h3Ffn10FILJoEZUXOv6km9FwA80-Rcj-f-6jadQ89VRswHNiEB9Lxk

DATA...

MORE DATA...

type: account
authority-id: canonical
account-id: 1234567890367OdMqoW9YLp3e0EgakQf
display-name: John Doe
timestamp: 2019-05-10T13:12:32.878905Z
username: jdoe
validation: unproven
sign-key-sha3-384: BWDEoaqyr25nF5SNCvEv2v7QnM9QsfCc0PBMYD_i2NGSQ32EF2d4D0hqUel3m8ul

DATA...

type: store
authority-id: canonical
store: 1234567890STOREIDENTIFIER0123456
operator-id: 0123456789067OdMqoW9YLp3e0EgakQf
timestamp: 2019-08-27T12:20:45.166790Z
url: 127.0.0.1
sign-key-sha3-384: BWDEoaqyr25nF5SNCvEv2v7QnM9QsfCc0PBMYD_i2NGSQ32EF2d4D0hqUel3m8ul

DATA...
DATA...
`
)

type SnapSuite struct {
	testing.IsolationSuite
}

func (s *SnapSuite) TestGetProxySettingsEmpty(c *gc.C) {
	const expected = `error: snap "system" has no "proxy" configuration option`

	cmdChan := s.HookCommandOutput(&manager.CommandOutput, []byte(expected), s.mockExitError(1))

	paccmder := commands.NewSnapPackageCommander()
	pacman := manager.NewSnapPackageManager()
	out, err := pacman.GetProxySettings()
	c.Assert(err, jc.ErrorIsNil)

	cmd := <-cmdChan
	c.Assert(cmd.Args, gc.DeepEquals, strings.Fields(paccmder.GetProxyCmd()))
	c.Assert(out, gc.Equals, proxy.Settings{})
}

func (s *SnapSuite) TestGetProxySettingsConfigured(c *gc.C) {
	const expected = `Key          Value
proxy.http   localhost:8080
proxy.https  localhost:8181
proxy.ftp  localhost:2121`
	cmdChan := s.HookCommandOutput(&manager.CommandOutput, []byte(expected), nil)

	paccmder := commands.NewSnapPackageCommander()
	pacman := manager.NewSnapPackageManager()
	out, err := pacman.GetProxySettings()
	c.Assert(err, gc.IsNil)

	cmd := <-cmdChan
	c.Assert(cmd.Args, gc.DeepEquals, strings.Fields(paccmder.GetProxyCmd()))

	c.Assert(out, gc.Equals, proxy.Settings{
		Http:  "localhost:8080",
		Https: "localhost:8181",
		Ftp:   "localhost:2121",
	})
}

func (s *SnapSuite) TestSearchForExistingPackage(c *gc.C) {
	const expected = `name:      juju
summary:   juju client
publisher: Canonical✓
contact:   https://jaas.ai/
license:   unset
description: |
  Juju is an open source modelling tool for operating software in the cloud.  Juju allows you to
  ...

  https://discourse.jujucharms.com/
  https://docs.jujucharms.com/
  https://github.com/juju/juju
commands:
  - juju
snap-id:      e2CPHpB1fUxcKtCyJTsm5t3hN9axJ0yj
tracking:     2.6/stable
refresh-date: today at 15:58 BST
channels:
  stable:        2.6.6                     2019-07-31 (8594) 68MB classic
  candidate:     ↑
  beta:          ↑
  edge:          2.7-beta1+develop-93d21f2 2019-08-19 (8756) 75MB classic
  2.6/stable:    2.6.6                     2019-07-31 (8594) 68MB classic
  ...
  2.3/beta:      ↑
  2.3/edge:      2.3.10+2.3-41313d1        2019-03-25 (7080) 55MB classic
installed:       2.6.6                                (8594) 68MB classic
`
	cmdChan := s.HookCommandOutput(&manager.CommandOutput, []byte(expected), nil)

	paccmder := commands.NewSnapPackageCommander()
	pacman := manager.NewSnapPackageManager()
	exists, err := pacman.Search("juju")
	c.Assert(err, gc.IsNil)
	c.Assert(exists, jc.IsTrue)

	cmd := <-cmdChan
	c.Assert(cmd.Args, gc.DeepEquals, strings.Fields(paccmder.SearchCmd("juju")))
}

func (s *SnapSuite) TestSearchForUnknownPackage(c *gc.C) {
	const expected = `error: no snap found for "foo"`

	cmdChan := s.HookCommandOutput(&manager.CommandOutput, []byte(expected), s.mockExitError(1))

	paccmder := commands.NewSnapPackageCommander()
	pacman := manager.NewSnapPackageManager()
	exists, err := pacman.Search("foo")
	c.Assert(err, gc.IsNil)
	c.Assert(exists, jc.IsFalse)

	cmd := <-cmdChan
	c.Assert(cmd.Args, gc.DeepEquals, strings.Fields(paccmder.SearchCmd("foo")))
}

func (s *SnapSuite) TestIsInstalled(c *gc.C) {
	const expected = `Name  Version  Rev   Tracking  Publisher   Notes
juju  2.6.6    8594  2.6       canonical✓  classic
`

	cmdChan := s.HookCommandOutput(&manager.CommandOutput, []byte(expected), nil)

	paccmder := commands.NewSnapPackageCommander()
	pacman := manager.NewSnapPackageManager()
	installed := pacman.IsInstalled("juju")
	c.Assert(installed, jc.IsTrue)

	cmd := <-cmdChan
	c.Assert(cmd.Args, gc.DeepEquals, strings.Fields(paccmder.IsInstalledCmd("juju")))
}

func (s *SnapSuite) TestIsInstalledForUnknownPackage(c *gc.C) {
	const expected = `error: no matching snaps installed`

	cmdChan := s.HookCommandOutput(&manager.CommandOutput, []byte(expected), s.mockExitError(1))

	paccmder := commands.NewSnapPackageCommander()
	pacman := manager.NewSnapPackageManager()
	installed := pacman.IsInstalled("foo")
	c.Assert(installed, jc.IsFalse)

	cmd := <-cmdChan
	c.Assert(cmd.Args, gc.DeepEquals, strings.Fields(paccmder.IsInstalledCmd("foo")))
}

func (s *SnapSuite) TestInstall(c *gc.C) {
	const expected = `juju 2.6.6 from Canonical✓ installed`

	cmdChan := s.HookCommandOutput(&manager.CommandOutput, []byte(expected), nil)

	paccmder := commands.NewSnapPackageCommander()
	pacman := manager.NewSnapPackageManager()
	err := pacman.Install("juju")
	c.Assert(err, gc.IsNil)

	cmd := <-cmdChan
	c.Assert(cmd.Args, gc.DeepEquals, strings.Fields(paccmder.InstallCmd("juju")))
}

func (s *SnapSuite) TestInstallWithMountFailure(c *gc.C) {
	const minRetries = 3
	var calls int
	state := os.ProcessState{}
	cmdError := &exec.ExitError{ProcessState: &state}
	s.PatchValue(&manager.SnapAttempts, minRetries)
	s.PatchValue(&manager.Delay, testing.ShortWait)
	s.PatchValue(&manager.ProcessStateSys, func(*os.ProcessState) interface{} {
		return mockExitStatuser(1) // retry each time.
	})
	s.PatchValue(&manager.CommandOutput, func(cmd *exec.Cmd) ([]byte, error) {
		calls++
		output := `
cannot perform the following tasks:
 - Mount snap "juju-db" (29) ([start snap-jujux2ddb-29.mount] failed with exit status 1: Job failed. See "journalctl -xe" for details.
)`[1:]
		return []byte(output), cmdError
	})

	pacman := manager.NewSnapPackageManager()
	err := pacman.Install("juju")
	c.Assert(err, gc.ErrorMatches, `packaging command failed: attempt count exceeded: .*`)
	c.Assert(calls, gc.Equals, minRetries)
}

func (s *SnapSuite) TestInstallWithUDevFailure(c *gc.C) {
	const minRetries = 3
	var calls int
	state := os.ProcessState{}
	cmdError := &exec.ExitError{ProcessState: &state}
	s.PatchValue(&manager.SnapAttempts, minRetries)
	s.PatchValue(&manager.Delay, testing.ShortWait)
	s.PatchValue(&manager.ProcessStateSys, func(*os.ProcessState) interface{} {
		return mockExitStatuser(2) // retry each time.
	})
	s.PatchValue(&manager.CommandOutput, func(cmd *exec.Cmd) ([]byte, error) {
		calls++
		output := `
error: cannot perform the following tasks:
 - Setup snap "snapd" (12159) security profiles (cannot reload udev rules: exit status 2
udev output:
)`[1:]
		return []byte(output), cmdError
	})

	pacman := manager.NewSnapPackageManager()
	err := pacman.Install("juju")
	c.Assert(err, gc.ErrorMatches, `packaging command failed: attempt count exceeded: .*`)
	c.Assert(calls, gc.Equals, minRetries)
}

func (s *SnapSuite) TestInstallWithFailureAndNonMatchingOutput(c *gc.C) {
	const minRetries = 3
	var calls int
	state := os.ProcessState{}
	cmdError := &exec.ExitError{ProcessState: &state}
	s.PatchValue(&manager.SnapAttempts, minRetries)
	s.PatchValue(&manager.Delay, testing.ShortWait)
	s.PatchValue(&manager.ProcessStateSys, func(*os.ProcessState) interface{} {
		return mockExitStatuser(1) // retry each time.
	})
	s.PatchValue(&manager.CommandOutput, func(cmd *exec.Cmd) ([]byte, error) {
		calls++
		// Replace the command path and args so it's a no-op.
		cmd.Path = ""
		cmd.Args = []string{"version"}
		// Call the real cmd.CombinedOutput to simulate better what
		// happens in production. See also http://pad.lv/1394524.
		output, _ := cmd.CombinedOutput()
		return output, cmdError
	})

	pacman := manager.NewSnapPackageManager()
	err := pacman.Install("juju")
	c.Assert(err, gc.ErrorMatches, `packaging command failed: exit status .*`)
	c.Assert(calls, gc.Equals, 1)
}

func (s *SnapSuite) TestInstallWithoutFailure(c *gc.C) {
	const minRetries = 3
	var calls int
	state := os.ProcessState{}
	cmdError := &exec.ExitError{ProcessState: &state}
	s.PatchValue(&manager.SnapAttempts, minRetries)
	s.PatchValue(&manager.Delay, testing.ShortWait)
	s.PatchValue(&manager.ProcessStateSys, func(*os.ProcessState) interface{} {
		return mockExitStatuser(0) // retry each time.
	})
	s.PatchValue(&manager.CommandOutput, func(cmd *exec.Cmd) ([]byte, error) {
		calls++
		// Replace the command path and args so it's a no-op.
		cmd.Path = ""
		cmd.Args = []string{"version"}
		// Call the real cmd.CombinedOutput to simulate better what
		// happens in production. See also http://pad.lv/1394524.
		output, _ := cmd.CombinedOutput()
		return output, cmdError
	})

	pacman := manager.NewSnapPackageManager()
	_ = pacman.Install("juju")
	c.Assert(calls, gc.Equals, 1)
}

func (s *SnapSuite) TestInstallWithDNSFailure(c *gc.C) {
	const minRetries = 3
	var calls int
	state := os.ProcessState{}
	cmdError := &exec.ExitError{ProcessState: &state}
	s.PatchValue(&manager.SnapAttempts, minRetries)
	s.PatchValue(&manager.Delay, testing.ShortWait)
	s.PatchValue(&manager.ProcessStateSys, func(*os.ProcessState) interface{} {
		return mockExitStatuser(100) // retry each time.
	})
	s.PatchValue(&manager.CommandOutput, func(cmd *exec.Cmd) ([]byte, error) {
		calls++
		// Replace the command path and args so it's a no-op.
		cmd.Path = ""
		cmd.Args = []string{"version"}
		// Call the real cmd.CombinedOutput to simulate better what
		// happens in production. See also http://pad.lv/1394524.
		output, _ := cmd.CombinedOutput()
		return output, cmdError
	})

	pacman := manager.NewSnapPackageManager()
	_ = pacman.Install("juju")
	c.Assert(calls, gc.Equals, 1)
}

func (s *SnapSuite) TestInstallForUnknownPackage(c *gc.C) {
	const minRetries = 3
	s.PatchValue(&manager.SnapAttempts, minRetries)
	s.PatchValue(&manager.Delay, testing.ShortWait)

	const expected = `error: snap "foo" not found`

	cmdChan := s.HookCommandOutput(&manager.CommandOutput, []byte(expected), s.mockExitError(1))

	paccmder := commands.NewSnapPackageCommander()
	pacman := manager.NewSnapPackageManager()
	err := pacman.Install("foo")
	c.Assert(err, gc.ErrorMatches, ".*unable to locate package")

	cmd := <-cmdChan
	c.Assert(cmd.Args, gc.DeepEquals, strings.Fields(paccmder.InstallCmd("foo")))
}

func (s *SnapSuite) TestConfigureProxy(c *gc.C) {
	cmdChan := s.HookCommandOutput(&manager.CommandOutput, nil, nil)

	pacman := manager.NewSnapPackageManager()
	err := pacman.ConfigureStoreProxy(snapProxyResponse, "1234567890STOREIDENTIFIER0123456")
	c.Assert(err, gc.IsNil)

	ackCmd := <-cmdChan
	c.Assert(strings.Join(ackCmd.Args, " "), gc.Matches, "snap ack .+")

	setCmd := <-cmdChan
	c.Assert(setCmd.Args, gc.DeepEquals, []string{"snap", "set", "system", "proxy.store=1234567890STOREIDENTIFIER0123456"})
}

func (s *SnapSuite) TestDisableStoreProxy(c *gc.C) {
	cmdChan := s.HookCommandOutput(&manager.CommandOutput, nil, nil)

	pacman := manager.NewSnapPackageManager()
	err := pacman.DisableStoreProxy()
	c.Assert(err, gc.IsNil)

	setCmd := <-cmdChan
	c.Assert(setCmd.Args, gc.DeepEquals, []string{"snap", "set", "system", "proxy.store="})
}

func (s *SnapSuite) TestInstalledChannel(c *gc.C) {
	const expected = `name:      juju
summary:   juju client
publisher: Canonical✓
contact:   https://jaas.ai/
license:   unset
description: |
  Juju is an open source modelling tool for operating software in the cloud.  Juju allows you to
  ...

  https://discourse.jujucharms.com/
  https://docs.jujucharms.com/
  https://github.com/juju/juju
commands:
  - juju
snap-id:      e2CPHpB1fUxcKtCyJTsm5t3hN9axJ0yj
tracking:     2.8/bleeding-edge
refresh-date: today at 15:58 BST
channels:
  stable:        2.6.6                     2019-07-31 (8594) 68MB classic
  candidate:     ↑
  beta:          ↑
  edge:          2.7-beta1+develop-93d21f2 2019-08-19 (8756) 75MB classic
  2.6/stable:    2.6.6                     2019-07-31 (8594) 68MB classic
  ...
  2.3/beta:      ↑
  2.3/edge:      2.3.10+2.3-41313d1        2019-03-25 (7080) 55MB classic
installed:       2.6.6                                (8594) 68MB classic
`
	cmdChan := s.HookCommandOutput(&manager.CommandOutput, []byte(expected), nil)

	pacman := manager.NewSnapPackageManager()
	channel := pacman.InstalledChannel("juju")
	c.Assert(channel, gc.Equals, "2.8/bleeding-edge")

	setCmd := <-cmdChan
	c.Assert(setCmd.Args, gc.DeepEquals, []string{"snap", "info", "juju"})
}

func (s *SnapSuite) TestInstalledChannelForNotInstalledSnap(c *gc.C) {
	const expected = `name:      juju
summary:   juju client
publisher: Canonical✓
contact:   https://jaas.ai/
license:   unset
description: |
  Juju is an open source modelling tool for operating software in the cloud.  Juju allows you to
  ...

  https://discourse.jujucharms.com/
  https://docs.jujucharms.com/
  https://github.com/juju/juju
commands:
  - juju
snap-id:      e2CPHpB1fUxcKtCyJTsm5t3hN9axJ0yj
refresh-date: today at 15:58 BST
channels:
  stable:        2.6.6                     2019-07-31 (8594) 68MB classic
  candidate:     ↑
  beta:          ↑
  edge:          2.7-beta1+develop-93d21f2 2019-08-19 (8756) 75MB classic
  2.6/stable:    2.6.6                     2019-07-31 (8594) 68MB classic
  ...
  2.3/beta:      ↑
  2.3/edge:      2.3.10+2.3-41313d1        2019-03-25 (7080) 55MB classic
installed:       2.6.6                                (8594) 68MB classic
`
	cmdChan := s.HookCommandOutput(&manager.CommandOutput, []byte(expected), nil)

	pacman := manager.NewSnapPackageManager()
	channel := pacman.InstalledChannel("juju")
	c.Assert(channel, gc.Equals, "")

	setCmd := <-cmdChan
	c.Assert(setCmd.Args, gc.DeepEquals, []string{"snap", "info", "juju"})
}

func (s *SnapSuite) TestChangeChannel(c *gc.C) {
	const expected = `lxd (candidate) 4.0.0 from Canonical✓ refreshed`
	cmdChan := s.HookCommandOutput(&manager.CommandOutput, []byte(expected), nil)

	pacman := manager.NewSnapPackageManager()
	err := pacman.ChangeChannel("lxd", "latest/candidate")
	c.Assert(err, jc.ErrorIsNil)

	setCmd := <-cmdChan
	c.Assert(setCmd.Args, gc.DeepEquals, []string{"snap", "refresh", "--channel", "latest/candidate", "lxd"})
}

func (s *SnapSuite) TestChangeChannelForNotInstalledSnap(c *gc.C) {
	const expected = `snap "lxd" is not installed`
	cmdChan := s.HookCommandOutput(&manager.CommandOutput, []byte(expected), nil)

	pacman := manager.NewSnapPackageManager()
	err := pacman.ChangeChannel("lxd", "latest/candidate")
	c.Assert(err, gc.ErrorMatches, "snap not installed")

	setCmd := <-cmdChan
	c.Assert(setCmd.Args, gc.DeepEquals, []string{"snap", "refresh", "--channel", "latest/candidate", "lxd"})
}

func (s *SnapSuite) mockExitError(code int) error {
	err := &exec.ExitError{ProcessState: new(os.ProcessState)}
	s.PatchValue(&manager.ProcessStateSys, func(*os.ProcessState) interface{} {
		return mockExitStatuser(code)
	})
	return err
}
