// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	"fmt"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/clock"
	"github.com/juju/utils/series"
	"github.com/juju/utils/set"
	"github.com/juju/utils/ssh"
	"gopkg.in/amz.v3/aws"
	amzec2 "gopkg.in/amz.v3/ec2"
	"gopkg.in/amz.v3/ec2/ec2test"
	"gopkg.in/amz.v3/s3/s3test"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/imagemetadata"
	imagetesting "github.com/juju/juju/environs/imagemetadata/testing"
	"github.com/juju/juju/environs/jujutest"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/environs/tags"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/ec2"
	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type ProviderSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&ProviderSuite{})

var localConfigAttrs = coretesting.FakeConfig().Merge(coretesting.Attrs{
	"name":          "sample",
	"type":          "ec2",
	"agent-version": coretesting.FakeVersionNumber.String(),
})

func registerLocalTests() {
	// N.B. Make sure the region we use here
	// has entries in the images/query txt files.
	aws.Regions["test"] = aws.Region{
		Name: "test",
	}

	gc.Suite(&localServerSuite{})
	gc.Suite(&localLiveSuite{})
	gc.Suite(&localNonUSEastSuite{})
}

// localLiveSuite runs tests from LiveTests using a fake
// EC2 server that runs within the test process itself.
type localLiveSuite struct {
	LiveTests
	srv                localServer
	restoreEC2Patching func()
}

func (t *localLiveSuite) SetUpSuite(c *gc.C) {
	t.LiveTests.SetUpSuite(c)
	t.Credential = cloud.NewCredential(
		cloud.AccessKeyAuthType,
		map[string]string{
			"access-key": "x",
			"secret-key": "x",
		},
	)
	t.CloudRegion = "test"

	// Upload arches that ec2 supports; add to this
	// as ec2 coverage expands.
	t.UploadArches = []string{arch.AMD64, arch.I386}
	t.TestConfig = localConfigAttrs
	t.restoreEC2Patching = patchEC2ForTesting(c)
	imagetesting.PatchOfficialDataSources(&t.BaseSuite.CleanupSuite, "test:")
	t.BaseSuite.PatchValue(&imagemetadata.SimplestreamsImagesPublicKey, sstesting.SignedMetadataPublicKey)
	t.BaseSuite.PatchValue(ec2.DeleteSecurityGroupInsistently, deleteSecurityGroupForTestFunc)
	t.srv.createRootDisks = true
	t.srv.startServer(c)
}

func (t *localLiveSuite) TearDownSuite(c *gc.C) {
	t.LiveTests.TearDownSuite(c)
	t.srv.stopServer(c)
	t.restoreEC2Patching()
}

// localServer represents a fake EC2 server running within
// the test process itself.
type localServer struct {
	// createRootDisks is used to decide whether or not
	// the ec2test server will create root disks for
	// instances.
	createRootDisks bool

	client *amzec2.EC2
	ec2srv *ec2test.Server
	s3srv  *s3test.Server
	config *s3test.Config

	defaultVPC *amzec2.VPC
	zones      []amzec2.AvailabilityZoneInfo
}

func (srv *localServer) startServer(c *gc.C) {
	var err error
	srv.ec2srv, err = ec2test.NewServer()
	if err != nil {
		c.Fatalf("cannot start ec2 test server: %v", err)
	}
	srv.ec2srv.SetCreateRootDisks(srv.createRootDisks)
	srv.s3srv, err = s3test.NewServer(srv.config)
	if err != nil {
		c.Fatalf("cannot start s3 test server: %v", err)
	}
	aws.Regions["test"] = aws.Region{
		Name:                 "test",
		EC2Endpoint:          srv.ec2srv.URL(),
		S3Endpoint:           srv.s3srv.URL(),
		S3LocationConstraint: true,
	}
	srv.addSpice(c)

	region := aws.Regions["test"]
	signer := aws.SignV4Factory(region.Name, "ec2")
	srv.client = amzec2.New(aws.Auth{}, region, signer)

	zones := make([]amzec2.AvailabilityZoneInfo, 3)
	zones[0].Region = "test"
	zones[0].Name = "test-available"
	zones[0].State = "available"
	zones[1].Region = "test"
	zones[1].Name = "test-impaired"
	zones[1].State = "impaired"
	zones[2].Region = "test"
	zones[2].Name = "test-unavailable"
	zones[2].State = "unavailable"
	srv.ec2srv.SetAvailabilityZones(zones)
	srv.ec2srv.SetInitialInstanceState(ec2test.Pending)
	srv.zones = zones

	defaultVPC, err := srv.ec2srv.AddDefaultVPCAndSubnets()
	c.Assert(err, jc.ErrorIsNil)
	srv.defaultVPC = &defaultVPC
}

// addSpice adds some "spice" to the local server
// by adding state that may cause tests to fail.
func (srv *localServer) addSpice(c *gc.C) {
	states := []amzec2.InstanceState{
		ec2test.ShuttingDown,
		ec2test.Terminated,
		ec2test.Stopped,
	}
	for _, state := range states {
		srv.ec2srv.NewInstances(1, "m1.small", "ami-a7f539ce", state, nil)
	}
}

func (srv *localServer) stopServer(c *gc.C) {
	srv.ec2srv.Reset(false)
	srv.ec2srv.Quit()
	srv.s3srv.Quit()
	// Clear out the region because the server address is
	// no longer valid.
	delete(aws.Regions, "test")

	srv.defaultVPC = nil
}

// localServerSuite contains tests that run against a fake EC2 server
// running within the test process itself.  These tests can test things that
// would be unreasonably slow or expensive to test on a live Amazon server.
// It starts a new local ec2test server for each test.  The server is
// accessed by using the "test" region, which is changed to point to the
// network address of the local server.
type localServerSuite struct {
	coretesting.BaseSuite
	jujutest.Tests
	srv                localServer
	restoreEC2Patching func()
}

func (t *localServerSuite) SetUpSuite(c *gc.C) {
	t.BaseSuite.SetUpSuite(c)
	t.Credential = cloud.NewCredential(
		cloud.AccessKeyAuthType,
		map[string]string{
			"access-key": "x",
			"secret-key": "x",
		},
	)
	t.CloudRegion = "test"

	// Upload arches that ec2 supports; add to this
	// as ec2 coverage expands.
	t.UploadArches = []string{arch.AMD64, arch.I386}
	t.TestConfig = localConfigAttrs
	t.restoreEC2Patching = patchEC2ForTesting(c)
	imagetesting.PatchOfficialDataSources(&t.BaseSuite.CleanupSuite, "test:")
	t.BaseSuite.PatchValue(&imagemetadata.SimplestreamsImagesPublicKey, sstesting.SignedMetadataPublicKey)
	t.BaseSuite.PatchValue(&juju.JujuPublicKey, sstesting.SignedMetadataPublicKey)
	t.BaseSuite.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	t.BaseSuite.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
	t.BaseSuite.PatchValue(&series.HostSeries, func() string { return series.LatestLts() })
	t.BaseSuite.PatchValue(ec2.DeleteSecurityGroupInsistently, deleteSecurityGroupForTestFunc)
	t.srv.createRootDisks = true
	t.srv.startServer(c)
	// TODO(jam) I don't understand why we shouldn't do this.
	// t.Tests embeds the sstesting.TestDataSuite, but if we call this
	// SetUpSuite, then all of the tests fail because they go to access
	// "test:/streams/..." and it isn't found
	// t.Tests.SetUpSuite(c)
}

func (t *localServerSuite) TearDownSuite(c *gc.C) {
	t.restoreEC2Patching()
	t.Tests.TearDownSuite(c)
	t.BaseSuite.TearDownSuite(c)
}

func (t *localServerSuite) SetUpTest(c *gc.C) {
	t.BaseSuite.SetUpTest(c)
	t.SetFeatureFlags(feature.AddressAllocation)
	t.srv.startServer(c)
	t.Tests.SetUpTest(c)
}

func (t *localServerSuite) TearDownTest(c *gc.C) {
	t.Tests.TearDownTest(c)
	t.srv.stopServer(c)
	t.BaseSuite.TearDownTest(c)
}

func (t *localServerSuite) prepareEnviron(c *gc.C) environs.NetworkingEnviron {
	env := t.Prepare(c)
	netenv, supported := environs.SupportsNetworking(env)
	c.Assert(supported, jc.IsTrue)
	return netenv
}

func (t *localServerSuite) TestPrepareForBootstrapWithInvalidVPCID(c *gc.C) {
	badVPCIDConfig := coretesting.Attrs{"vpc-id": "bad"}

	expectedError := `invalid EC2 provider config: vpc-id: "bad" is not a valid AWS VPC ID`
	t.AssertPrepareFailsWithConfig(c, badVPCIDConfig, expectedError)
}

