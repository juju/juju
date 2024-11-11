// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujutest

import (
	stdcontext "context"
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	apiclient "github.com/juju/juju/api/client/client"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/models"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/environs/sync"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	envtoolstesting "github.com/juju/juju/environs/tools/testing"
	"github.com/juju/juju/juju/keys"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	stateerrors "github.com/juju/juju/state/errors"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
)

const (
	AdminSecret = "admin-secret"
)

// LiveTests contains tests that are designed to run against a live server
// (e.g. Amazon EC2).  The Environ is opened once only for all the tests
// in the suite, stored in Env, and Destroyed after the suite has completed.
type LiveTests struct {
	gitjujutesting.CleanupSuite

	envtesting.ToolsFixture
	sstesting.TestDataSuite

	// TestConfig contains the configuration attributes for opening an environment.
	TestConfig coretesting.Attrs

	// Credential contains the credential for preparing an environment for
	// bootstrapping. If this is unset, empty credentials will be used.
	Credential cloud.Credential

	// CloudRegion contains the cloud region name to create resources in.
	CloudRegion string

	// CloudEndpoint contains the cloud API endpoint to communicate with.
	CloudEndpoint string

	// Attempt holds a strategy for waiting until the environment
	// becomes logically consistent.
	//
	// TODO(katco): 2016-08-09: lp:1611427
	Attempt utils.AttemptStrategy

	// CanOpenState should be true if the testing environment allows
	// the state to be opened after bootstrapping.
	CanOpenState bool

	// HasProvisioner should be true if the environment has
	// a provisioning agent.
	HasProvisioner bool

	// Env holds the currently opened environment.
	// This is set by PrepareOnce and BootstrapOnce.
	Env environs.Environ

	// ControllerStore holds the controller related information
	// such as controllers, accounts, etc., used when preparing
	// the environment. This is initialized by SetUpSuite.
	ControllerStore jujuclient.ClientStore

	// ControllerUUID is the uuid of the bootstrapped controller.
	ControllerUUID string

	// ProviderCallContext holds the context to be used to make
	// calls to a cloud provider.
	ProviderCallContext context.ProviderCallContext

	// BootstrapContext holds the context to bootstrap a test environment.
	BootstrapContext environs.BootstrapContext

	prepared     bool
	bootstrapped bool
	toolsStorage storage.Storage
}

func (t *LiveTests) SetUpSuite(c *gc.C) {
	t.CleanupSuite.SetUpSuite(c)
	t.TestDataSuite.SetUpSuite(c)
	t.ControllerStore = jujuclient.NewMemStore()
	t.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)

	// Setup the simplestreams bootstrap context.
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())

	ctx := stdcontext.TODO()
	ctx = stdcontext.WithValue(ctx, bootstrap.SimplestreamsFetcherContextKey, ss)

	t.BootstrapContext = envtesting.BootstrapContext(ctx, c)
	t.ProviderCallContext = context.NewCloudCallContext(ctx)
}

func (t *LiveTests) SetUpTest(c *gc.C) {
	t.CleanupSuite.SetUpTest(c)
	t.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	storageDir := c.MkDir()
	baseURLPath := filepath.Join(storageDir, "tools")
	t.DefaultBaseURL = utils.MakeFileURL(baseURLPath)
	t.ToolsFixture.SetUpTest(c)
	stor, err := filestorage.NewFileStorageWriter(storageDir)
	c.Assert(err, jc.ErrorIsNil)
	t.UploadFakeTools(c, stor, "released", "released")
	t.toolsStorage = stor
	t.CleanupSuite.PatchValue(&envtools.BundleTools, envtoolstesting.GetMockBundleTools(coretesting.FakeVersionNumber))
}

func (t *LiveTests) TearDownSuite(c *gc.C) {
	t.Destroy(c)
	t.TestDataSuite.TearDownSuite(c)
	t.CleanupSuite.TearDownSuite(c)
}

func (t *LiveTests) TearDownTest(c *gc.C) {
	t.ToolsFixture.TearDownTest(c)
	t.CleanupSuite.TearDownTest(c)
}

// PrepareOnce ensures that the environment is
// available and prepared. It sets t.Env appropriately.
func (t *LiveTests) PrepareOnce(c *gc.C) {
	if t.prepared {
		return
	}

	args := t.prepareForBootstrapParams(c)
	e, err := bootstrap.PrepareController(false, t.BootstrapContext, t.ControllerStore, args)
	c.Assert(err, gc.IsNil, gc.Commentf("preparing environ %#v", t.TestConfig))
	c.Assert(e, gc.NotNil)
	t.Env = e.(environs.Environ)
	t.prepared = true
	t.ControllerUUID = coretesting.FakeControllerConfig().ControllerUUID()
}

