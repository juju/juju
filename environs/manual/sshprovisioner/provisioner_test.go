// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package sshprovisioner_test

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/utils/v4/shell"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs/manual"
	"github.com/juju/juju/environs/manual/sshprovisioner"
	"github.com/juju/juju/internal/cloudconfig"
	"github.com/juju/juju/internal/cloudconfig/cloudinit"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/testing"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/rpc/params"
)

type provisionerSuite struct {
	testhelpers.LoggingCleanupSuite
}

func TestProvisionerSuite(t *stdtesting.T) {
	tc.Run(t, &provisionerSuite{})
}

type mockMachineManager struct {
	manual.ProvisioningClientAPI
}

func (m *mockMachineManager) ProvisioningScript(context.Context, params.ProvisioningScriptParams) (script string, err error) {
	return "echo hello", nil
}

func (m *mockMachineManager) AddMachines(ctx context.Context, args []params.AddMachineParams) ([]params.AddMachinesResult, error) {
	if len(args) != 1 {
		return nil, errors.Errorf("unexpected args: %+v", args)
	}
	a := args[0]
	b := jujuversion.DefaultSupportedLTSBase()
	if a.Base == nil || a.Base.Name != b.OS || a.Base.Channel != b.Channel.String() {
		return nil, errors.Errorf("unexpected base: %v", a.Base)
	}
	if a.Nonce == "" {
		return nil, errors.Errorf("unexpected empty nonce")
	}
	if !strings.HasPrefix(a.InstanceId.String(), "manual:") {
		return nil, errors.Errorf("unexpected instanceId: %s", a.InstanceId)
	}
	if len(a.Jobs) != 1 && a.Jobs[0] != model.JobHostUnits {
		return nil, errors.Errorf("unexpected jobs: %v", a.Jobs)
	}
	if len(a.Addrs) > 0 {
		return nil, errors.Errorf("unexpected addresses: %v", a.Addrs)
	}
	return []params.AddMachinesResult{{
		Machine: "2",
	}}, nil
}

func (m *mockMachineManager) DestroyMachinesWithParams(ctx context.Context, force, keep, dryRun bool, maxWait *time.Duration, machines ...string) ([]params.DestroyMachineResult, error) {
	if len(machines) == 0 || machines[0] != "2" {
		return nil, errors.Errorf("unexpected machines: %v", machines)
	}
	return []params.DestroyMachineResult{{
		Info: &params.DestroyMachineInfo{MachineId: machines[0]},
	}}, nil
}

func (s *provisionerSuite) getArgs(c *tc.C) manual.ProvisionMachineArgs {
	hostname, err := os.Hostname()
	c.Assert(err, tc.ErrorIsNil)
	client := &mockMachineManager{}
	return manual.ProvisionMachineArgs{
		Host:           hostname,
		Client:         client,
		UpdateBehavior: &params.UpdateBehavior{true, true},
	}
}

func (s *provisionerSuite) TestProvisionMachine(c *tc.C) {
	base := jujuversion.DefaultSupportedLTSBase()

	args := s.getArgs(c)
	hostname := args.Host
	args.Host = hostname
	args.User = "ubuntu"

	defer fakeSSH{
		Base:               base,
		Arch:               arch.AMD64,
		InitUbuntuUser:     true,
		SkipProvisionAgent: true,
	}.install(c).Restore()

	for i, errorCode := range []int{255, 0} {
		c.Logf("test %d: code %d", i, errorCode)
		defer fakeSSH{
			Base:                   base,
			Arch:                   arch.AMD64,
			InitUbuntuUser:         true,
			ProvisionAgentExitCode: errorCode,
		}.install(c).Restore()
		machineId, err := sshprovisioner.ProvisionMachine(c.Context(), args)
		if errorCode != 0 {
			c.Assert(err, tc.ErrorMatches, fmt.Sprintf("subprocess encountered error code %d", errorCode))
			c.Assert(machineId, tc.Equals, "")
		} else {
			c.Assert(err, tc.ErrorIsNil)
			c.Check(machineId, tc.Not(tc.Equals), "")
			// machine ID will be incremented. Even though we failed and the
			// machine is removed, the ID is not reused.
			c.Assert(machineId, tc.Equals, fmt.Sprint(i+1))
		}
	}

	// Attempting to provision a machine twice should fail. We effect
	// this by checking for existing juju systemd configurations.
	defer fakeSSH{
		Provisioned:        true,
		InitUbuntuUser:     true,
		SkipDetection:      true,
		SkipProvisionAgent: true,
	}.install(c).Restore()
	_, err := sshprovisioner.ProvisionMachine(c.Context(), args)
	c.Assert(err, tc.Equals, manual.ErrProvisioned)
	defer fakeSSH{
		Provisioned:              true,
		CheckProvisionedExitCode: 255,
		InitUbuntuUser:           true,
		SkipDetection:            true,
		SkipProvisionAgent:       true,
	}.install(c).Restore()
	_, err = sshprovisioner.ProvisionMachine(c.Context(), args)
	c.Assert(err, tc.ErrorMatches, "error checking if provisioned: subprocess encountered error code 255")
}

func (s *provisionerSuite) TestProvisioningScript(c *tc.C) {
	base := jujuversion.DefaultSupportedLTSBase()

	defer fakeSSH{
		Base:           base,
		Arch:           arch.AMD64,
		InitUbuntuUser: true,
	}.install(c).Restore()

	logDir := "/var/log"
	icfg := &instancecfg.InstanceConfig{
		ControllerTag: testing.ControllerTag,
		MachineId:     "10",
		MachineNonce:  "5432",
		Base:          corebase.MustParseBaseFromString("ubuntu@22.04"),
		APIInfo: &api.Info{
			Addrs:    []string{"127.0.0.1:1234"},
			Password: "pw2",
			CACert:   "CA CERT\n" + testing.CACert,
			Tag:      names.NewMachineTag("10"),
			ModelTag: testing.ModelTag,
		},
		DataDir:                 c.MkDir(),
		LogDir:                  path.Join(logDir, "juju"),
		MetricsSpoolDir:         c.MkDir(),
		Jobs:                    []model.MachineJob{model.JobManageModel, model.JobHostUnits},
		CloudInitOutputLog:      path.Join(logDir, "cloud-init-output.log"),
		AgentEnvironment:        map[string]string{agent.ProviderType: "dummy"},
		AuthorizedKeys:          "wheredidileavemykeys",
		MachineAgentServiceName: "jujud-machine-10",
	}
	tools := []*coretools.Tools{{
		Version: semversion.MustParseBinary("6.6.6-ubuntu-amd64"),
		URL:     "https://example.org",
	}}
	err := icfg.SetTools(tools)
	c.Assert(err, tc.ErrorIsNil)

	script, err := sshprovisioner.ProvisioningScript(icfg)
	c.Assert(err, tc.ErrorIsNil)

	cloudcfg, err := cloudinit.New("ubuntu")
	c.Assert(err, tc.ErrorIsNil)
	udata, err := cloudconfig.NewUserdataConfig(icfg, cloudcfg)
	c.Assert(err, tc.ErrorIsNil)
	err = udata.ConfigureJuju()
	c.Assert(err, tc.ErrorIsNil)
	cloudcfg.SetSystemUpgrade(false)
	provisioningScript, err := cloudcfg.RenderScript()
	c.Assert(err, tc.ErrorIsNil)

	removeLogFile := "rm -f '/var/log/cloud-init-output.log'\n"
	expectedScript := removeLogFile + shell.DumpFileOnErrorScript("/var/log/cloud-init-output.log") + provisioningScript
	c.Assert(script, tc.Equals, expectedScript)
}