func (t *localServerSuite) TestPrepareForBootstrapWithUnknownVPCID(c *gc.C) {
	unknownVPCIDConfig := coretesting.Attrs{"vpc-id": "vpc-unknown"}

	expectedError := `Juju cannot use the given vpc-id for bootstrapping(.|\n)*Error details: VPC "vpc-unknown" not found`
	err := t.AssertPrepareFailsWithConfig(c, unknownVPCIDConfig, expectedError)
	c.Check(err, jc.Satisfies, ec2.IsVPCNotUsableError)
}

func (t *localServerSuite) TestPrepareForBootstrapWithNotRecommendedVPCID(c *gc.C) {
	t.makeTestingDefaultVPCUnavailable(c)
	notRecommendedVPCIDConfig := coretesting.Attrs{"vpc-id": t.srv.defaultVPC.Id}

	expectedError := `The given vpc-id does not meet one or more(.|\n)*Error details: VPC has unexpected state "unavailable"`
	err := t.AssertPrepareFailsWithConfig(c, notRecommendedVPCIDConfig, expectedError)
	c.Check(err, jc.Satisfies, ec2.IsVPCNotRecommendedError)
}

func (t *localServerSuite) makeTestingDefaultVPCUnavailable(c *gc.C) {
	// For simplicity, here the test server's default VPC is updated to change
	// its state to unavailable, we just verify the behavior of a "not
	// recommended VPC".
	t.srv.defaultVPC.State = "unavailable"
	err := t.srv.ec2srv.UpdateVPC(*t.srv.defaultVPC)
	c.Assert(err, jc.ErrorIsNil)
}

func (t *localServerSuite) TestPrepareForBootstrapWithNotRecommendedButForcedVPCID(c *gc.C) {
	t.makeTestingDefaultVPCUnavailable(c)
	params := t.PrepareParams(c)
	params.BaseConfig["vpc-id"] = t.srv.defaultVPC.Id
	params.BaseConfig["vpc-id-force"] = true

	t.prepareWithParamsAndBootstrapWithVPCID(c, params, t.srv.defaultVPC.Id)
}

func (t *localServerSuite) TestPrepareForBootstrapWithEmptyVPCID(c *gc.C) {
	const emptyVPCID = ""

	params := t.PrepareParams(c)
	params.BaseConfig["vpc-id"] = emptyVPCID

	t.prepareWithParamsAndBootstrapWithVPCID(c, params, emptyVPCID)
}

func (t *localServerSuite) prepareWithParamsAndBootstrapWithVPCID(c *gc.C, params environs.PrepareParams, expectedVPCID string) {
	env := t.PrepareWithParams(c, params)
	unknownAttrs := env.Config().UnknownAttrs()
	vpcID, ok := unknownAttrs["vpc-id"]
	c.Check(vpcID, gc.Equals, expectedVPCID)
	c.Check(ok, jc.IsTrue)

	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)
}

func (t *localServerSuite) TestPrepareForBootstrapWithVPCIDNone(c *gc.C) {
	params := t.PrepareParams(c)
	params.BaseConfig["vpc-id"] = "none"

	t.prepareWithParamsAndBootstrapWithVPCID(c, params, ec2.VPCIDNone)
}

func (t *localServerSuite) TestPrepareForBootstrapWithDefaultVPCID(c *gc.C) {
	params := t.PrepareParams(c)
	params.BaseConfig["vpc-id"] = t.srv.defaultVPC.Id

	t.prepareWithParamsAndBootstrapWithVPCID(c, params, t.srv.defaultVPC.Id)
}

func (t *localServerSuite) TestSystemdBootstrapInstanceUserDataAndState(c *gc.C) {
	env := t.Prepare(c)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{
		// TODO(redir): BBB: When we no longer support upstart based systems this can change to series.LatestLts()
		BootstrapSeries: "xenial",
	})
	c.Assert(err, jc.ErrorIsNil)

	// check that ControllerInstances returns the id of the bootstrap machine.
	instanceIds, err := env.ControllerInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instanceIds, gc.HasLen, 1)

	insts, err := env.AllInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(insts, gc.HasLen, 1)
	c.Check(insts[0].Id(), gc.Equals, instanceIds[0])

	// check that the user data is configured to and the machine and
	// provisioning agents.  check that the user data is configured to only
	// configure authorized SSH keys and set the log output; everything else
	// happens after the machine is brought up.
	inst := t.srv.ec2srv.Instance(string(insts[0].Id()))
	c.Assert(inst, gc.NotNil)
	addresses, err := insts[0].Addresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addresses, gc.Not(gc.HasLen), 0)
	userData, err := utils.Gunzip(inst.UserData)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(userData), jc.YAMLEquals, map[interface{}]interface{}{
		"output": map[interface{}]interface{}{
			"all": "| tee -a /var/log/cloud-init-output.log",
		},
		"users": []interface{}{
			map[interface{}]interface{}{
				"name":        "ubuntu",
				"lock_passwd": true,
				"groups": []interface{}{"adm", "audio",
					"cdrom", "dialout", "dip", "floppy",
					"netdev", "plugdev", "sudo", "video"},
				"shell":               "/bin/bash",
				"sudo":                []interface{}{"ALL=(ALL) NOPASSWD:ALL"},
				"ssh-authorized-keys": splitAuthKeys(env.Config().AuthorizedKeys()),
			},
		},
		"runcmd": []interface{}{
			"set -xe",
			"install -D -m 644 /dev/null '/etc/systemd/system/juju-clean-shutdown.service'",
			"printf '%s\\n' '\n[Unit]\nDescription=Stop all network interfaces on shutdown\nDefaultDependencies=false\nAfter=final.target\n\n[Service]\nType=oneshot\nExecStart=/sbin/ifdown -a -v --force\nStandardOutput=tty\nStandardError=tty\n\n[Install]\nWantedBy=final.target\n' > '/etc/systemd/system/juju-clean-shutdown.service'", "/bin/systemctl enable '/etc/systemd/system/juju-clean-shutdown.service'",
			"install -D -m 644 /dev/null '/var/lib/juju/nonce.txt'",
			"printf '%s\\n' 'user-admin:bootstrap' > '/var/lib/juju/nonce.txt'",
		},
	})

	// check that a new instance will be started with a machine agent
	inst1, hc := testing.AssertStartInstance(c, env, "1")
	c.Check(*hc.Arch, gc.Equals, "amd64")
	c.Check(*hc.Mem, gc.Equals, uint64(3840))
	c.Check(*hc.CpuCores, gc.Equals, uint64(1))
	c.Assert(*hc.CpuPower, gc.Equals, uint64(300))
	inst = t.srv.ec2srv.Instance(string(inst1.Id()))
	c.Assert(inst, gc.NotNil)
	userData, err = utils.Gunzip(inst.UserData)
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("second instance: UserData: %q", userData)
	var userDataMap map[interface{}]interface{}
	err = goyaml.Unmarshal(userData, &userDataMap)
	c.Assert(err, jc.ErrorIsNil)
	CheckPackage(c, userDataMap, "curl", true)
	CheckPackage(c, userDataMap, "mongodb-server", false)
	CheckScripts(c, userDataMap, "jujud bootstrap-state", false)
	CheckScripts(c, userDataMap, "/var/lib/juju/agents/machine-1/agent.conf", true)
	// TODO check for provisioning agent

	err = env.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	_, err = env.ControllerInstances()
	c.Assert(err, gc.Equals, environs.ErrNotBootstrapped)
}