func (t *LiveTests) prepareForBootstrapParams(c *gc.C) bootstrap.PrepareParams {
	credential := t.Credential
	if credential.AuthType() == "" {
		credential = cloud.NewEmptyCredential()
	}
	return bootstrap.PrepareParams{
		ControllerConfig: coretesting.FakeControllerConfig(),
		ModelConfig:      t.TestConfig,
		Cloud: environscloudspec.CloudSpec{
			Type:       t.TestConfig["type"].(string),
			Name:       t.TestConfig["type"].(string),
			Region:     t.CloudRegion,
			Endpoint:   t.CloudEndpoint,
			Credential: &credential,
		},
		ControllerName: t.TestConfig["name"].(string),
		AdminSecret:    AdminSecret,
	}
}

func (t *LiveTests) bootstrapParams() bootstrap.BootstrapParams {
	credential := t.Credential
	if credential.AuthType() == "" {
		credential = cloud.NewEmptyCredential()
	}
	var regions []cloud.Region
	if t.CloudRegion != "" {
		regions = []cloud.Region{{
			Name:     t.CloudRegion,
			Endpoint: t.CloudEndpoint,
		}}
	}
	return bootstrap.BootstrapParams{
		ControllerConfig: coretesting.FakeControllerConfig(),
		Cloud: cloud.Cloud{
			Name:      t.TestConfig["type"].(string),
			Type:      t.TestConfig["type"].(string),
			AuthTypes: []cloud.AuthType{credential.AuthType()},
			Regions:   regions,
			Endpoint:  t.CloudEndpoint,
		},
		CloudRegion:             t.CloudRegion,
		CloudCredential:         &credential,
		CloudCredentialName:     "credential",
		AdminSecret:             AdminSecret,
		CAPrivateKey:            coretesting.CAKey,
		SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
	}
}

func (t *LiveTests) BootstrapOnce(c *gc.C) {
	if t.bootstrapped {
		return
	}

	t.PrepareOnce(c)
	// We only build and upload tools if there will be a state agent that
	// we could connect to (actual live tests, rather than local-only)
	cons := constraints.MustParse("mem=2G")
	if t.CanOpenState {
		ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
		_, err := sync.Upload(ss, t.toolsStorage, "released", nil)
		c.Assert(err, jc.ErrorIsNil)
	}
	args := t.bootstrapParams()
	args.BootstrapConstraints = cons
	args.ModelConstraints = cons

	err := bootstrap.Bootstrap(t.BootstrapContext, t.Env, t.ProviderCallContext, args)
	c.Assert(err, jc.ErrorIsNil)
	t.bootstrapped = true
}

func (t *LiveTests) Destroy(c *gc.C) {
	if t.Env == nil {
		return
	}
	err := environs.Destroy(t.Env.Config().Name(), t.Env, t.ProviderCallContext, t.ControllerStore)
	c.Assert(err, jc.ErrorIsNil)
	t.bootstrapped = false
	t.prepared = false
	t.ControllerUUID = ""
	t.Env = nil
}

func (t *LiveTests) TestPrechecker(c *gc.C) {
	// All implementations of InstancePrechecker should
	// return nil for empty constraints (excluding the
	// manual provider).
	t.PrepareOnce(c)
	err := t.Env.PrecheckInstance(t.ProviderCallContext,
		environs.PrecheckInstanceParams{
			Base: corebase.MakeDefaultBase("ubuntu", "24.04"),
		})
	c.Assert(err, jc.ErrorIsNil)
}

// TestStartStop is similar to Tests.TestStartStop except
// that it does not assume a pristine environment.
func (t *LiveTests) TestStartStop(c *gc.C) {
	t.BootstrapOnce(c)

	inst, _ := jujutesting.AssertStartInstance(c, t.Env, t.ProviderCallContext, t.ControllerUUID, "0")
	c.Assert(inst, gc.NotNil)
	id0 := inst.Id()

	insts, err := t.Env.Instances(t.ProviderCallContext, []instance.Id{id0, id0})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(insts, gc.HasLen, 2)
	c.Assert(insts[0].Id(), gc.Equals, id0)
	c.Assert(insts[1].Id(), gc.Equals, id0)

	// Asserting on the return of AllInstances makes the test fragile,
	// as even comparing the before and after start values can be thrown
	// off if other instances have been created or destroyed in the same
	// time frame. Instead, just check the instance we created exists.
	insts, err = t.Env.AllInstances(t.ProviderCallContext)
	c.Assert(err, jc.ErrorIsNil)
	found := false
	for _, inst := range insts {
		if inst.Id() == id0 {
			c.Assert(found, gc.Equals, false, gc.Commentf("%v", insts))
			found = true
		}
	}
	c.Assert(found, gc.Equals, true, gc.Commentf("expected %v in %v", inst, insts))

	addresses, err := jujutesting.WaitInstanceAddresses(t.Env, t.ProviderCallContext, inst.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addresses, gc.Not(gc.HasLen), 0)

	insts, err = t.Env.Instances(t.ProviderCallContext, []instance.Id{id0, ""})
	c.Assert(err, gc.Equals, environs.ErrPartialInstances)
	c.Assert(insts, gc.HasLen, 2)
	c.Check(insts[0].Id(), gc.Equals, id0)
	c.Check(insts[1], gc.IsNil)

	err = t.Env.StopInstances(t.ProviderCallContext, inst.Id())
	c.Assert(err, jc.ErrorIsNil)

	// The machine may not be marked as shutting down
	// immediately. Repeat a few times to ensure we get the error.
	for a := t.Attempt.Start(); a.Next(); {
		insts, err = t.Env.Instances(t.ProviderCallContext, []instance.Id{id0})
		if err != nil {
			break
		}
	}
	c.Assert(err, gc.Equals, environs.ErrNoInstances)
	c.Assert(insts, gc.HasLen, 0)
}