// TestUpstartBoostrapInstanceUserDataAndState is a test for legacy systems
// using upstart which will be around until trusty is no longer supported.
// TODO(redir): BBB: remove when trusty is no longer supported
func (t *localServerSuite) TestUpstartBootstrapInstanceUserDataAndState(c *gc.C) {
	env := t.Prepare(c)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{
		BootstrapSeries: "trusty",
	})
	c.Assert(err, jc.ErrorIsNil)

	// check that ControllerInstances returns the id of the bootstrap machine.
	instanceIds, err := env.ControllerInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instanceIds, gc.HasLen, 1)

	insts, err := env.AllInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(insts, gc.HasLen, 1)
	c.Check(insts[0].Id(), gc.Equals, instanceIds[0])

	// check that the user data is configured to and the machine and
	// provisioning agents.  check that the user data is configured to only
	// configure authorized SSH keys and set the log output; everything else
	// happens after the machine is brought up.
	inst := t.srv.ec2srv.Instance(string(insts[0].Id()))
	c.Assert(inst, gc.NotNil)
	addresses, err := insts[0].Addresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addresses, gc.Not(gc.HasLen), 0)
	userData, err := utils.Gunzip(inst.UserData)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(userData), jc.YAMLEquals, map[interface{}]interface{}{
		"output": map[interface{}]interface{}{
			"all": "| tee -a /var/log/cloud-init-output.log",
		},
		"users": []interface{}{
			map[interface{}]interface{}{
				"name":        "ubuntu",
				"lock_passwd": true,
				"groups": []interface{}{"adm", "audio",
					"cdrom", "dialout", "dip", "floppy",
					"netdev", "plugdev", "sudo", "video"},
				"shell":               "/bin/bash",
				"sudo":                []interface{}{"ALL=(ALL) NOPASSWD:ALL"},
				"ssh-authorized-keys": splitAuthKeys(env.Config().AuthorizedKeys()),
			},
		},
		"runcmd": []interface{}{
			"set -xe",
			"install -D -m 644 /dev/null '/etc/init/juju-clean-shutdown.conf'",
			"printf '%s\\n' '\nauthor \"Juju Team <juju@lists.ubuntu.com>\"\ndescription \"Stop all network interfaces on shutdown\"\nstart on runlevel [016]\ntask\nconsole output\n\nexec /sbin/ifdown -a -v --force\n' > '/etc/init/juju-clean-shutdown.conf'",
			"install -D -m 644 /dev/null '/var/lib/juju/nonce.txt'",
			"printf '%s\\n' 'user-admin:bootstrap' > '/var/lib/juju/nonce.txt'",
		},
	})

	// check that a new instance will be started with a machine agent
	inst1, hc := testing.AssertStartInstance(c, env, "1")
	c.Check(*hc.Arch, gc.Equals, "amd64")
	c.Check(*hc.Mem, gc.Equals, uint64(3840))
	c.Check(*hc.CpuCores, gc.Equals, uint64(1))
	c.Assert(*hc.CpuPower, gc.Equals, uint64(300))
	inst = t.srv.ec2srv.Instance(string(inst1.Id()))
	c.Assert(inst, gc.NotNil)
	userData, err = utils.Gunzip(inst.UserData)
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("second instance: UserData: %q", userData)
	var userDataMap map[interface{}]interface{}
	err = goyaml.Unmarshal(userData, &userDataMap)
	c.Assert(err, jc.ErrorIsNil)
	CheckPackage(c, userDataMap, "curl", true)
	CheckPackage(c, userDataMap, "mongodb-server", false)
	CheckScripts(c, userDataMap, "jujud bootstrap-state", false)
	CheckScripts(c, userDataMap, "/var/lib/juju/agents/machine-1/agent.conf", true)
	// TODO check for provisioning agent

	err = env.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	_, err = env.ControllerInstances()
	c.Assert(err, gc.Equals, environs.ErrNotBootstrapped)
}

func (t *localServerSuite) TestTerminateInstancesIgnoresNotFound(c *gc.C) {
	env := t.Prepare(c)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)

	t.BaseSuite.PatchValue(ec2.DeleteSecurityGroupInsistently, deleteSecurityGroupForTestFunc)
	insts, err := env.AllInstances()
	c.Assert(err, jc.ErrorIsNil)
	idsToStop := make([]instance.Id, len(insts)+1)
	for i, one := range insts {
		idsToStop[i] = one.Id()
	}
	idsToStop[len(insts)] = instance.Id("i-am-not-found")

	err = env.StopInstances(idsToStop...)
	// NotFound should be ignored
	c.Assert(err, jc.ErrorIsNil)
}

func (t *localServerSuite) TestDestroyErr(c *gc.C) {
	env := t.Prepare(c)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)

	msg := "terminate instances error"
	t.BaseSuite.PatchValue(ec2.TerminateInstancesById, func(ec2inst *amzec2.EC2, ids ...instance.Id) (*amzec2.TerminateInstancesResp, error) {
		return nil, errors.New(msg)
	})

	err = env.Destroy()
	c.Assert(errors.Cause(err).Error(), jc.Contains, msg)
}

func (t *localServerSuite) TestGetTerminatedInstances(c *gc.C) {
	env := t.Prepare(c)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)

	// create another instance to terminate
	inst1, _ := testing.AssertStartInstance(c, env, "1")
	inst := t.srv.ec2srv.Instance(string(inst1.Id()))
	c.Assert(inst, gc.NotNil)
	t.BaseSuite.PatchValue(ec2.TerminateInstancesById, func(ec2inst *amzec2.EC2, ids ...instance.Id) (*amzec2.TerminateInstancesResp, error) {
		// Terminate the one destined for termination and
		// err out to ensure that one instance will be terminated, the other - not.
		_, err = ec2inst.TerminateInstances([]string{string(inst1.Id())})
		c.Assert(err, jc.ErrorIsNil)
		return nil, errors.New("terminate instances error")
	})
	err = env.Destroy()
	c.Assert(err, gc.NotNil)

	terminated, err := ec2.TerminatedInstances(env)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(terminated, gc.HasLen, 1)
	c.Assert(terminated[0].Id(), jc.DeepEquals, inst1.Id())
}

func (t *localServerSuite) TestInstanceSecurityGroupsWitheInstanceStatusFilter(c *gc.C) {
	env := t.Prepare(c)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)

	insts, err := env.AllInstances()
	c.Assert(err, jc.ErrorIsNil)
	ids := make([]instance.Id, len(insts))
	for i, one := range insts {
		ids[i] = one.Id()
	}

	groupsNoInstanceFilter, err := ec2.InstanceSecurityGroups(env, ids)
	c.Assert(err, jc.ErrorIsNil)
	// get all security groups for test instances
	c.Assert(groupsNoInstanceFilter, gc.HasLen, 2)

	groupsFilteredForTerminatedInstances, err := ec2.InstanceSecurityGroups(env, ids, "shutting-down", "terminated")
	c.Assert(err, jc.ErrorIsNil)
	// get all security groups for terminated test instances
	c.Assert(groupsFilteredForTerminatedInstances, gc.HasLen, 0)
}

func (t *localServerSuite) TestDestroyControllerModelDeleteSecurityGroupInsistentlyError(c *gc.C) {
	env := t.Prepare(c)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)
	t.testDestroyModelDeleteSecurityGroupInsistentlyError(
		c, env, "destroying managed environs: cannot delete security group .*: ",
	)
}

func (t *localServerSuite) TestDestroyHostedModelDeleteSecurityGroupInsistentlyError(c *gc.C) {
	env := t.Prepare(c)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})

	cfg, err := env.Config().Apply(map[string]interface{}{"controller-uuid": "7e386e08-cba7-44a4-a76e-7c1633584210"})
	c.Assert(err, jc.ErrorIsNil)
	env, err = environs.New(cfg)
	c.Assert(err, jc.ErrorIsNil)
	t.testDestroyModelDeleteSecurityGroupInsistentlyError(
		c, env, "cannot delete environment security groups: cannot delete default security group: ",
	)
}

func (t *localServerSuite) testDestroyModelDeleteSecurityGroupInsistentlyError(c *gc.C, env environs.Environ, errPrefix string) {
	msg := "destroy security group error"
	t.BaseSuite.PatchValue(ec2.DeleteSecurityGroupInsistently, func(
		ec2.SecurityGroupCleaner, amzec2.SecurityGroup, clock.Clock,
	) error {
		return errors.New(msg)
	})
	err := env.Destroy()
	c.Assert(err, gc.ErrorMatches, errPrefix+"destroy security group error")
}

func (t *localServerSuite) TestDestroyControllerDestroysHostedModelResources(c *gc.C) {
	controllerEnv := t.Prepare(c)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), controllerEnv, bootstrap.BootstrapParams{})

	// Create a hosted model environment with an instance and a volume.
	t.srv.ec2srv.SetInitialInstanceState(ec2test.Running)
	cfg, err := controllerEnv.Config().Apply(map[string]interface{}{
		"uuid":          "7e386e08-cba7-44a4-a76e-7c1633584210",
		"firewall-mode": "global",
	})
	c.Assert(err, jc.ErrorIsNil)
	env, err := environs.New(cfg)
	c.Assert(err, jc.ErrorIsNil)
	inst, _ := testing.AssertStartInstance(c, env, "0")
	c.Assert(err, jc.ErrorIsNil)
	ebsProvider := ec2.EBSProvider()
	vs, err := ebsProvider.VolumeSource(env.Config(), nil)
	c.Assert(err, jc.ErrorIsNil)
	volumeResults, err := vs.CreateVolumes([]storage.VolumeParams{{
		Tag:      names.NewVolumeTag("0"),
		Size:     1024,
		Provider: ec2.EBS_ProviderType,
		ResourceTags: map[string]string{
			tags.JujuController: coretesting.ModelTag.Id(),
			tags.JujuModel:      "7e386e08-cba7-44a4-a76e-7c1633584210",
		},
		Attachment: &storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				InstanceId: inst.Id(),
			},
		},
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumeResults, gc.HasLen, 1)
	c.Assert(volumeResults[0].Error, jc.ErrorIsNil)

	assertInstances := func(expect ...instance.Id) {
		insts, err := env.AllInstances()
		c.Assert(err, jc.ErrorIsNil)
		ids := make([]instance.Id, len(insts))
		for i, inst := range insts {
			ids[i] = inst.Id()
		}
		c.Assert(ids, jc.SameContents, expect)
	}
	assertVolumes := func(expect ...string) {
		volIds, err := vs.ListVolumes()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(volIds, jc.SameContents, expect)
	}
	assertGroups := func(expect ...string) {
		groupsResp, err := t.srv.client.SecurityGroups(nil, nil)
		c.Assert(err, jc.ErrorIsNil)
		names := make([]string, len(groupsResp.Groups))
		for i, group := range groupsResp.Groups {
			names[i] = group.Name
		}
		c.Assert(names, jc.SameContents, expect)
	}

	assertInstances(inst.Id())
	assertVolumes(volumeResults[0].Volume.VolumeId)
	assertGroups(
		"default",
		"juju-"+coretesting.ModelTag.Id(),
		"juju-"+coretesting.ModelTag.Id()+"-0",
		"juju-7e386e08-cba7-44a4-a76e-7c1633584210",
		"juju-7e386e08-cba7-44a4-a76e-7c1633584210-global",
	)

	// Destroy the controller environment. This should destroy the hosted
	// environment too.
	err = controllerEnv.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	assertInstances()
	assertVolumes()
	assertGroups("default")
}

// splitAuthKeys splits the given authorized keys
// into the form expected to be found in the
// user data.
func splitAuthKeys(keys string) []interface{} {
	slines := strings.FieldsFunc(keys, func(r rune) bool {
		return r == '\n'
	})
	var lines []interface{}
	for _, line := range slines {
		lines = append(lines, ssh.EnsureJujuComment(strings.TrimSpace(line)))
	}
	return lines
}

func (t *localServerSuite) TestInstanceStatus(c *gc.C) {
	env := t.Prepare(c)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)
	t.srv.ec2srv.SetInitialInstanceState(ec2test.Terminated)
	inst, _ := testing.AssertStartInstance(c, env, "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(inst.Status().Message, gc.Equals, "terminated")
}

func (t *localServerSuite) TestStartInstanceHardwareCharacteristics(c *gc.C) {
	env := t.Prepare(c)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)
	_, hc := testing.AssertStartInstance(c, env, "1")
	c.Check(*hc.Arch, gc.Equals, "amd64")
	c.Check(*hc.Mem, gc.Equals, uint64(3840))
	c.Check(*hc.CpuCores, gc.Equals, uint64(1))
	c.Assert(*hc.CpuPower, gc.Equals, uint64(300))
}

func (t *localServerSuite) TestStartInstanceAvailZone(c *gc.C) {
	inst, err := t.testStartInstanceAvailZone(c, "test-available")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ec2.InstanceEC2(inst).AvailZone, gc.Equals, "test-available")
}

func (t *localServerSuite) TestStartInstanceAvailZoneImpaired(c *gc.C) {
	_, err := t.testStartInstanceAvailZone(c, "test-impaired")
	c.Assert(err, gc.ErrorMatches, `availability zone "test-impaired" is impaired`)
}

func (t *localServerSuite) TestStartInstanceAvailZoneUnknown(c *gc.C) {
	_, err := t.testStartInstanceAvailZone(c, "test-unknown")
	c.Assert(err, gc.ErrorMatches, `invalid availability zone "test-unknown"`)
}

func (t *localServerSuite) testStartInstanceAvailZone(c *gc.C, zone string) (instance.Instance, error) {
	env := t.Prepare(c)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)

	params := environs.StartInstanceParams{Placement: "zone=" + zone}
	result, err := testing.StartInstanceWithParams(env, "1", params)
	if err != nil {
		return nil, err
	}
	return result.Instance, nil
}

func (t *localServerSuite) TestGetAvailabilityZones(c *gc.C) {
	var resultZones []amzec2.AvailabilityZoneInfo
	var resultErr error
	t.PatchValue(ec2.EC2AvailabilityZones, func(e *amzec2.EC2, f *amzec2.Filter) (*amzec2.AvailabilityZonesResp, error) {
		resp := &amzec2.AvailabilityZonesResp{
			Zones: append([]amzec2.AvailabilityZoneInfo{}, resultZones...),
		}
		return resp, resultErr
	})
	env := t.Prepare(c).(common.ZonedEnviron)

	resultErr = fmt.Errorf("failed to get availability zones")
	zones, err := env.AvailabilityZones()
	c.Assert(err, gc.Equals, resultErr)
	c.Assert(zones, gc.IsNil)

	resultErr = nil
	resultZones = make([]amzec2.AvailabilityZoneInfo, 1)
	resultZones[0].Name = "whatever"
	zones, err = env.AvailabilityZones()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zones, gc.HasLen, 1)
	c.Assert(zones[0].Name(), gc.Equals, "whatever")

	// A successful result is cached, currently for the lifetime
	// of the Environ. This will change if/when we have long-lived
	// Environs to cut down repeated IaaS requests.
	resultErr = fmt.Errorf("failed to get availability zones")
	resultZones[0].Name = "andever"
	zones, err = env.AvailabilityZones()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zones, gc.HasLen, 1)
	c.Assert(zones[0].Name(), gc.Equals, "whatever")
}

func (t *localServerSuite) TestGetAvailabilityZonesCommon(c *gc.C) {
	var resultZones []amzec2.AvailabilityZoneInfo
	t.PatchValue(ec2.EC2AvailabilityZones, func(e *amzec2.EC2, f *amzec2.Filter) (*amzec2.AvailabilityZonesResp, error) {
		resp := &amzec2.AvailabilityZonesResp{
			Zones: append([]amzec2.AvailabilityZoneInfo{}, resultZones...),
		}
		return resp, nil
	})
	env := t.Prepare(c).(common.ZonedEnviron)
	resultZones = make([]amzec2.AvailabilityZoneInfo, 2)
	resultZones[0].Name = "az1"
	resultZones[1].Name = "az2"
	resultZones[0].State = "available"
	resultZones[1].State = "impaired"
	zones, err := env.AvailabilityZones()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zones, gc.HasLen, 2)
	c.Assert(zones[0].Name(), gc.Equals, resultZones[0].Name)
	c.Assert(zones[1].Name(), gc.Equals, resultZones[1].Name)
	c.Assert(zones[0].Available(), jc.IsTrue)
	c.Assert(zones[1].Available(), jc.IsFalse)
}

type mockAvailabilityZoneAllocations struct {
	group  []instance.Id // input param
	result []common.AvailabilityZoneInstances
	err    error
}

func (t *mockAvailabilityZoneAllocations) AvailabilityZoneAllocations(
	e common.ZonedEnviron, group []instance.Id,
) ([]common.AvailabilityZoneInstances, error) {
	t.group = group
	return t.result, t.err
}

func (t *localServerSuite) TestStartInstanceDistributionParams(c *gc.C) {
	env := t.Prepare(c)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)

	mock := mockAvailabilityZoneAllocations{
		result: []common.AvailabilityZoneInstances{{ZoneName: "az1"}},
	}
	t.PatchValue(ec2.AvailabilityZoneAllocations, mock.AvailabilityZoneAllocations)

	// no distribution group specified
	testing.AssertStartInstance(c, env, "1")
	c.Assert(mock.group, gc.HasLen, 0)

	// distribution group specified: ensure it's passed through to AvailabilityZone.
	expectedInstances := []instance.Id{"i-0", "i-1"}
	params := environs.StartInstanceParams{
		DistributionGroup: func() ([]instance.Id, error) {
			return expectedInstances, nil
		},
	}
	_, err = testing.StartInstanceWithParams(env, "1", params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mock.group, gc.DeepEquals, expectedInstances)
}

func (t *localServerSuite) TestStartInstanceDistributionErrors(c *gc.C) {
	env := t.Prepare(c)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)

	mock := mockAvailabilityZoneAllocations{
		err: fmt.Errorf("AvailabilityZoneAllocations failed"),
	}
	t.PatchValue(ec2.AvailabilityZoneAllocations, mock.AvailabilityZoneAllocations)
	_, _, _, err = testing.StartInstance(env, "1")
	c.Assert(errors.Cause(err), gc.Equals, mock.err)

	mock.err = nil
	dgErr := fmt.Errorf("DistributionGroup failed")
	params := environs.StartInstanceParams{
		DistributionGroup: func() ([]instance.Id, error) {
			return nil, dgErr
		},
	}
	_, err = testing.StartInstanceWithParams(env, "1", params)
	c.Assert(errors.Cause(err), gc.Equals, dgErr)
}