func (t *LiveTests) TestPorts(c *gc.C) {
	t.BootstrapOnce(c)

	inst1, _ := jujutesting.AssertStartInstance(c, t.Env, t.ProviderCallContext, t.ControllerUUID, "1")
	c.Assert(inst1, gc.NotNil)
	defer func() { _ = t.Env.StopInstances(t.ProviderCallContext, inst1.Id()) }()
	fwInst1, ok := inst1.(instances.InstanceFirewaller)
	c.Assert(ok, gc.Equals, true)

	rules, err := fwInst1.IngressRules(t.ProviderCallContext, "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rules, gc.HasLen, 0)

	inst2, _ := jujutesting.AssertStartInstance(c, t.Env, t.ProviderCallContext, t.ControllerUUID, "2")
	c.Assert(inst2, gc.NotNil)
	fwInst2, ok := inst2.(instances.InstanceFirewaller)
	c.Assert(ok, gc.Equals, true)
	rules, err = fwInst2.IngressRules(t.ProviderCallContext, "2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rules, gc.HasLen, 0)
	defer func() { _ = t.Env.StopInstances(t.ProviderCallContext, inst2.Id()) }()

	// Open some ports and check they're there.
	err = fwInst1.OpenPorts(t.ProviderCallContext,
		"1", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("67/udp")),
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("80-100/tcp")),
		})

	c.Assert(err, jc.ErrorIsNil)
	rules, err = fwInst1.IngressRules(t.ProviderCallContext, "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		rules, jc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("80-100/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("67/udp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		},
	)
	rules, err = fwInst2.IngressRules(t.ProviderCallContext, "2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rules, gc.HasLen, 0)

	err = fwInst2.OpenPorts(t.ProviderCallContext,
		"2", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("20-30/tcp")),
		})
	c.Assert(err, jc.ErrorIsNil)

	// Check there's no crosstalk to another machine
	rules, err = fwInst2.IngressRules(t.ProviderCallContext, "2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		rules, jc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("20-30/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		},
	)
	rules, err = fwInst1.IngressRules(t.ProviderCallContext, "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		rules, jc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("80-100/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("67/udp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		},
	)

	// Check that opening the same port again is ok.
	oldRules, err := fwInst2.IngressRules(t.ProviderCallContext, "2")
	c.Assert(err, jc.ErrorIsNil)
	err = fwInst2.OpenPorts(t.ProviderCallContext,
		"2", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp")),
		})
	c.Assert(err, jc.ErrorIsNil)
	err = fwInst2.OpenPorts(t.ProviderCallContext,
		"2", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("20-30/tcp")),
		})
	c.Assert(err, jc.ErrorIsNil)
	rules, err = fwInst2.IngressRules(t.ProviderCallContext, "2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rules, jc.DeepEquals, oldRules)

	// Check that opening the same port again and another port is ok.
	err = fwInst2.OpenPorts(t.ProviderCallContext,
		"2", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("99/tcp")),
		})
	c.Assert(err, jc.ErrorIsNil)
	rules, err = fwInst2.IngressRules(t.ProviderCallContext, "2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		rules, jc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("20-30/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("99/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		},
	)
	err = fwInst2.ClosePorts(t.ProviderCallContext,
		"2", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("99/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("20-30/tcp")),
		})
	c.Assert(err, jc.ErrorIsNil)

	// Check that we can close ports and that there's no crosstalk.
	rules, err = fwInst2.IngressRules(t.ProviderCallContext, "2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		rules, jc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		},
	)
	rules, err = fwInst1.IngressRules(t.ProviderCallContext, "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		rules, jc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("80-100/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("67/udp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		},
	)

	// Check that we can close multiple ports.
	err = fwInst1.ClosePorts(t.ProviderCallContext,
		"1", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("67/udp")),
			firewall.NewIngressRule(network.MustParsePortRange("80-100/tcp")),
		})
	c.Assert(err, jc.ErrorIsNil)
	rules, err = fwInst1.IngressRules(t.ProviderCallContext, "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rules, gc.HasLen, 0)

	// Check that we can close ports that aren't there.
	err = fwInst2.ClosePorts(t.ProviderCallContext,
		"2", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("111/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("222/udp")),
			firewall.NewIngressRule(network.MustParsePortRange("600-700/tcp")),
		})
	c.Assert(err, jc.ErrorIsNil)
	rules, err = fwInst2.IngressRules(t.ProviderCallContext, "2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		rules, jc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		},
	)

	// Check errors when acting on environment.
	fwEnv, ok := t.Env.(environs.Firewaller)
	c.Assert(ok, gc.Equals, true)
	err = fwEnv.OpenPorts(t.ProviderCallContext, firewall.IngressRules{firewall.NewIngressRule(network.MustParsePortRange("80/tcp"))})
	c.Assert(err, gc.ErrorMatches, `invalid firewall mode "instance" for opening ports on model`)

	err = fwEnv.ClosePorts(t.ProviderCallContext, firewall.IngressRules{firewall.NewIngressRule(network.MustParsePortRange("80/tcp"))})
	c.Assert(err, gc.ErrorMatches, `invalid firewall mode "instance" for closing ports on model`)

	_, err = fwEnv.IngressRules(t.ProviderCallContext)
	c.Assert(err, gc.ErrorMatches, `invalid firewall mode "instance" for retrieving ingress rules from model`)
}

func (t *LiveTests) TestGlobalPorts(c *gc.C) {
	t.BootstrapOnce(c)

	// Change configuration.
	oldConfig := t.Env.Config()
	defer func() {
		err := t.Env.SetConfig(oldConfig)
		c.Assert(err, jc.ErrorIsNil)
	}()

	attrs := t.Env.Config().AllAttrs()
	attrs["firewall-mode"] = config.FwGlobal
	newConfig, err := t.Env.Config().Apply(attrs)
	c.Assert(err, jc.ErrorIsNil)
	err = t.Env.SetConfig(newConfig)
	c.Assert(err, jc.ErrorIsNil)

	// Create instances and check open ports on both instances.
	inst1, _ := jujutesting.AssertStartInstance(c, t.Env, t.ProviderCallContext, t.ControllerUUID, "1")
	defer func() { _ = t.Env.StopInstances(t.ProviderCallContext, inst1.Id()) }()

	fwEnv, ok := t.Env.(environs.Firewaller)
	c.Assert(ok, gc.Equals, true)

	rules, err := fwEnv.IngressRules(t.ProviderCallContext)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rules, gc.HasLen, 0)

	inst2, _ := jujutesting.AssertStartInstance(c, t.Env, t.ProviderCallContext, t.ControllerUUID, "2")
	rules, err = fwEnv.IngressRules(t.ProviderCallContext)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rules, gc.HasLen, 0)
	defer func() { _ = t.Env.StopInstances(t.ProviderCallContext, inst2.Id()) }()

	err = fwEnv.OpenPorts(t.ProviderCallContext,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("67/udp")),
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("99/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("100-110/tcp")),
		})
	c.Assert(err, jc.ErrorIsNil)

	rules, err = fwEnv.IngressRules(t.ProviderCallContext)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		rules, jc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("99/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("100-110/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("67/udp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		},
	)

	// Check closing some ports.
	err = fwEnv.ClosePorts(t.ProviderCallContext,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("99/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("67/udp")),
		})
	c.Assert(err, jc.ErrorIsNil)

	rules, err = fwEnv.IngressRules(t.ProviderCallContext)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		rules, jc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("100-110/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		},
	)

	// Check that we can close ports that aren't there.
	err = fwEnv.ClosePorts(t.ProviderCallContext,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("111/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("222/udp")),
			firewall.NewIngressRule(network.MustParsePortRange("2000-2500/tcp")),
		})
	c.Assert(err, jc.ErrorIsNil)

	rules, err = fwEnv.IngressRules(t.ProviderCallContext)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		rules, jc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("100-110/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		},
	)

	fwInst1, ok := inst1.(instances.InstanceFirewaller)
	c.Assert(ok, gc.Equals, true)
	// Check errors when acting on instances.
	err = fwInst1.OpenPorts(t.ProviderCallContext,
		"1", firewall.IngressRules{firewall.NewIngressRule(network.MustParsePortRange("80/tcp"))})
	c.Assert(err, gc.ErrorMatches, `invalid firewall mode "global" for opening ports on instance`)

	err = fwInst1.ClosePorts(t.ProviderCallContext,
		"1", firewall.IngressRules{firewall.NewIngressRule(network.MustParsePortRange("80/tcp"))})
	c.Assert(err, gc.ErrorMatches, `invalid firewall mode "global" for closing ports on instance`)

	_, err = fwInst1.IngressRules(t.ProviderCallContext, "1")
	c.Assert(err, gc.ErrorMatches, `invalid firewall mode "global" for retrieving ingress rules from instance`)
}