func (t *localServerSuite) TestStartInstanceDistribution(c *gc.C) {
	env := t.Prepare(c)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)

	// test-available is the only available AZ, so AvailabilityZoneAllocations
	// is guaranteed to return that.
	inst, _ := testing.AssertStartInstance(c, env, "1")
	c.Assert(ec2.InstanceEC2(inst).AvailZone, gc.Equals, "test-available")
}

var azConstrainedErr = &amzec2.Error{
	Code:    "Unsupported",
	Message: "The requested Availability Zone is currently constrained etc.",
}

var azVolumeTypeNotAvailableInZoneErr = &amzec2.Error{
	Code:    "VolumeTypeNotAvailableInZone",
	Message: "blah blah",
}

var azInsufficientInstanceCapacityErr = &amzec2.Error{
	Code: "InsufficientInstanceCapacity",
	Message: "We currently do not have sufficient m1.small capacity in the " +
		"Availability Zone you requested (us-east-1d). Our system will " +
		"be working on provisioning additional capacity. You can currently get m1.small " +
		"capacity by not specifying an Availability Zone in your request or choosing " +
		"us-east-1c, us-east-1a.",
}

var azNoDefaultSubnetErr = &amzec2.Error{
	Code:    "InvalidInput",
	Message: "No default subnet for availability zone: ''us-east-1e''.",
}

func (t *localServerSuite) TestStartInstanceAvailZoneAllConstrained(c *gc.C) {
	t.testStartInstanceAvailZoneAllConstrained(c, azConstrainedErr)
}

func (t *localServerSuite) TestStartInstanceVolumeTypeNotAvailable(c *gc.C) {
	t.testStartInstanceAvailZoneAllConstrained(c, azVolumeTypeNotAvailableInZoneErr)
}

func (t *localServerSuite) TestStartInstanceAvailZoneAllInsufficientInstanceCapacity(c *gc.C) {
	t.testStartInstanceAvailZoneAllConstrained(c, azInsufficientInstanceCapacityErr)
}

func (t *localServerSuite) TestStartInstanceAvailZoneAllNoDefaultSubnet(c *gc.C) {
	t.testStartInstanceAvailZoneAllConstrained(c, azNoDefaultSubnetErr)
}

func (t *localServerSuite) testStartInstanceAvailZoneAllConstrained(c *gc.C, runInstancesError *amzec2.Error) {
	env := t.Prepare(c)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)

	mock := mockAvailabilityZoneAllocations{
		result: []common.AvailabilityZoneInstances{
			{ZoneName: "az1"}, {ZoneName: "az2"},
		},
	}
	t.PatchValue(ec2.AvailabilityZoneAllocations, mock.AvailabilityZoneAllocations)

	var azArgs []string
	t.PatchValue(ec2.RunInstances, func(e *amzec2.EC2, ri *amzec2.RunInstances) (*amzec2.RunInstancesResp, error) {
		azArgs = append(azArgs, ri.AvailZone)
		return nil, runInstancesError
	})
	_, _, _, err = testing.StartInstance(env, "1")
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(
		"cannot run instances: %s \\(%s\\)",
		regexp.QuoteMeta(runInstancesError.Message),
		runInstancesError.Code,
	))
	c.Assert(azArgs, gc.DeepEquals, []string{"az1", "az2"})
}

// addTestingSubnets adds a testing default VPC with 3 subnets in the EC2 test
// server: 2 of the subnets are in the "test-available" AZ, the remaining - in
// "test-unavailable". Returns a slice with the IDs of the created subnets.
func (t *localServerSuite) addTestingSubnets(c *gc.C) []network.Id {
	vpc := t.srv.ec2srv.AddVPC(amzec2.VPC{
		CIDRBlock: "0.1.0.0/16",
		IsDefault: true,
	})
	results := make([]network.Id, 3)
	sub1, err := t.srv.ec2srv.AddSubnet(amzec2.Subnet{
		VPCId:        vpc.Id,
		CIDRBlock:    "0.1.2.0/24",
		AvailZone:    "test-available",
		DefaultForAZ: true,
	})
	c.Assert(err, jc.ErrorIsNil)
	results[0] = network.Id(sub1.Id)
	sub2, err := t.srv.ec2srv.AddSubnet(amzec2.Subnet{
		VPCId:     vpc.Id,
		CIDRBlock: "0.1.3.0/24",
		AvailZone: "test-available",
	})
	c.Assert(err, jc.ErrorIsNil)
	results[1] = network.Id(sub2.Id)
	sub3, err := t.srv.ec2srv.AddSubnet(amzec2.Subnet{
		VPCId:        vpc.Id,
		CIDRBlock:    "0.1.4.0/24",
		AvailZone:    "test-unavailable",
		DefaultForAZ: true,
	})
	c.Assert(err, jc.ErrorIsNil)
	results[2] = network.Id(sub3.Id)
	return results
}

func (t *localServerSuite) prepareAndBootstrap(c *gc.C) environs.Environ {
	env := t.Prepare(c)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)
	return env
}

func (t *localServerSuite) TestSpaceConstraintsSpaceNotInPlacementZone(c *gc.C) {
	c.Skip("temporarily disabled")
	env := t.prepareAndBootstrap(c)
	subIDs := t.addTestingSubnets(c)

	// Expect an error because zone test-available isn't in SubnetsToZones
	params := environs.StartInstanceParams{
		Placement:   "zone=test-available",
		Constraints: constraints.MustParse("spaces=aaaaaaaaaa"),
		SubnetsToZones: map[network.Id][]string{
			subIDs[0]: []string{"zone2"},
			subIDs[1]: []string{"zone3"},
			subIDs[2]: []string{"zone4"},
		},
	}
	_, err := testing.StartInstanceWithParams(env, "1", params)
	c.Assert(err, gc.ErrorMatches, `unable to resolve constraints: space and/or subnet unavailable in zones \[test-available\]`)
}

func (t *localServerSuite) TestSpaceConstraintsSpaceInPlacementZone(c *gc.C) {
	env := t.prepareAndBootstrap(c)
	subIDs := t.addTestingSubnets(c)

	// Should work - test-available is in SubnetsToZones and in myspace.
	params := environs.StartInstanceParams{
		Placement:   "zone=test-available",
		Constraints: constraints.MustParse("spaces=aaaaaaaaaa"),
		SubnetsToZones: map[network.Id][]string{
			subIDs[0]: []string{"test-available"},
			subIDs[1]: []string{"zone3"},
		},
	}
	_, err := testing.StartInstanceWithParams(env, "1", params)
	c.Assert(err, jc.ErrorIsNil)
}

func (t *localServerSuite) TestSpaceConstraintsNoPlacement(c *gc.C) {
	env := t.prepareAndBootstrap(c)
	subIDs := t.addTestingSubnets(c)

	// Shoule work because zone is not specified so we can resolve the constraints
	params := environs.StartInstanceParams{
		Constraints: constraints.MustParse("spaces=aaaaaaaaaa"),
		SubnetsToZones: map[network.Id][]string{
			subIDs[0]: []string{"test-available"},
			subIDs[1]: []string{"zone3"},
		},
	}
	_, err := testing.StartInstanceWithParams(env, "1", params)
	c.Assert(err, jc.ErrorIsNil)
}

func (t *localServerSuite) TestSpaceConstraintsNoAvailableSubnets(c *gc.C) {
	c.Skip("temporarily disabled")

	env := t.prepareAndBootstrap(c)
	subIDs := t.addTestingSubnets(c)

	// We requested a space, but there are no subnets in SubnetsToZones, so we can't resolve
	// the constraints
	params := environs.StartInstanceParams{
		Constraints: constraints.MustParse("spaces=aaaaaaaaaa"),
		SubnetsToZones: map[network.Id][]string{
			subIDs[0]: []string{""},
		},
	}
	_, err := testing.StartInstanceWithParams(env, "1", params)
	c.Assert(err, gc.ErrorMatches, `unable to resolve constraints: space and/or subnet unavailable in zones \[test-available\]`)
}

func (t *localServerSuite) TestStartInstanceAvailZoneOneConstrained(c *gc.C) {
	t.testStartInstanceAvailZoneOneConstrained(c, azConstrainedErr)
}

func (t *localServerSuite) TestStartInstanceAvailZoneOneInsufficientInstanceCapacity(c *gc.C) {
	t.testStartInstanceAvailZoneOneConstrained(c, azInsufficientInstanceCapacityErr)
}