func (t *LiveTests) TestModelPorts(c *gc.C) {
	t.BootstrapOnce(c)

	fwModelEnv, ok := t.Env.(models.ModelFirewaller)
	c.Assert(ok, gc.Equals, true)

	rules, err := fwModelEnv.ModelIngressRules(t.ProviderCallContext)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rules, jc.SameContents, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("22/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		// TODO: extend tests to check the api port isn't on hosted models.
		firewall.NewIngressRule(network.MustParsePortRange(strconv.Itoa(coretesting.FakeControllerConfig().APIPort())), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
	})

	err = fwModelEnv.OpenModelPorts(t.ProviderCallContext,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("67/udp")),
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("100-110/tcp")),
		})
	c.Assert(err, jc.ErrorIsNil)

	rules, err = fwModelEnv.ModelIngressRules(t.ProviderCallContext)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rules, jc.SameContents, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("22/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		// TODO: extend tests to check the api port isn't on hosted models.
		firewall.NewIngressRule(network.MustParsePortRange(strconv.Itoa(coretesting.FakeControllerConfig().APIPort())), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("100-110/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("67/udp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
	})

	// Check closing some ports.
	err = fwModelEnv.CloseModelPorts(t.ProviderCallContext,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("67/udp")),
		})
	c.Assert(err, jc.ErrorIsNil)

	rules, err = fwModelEnv.ModelIngressRules(t.ProviderCallContext)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rules, jc.SameContents, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("22/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		// TODO: extend tests to check the api port isn't on hosted models.
		firewall.NewIngressRule(network.MustParsePortRange(strconv.Itoa(coretesting.FakeControllerConfig().APIPort())), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("100-110/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
	})

	// Check that we can close ports that aren't there.
	err = fwModelEnv.CloseModelPorts(t.ProviderCallContext,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("111/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("222/udp")),
			firewall.NewIngressRule(network.MustParsePortRange("2000-2500/tcp")),
		})
	c.Assert(err, jc.ErrorIsNil)

	rules, err = fwModelEnv.ModelIngressRules(t.ProviderCallContext)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rules, jc.SameContents, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("22/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		// TODO: extend tests to check the api port isn't on hosted models.
		firewall.NewIngressRule(network.MustParsePortRange(strconv.Itoa(coretesting.FakeControllerConfig().APIPort())), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("100-110/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
	})

	// Cleanup
	err = fwModelEnv.CloseModelPorts(t.ProviderCallContext, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("100-110/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (t *LiveTests) TestBootstrapMultiple(c *gc.C) {
	// bootstrap.Bootstrap no longer raises errors if the environment is
	// already up, this has been moved into the bootstrap command.
	t.BootstrapOnce(c)

	c.Logf("destroy env")
	env := t.Env
	t.Destroy(c)
	err := env.Destroy(t.ProviderCallContext) // Again, should work fine and do nothing.
	c.Assert(err, jc.ErrorIsNil)

	// check that we can bootstrap after destroy
	t.BootstrapOnce(c)
}

func (t *LiveTests) TestBootstrapAndDeploy(c *gc.C) {
	if !t.CanOpenState || !t.HasProvisioner {
		c.Skip(fmt.Sprintf("skipping provisioner test, CanOpenState: %v, HasProvisioner: %v", t.CanOpenState, t.HasProvisioner))
	}
	t.BootstrapOnce(c)

	// TODO(niemeyer): Stop growing this kitchen sink test and split it into proper parts.

	c.Logf("opening state")
	st := t.Env.(jujutesting.GetStater).GetStateInAPIServer()

	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	owner := model.Owner()

	c.Logf("opening API connection")
	controllerCfg, err := st.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	caCert, _ := controllerCfg.CACert()
	apiInfo, err := environs.APIInfo(t.ProviderCallContext, model.Tag().Id(), model.Tag().Id(), caCert, controllerCfg.APIPort(), t.Env)
	c.Assert(err, jc.ErrorIsNil)
	apiInfo.Tag = owner
	apiInfo.Password = AdminSecret
	apiState, err := api.Open(apiInfo, api.DefaultDialOpts())
	c.Assert(err, jc.ErrorIsNil)
	defer apiState.Close()

	// Check that the agent version has made it through the
	// bootstrap process (it's optional in the config.Config)
	cfg, err := model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, ok := cfg.AgentVersion()
	c.Check(ok, jc.IsTrue)
	c.Check(agentVersion, gc.Equals, jujuversion.Current)

	// Check that the constraints have been set in the environment.
	cons, err := st.ModelConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons.String(), gc.Equals, "mem=2048M")

	// Wait for machine agent to come up on the bootstrap
	// machine and find the deployed series from that.
	m0, err := st.Machine("0")
	c.Assert(err, jc.ErrorIsNil)

	instId0, err := m0.InstanceId()
	c.Assert(err, jc.ErrorIsNil)

	// Check that the API connection is working.
	status, err := apiclient.NewClient(apiState, coretesting.NoopLogger{}).Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Machines["0"].InstanceId, gc.Equals, string(instId0))

	mw0 := newMachineToolWaiter(m0)
	defer func() { _ = mw0.Stop() }()

	// Controllers always run on Ubuntu.
	expectedVersion := version.Binary{
		Number:  jujuversion.Current,
		Arch:    arch.HostArch(),
		Release: "ubuntu",
	}

	mtools0 := waitAgentTools(c, mw0, expectedVersion)

	// Create a new application and deploy a unit of it.
	c.Logf("deploying application")
	ch := testcharms.Repo.ClonedDir(c.MkDir(), "dummy")
	sch, err := jujutesting.PutCharm(st, charm.MustParseURL("local:dummy"), ch)
	c.Assert(err, jc.ErrorIsNil)
	svc, err := st.AddApplication(state.AddApplicationArgs{Name: "dummy", Charm: sch})
	c.Assert(err, jc.ErrorIsNil)
	unit, err := svc.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = st.AssignUnit(unit, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	// Wait for the unit's machine and associated agent to come up
	// and announce itself.
	mid1, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	m1, err := st.Machine(mid1)
	c.Assert(err, jc.ErrorIsNil)
	mw1 := newMachineToolWaiter(m1)
	defer func() { _ = mw1.Stop() }()
	waitAgentTools(c, mw1, mtools0.Version)

	err = m1.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	instId1, err := m1.InstanceId()
	c.Assert(err, jc.ErrorIsNil)
	uw := newUnitToolWaiter(unit)
	defer func() { _ = uw.Stop() }()
	utools := waitAgentTools(c, uw, expectedVersion)

	// Check that we can upgrade the environment.
	newVersion := utools.Version
	newVersion.Patch++
	t.checkUpgrade(c, st, newVersion, mw0, mw1, uw)

	// BUG(niemeyer): Logic below is very much wrong. Must be:
	//
	// 1. EnsureDying on the unit and EnsureDying on the machine
	// 2. Unit dies by itself
	// 3. Machine removes dead unit
	// 4. Machine dies by itself
	// 5. Provisioner removes dead machine
	//

	// Now remove the unit and its assigned machine and
	// check that the PA removes it.
	c.Logf("removing unit")
	err = unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	// Wait until unit is dead
	uwatch := unit.Watch()
	defer func() { _ = uwatch.Stop() }()
	for unit.Life() != state.Dead {
		c.Logf("waiting for unit change")
		<-uwatch.Changes()
		err := unit.Refresh()
		c.Logf("refreshed; err %v", err)
		if errors.IsNotFound(err) {
			c.Logf("unit has been removed")
			break
		}
		c.Assert(err, jc.ErrorIsNil)
	}
	for {
		c.Logf("destroying machine")
		err := m1.Destroy()
		if err == nil {
			break
		}
		c.Assert(errors.Is(err, stateerrors.HasAssignedUnitsError), jc.IsTrue)
		time.Sleep(5 * time.Second)
		err = m1.Refresh()
		if errors.IsNotFound(err) {
			break
		}
		c.Assert(err, jc.ErrorIsNil)
	}
	c.Logf("waiting for instance to be removed")
	t.assertStopInstance(c, t.Env, instId1)
}

type tooler interface {
	Life() state.Life
	AgentTools() (*coretools.Tools, error)
	Refresh() error
	String() string
}

type watcher interface {
	Stop() error
	Err() error
}

type toolsWaiter struct {
	lastTools *coretools.Tools
	// changes is a chan of struct{} so that it can
	// be used with different kinds of entity watcher.
	changes chan struct{}
	watcher watcher
	tooler  tooler
}

func newMachineToolWaiter(m *state.Machine) *toolsWaiter {
	w := m.Watch()
	waiter := &toolsWaiter{
		changes: make(chan struct{}, 1),
		watcher: w,
		tooler:  m,
	}
	go func() {
		for range w.Changes() {
			waiter.changes <- struct{}{}
		}
		close(waiter.changes)
	}()
	return waiter
}

func newUnitToolWaiter(u *state.Unit) *toolsWaiter {
	w := u.Watch()
	waiter := &toolsWaiter{
		changes: make(chan struct{}, 1),
		watcher: w,
		tooler:  u,
	}
	go func() {
		for range w.Changes() {
			waiter.changes <- struct{}{}
		}
		close(waiter.changes)
	}()
	return waiter
}

func (w *toolsWaiter) Stop() error {
	return w.watcher.Stop()
}

// NextTools returns the next changed tools, waiting
// until the tools are actually set.
func (w *toolsWaiter) NextTools(c *gc.C) (*coretools.Tools, error) {
	for range w.changes {
		err := w.tooler.Refresh()
		if err != nil {
			return nil, fmt.Errorf("cannot refresh: %v", err)
		}
		if w.tooler.Life() == state.Dead {
			return nil, fmt.Errorf("object is dead")
		}
		tools, err := w.tooler.AgentTools()
		if errors.IsNotFound(err) {
			c.Logf("tools not yet set")
			continue
		}
		if err != nil {
			return nil, err
		}
		changed := w.lastTools == nil || *tools != *w.lastTools
		w.lastTools = tools
		if changed {
			return tools, nil
		}
		c.Logf("found same tools")
	}
	return nil, fmt.Errorf("watcher closed prematurely: %v", w.watcher.Err())
}

// waitAgentTools waits for the given agent
// to start and returns the tools that it is running.
func waitAgentTools(c *gc.C, w *toolsWaiter, expect version.Binary) *coretools.Tools {
	c.Logf("waiting for %v to signal agent version", w.tooler.String())
	tools, err := w.NextTools(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(tools.Version, gc.Equals, expect)
	return tools
}

// checkUpgrade sets the environment agent version and checks that
// all the provided watchers upgrade to the requested version.
func (t *LiveTests) checkUpgrade(c *gc.C, st *state.State, newVersion version.Binary, waiters ...*toolsWaiter) {
	c.Logf("putting testing version of juju tools")
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	upgradeTools, err := sync.Upload(
		ss, t.toolsStorage, "released",
		func(version.Number) version.Number { return newVersion.Number },
	)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the put version really is the version we expect.
	c.Assert(upgradeTools.Version, gc.Equals, newVersion)
	err = statetesting.SetAgentVersion(st, newVersion.Number)
	c.Assert(err, jc.ErrorIsNil)

	for i, w := range waiters {
		c.Logf("waiting for upgrade of %d: %v", i, w.tooler.String())

		waitAgentTools(c, w, newVersion)
		c.Logf("upgrade %d successful", i)
	}
}

// TODO(katco): 2016-08-09: lp:1611427
var waitAgent = utils.AttemptStrategy{
	Total: 30 * time.Second,
	Delay: 1 * time.Second,
}

func (t *LiveTests) assertStopInstance(c *gc.C, env environs.Environ, instId instance.Id) {
	var err error
	for a := waitAgent.Start(); a.Next(); {
		_, err = t.Env.Instances(t.ProviderCallContext, []instance.Id{instId})
		if err == nil {
			continue
		}
		if err == environs.ErrNoInstances {
			return
		}
		c.Logf("error from Instances: %v", err)
	}
	c.Fatalf("provisioner failed to stop machine after %v", waitAgent.Total)
}

func (t *LiveTests) TestStartInstanceWithEmptyNonceFails(c *gc.C) {
	// Check that we get a consistent error when asking for an instance without
	// a valid machine config.
	machineId := "4"
	apiInfo := jujutesting.FakeAPIInfo(machineId)
	instanceConfig, err := instancecfg.NewInstanceConfig(coretesting.ControllerTag, machineId, "",
		"released", corebase.MakeDefaultBase("ubuntu", "24.04"), apiInfo)
	c.Assert(err, jc.ErrorIsNil)

	t.PrepareOnce(c)
	possibleTools := coretools.List(envtesting.AssertUploadFakeToolsVersions(
		c, t.toolsStorage, "released", "released", version.MustParseBinary("5.4.5-ubuntu-amd64"),
	))
	fakeCallback := func(_ status.Status, _ string, _ map[string]interface{}) error {
		return nil
	}
	params := environs.StartInstanceParams{
		ControllerUUID: coretesting.ControllerTag.Id(),
		Tools:          possibleTools,
		InstanceConfig: instanceConfig,
		StatusCallback: fakeCallback,
	}
	err = jujutesting.SetImageMetadata(
		t.Env,
		simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory()),
		[]string{"24.04"},
		[]string{"amd64"},
		&params.ImageMetadata,
	)
	c.Check(err, jc.ErrorIsNil)
	result, err := t.Env.StartInstance(t.ProviderCallContext, params)
	if result != nil && result.Instance != nil {
		err := t.Env.StopInstances(t.ProviderCallContext, result.Instance.Id())
		c.Check(err, jc.ErrorIsNil)
	}
	c.Assert(result, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, ".*missing machine nonce")
}

func (t *LiveTests) TestBootstrapWithDefaultSeries(c *gc.C) {
	if !t.HasProvisioner {
		c.Skip("HasProvisioner is false; cannot test deployment")
	}

	current := coretesting.CurrentVersion()
	other := current
	other.Release = "quantal"

	dummyCfg := dummy.SampleConfig().Merge(coretesting.Attrs{
		"controller": false,
		"name":       "dummy storage",
	})
	args := t.prepareForBootstrapParams(c)
	args.ModelConfig = dummyCfg
	e, err := bootstrap.PrepareController(false, t.BootstrapContext,
		jujuclient.NewMemStore(),
		args,
	)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = e.(environs.Environ).Destroy(t.ProviderCallContext) }()

	t.Destroy(c)

	attrs := t.TestConfig.Merge(coretesting.Attrs{
		"name":           "livetests",
		"default-series": "quantal",
	})
	args.ModelConfig = attrs
	env, err := bootstrap.PrepareController(false, t.BootstrapContext,
		t.ControllerStore,
		args)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = environs.Destroy("livetests", env, t.ProviderCallContext, t.ControllerStore) }()

	err = bootstrap.Bootstrap(t.BootstrapContext, env, t.ProviderCallContext, t.bootstrapParams())
	c.Assert(err, jc.ErrorIsNil)

	st := t.Env.(jujutesting.GetStater).GetStateInAPIServer()
	// Wait for machine agent to come up on the bootstrap
	// machine and ensure it deployed the proper series.
	m0, err := st.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	mw0 := newMachineToolWaiter(m0)
	defer func() { _ = mw0.Stop() }()

	waitAgentTools(c, mw0, other)
}