func (t *localServerSuite) TestStartInstanceAvailZoneOneNoDefaultSubnetErr(c *gc.C) {
	t.testStartInstanceAvailZoneOneConstrained(c, azNoDefaultSubnetErr)
}

func (t *localServerSuite) testStartInstanceAvailZoneOneConstrained(c *gc.C, runInstancesError *amzec2.Error) {
	env := t.Prepare(c)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)

	mock := mockAvailabilityZoneAllocations{
		result: []common.AvailabilityZoneInstances{
			{ZoneName: "az1"}, {ZoneName: "az2"},
		},
	}
	t.PatchValue(ec2.AvailabilityZoneAllocations, mock.AvailabilityZoneAllocations)

	// The first call to RunInstances fails with an error indicating the AZ
	// is constrained. The second attempt succeeds, and so allocates to az2.
	var azArgs []string
	realRunInstances := *ec2.RunInstances
	t.PatchValue(ec2.RunInstances, func(e *amzec2.EC2, ri *amzec2.RunInstances) (*amzec2.RunInstancesResp, error) {
		azArgs = append(azArgs, ri.AvailZone)
		if len(azArgs) == 1 {
			return nil, runInstancesError
		}
		return realRunInstances(e, ri)
	})
	inst, hwc := testing.AssertStartInstance(c, env, "1")
	c.Assert(azArgs, gc.DeepEquals, []string{"az1", "az2"})
	c.Assert(ec2.InstanceEC2(inst).AvailZone, gc.Equals, "az2")
	c.Check(*hwc.AvailabilityZone, gc.Equals, "az2")
}

func (t *localServerSuite) TestAddresses(c *gc.C) {
	env := t.Prepare(c)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)
	inst, _ := testing.AssertStartInstance(c, env, "1")
	c.Assert(err, jc.ErrorIsNil)
	addrs, err := inst.Addresses()
	c.Assert(err, jc.ErrorIsNil)
	// Expected values use Address type but really contain a regexp for
	// the value rather than a valid ip or hostname.
	expected := []network.Address{{
		Value: "8.0.0.*",
		Type:  network.IPv4Address,
		Scope: network.ScopePublic,
	}, {
		Value: "127.0.0.*",
		Type:  network.IPv4Address,
		Scope: network.ScopeCloudLocal,
	}}
	c.Assert(addrs, gc.HasLen, len(expected))
	for i, addr := range addrs {
		c.Check(addr.Value, gc.Matches, expected[i].Value)
		c.Check(addr.Type, gc.Equals, expected[i].Type)
		c.Check(addr.Scope, gc.Equals, expected[i].Scope)
	}
}

func (t *localServerSuite) TestConstraintsValidatorUnsupported(c *gc.C) {
	env := t.Prepare(c)
	validator, err := env.ConstraintsValidator()
	c.Assert(err, jc.ErrorIsNil)
	cons := constraints.MustParse("arch=amd64 tags=foo virt-type=kvm")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unsupported, jc.SameContents, []string{"tags", "virt-type"})
}

func (t *localServerSuite) TestConstraintsValidatorVocab(c *gc.C) {
	env := t.Prepare(c)
	validator, err := env.ConstraintsValidator()
	c.Assert(err, jc.ErrorIsNil)
	cons := constraints.MustParse("arch=ppc64el")
	_, err = validator.Validate(cons)
	c.Assert(err, gc.ErrorMatches, "invalid constraint value: arch=ppc64el\nvalid values are:.*")
	cons = constraints.MustParse("instance-type=foo")
	_, err = validator.Validate(cons)
	c.Assert(err, gc.ErrorMatches, "invalid constraint value: instance-type=foo\nvalid values are:.*")
}

func (t *localServerSuite) TestConstraintsMerge(c *gc.C) {
	env := t.Prepare(c)
	validator, err := env.ConstraintsValidator()
	c.Assert(err, jc.ErrorIsNil)
	consA := constraints.MustParse("arch=amd64 mem=1G cpu-power=10 cpu-cores=2 tags=bar")
	consB := constraints.MustParse("arch=i386 instance-type=m1.small")
	cons, err := validator.Merge(consA, consB)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, gc.DeepEquals, constraints.MustParse("arch=i386 instance-type=m1.small tags=bar"))
}

func (t *localServerSuite) TestPrecheckInstanceValidInstanceType(c *gc.C) {
	env := t.Prepare(c)
	cons := constraints.MustParse("instance-type=m1.small root-disk=1G")
	placement := ""
	err := env.PrecheckInstance(series.LatestLts(), cons, placement)
	c.Assert(err, jc.ErrorIsNil)
}

func (t *localServerSuite) TestPrecheckInstanceInvalidInstanceType(c *gc.C) {
	env := t.Prepare(c)
	cons := constraints.MustParse("instance-type=m1.invalid")
	placement := ""
	err := env.PrecheckInstance(series.LatestLts(), cons, placement)
	c.Assert(err, gc.ErrorMatches, `invalid AWS instance type "m1.invalid" specified`)
}

func (t *localServerSuite) TestPrecheckInstanceUnsupportedArch(c *gc.C) {
	env := t.Prepare(c)
	cons := constraints.MustParse("instance-type=cc1.4xlarge arch=i386")
	placement := ""
	err := env.PrecheckInstance(series.LatestLts(), cons, placement)
	c.Assert(err, gc.ErrorMatches, `invalid AWS instance type "cc1.4xlarge" and arch "i386" specified`)
}

func (t *localServerSuite) TestPrecheckInstanceAvailZone(c *gc.C) {
	env := t.Prepare(c)
	placement := "zone=test-available"
	err := env.PrecheckInstance(series.LatestLts(), constraints.Value{}, placement)
	c.Assert(err, jc.ErrorIsNil)
}

func (t *localServerSuite) TestPrecheckInstanceAvailZoneUnavailable(c *gc.C) {
	env := t.Prepare(c)
	placement := "zone=test-unavailable"
	err := env.PrecheckInstance(series.LatestLts(), constraints.Value{}, placement)
	c.Assert(err, jc.ErrorIsNil)
}

func (t *localServerSuite) TestPrecheckInstanceAvailZoneUnknown(c *gc.C) {
	env := t.Prepare(c)
	placement := "zone=test-unknown"
	err := env.PrecheckInstance(series.LatestLts(), constraints.Value{}, placement)
	c.Assert(err, gc.ErrorMatches, `invalid availability zone "test-unknown"`)
}

func (t *localServerSuite) TestValidateImageMetadata(c *gc.C) {
	env := t.Prepare(c)
	params, err := env.(simplestreams.MetadataValidator).MetadataLookupParams("test")
	c.Assert(err, jc.ErrorIsNil)
	params.Series = series.LatestLts()
	params.Endpoint = "https://ec2.endpoint.com"
	params.Sources, err = environs.ImageMetadataSources(env)
	c.Assert(err, jc.ErrorIsNil)
	image_ids, _, err := imagemetadata.ValidateImageMetadata(params)
	c.Assert(err, jc.ErrorIsNil)
	sort.Strings(image_ids)
	c.Assert(image_ids, gc.DeepEquals, []string{"ami-00000133", "ami-00000135", "ami-00000139"})
}

func (t *localServerSuite) TestGetToolsMetadataSources(c *gc.C) {
	t.PatchValue(&tools.DefaultBaseURL, "")

	env := t.Prepare(c)
	sources, err := tools.GetMetadataSources(env)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sources, gc.HasLen, 0)
}

func (t *localServerSuite) TestSupportedArchitectures(c *gc.C) {
	env := t.Prepare(c)
	a, err := env.SupportedArchitectures()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(a, jc.SameContents, []string{"amd64", "i386"})
}

func (t *localServerSuite) TestSupportsNetworking(c *gc.C) {
	env := t.Prepare(c)
	_, supported := environs.SupportsNetworking(env)
	c.Assert(supported, jc.IsTrue)
}

func (t *localServerSuite) setUpInstanceWithDefaultVpc(c *gc.C) (environs.NetworkingEnviron, instance.Id) {
	env := t.prepareEnviron(c)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)

	instanceIds, err := env.ControllerInstances()
	c.Assert(err, jc.ErrorIsNil)
	return env, instanceIds[0]
}

func (t *localServerSuite) TestAllocateAddress(c *gc.C) {
	env, instId := t.setUpInstanceWithDefaultVpc(c)

	addr := network.Address{Value: "8.0.0.4"}
	var actualAddr network.Address
	mockAssign := func(ec2Inst *amzec2.EC2, netId string, addr network.Address) error {
		actualAddr = addr
		return nil
	}
	t.PatchValue(&ec2.AssignPrivateIPAddress, mockAssign)

	err := env.AllocateAddress(instId, "", &addr, "foo", "bar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actualAddr, gc.Equals, addr)
}

func (t *localServerSuite) TestAllocateAddressIPAddressInUseOrEmpty(c *gc.C) {
	env, instId := t.setUpInstanceWithDefaultVpc(c)

	addr := network.Address{Value: "8.0.0.4"}
	mockAssign := func(ec2Inst *amzec2.EC2, netId string, addr network.Address) error {
		return &amzec2.Error{Code: "InvalidParameterValue"}
	}
	t.PatchValue(&ec2.AssignPrivateIPAddress, mockAssign)

	err := env.AllocateAddress(instId, "", &addr, "foo", "bar")
	c.Assert(errors.Cause(err), gc.Equals, environs.ErrIPAddressUnavailable)
}

func (t *localServerSuite) TestAllocateAddressNetworkInterfaceFull(c *gc.C) {
	env, instId := t.setUpInstanceWithDefaultVpc(c)

	addr := network.Address{Value: "8.0.0.4"}
	mockAssign := func(ec2Inst *amzec2.EC2, netId string, addr network.Address) error {
		return &amzec2.Error{Code: "PrivateIpAddressLimitExceeded"}
	}
	t.PatchValue(&ec2.AssignPrivateIPAddress, mockAssign)

	err := env.AllocateAddress(instId, "", &addr, "foo", "bar")
	c.Assert(errors.Cause(err), gc.Equals, environs.ErrIPAddressesExhausted)
}

func (t *localServerSuite) TestReleaseAddress(c *gc.C) {
	env, instId := t.setUpInstanceWithDefaultVpc(c)
	addr := network.Address{Value: "8.0.0.4"}
	// Allocate the address first so we can release it
	err := env.AllocateAddress(instId, "", &addr, "foo", "bar")
	c.Assert(err, jc.ErrorIsNil)

	err = env.ReleaseAddress(instId, "", addr, "", "")
	c.Assert(err, jc.ErrorIsNil)

	// Releasing a second time tests that the first call actually released
	// it plus tests the error handling of ReleaseAddress
	err = env.ReleaseAddress(instId, "", addr, "", "")
	msg := fmt.Sprintf(`failed to release address "8\.0\.0\.4" from instance %q.*`, instId)
	c.Assert(err, gc.ErrorMatches, msg)
}

func (t *localServerSuite) TestReleaseAddressUnknownInstance(c *gc.C) {
	env, _ := t.setUpInstanceWithDefaultVpc(c)

	// We should be able to release an address with an unknown instance id
	// without it being allocated.
	addr := network.Address{Value: "8.0.0.4"}
	err := env.ReleaseAddress(instance.UnknownId, "", addr, "", "")
	c.Assert(err, jc.ErrorIsNil)
}

func (t *localServerSuite) TestNetworkInterfaces(c *gc.C) {
	env, instId := t.setUpInstanceWithDefaultVpc(c)
	interfaces, err := env.NetworkInterfaces(instId)
	c.Assert(err, jc.ErrorIsNil)

	// The CIDR isn't predictable, but it is in the 10.10.x.0/24 format
	// The subnet ID is in the form "subnet-x", where x matches the same
	// number from the CIDR. The interfaces address is part of the CIDR.
	// For these reasons we check that the CIDR is in the expected format
	// and derive the expected values for ProviderSubnetId and Address.
	c.Assert(interfaces, gc.HasLen, 1)
	cidr := interfaces[0].CIDR
	re := regexp.MustCompile(`10\.10\.(\d+)\.0/24`)
	c.Assert(re.Match([]byte(cidr)), jc.IsTrue)
	index := re.FindStringSubmatch(cidr)[1]
	addr := fmt.Sprintf("10.10.%s.5", index)
	subnetId := network.Id("subnet-" + index)

	// AvailabilityZones will either contain "test-available",
	// "test-impaired" or "test-unavailable" depending on which subnet is
	// picked. Any of these is fine.
	zones := interfaces[0].AvailabilityZones
	c.Assert(zones, gc.HasLen, 1)
	re = regexp.MustCompile("test-available|test-unavailable|test-impaired")
	c.Assert(re.Match([]byte(zones[0])), jc.IsTrue)

	expectedInterfaces := []network.InterfaceInfo{{
		DeviceIndex:       0,
		MACAddress:        "20:01:60:cb:27:37",
		CIDR:              cidr,
		ProviderId:        "eni-0",
		ProviderSubnetId:  subnetId,
		VLANTag:           0,
		InterfaceName:     "unsupported0",
		Disabled:          false,
		NoAutoStart:       false,
		ConfigType:        network.ConfigDHCP,
		InterfaceType:     network.EthernetInterface,
		Address:           network.NewScopedAddress(addr, network.ScopeCloudLocal),
		AvailabilityZones: zones,
	}}
	c.Assert(interfaces, jc.DeepEquals, expectedInterfaces)
}

func (t *localServerSuite) TestSubnetsWithInstanceId(c *gc.C) {
	env, instId := t.setUpInstanceWithDefaultVpc(c)
	subnets, err := env.Subnets(instId, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnets, gc.HasLen, 1)
	validateSubnets(c, subnets)

	interfaces, err := env.NetworkInterfaces(instId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(interfaces, gc.HasLen, 1)
	c.Assert(interfaces[0].ProviderSubnetId, gc.Equals, subnets[0].ProviderId)
}

func (t *localServerSuite) TestSubnetsWithInstanceIdAndSubnetId(c *gc.C) {
	env, instId := t.setUpInstanceWithDefaultVpc(c)
	interfaces, err := env.NetworkInterfaces(instId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(interfaces, gc.HasLen, 1)

	subnets, err := env.Subnets(instId, []network.Id{interfaces[0].ProviderSubnetId})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnets, gc.HasLen, 1)
	c.Assert(subnets[0].ProviderId, gc.Equals, interfaces[0].ProviderSubnetId)
	validateSubnets(c, subnets)
}

func (t *localServerSuite) TestSubnetsWithInstanceIdMissingSubnet(c *gc.C) {
	env, instId := t.setUpInstanceWithDefaultVpc(c)
	subnets, err := env.Subnets(instId, []network.Id{"missing"})
	c.Assert(err, gc.ErrorMatches, `failed to find the following subnet ids: \[missing\]`)
	c.Assert(subnets, gc.HasLen, 0)
}

func validateSubnets(c *gc.C, subnets []network.SubnetInfo) {
	// These are defined in the test server for the testing default
	// VPC.
	defaultSubnets := []network.SubnetInfo{{
		CIDR:              "10.10.0.0/24",
		ProviderId:        "subnet-0",
		VLANTag:           0,
		AllocatableIPLow:  net.ParseIP("10.10.0.4").To4(),
		AllocatableIPHigh: net.ParseIP("10.10.0.254").To4(),
		AvailabilityZones: []string{"test-available"},
	}, {
		CIDR:              "10.10.1.0/24",
		ProviderId:        "subnet-1",
		VLANTag:           0,
		AllocatableIPLow:  net.ParseIP("10.10.1.4").To4(),
		AllocatableIPHigh: net.ParseIP("10.10.1.254").To4(),
		AvailabilityZones: []string{"test-impaired"},
	}, {
		CIDR:              "10.10.2.0/24",
		ProviderId:        "subnet-2",
		VLANTag:           0,
		AllocatableIPLow:  net.ParseIP("10.10.2.4").To4(),
		AllocatableIPHigh: net.ParseIP("10.10.2.254").To4(),
		AvailabilityZones: []string{"test-unavailable"},
	}}

	re := regexp.MustCompile(`10\.10\.(\d+)\.0/24`)
	for _, subnet := range subnets {
		// We can find the expected data by looking at the CIDR.
		// subnets isn't in a predictable order due to the use of maps.
		c.Assert(re.Match([]byte(subnet.CIDR)), jc.IsTrue)
		index, err := strconv.Atoi(re.FindStringSubmatch(subnet.CIDR)[1])
		c.Assert(err, jc.ErrorIsNil)
		// Don't know which AZ the subnet will end up in.
		defaultSubnets[index].AvailabilityZones = subnet.AvailabilityZones
		c.Assert(subnet, jc.DeepEquals, defaultSubnets[index])
	}
}

func (t *localServerSuite) TestSubnets(c *gc.C) {
	env, _ := t.setUpInstanceWithDefaultVpc(c)

	subnets, err := env.Subnets(instance.UnknownId, []network.Id{"subnet-0"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnets, gc.HasLen, 1)
	validateSubnets(c, subnets)

	subnets, err = env.Subnets(instance.UnknownId, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnets, gc.HasLen, 3)
	validateSubnets(c, subnets)
}

func (t *localServerSuite) TestSubnetsMissingSubnet(c *gc.C) {
	env, _ := t.setUpInstanceWithDefaultVpc(c)

	_, err := env.Subnets("", []network.Id{"subnet-0", "Missing"})
	c.Assert(err, gc.ErrorMatches, `failed to find the following subnet ids: \[Missing\]`)
}

func (t *localServerSuite) TestSupportsAddressAllocationTrue(c *gc.C) {
	env := t.prepareEnviron(c)
	result, err := env.SupportsAddressAllocation("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.IsTrue)
}

func (t *localServerSuite) TestSupportsAddressAllocationWithNoFeatureFlag(c *gc.C) {
	t.SetFeatureFlags() // clear the flags.
	env := t.prepareEnviron(c)
	result, err := env.SupportsAddressAllocation("")
	c.Assert(err, gc.ErrorMatches, "address allocation not supported")
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
	c.Assert(result, jc.IsFalse)
}

func (t *localServerSuite) TestAllocateAddressWithNoFeatureFlag(c *gc.C) {
	t.SetFeatureFlags() // clear the flags.
	env := t.prepareEnviron(c)
	addr := network.NewAddresses("1.2.3.4")[0]
	err := env.AllocateAddress("i-foo", "net1", &addr, "foo", "bar")
	c.Assert(err, gc.ErrorMatches, "address allocation not supported")
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}

func (t *localServerSuite) TestReleaseAddressWithNoFeatureFlag(c *gc.C) {
	t.SetFeatureFlags() // clear the flags.
	env := t.prepareEnviron(c)
	err := env.ReleaseAddress("i-foo", "net1", network.NewAddress("1.2.3.4"), "", "")
	c.Assert(err, gc.ErrorMatches, "address allocation not supported")
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}

func (t *localServerSuite) TestSupportsAddressAllocationCaches(c *gc.C) {
	t.srv.ec2srv.SetAccountAttributes(map[string][]string{
		"default-vpc": {"none"},
	})
	env := t.prepareEnviron(c)
	result, err := env.SupportsAddressAllocation("")
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
	c.Assert(result, jc.IsFalse)

	// this value won't change normally, the change here is to
	// ensure that subsequent calls use the cached value
	t.srv.ec2srv.SetAccountAttributes(map[string][]string{
		"default-vpc": {"vpc-xxxxxxx"},
	})
	result, err = env.SupportsAddressAllocation("")
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
	c.Assert(result, jc.IsFalse)
}

func (t *localServerSuite) TestSupportsAddressAllocationFalse(c *gc.C) {
	t.srv.ec2srv.SetAccountAttributes(map[string][]string{
		"default-vpc": {"none"},
	})
	env := t.prepareEnviron(c)
	result, err := env.SupportsAddressAllocation("")
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
	c.Assert(result, jc.IsFalse)
}

func (t *localServerSuite) TestInstanceTags(c *gc.C) {
	env := t.Prepare(c)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)

	instances, err := env.AllInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instances, gc.HasLen, 1)

	ec2Inst := ec2.InstanceEC2(instances[0])
	c.Assert(ec2Inst.Tags, jc.SameContents, []amzec2.Tag{
		{"Name", "juju-sample-machine-0"},
		{"juju-model-uuid", coretesting.ModelTag.Id()},
		{"juju-controller-uuid", coretesting.ModelTag.Id()},
		{"juju-is-controller", "true"},
	})
}

func (t *localServerSuite) TestRootDiskTags(c *gc.C) {
	env := t.Prepare(c)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)

	instances, err := env.AllInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instances, gc.HasLen, 1)

	ec2conn := ec2.EnvironEC2(env)
	resp, err := ec2conn.Volumes(nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.Volumes, gc.Not(gc.HasLen), 0)

	var found *amzec2.Volume
	for _, vol := range resp.Volumes {
		if len(vol.Tags) != 0 {
			found = &vol
			break
		}
	}
	c.Assert(found, gc.NotNil)
	c.Assert(found.Tags, jc.SameContents, []amzec2.Tag{
		{"Name", "juju-sample-machine-0-root"},
		{"juju-model-uuid", coretesting.ModelTag.Id()},
		{"juju-controller-uuid", coretesting.ModelTag.Id()},
	})
}