func (t *LiveTests) TestIngressRulesWithPartiallyMatchingCIDRs(c *gc.C) {
	t.BootstrapOnce(c)

	inst1, _ := jujutesting.AssertStartInstance(c, t.Env, t.ProviderCallContext, t.ControllerUUID, "1")
	c.Assert(inst1, gc.NotNil)
	defer func() { _ = t.Env.StopInstances(t.ProviderCallContext, inst1.Id()) }()
	fwInst1, ok := inst1.(instances.InstanceFirewaller)
	c.Assert(ok, gc.Equals, true)

	rules, err := fwInst1.IngressRules(t.ProviderCallContext, "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rules, gc.HasLen, 0)

	// Open ports with different CIDRs. Check that rules with same port range
	// get merged.
	err = fwInst1.OpenPorts(t.ProviderCallContext,
		"1", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("42/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("42/tcp"), "10.0.0.0/24"),
			firewall.NewIngressRule(network.MustParsePortRange("80/tcp")), // open to 0.0.0.0/0
		})

	c.Assert(err, jc.ErrorIsNil)
	rules, err = fwInst1.IngressRules(t.ProviderCallContext, "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		rules, jc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("42/tcp"), firewall.AllNetworksIPV4CIDR, "10.0.0.0/24"),
			firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		},
	)

	// Open same port with different CIDRs and check that the CIDR gets
	// appended to the existing rule's CIDR list.
	err = fwInst1.OpenPorts(t.ProviderCallContext,
		"1", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("42/tcp"), "192.168.0.0/24"),
		})

	c.Assert(err, jc.ErrorIsNil)
	rules, err = fwInst1.IngressRules(t.ProviderCallContext, "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		rules, jc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("42/tcp"), firewall.AllNetworksIPV4CIDR, "10.0.0.0/24", "192.168.0.0/24"),
			firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		},
	)

	// Close port on a subset of the CIDRs and ensure that that CIDR gets
	// removed from the ingress rules
	err = fwInst1.ClosePorts(t.ProviderCallContext,
		"1", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("42/tcp"), "192.168.0.0/24"),
		})

	c.Assert(err, jc.ErrorIsNil)
	rules, err = fwInst1.IngressRules(t.ProviderCallContext, "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		rules, jc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("42/tcp"), firewall.AllNetworksIPV4CIDR, "10.0.0.0/24"),
			firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		},
	)

	// Remove all CIDRs from the rule and check that rules without CIDRs
	// get dropped.
	err = fwInst1.ClosePorts(t.ProviderCallContext,
		"1", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("42/tcp"), firewall.AllNetworksIPV4CIDR, "10.0.0.0/24"),
		})

	c.Assert(err, jc.ErrorIsNil)
	rules, err = fwInst1.IngressRules(t.ProviderCallContext, "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		rules, jc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		},
	)
}