// localNonUSEastSuite is similar to localServerSuite but the S3 mock server
// behaves as if it is not in the us-east region.
type localNonUSEastSuite struct {
	coretesting.BaseSuite
	sstesting.TestDataSuite

	restoreEC2Patching func()
	srv                localServer
	env                environs.Environ
}

func (t *localNonUSEastSuite) SetUpSuite(c *gc.C) {
	t.BaseSuite.SetUpSuite(c)
	t.TestDataSuite.SetUpSuite(c)

	t.PatchValue(&imagemetadata.SimplestreamsImagesPublicKey, sstesting.SignedMetadataPublicKey)
	t.PatchValue(&juju.JujuPublicKey, sstesting.SignedMetadataPublicKey)

	t.restoreEC2Patching = patchEC2ForTesting(c)
	t.BaseSuite.PatchValue(ec2.DeleteSecurityGroupInsistently, deleteSecurityGroupForTestFunc)
}

func (t *localNonUSEastSuite) TearDownSuite(c *gc.C) {
	t.restoreEC2Patching()
	t.TestDataSuite.TearDownSuite(c)
	t.BaseSuite.TearDownSuite(c)
}

func (t *localNonUSEastSuite) SetUpTest(c *gc.C) {
	t.BaseSuite.SetUpTest(c)
	t.srv.config = &s3test.Config{
		Send409Conflict: true,
	}
	t.srv.startServer(c)

	env, err := environs.Prepare(
		envtesting.BootstrapContext(c),
		jujuclienttesting.NewMemStore(),
		environs.PrepareParams{
			BaseConfig: localConfigAttrs,
			Credential: cloud.NewCredential(
				cloud.AccessKeyAuthType,
				map[string]string{
					"access-key": "x",
					"secret-key": "x",
				},
			),
			ControllerName: localConfigAttrs["name"].(string),
			CloudName:      "ec2",
			CloudRegion:    "test",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	t.env = env
}

func (t *localNonUSEastSuite) TearDownTest(c *gc.C) {
	t.srv.stopServer(c)
	t.BaseSuite.TearDownTest(c)
}

func patchEC2ForTesting(c *gc.C) func() {
	ec2.UseTestImageData(c, ec2.TestImagesData)
	ec2.UseTestInstanceTypeData(ec2.TestInstanceTypeCosts)
	ec2.UseTestRegionData(ec2.TestRegions)
	restoreTimeouts := envtesting.PatchAttemptStrategies(ec2.ShortAttempt, ec2.StorageAttempt)
	restoreFinishBootstrap := envtesting.DisableFinishBootstrap()
	return func() {
		restoreFinishBootstrap()
		restoreTimeouts()
		ec2.UseTestImageData(c, nil)
		ec2.UseTestInstanceTypeData(nil)
		ec2.UseTestRegionData(nil)
	}
}

// If match is true, CheckScripts checks that at least one script started
// by the cloudinit data matches the given regexp pattern, otherwise it
// checks that no script matches.  It's exported so it can be used by tests
// defined in ec2_test.
func CheckScripts(c *gc.C, userDataMap map[interface{}]interface{}, pattern string, match bool) {
	scripts0 := userDataMap["runcmd"]
	if scripts0 == nil {
		c.Errorf("cloudinit has no entry for runcmd")
		return
	}
	scripts := scripts0.([]interface{})
	re := regexp.MustCompile(pattern)
	found := false
	for _, s0 := range scripts {
		s := s0.(string)
		if re.MatchString(s) {
			found = true
		}
	}
	switch {
	case match && !found:
		c.Errorf("script %q not found in %q", pattern, scripts)
	case !match && found:
		c.Errorf("script %q found but not expected in %q", pattern, scripts)
	}
}

// CheckPackage checks that the cloudinit will or won't install the given
// package, depending on the value of match.  It's exported so it can be
// used by tests defined outside the ec2 package.
func CheckPackage(c *gc.C, userDataMap map[interface{}]interface{}, pkg string, match bool) {
	pkgs0 := userDataMap["packages"]
	if pkgs0 == nil {
		if match {
			c.Errorf("cloudinit has no entry for packages")
		}
		return
	}

	pkgs := pkgs0.([]interface{})

	found := false
	for _, p0 := range pkgs {
		p := p0.(string)
		// p might be a space separate list of packages eg 'foo bar qed' so split them up
		manyPkgs := set.NewStrings(strings.Split(p, " ")...)
		hasPkg := manyPkgs.Contains(pkg)
		if p == pkg || hasPkg {
			found = true
			break
		}
	}
	switch {
	case match && !found:
		c.Errorf("package %q not found in %v", pkg, pkgs)
	case !match && found:
		c.Errorf("%q found but not expected in %v", pkg, pkgs)
	}
}
