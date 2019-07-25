// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	"fmt"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/os/series"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	"gopkg.in/amz.v3/aws"
	amzec2 "gopkg.in/amz.v3/ec2"
	"gopkg.in/amz.v3/ec2/ec2test"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/imagemetadata"
	imagetesting "github.com/juju/juju/environs/imagemetadata/testing"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/jujutest"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/environs/tags"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/juju/keys"
	"github.com/juju/juju/juju/testing"
	supportedversion "github.com/juju/juju/juju/version"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/ec2"
	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

var localConfigAttrs = coretesting.FakeConfig().Merge(coretesting.Attrs{
	"name":          "sample",
	"type":          "ec2",
	"agent-version": coretesting.FakeVersionNumber.String(),
})

func fakeCallback(_ status.Status, _ string, _ map[string]interface{}) error {
	return nil
}

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
	srv localServer
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

	// Upload arches that ec2 supports; add to this
	// as ec2 coverage expands.
	t.UploadArches = []string{arch.AMD64, arch.I386}
	t.TestConfig = localConfigAttrs
	imagetesting.PatchOfficialDataSources(&t.BaseSuite.CleanupSuite, "test:")
	t.BaseSuite.PatchValue(&imagemetadata.SimplestreamsImagesPublicKey, sstesting.SignedMetadataPublicKey)
	t.BaseSuite.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)
	t.BaseSuite.PatchValue(ec2.DeleteSecurityGroupInsistently, deleteSecurityGroupForTestFunc)
	t.srv.createRootDisks = true
	t.srv.startServer(c)

	region := t.srv.region
	t.CloudRegion = region.Name
	t.CloudEndpoint = region.EC2Endpoint
	restoreEC2Patching := patchEC2ForTesting(c, region)
	t.BaseSuite.AddCleanup(func(c *gc.C) { restoreEC2Patching() })
}

func (t *localLiveSuite) TearDownSuite(c *gc.C) {
	t.LiveTests.TearDownSuite(c)
	t.srv.stopServer(c)
}

// localServer represents a fake EC2 server running within
// the test process itself.
type localServer struct {
	// createRootDisks is used to decide whether or not
	// the ec2test server will create root disks for
	// instances.
	createRootDisks bool

	ec2srv      *ec2test.Server
	proxy       *httputil.ReverseProxy
	proxyServer *httptest.Server
	client      *amzec2.EC2
	region      aws.Region

	defaultVPC *amzec2.VPC
	zones      []amzec2.AvailabilityZoneInfo
	subnets    []amzec2.Subnet
}

func (srv *localServer) startServer(c *gc.C) {
	var err error
	srv.ec2srv, err = ec2test.NewServer()
	if err != nil {
		c.Fatalf("cannot start ec2 test server: %v", err)
	}
	srv.ec2srv.SetCreateRootDisks(srv.createRootDisks)
	srv.addSpice(c)

	// Create a reverse proxy, so we can override responses.
	endpointURL, err := url.Parse(srv.ec2srv.URL())
	c.Assert(err, jc.ErrorIsNil)
	backendURL := &url.URL{
		Scheme: endpointURL.Scheme,
		Host:   endpointURL.Host,
	}
	srv.proxy = httputil.NewSingleHostReverseProxy(backendURL)
	srv.proxyServer = httptest.NewServer(srv.proxy)
	endpointURL, err = url.Parse(srv.proxyServer.URL)
	c.Assert(err, jc.ErrorIsNil)
	srv.region = aws.Region{
		Name:        "test",
		EC2Endpoint: endpointURL.String(),
	}
	srv.client = amzec2.New(aws.Auth{}, srv.region, aws.SignV4Factory(srv.region.Name, "ec2"))

	zones := make([]amzec2.AvailabilityZoneInfo, 4)
	zones[0].Region = srv.region.Name
	zones[0].Name = srv.region.Name + "-available"
	zones[0].State = "available"
	zones[1].Region = srv.region.Name
	zones[1].Name = srv.region.Name + "-impaired"
	zones[1].State = "impaired"
	zones[2].Region = srv.region.Name
	zones[2].Name = srv.region.Name + "-unavailable"
	zones[2].State = "unavailable"
	zones[3].Region = srv.region.Name
	zones[3].Name = srv.region.Name + "-available2"
	zones[3].State = "available"
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
	srv.proxyServer.Close()
	srv.ec2srv.Reset(false)
	srv.ec2srv.Quit()
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
	srv    localServer
	client *amzec2.EC2

	callCtx context.ProviderCallContext
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

	// Upload arches that ec2 supports; add to this
	// as ec2 coverage expands.
	t.UploadArches = []string{arch.AMD64, arch.I386}
	t.TestConfig = localConfigAttrs
	imagetesting.PatchOfficialDataSources(&t.BaseSuite.CleanupSuite, "test:")
	t.BaseSuite.PatchValue(&imagemetadata.SimplestreamsImagesPublicKey, sstesting.SignedMetadataPublicKey)
	t.BaseSuite.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)
	t.BaseSuite.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	t.BaseSuite.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
	t.BaseSuite.PatchValue(&series.MustHostSeries, func() string { return supportedversion.SupportedLTS() })
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
	t.Tests.TearDownSuite(c)
	t.BaseSuite.TearDownSuite(c)
}

func (t *localServerSuite) SetUpTest(c *gc.C) {
	t.BaseSuite.SetUpTest(c)
	t.srv.startServer(c)
	region := t.srv.region
	t.CloudRegion = region.Name
	t.CloudEndpoint = region.EC2Endpoint
	t.client = t.srv.client
	restoreEC2Patching := patchEC2ForTesting(c, region)
	t.AddCleanup(func(c *gc.C) { restoreEC2Patching() })
	t.Tests.SetUpTest(c)

	t.callCtx = context.NewCloudCallContext()
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
	params.ModelConfig["vpc-id"] = t.srv.defaultVPC.Id
	params.ModelConfig["vpc-id-force"] = true

	t.prepareWithParamsAndBootstrapWithVPCID(c, params, t.srv.defaultVPC.Id)
}

func (t *localServerSuite) TestPrepareForBootstrapWithEmptyVPCID(c *gc.C) {
	const emptyVPCID = ""

	params := t.PrepareParams(c)
	params.ModelConfig["vpc-id"] = emptyVPCID

	t.prepareWithParamsAndBootstrapWithVPCID(c, params, emptyVPCID)
}

func (t *localServerSuite) prepareWithParamsAndBootstrapWithVPCID(c *gc.C, params bootstrap.PrepareParams, expectedVPCID string) {
	env := t.PrepareWithParams(c, params)
	unknownAttrs := env.Config().UnknownAttrs()
	vpcID, ok := unknownAttrs["vpc-id"]
	c.Check(vpcID, gc.Equals, expectedVPCID)
	c.Check(ok, jc.IsTrue)

	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		t.callCtx, bootstrap.BootstrapParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			AdminSecret:      testing.AdminSecret,
			CAPrivateKey:     coretesting.CAKey,
			Placement:        "zone=test-available",
		})
	c.Assert(err, jc.ErrorIsNil)
}

func (t *localServerSuite) TestPrepareForBootstrapWithVPCIDNone(c *gc.C) {
	params := t.PrepareParams(c)
	params.ModelConfig["vpc-id"] = "none"

	t.prepareWithParamsAndBootstrapWithVPCID(c, params, ec2.VPCIDNone)
}

func (t *localServerSuite) TestPrepareForBootstrapWithDefaultVPCID(c *gc.C) {
	params := t.PrepareParams(c)
	params.ModelConfig["vpc-id"] = t.srv.defaultVPC.Id

	t.prepareWithParamsAndBootstrapWithVPCID(c, params, t.srv.defaultVPC.Id)
}

func (t *localServerSuite) TestSystemdBootstrapInstanceUserDataAndState(c *gc.C) {
	env := t.Prepare(c)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		t.callCtx, bootstrap.BootstrapParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			// TODO(redir): BBB: When we no longer support upstart based systems this can change to series.LatestLts()
			BootstrapSeries: "xenial",
			AdminSecret:     testing.AdminSecret,
			CAPrivateKey:    coretesting.CAKey,
		})
	c.Assert(err, jc.ErrorIsNil)

	// check that ControllerInstances returns the id of the bootstrap machine.
	instanceIds, err := env.ControllerInstances(t.callCtx, t.ControllerUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instanceIds, gc.HasLen, 1)

	insts, err := env.AllRunningInstances(t.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(insts, gc.HasLen, 1)
	c.Check(insts[0].Id(), gc.Equals, instanceIds[0])

	// check that the user data is configured to and the machine and
	// provisioning agents.  check that the user data is configured to only
	// configure authorized SSH keys and set the log output; everything else
	// happens after the machine is brought up.
	inst := t.srv.ec2srv.Instance(string(insts[0].Id()))
	c.Assert(inst, gc.NotNil)
	addresses, err := insts[0].Addresses(t.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addresses, gc.Not(gc.HasLen), 0)
	userData, err := utils.Gunzip(inst.UserData)
	c.Assert(err, jc.ErrorIsNil)

	var userDataMap map[string]interface{}
	err = goyaml.Unmarshal(userData, &userDataMap)
	c.Assert(err, jc.ErrorIsNil)
	var keys []string
	for key := range userDataMap {
		keys = append(keys, key)
	}
	c.Assert(keys, jc.SameContents, []string{"output", "users", "runcmd", "ssh_keys"})
	c.Assert(userDataMap["runcmd"], jc.DeepEquals, []interface{}{
		"set -xe",
		"install -D -m 644 /dev/null '/etc/systemd/system/juju-clean-shutdown.service'",
		"printf '%s\\n' '\n[Unit]\nDescription=Stop all network interfaces on shutdown\nDefaultDependencies=false\nAfter=final.target\n\n[Service]\nType=oneshot\nExecStart=/sbin/ifdown -a -v --force\nStandardOutput=tty\nStandardError=tty\n\n[Install]\nWantedBy=final.target\n' > '/etc/systemd/system/juju-clean-shutdown.service'", "/bin/systemctl enable '/etc/systemd/system/juju-clean-shutdown.service'",
		"install -D -m 644 /dev/null '/var/lib/juju/nonce.txt'",
		"printf '%s\\n' 'user-admin:bootstrap' > '/var/lib/juju/nonce.txt'",
	})

	// check that a new instance will be started with a machine agent
	inst1, hc := testing.AssertStartInstance(c, env, t.callCtx, t.ControllerUUID, "1")
	c.Check(*hc.Arch, gc.Equals, "amd64")
	c.Check(*hc.Mem, gc.Equals, uint64(1024))
	c.Check(*hc.CpuCores, gc.Equals, uint64(2))
	inst = t.srv.ec2srv.Instance(string(inst1.Id()))
	c.Assert(inst, gc.NotNil)
	userData, err = utils.Gunzip(inst.UserData)
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("second instance: UserData: %q", userData)
	userDataMap = nil
	err = goyaml.Unmarshal(userData, &userDataMap)
	c.Assert(err, jc.ErrorIsNil)
	CheckPackage(c, userDataMap, "curl", true)
	CheckPackage(c, userDataMap, "mongodb-server", false)
	CheckScripts(c, userDataMap, "jujud bootstrap-state", false)
	CheckScripts(c, userDataMap, "/var/lib/juju/agents/machine-1/agent.conf", true)
	// TODO check for provisioning agent

	err = env.Destroy(t.callCtx)
	c.Assert(err, jc.ErrorIsNil)

	_, err = env.ControllerInstances(t.callCtx, t.ControllerUUID)
	c.Assert(err, gc.Equals, environs.ErrNotBootstrapped)
}

// TestUpstartBoostrapInstanceUserDataAndState is a test for legacy systems
// using upstart which will be around until trusty is no longer supported.
// TODO(redir): BBB: remove when trusty is no longer supported
func (t *localServerSuite) TestUpstartBootstrapInstanceUserDataAndState(c *gc.C) {
	env := t.Prepare(c)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		t.callCtx, bootstrap.BootstrapParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			BootstrapSeries:  "trusty",
			AdminSecret:      testing.AdminSecret,
			CAPrivateKey:     coretesting.CAKey,
		})
	c.Assert(err, jc.ErrorIsNil)

	// check that ControllerInstances returns the id of the bootstrap machine.
	instanceIds, err := env.ControllerInstances(t.callCtx, t.ControllerUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instanceIds, gc.HasLen, 1)

	insts, err := env.AllRunningInstances(t.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(insts, gc.HasLen, 1)
	c.Check(insts[0].Id(), gc.Equals, instanceIds[0])

	// check that the user data is configured to and the machine and
	// provisioning agents.  check that the user data is configured to only
	// configure authorized SSH keys and set the log output; everything else
	// happens after the machine is brought up.
	inst := t.srv.ec2srv.Instance(string(insts[0].Id()))
	c.Assert(inst, gc.NotNil)
	addresses, err := insts[0].Addresses(t.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addresses, gc.Not(gc.HasLen), 0)
	userData, err := utils.Gunzip(inst.UserData)
	c.Assert(err, jc.ErrorIsNil)

	var userDataMap map[string]interface{}
	err = goyaml.Unmarshal(userData, &userDataMap)
	c.Assert(err, jc.ErrorIsNil)
	var keys []string
	for key := range userDataMap {
		keys = append(keys, key)
	}
	c.Assert(keys, jc.SameContents, []string{"output", "users", "runcmd", "ssh_keys"})
	c.Assert(userDataMap["runcmd"], jc.DeepEquals, []interface{}{
		"set -xe",
		"install -D -m 644 /dev/null '/etc/init/juju-clean-shutdown.conf'",
		"printf '%s\\n' '\nauthor \"Juju Team <juju@lists.ubuntu.com>\"\ndescription \"Stop all network interfaces on shutdown\"\nstart on runlevel [016]\ntask\nconsole output\n\nexec /sbin/ifdown -a -v --force\n' > '/etc/init/juju-clean-shutdown.conf'",
		"install -D -m 644 /dev/null '/var/lib/juju/nonce.txt'",
		"printf '%s\\n' 'user-admin:bootstrap' > '/var/lib/juju/nonce.txt'",
	})

	// check that a new instance will be started with a machine agent
	inst1, hc := testing.AssertStartInstance(c, env, t.callCtx, t.ControllerUUID, "1")
	c.Check(*hc.Arch, gc.Equals, "amd64")
	c.Check(*hc.Mem, gc.Equals, uint64(1024))
	c.Check(*hc.CpuCores, gc.Equals, uint64(2))
	inst = t.srv.ec2srv.Instance(string(inst1.Id()))
	c.Assert(inst, gc.NotNil)
	userData, err = utils.Gunzip(inst.UserData)
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("second instance: UserData: %q", userData)
	userDataMap = nil
	err = goyaml.Unmarshal(userData, &userDataMap)
	c.Assert(err, jc.ErrorIsNil)
	CheckPackage(c, userDataMap, "curl", true)
	CheckPackage(c, userDataMap, "mongodb-server", false)
	CheckScripts(c, userDataMap, "jujud bootstrap-state", false)
	CheckScripts(c, userDataMap, "/var/lib/juju/agents/machine-1/agent.conf", true)
	// TODO check for provisioning agent

	err = env.Destroy(t.callCtx)
	c.Assert(err, jc.ErrorIsNil)

	_, err = env.ControllerInstances(t.callCtx, t.ControllerUUID)
	c.Assert(err, gc.Equals, environs.ErrNotBootstrapped)
}

func (t *localServerSuite) TestTerminateInstancesIgnoresNotFound(c *gc.C) {
	env := t.Prepare(c)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		t.callCtx, bootstrap.BootstrapParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			AdminSecret:      testing.AdminSecret,
			CAPrivateKey:     coretesting.CAKey,
		})
	c.Assert(err, jc.ErrorIsNil)

	t.BaseSuite.PatchValue(ec2.DeleteSecurityGroupInsistently, deleteSecurityGroupForTestFunc)
	insts, err := env.AllRunningInstances(t.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	idsToStop := make([]instance.Id, len(insts)+1)
	for i, one := range insts {
		idsToStop[i] = one.Id()
	}
	idsToStop[len(insts)] = instance.Id("i-am-not-found")

	err = env.StopInstances(t.callCtx, idsToStop...)
	// NotFound should be ignored
	c.Assert(err, jc.ErrorIsNil)
}

func (t *localServerSuite) TestDestroyErr(c *gc.C) {
	env := t.prepareAndBootstrap(c)

	msg := "terminate instances error"
	t.BaseSuite.PatchValue(ec2.TerminateInstancesById, func(ec2inst *amzec2.EC2, ctx context.ProviderCallContext, ids ...instance.Id) (*amzec2.TerminateInstancesResp, error) {
		return nil, errors.New(msg)
	})

	err := env.Destroy(t.callCtx)
	c.Assert(errors.Cause(err).Error(), jc.Contains, msg)
}

func (t *localServerSuite) TestGetTerminatedInstances(c *gc.C) {
	env := t.Prepare(c)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		t.callCtx, bootstrap.BootstrapParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			AdminSecret:      testing.AdminSecret,
			CAPrivateKey:     coretesting.CAKey,
		})
	c.Assert(err, jc.ErrorIsNil)

	// create another instance to terminate
	inst1, _ := testing.AssertStartInstance(c, env, t.callCtx, t.ControllerUUID, "1")
	inst := t.srv.ec2srv.Instance(string(inst1.Id()))
	c.Assert(inst, gc.NotNil)
	t.BaseSuite.PatchValue(ec2.TerminateInstancesById, func(ec2inst *amzec2.EC2, ctx context.ProviderCallContext, ids ...instance.Id) (*amzec2.TerminateInstancesResp, error) {
		// Terminate the one destined for termination and
		// err out to ensure that one instance will be terminated, the other - not.
		_, err = ec2inst.TerminateInstances([]string{string(inst1.Id())})
		c.Assert(err, jc.ErrorIsNil)
		return nil, errors.New("terminate instances error")
	})
	err = env.Destroy(t.callCtx)
	c.Assert(err, gc.NotNil)

	terminated, err := ec2.TerminatedInstances(env)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(terminated, gc.HasLen, 1)
	c.Assert(terminated[0].Id(), jc.DeepEquals, inst1.Id())
}

func (t *localServerSuite) TestInstanceSecurityGroupsWitheInstanceStatusFilter(c *gc.C) {
	env := t.prepareAndBootstrap(c)

	insts, err := env.AllRunningInstances(t.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	ids := make([]instance.Id, len(insts))
	for i, one := range insts {
		ids[i] = one.Id()
	}

	groupsNoInstanceFilter, err := ec2.InstanceSecurityGroups(env, t.callCtx, ids)
	c.Assert(err, jc.ErrorIsNil)
	// get all security groups for test instances
	c.Assert(groupsNoInstanceFilter, gc.HasLen, 2)

	groupsFilteredForTerminatedInstances, err := ec2.InstanceSecurityGroups(env, t.callCtx, ids, "shutting-down", "terminated")
	c.Assert(err, jc.ErrorIsNil)
	// get all security groups for terminated test instances
	c.Assert(groupsFilteredForTerminatedInstances, gc.HasLen, 0)
}

func (t *localServerSuite) TestDestroyControllerModelDeleteSecurityGroupInsistentlyError(c *gc.C) {
	env := t.prepareAndBootstrap(c)
	msg := "destroy security group error"
	t.BaseSuite.PatchValue(ec2.DeleteSecurityGroupInsistently, func(
		ec2.SecurityGroupCleaner, context.ProviderCallContext, amzec2.SecurityGroup, clock.Clock,
	) error {
		return errors.New(msg)
	})
	err := env.DestroyController(t.callCtx, t.ControllerUUID)
	c.Assert(err, gc.ErrorMatches, "destroying managed environs: cannot delete security group .*: "+msg)
}

func (t *localServerSuite) TestDestroyHostedModelDeleteSecurityGroupInsistentlyError(c *gc.C) {
	env := t.prepareAndBootstrap(c)
	hostedEnv, err := environs.New(environs.OpenParams{
		Cloud:  t.CloudSpec(),
		Config: env.Config(),
	})
	c.Assert(err, jc.ErrorIsNil)

	msg := "destroy security group error"
	t.BaseSuite.PatchValue(ec2.DeleteSecurityGroupInsistently, func(
		ec2.SecurityGroupCleaner, context.ProviderCallContext, amzec2.SecurityGroup, clock.Clock,
	) error {
		return errors.New(msg)
	})
	err = hostedEnv.Destroy(t.callCtx)
	c.Assert(err, gc.ErrorMatches, "cannot delete environment security groups: cannot delete default security group: "+msg)
}

func (t *localServerSuite) TestDestroyControllerDestroysHostedModelResources(c *gc.C) {
	controllerEnv := t.prepareAndBootstrap(c)

	// Create a hosted model environment with an instance and a volume.
	hostedModelUUID := "7e386e08-cba7-44a4-a76e-7c1633584210"
	t.srv.ec2srv.SetInitialInstanceState(ec2test.Running)
	cfg, err := controllerEnv.Config().Apply(map[string]interface{}{
		"uuid":          hostedModelUUID,
		"firewall-mode": "global",
	})
	c.Assert(err, jc.ErrorIsNil)
	env, err := environs.New(environs.OpenParams{
		Cloud:  t.CloudSpec(),
		Config: cfg,
	})
	c.Assert(err, jc.ErrorIsNil)
	inst, _ := testing.AssertStartInstance(c, env, t.callCtx, t.ControllerUUID, "0")
	c.Assert(err, jc.ErrorIsNil)
	ebsProvider, err := env.StorageProvider(ec2.EBS_ProviderType)
	c.Assert(err, jc.ErrorIsNil)
	vs, err := ebsProvider.VolumeSource(nil)
	c.Assert(err, jc.ErrorIsNil)
	volumeResults, err := vs.CreateVolumes(t.callCtx, []storage.VolumeParams{{
		Tag:      names.NewVolumeTag("0"),
		Size:     1024,
		Provider: ec2.EBS_ProviderType,
		ResourceTags: map[string]string{
			tags.JujuController: t.ControllerUUID,
			tags.JujuModel:      hostedModelUUID,
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
		insts, err := env.AllRunningInstances(t.callCtx)
		c.Assert(err, jc.ErrorIsNil)
		ids := make([]instance.Id, len(insts))
		for i, inst := range insts {
			ids[i] = inst.Id()
		}
		c.Assert(ids, jc.SameContents, expect)
	}
	assertVolumes := func(expect ...string) {
		volIds, err := vs.ListVolumes(t.callCtx)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(volIds, jc.SameContents, expect)
	}
	assertGroups := func(expect ...string) {
		groupsResp, err := t.client.SecurityGroups(nil, nil)
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
		"juju-"+controllerEnv.Config().UUID(),
		"juju-"+controllerEnv.Config().UUID()+"-0",
		"juju-"+hostedModelUUID,
		"juju-"+hostedModelUUID+"-global",
	)

	// Destroy the controller resources. This should destroy the hosted
	// environment too.
	err = controllerEnv.DestroyController(t.callCtx, t.ControllerUUID)
	c.Assert(err, jc.ErrorIsNil)

	assertInstances()
	assertVolumes()
	assertGroups("default")
}

func (t *localServerSuite) TestInstanceStatus(c *gc.C) {
	env := t.Prepare(c)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		t.callCtx, bootstrap.BootstrapParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			AdminSecret:      testing.AdminSecret,
			CAPrivateKey:     coretesting.CAKey,
		})
	c.Assert(err, jc.ErrorIsNil)
	t.srv.ec2srv.SetInitialInstanceState(ec2test.Terminated)
	inst, _ := testing.AssertStartInstance(c, env, t.callCtx, t.ControllerUUID, "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(inst.Status(t.callCtx).Message, gc.Equals, "terminated")
}

func (t *localServerSuite) TestStartInstanceHardwareCharacteristics(c *gc.C) {
	env := t.prepareAndBootstrap(c)
	_, hc := testing.AssertStartInstance(c, env, t.callCtx, t.ControllerUUID, "1")
	c.Check(*hc.Arch, gc.Equals, "amd64")
	c.Check(*hc.Mem, gc.Equals, uint64(1024))
	c.Check(*hc.CpuCores, gc.Equals, uint64(2))
}

func (t *localServerSuite) TestStartInstanceAvailZone(c *gc.C) {
	inst, err := t.testStartInstanceAvailZone(c, "test-available")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ec2.InstanceEC2(inst).AvailZone, gc.Equals, "test-available")
}

func (t *localServerSuite) TestStartInstanceAvailZoneImpaired(c *gc.C) {
	_, err := t.testStartInstanceAvailZone(c, "test-impaired")
	c.Assert(err, gc.ErrorMatches, `availability zone "test-impaired" is "impaired"`)
}

func (t *localServerSuite) TestStartInstanceAvailZoneUnknown(c *gc.C) {
	_, err := t.testStartInstanceAvailZone(c, "test-unknown")
	c.Assert(err, gc.Not(jc.Satisfies), environs.IsAvailabilityZoneIndependent)
	c.Assert(errors.Details(err), gc.Matches, `.*availability zone \"\" not valid.*`)
}

func (t *localServerSuite) testStartInstanceAvailZone(c *gc.C, zone string) (instances.Instance, error) {
	env := t.prepareAndBootstrap(c)

	params := environs.StartInstanceParams{ControllerUUID: t.ControllerUUID, AvailabilityZone: zone, StatusCallback: fakeCallback}
	result, err := testing.StartInstanceWithParams(env, t.callCtx, "1", params)
	if err != nil {
		return nil, err
	}
	return result.Instance, nil
}

func (t *localServerSuite) TestStartInstanceVolumeAttachmentsAvailZone(c *gc.C) {
	env := t.prepareAndBootstrap(c)
	resp, err := t.client.CreateVolume(amzec2.CreateVolume{
		VolumeSize: 1,
		VolumeType: "gp2",
		AvailZone:  "volume-zone",
	})
	c.Assert(err, jc.ErrorIsNil)

	args := environs.StartInstanceParams{
		ControllerUUID: t.ControllerUUID,
		StatusCallback: fakeCallback,
		VolumeAttachments: []storage.VolumeAttachmentParams{{
			AttachmentParams: storage.AttachmentParams{
				Provider: "ebs",
				Machine:  names.NewMachineTag("1"),
			},
			Volume:   names.NewVolumeTag("23"),
			VolumeId: resp.Id,
		}},
	}
	result, err := testing.StartInstanceWithParams(env, t.callCtx, "1", args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ec2.InstanceEC2(result.Instance).AvailZone, gc.Equals, "volume-zone")
}

func (t *localServerSuite) TestStartInstanceVolumeAttachmentsAvailZonePlacementConflicts(c *gc.C) {
	env := t.prepareAndBootstrap(c)
	resp, err := t.client.CreateVolume(amzec2.CreateVolume{
		VolumeSize: 1,
		VolumeType: "gp2",
		AvailZone:  "volume-zone",
	})
	c.Assert(err, jc.ErrorIsNil)

	args := environs.StartInstanceParams{
		ControllerUUID: t.ControllerUUID,
		StatusCallback: fakeCallback,
		Placement:      "zone=test-available",
		VolumeAttachments: []storage.VolumeAttachmentParams{{
			AttachmentParams: storage.AttachmentParams{
				Provider: "ebs",
				Machine:  names.NewMachineTag("1"),
			},
			Volume:   names.NewVolumeTag("23"),
			VolumeId: resp.Id,
		}},
	}
	_, err = testing.StartInstanceWithParams(env, t.callCtx, "1", args)
	c.Assert(err, gc.ErrorMatches, `cannot create instance with placement "zone=test-available", as this will prevent attaching the requested EBS volumes in zone "volume-zone"`)
}

func (t *localServerSuite) TestStartInstanceZoneIndependent(c *gc.C) {
	env := t.prepareAndBootstrap(c)
	params := environs.StartInstanceParams{
		ControllerUUID:   t.ControllerUUID,
		StatusCallback:   fakeCallback,
		AvailabilityZone: "test-available",
		Placement:        "nonsense",
	}
	_, err := testing.StartInstanceWithParams(env, t.callCtx, "1", params)
	c.Assert(err, gc.ErrorMatches, "unknown placement directive: nonsense")
	// The returned error should indicate that it is independent
	// of the availability zone specified.
	c.Assert(err, jc.Satisfies, environs.IsAvailabilityZoneIndependent)
}

func (t *localServerSuite) TestStartInstanceSubnet(c *gc.C) {
	inst, err := t.testStartInstanceSubnet(c, "0.1.2.0/24")
	c.Assert(err, jc.ErrorIsNil)
	ec2Inst := ec2.InstanceEC2(inst)
	c.Assert(ec2Inst.AvailZone, gc.Equals, "test-available")
}

func (t *localServerSuite) TestStartInstanceSubnetUnavailable(c *gc.C) {
	// See addTestingSubnets, 0.1.3.0/24 is in state "unavailable", but is in
	// an AZ that would otherwise be available
	_, err := t.testStartInstanceSubnet(c, "0.1.3.0/24")
	c.Assert(err, gc.ErrorMatches, `subnet "0.1.3.0/24" is "unavailable"`)
}

func (t *localServerSuite) TestStartInstanceSubnetAZUnavailable(c *gc.C) {
	// See addTestingSubnets, 0.1.4.0/24 is in an AZ that is unavailable
	_, err := t.testStartInstanceSubnet(c, "0.1.4.0/24")
	c.Assert(err, gc.ErrorMatches, `availability zone "test-unavailable" is "unavailable"`)
}

func (t *localServerSuite) testStartInstanceSubnet(c *gc.C, subnet string) (instances.Instance, error) {
	subIDs, vpcId := t.addTestingSubnets(c)
	env := t.prepareAndBootstrapWithConfig(c, coretesting.Attrs{"vpc-id": vpcId, "vpc-id-force": true})
	params := environs.StartInstanceParams{
		ControllerUUID: t.ControllerUUID,
		Placement:      fmt.Sprintf("subnet=%s", subnet),
		SubnetsToZones: map[corenetwork.Id][]string{
			subIDs[0]: {"test-available"},
			subIDs[1]: {"test-available"},
			subIDs[2]: {"test-unavailable"},
		},
	}
	zonedEnviron := env.(common.ZonedEnviron)
	zones, err := zonedEnviron.DeriveAvailabilityZones(t.callCtx, params)
	if err != nil {
		return nil, err
	}
	if len(zones) > 0 {
		params.AvailabilityZone = zones[0]
		result, err := testing.StartInstanceWithParams(env, t.callCtx, "1", params)
		if err != nil {
			return nil, err
		}
		return result.Instance, nil
	}
	return nil, errors.Errorf("testStartInstanceSubnet failed")
}

func (t *localServerSuite) TestDeriveAvailabilityZoneSubnetWrongVPC(c *gc.C) {
	subIDs, vpcId := t.addTestingSubnets(c)
	c.Assert(vpcId, gc.Not(gc.Equals), "vpc-0")
	env := t.prepareAndBootstrapWithConfig(c, coretesting.Attrs{"vpc-id": "vpc-0", "vpc-id-force": true})
	params := environs.StartInstanceParams{
		ControllerUUID: t.ControllerUUID,
		Placement:      "subnet=0.1.2.0/24",
		SubnetsToZones: map[corenetwork.Id][]string{
			subIDs[0]: {"test-available"},
			subIDs[1]: {"test-available"},
			subIDs[2]: {"test-unavailable"},
		},
	}
	zonedEnviron := env.(common.ZonedEnviron)
	_, err := zonedEnviron.DeriveAvailabilityZones(t.callCtx, params)
	c.Assert(err, gc.ErrorMatches, `unknown placement directive: subnet=0.1.2.0/24`)
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
	zones, err := env.AvailabilityZones(t.callCtx)
	c.Assert(err, gc.Equals, resultErr)
	c.Assert(zones, gc.IsNil)

	resultErr = nil
	resultZones = make([]amzec2.AvailabilityZoneInfo, 1)
	resultZones[0].Name = "whatever"
	zones, err = env.AvailabilityZones(t.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zones, gc.HasLen, 1)
	c.Assert(zones[0].Name(), gc.Equals, "whatever")

	// A successful result is cached, currently for the lifetime
	// of the Environ. This will change if/when we have long-lived
	// Environs to cut down repeated IaaS requests.
	resultErr = fmt.Errorf("failed to get availability zones")
	resultZones[0].Name = "andever"
	zones, err = env.AvailabilityZones(t.callCtx)
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
	zones, err := env.AvailabilityZones(t.callCtx)
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

func (t *localServerSuite) TestDeriveAvailabilityZones(c *gc.C) {
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

	zones, err := env.DeriveAvailabilityZones(t.callCtx, environs.StartInstanceParams{Placement: "zone=az1"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zones, gc.DeepEquals, []string{"az1"})
}

func (t *localServerSuite) TestDeriveAvailabilityZonesImpaired(c *gc.C) {
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

	zones, err := env.DeriveAvailabilityZones(t.callCtx, environs.StartInstanceParams{Placement: "zone=az2"})
	c.Assert(err, gc.ErrorMatches, "availability zone \"az2\" is \"impaired\"")
	c.Assert(zones, gc.HasLen, 0)
}

func (t *localServerSuite) TestDeriveAvailabilityZonesConflictVolume(c *gc.C) {
	resp, err := t.client.CreateVolume(amzec2.CreateVolume{
		VolumeSize: 1,
		VolumeType: "gp2",
		AvailZone:  "volume-zone",
	})
	c.Assert(err, jc.ErrorIsNil)

	args := environs.StartInstanceParams{
		ControllerUUID: t.ControllerUUID,
		StatusCallback: fakeCallback,
		Placement:      "zone=test-available",
		VolumeAttachments: []storage.VolumeAttachmentParams{{
			AttachmentParams: storage.AttachmentParams{
				Provider: "ebs",
				Machine:  names.NewMachineTag("1"),
			},
			Volume:   names.NewVolumeTag("23"),
			VolumeId: resp.Id,
		}},
	}
	env := t.Prepare(c).(common.ZonedEnviron)
	zones, err := env.DeriveAvailabilityZones(t.callCtx, args)
	c.Assert(err, gc.ErrorMatches, `cannot create instance with placement "zone=test-available", as this will prevent attaching the requested EBS volumes in zone "volume-zone"`)
	c.Assert(zones, gc.HasLen, 0)
}

func (t *localServerSuite) TestDeriveAvailabilityZonesVolumeNoPlacement(c *gc.C) {
	resp, err := t.client.CreateVolume(amzec2.CreateVolume{
		VolumeSize: 1,
		VolumeType: "gp2",
		AvailZone:  "volume-zone",
	})
	c.Assert(err, jc.ErrorIsNil)

	args := environs.StartInstanceParams{
		ControllerUUID: t.ControllerUUID,
		StatusCallback: fakeCallback,
		VolumeAttachments: []storage.VolumeAttachmentParams{{
			AttachmentParams: storage.AttachmentParams{
				Provider: "ebs",
				Machine:  names.NewMachineTag("1"),
			},
			Volume:   names.NewVolumeTag("23"),
			VolumeId: resp.Id,
		}},
	}
	env := t.Prepare(c).(common.ZonedEnviron)
	zones, err := env.DeriveAvailabilityZones(t.callCtx, args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zones, gc.DeepEquals, []string{"volume-zone"})
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
	env := t.prepareAndBootstrap(c)

	t.PatchValue(ec2.RunInstances, func(e *amzec2.EC2, ctx context.ProviderCallContext, ri *amzec2.RunInstances, c environs.StatusCallbackFunc) (*amzec2.RunInstancesResp, error) {
		return nil, runInstancesError
	})

	params := environs.StartInstanceParams{
		ControllerUUID:   t.ControllerUUID,
		StatusCallback:   fakeCallback,
		AvailabilityZone: "test-available",
	}

	_, err := testing.StartInstanceWithParams(env, t.callCtx, "1", params)
	// All AZConstrained failures should return an error that does
	// *not* satisfy environs.IsAvailabilityZoneIndependent,
	// so the caller knows to try a new zone, rather than fail.
	c.Assert(err, gc.Not(jc.Satisfies), environs.IsAvailabilityZoneIndependent)
	c.Assert(errors.Details(err), jc.Contains, runInstancesError.Message)
}

// addTestingSubnets adds a testing default VPC with 3 subnets in the EC2 test
// server: 2 of the subnets are in the "test-available" AZ, the remaining - in
// "test-unavailable". Returns a slice with the IDs of the created subnets and
// vpc id that those were added to
func (t *localServerSuite) addTestingSubnets(c *gc.C) ([]corenetwork.Id, string) {
	vpc := t.srv.ec2srv.AddVPC(amzec2.VPC{
		CIDRBlock: "0.1.0.0/16",
		IsDefault: true,
	})
	results := make([]corenetwork.Id, 3)
	sub1, err := t.srv.ec2srv.AddSubnet(amzec2.Subnet{
		VPCId:        vpc.Id,
		CIDRBlock:    "0.1.2.0/24",
		AvailZone:    "test-available",
		State:        "available",
		DefaultForAZ: true,
	})
	c.Assert(err, jc.ErrorIsNil)
	results[0] = corenetwork.Id(sub1.Id)
	sub2, err := t.srv.ec2srv.AddSubnet(amzec2.Subnet{
		VPCId:     vpc.Id,
		CIDRBlock: "0.1.3.0/24",
		AvailZone: "test-available",
		State:     "unavailable",
	})
	c.Assert(err, jc.ErrorIsNil)
	results[1] = corenetwork.Id(sub2.Id)
	sub3, err := t.srv.ec2srv.AddSubnet(amzec2.Subnet{
		VPCId:        vpc.Id,
		CIDRBlock:    "0.1.4.0/24",
		AvailZone:    "test-unavailable",
		DefaultForAZ: true,
		State:        "unavailable",
	})
	c.Assert(err, jc.ErrorIsNil)
	results[2] = corenetwork.Id(sub3.Id)
	return results, vpc.Id
}

func (t *localServerSuite) prepareAndBootstrap(c *gc.C) environs.Environ {
	return t.prepareAndBootstrapWithConfig(c, coretesting.Attrs{})
}

func (t *localServerSuite) prepareAndBootstrapWithConfig(c *gc.C, config coretesting.Attrs) environs.Environ {
	args := t.PrepareParams(c)
	args.ModelConfig = coretesting.Attrs(args.ModelConfig).Merge(config)
	env := t.PrepareWithParams(c, args)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		t.callCtx, bootstrap.BootstrapParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			AdminSecret:      testing.AdminSecret,
			CAPrivateKey:     coretesting.CAKey,
			Placement:        "zone=test-available",
		})
	c.Assert(err, jc.ErrorIsNil)
	return env
}

func (t *localServerSuite) TestSpaceConstraintsSpaceNotInPlacementZone(c *gc.C) {
	env := t.prepareAndBootstrap(c)
	subIDs, _ := t.addTestingSubnets(c)

	// Expect an error because zone test-available isn't in SubnetsToZones
	params := environs.StartInstanceParams{
		ControllerUUID: t.ControllerUUID,
		Placement:      "zone=test-available",
		Constraints:    constraints.MustParse("spaces=aaaaaaaaaa"),
		SubnetsToZones: map[corenetwork.Id][]string{
			subIDs[0]: {"zone2"},
			subIDs[1]: {"zone3"},
			subIDs[2]: {"zone4"},
		},
		StatusCallback: fakeCallback,
	}
	_, err := testing.StartInstanceWithParams(env, t.callCtx, "1", params)
	c.Assert(err, gc.Not(jc.Satisfies), environs.IsAvailabilityZoneIndependent)
	c.Assert(errors.Details(err), gc.Matches, `.*subnets in AZ "test-available" not found.*`)
}

func (t *localServerSuite) TestSpaceConstraintsSpaceInPlacementZone(c *gc.C) {
	env := t.prepareAndBootstrap(c)
	subIDs, _ := t.addTestingSubnets(c)

	// Should work - test-available is in SubnetsToZones and in myspace.
	params := environs.StartInstanceParams{
		ControllerUUID: t.ControllerUUID,
		Placement:      "zone=test-available",
		Constraints:    constraints.MustParse("spaces=aaaaaaaaaa"),
		SubnetsToZones: map[corenetwork.Id][]string{
			subIDs[0]: {"test-available"},
			subIDs[1]: {"zone3"},
		},
		StatusCallback: fakeCallback,
	}
	_, err := testing.StartInstanceWithParams(env, t.callCtx, "1", params)
	c.Assert(err, jc.ErrorIsNil)
}

func (t *localServerSuite) TestSpaceConstraintsNoPlacement(c *gc.C) {
	env := t.prepareAndBootstrap(c)
	subIDs, _ := t.addTestingSubnets(c)

	params := environs.StartInstanceParams{
		ControllerUUID: t.ControllerUUID,
		Constraints:    constraints.MustParse("spaces=aaaaaaaaaa"),
		SubnetsToZones: map[corenetwork.Id][]string{
			subIDs[0]: {"test-available"},
			subIDs[1]: {"zone3"},
		},
		StatusCallback: fakeCallback,
	}
	t.assertStartInstanceWithParamsFindAZ(c, env, "1", params)
}

func (t *localServerSuite) assertStartInstanceWithParamsFindAZ(
	c *gc.C,
	env environs.Environ,
	machineId string,
	params environs.StartInstanceParams,
) {
	zonedEnviron := env.(common.ZonedEnviron)
	zones, err := zonedEnviron.DeriveAvailabilityZones(t.callCtx, params)
	c.Assert(err, jc.ErrorIsNil)
	if len(zones) > 0 {
		params.AvailabilityZone = zones[0]
		_, err = testing.StartInstanceWithParams(env, t.callCtx, "1", params)
		c.Assert(err, jc.ErrorIsNil)
		return
	}
	availabilityZones, err := zonedEnviron.AvailabilityZones(t.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	for _, zone := range availabilityZones {
		if !zone.Available() {
			continue
		}
		params.AvailabilityZone = zone.Name()
		_, err = testing.StartInstanceWithParams(env, t.callCtx, "1", params)
		if err == nil {
			return
		} else if !environs.IsAvailabilityZoneIndependent(err) {
			continue
		}
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (t *localServerSuite) TestSpaceConstraintsNoAvailableSubnets(c *gc.C) {
	c.Skip("temporarily disabled")
	subIDs, vpcId := t.addTestingSubnets(c)
	env := t.prepareAndBootstrapWithConfig(c, coretesting.Attrs{"vpc-id": vpcId})

	// We requested a space, but there are no subnets in SubnetsToZones, so we can't resolve
	// the constraints
	params := environs.StartInstanceParams{
		ControllerUUID: t.ControllerUUID,
		Constraints:    constraints.MustParse("spaces=aaaaaaaaaa"),
		SubnetsToZones: map[corenetwork.Id][]string{
			subIDs[0]: {""},
		},
		StatusCallback: fakeCallback,
	}
	//_, err := testing.StartInstanceWithParams(env, "1", params)
	zonedEnviron := env.(common.ZonedEnviron)
	_, err := zonedEnviron.DeriveAvailabilityZones(t.callCtx, params)
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
	env := t.prepareAndBootstrap(c)

	// The first call to RunInstances fails with an error indicating the AZ
	// is constrained. The second attempt succeeds, and so allocates to az2.
	var azArgs []string
	realRunInstances := *ec2.RunInstances

	t.PatchValue(ec2.RunInstances, func(e *amzec2.EC2, ctx context.ProviderCallContext, ri *amzec2.RunInstances, c environs.StatusCallbackFunc) (*amzec2.RunInstancesResp, error) {
		azArgs = append(azArgs, ri.AvailZone)
		if len(azArgs) == 1 {
			return nil, runInstancesError
		}
		return realRunInstances(e, ctx, ri, fakeCallback)
	})

	params := environs.StartInstanceParams{ControllerUUID: t.ControllerUUID}
	zonedEnviron := env.(common.ZonedEnviron)
	availabilityZones, err := zonedEnviron.AvailabilityZones(t.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	for _, zone := range availabilityZones {
		if !zone.Available() {
			continue
		}
		params.AvailabilityZone = zone.Name()
		_, err = testing.StartInstanceWithParams(env, t.callCtx, "1", params)
		if err == nil {
			break
		} else if !environs.IsAvailabilityZoneIndependent(err) {
			continue
		}
		c.Assert(err, jc.ErrorIsNil)
	}
	sort.Strings(azArgs)
	c.Assert(azArgs, gc.DeepEquals, []string{"test-available", "test-available2"})
}

func (t *localServerSuite) TestAddresses(c *gc.C) {
	env := t.prepareAndBootstrap(c)
	inst, _ := testing.AssertStartInstance(c, env, t.callCtx, t.ControllerUUID, "1")
	addrs, err := inst.Addresses(t.callCtx)
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
	validator, err := env.ConstraintsValidator(t.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	cons := constraints.MustParse("arch=amd64 tags=foo virt-type=kvm")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unsupported, jc.SameContents, []string{"tags", "virt-type"})
}

func (t *localServerSuite) TestConstraintsValidatorVocab(c *gc.C) {
	env := t.Prepare(c)
	validator, err := env.ConstraintsValidator(t.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	cons := constraints.MustParse("instance-type=foo")
	_, err = validator.Validate(cons)
	c.Assert(err, gc.ErrorMatches, "invalid constraint value: instance-type=foo\nvalid values are:.*")
}

func (t *localServerSuite) TestConstraintsValidatorVocabNoDefaultOrSpecifiedVPC(c *gc.C) {
	t.srv.defaultVPC.IsDefault = false
	err := t.srv.ec2srv.UpdateVPC(*t.srv.defaultVPC)
	c.Assert(err, jc.ErrorIsNil)

	env := t.Prepare(c)
	assertVPCInstanceTypeNotAvailable(c, env, t.callCtx)
}

func (t *localServerSuite) TestConstraintsValidatorVocabDefaultVPC(c *gc.C) {
	env := t.Prepare(c)
	assertVPCInstanceTypeAvailable(c, env, t.callCtx)
}

func (t *localServerSuite) TestConstraintsValidatorVocabSpecifiedVPC(c *gc.C) {
	t.srv.defaultVPC.IsDefault = false
	err := t.srv.ec2srv.UpdateVPC(*t.srv.defaultVPC)
	c.Assert(err, jc.ErrorIsNil)

	t.TestConfig["vpc-id"] = t.srv.defaultVPC.Id
	defer delete(t.TestConfig, "vpc-id")

	env := t.Prepare(c)
	assertVPCInstanceTypeAvailable(c, env, t.callCtx)
}

func assertVPCInstanceTypeAvailable(c *gc.C, env environs.Environ, ctx context.ProviderCallContext) {
	validator, err := env.ConstraintsValidator(ctx)
	c.Assert(err, jc.ErrorIsNil)
	_, err = validator.Validate(constraints.MustParse("instance-type=t2.medium"))
	c.Assert(err, jc.ErrorIsNil)
}

func assertVPCInstanceTypeNotAvailable(c *gc.C, env environs.Environ, ctx context.ProviderCallContext) {
	validator, err := env.ConstraintsValidator(ctx)
	c.Assert(err, jc.ErrorIsNil)
	_, err = validator.Validate(constraints.MustParse("instance-type=t2.medium"))
	c.Assert(err, gc.ErrorMatches, "invalid constraint value: instance-type=t2.medium\n.*")
}

func (t *localServerSuite) TestConstraintsMerge(c *gc.C) {
	env := t.Prepare(c)
	validator, err := env.ConstraintsValidator(t.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	consA := constraints.MustParse("arch=amd64 mem=1G cpu-power=10 cores=2 tags=bar")
	consB := constraints.MustParse("arch=i386 instance-type=m1.small")
	cons, err := validator.Merge(consA, consB)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, gc.DeepEquals, constraints.MustParse("arch=i386 instance-type=m1.small tags=bar"))
}

func (t *localServerSuite) TestPrecheckInstanceValidInstanceType(c *gc.C) {
	env := t.Prepare(c)
	cons := constraints.MustParse("instance-type=m1.small root-disk=1G")
	err := env.PrecheckInstance(t.callCtx, environs.PrecheckInstanceParams{
		Series:      supportedversion.SupportedLTS(),
		Constraints: cons,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (t *localServerSuite) TestPrecheckInstanceInvalidInstanceType(c *gc.C) {
	env := t.Prepare(c)
	cons := constraints.MustParse("instance-type=m1.invalid")
	err := env.PrecheckInstance(t.callCtx, environs.PrecheckInstanceParams{
		Series:      supportedversion.SupportedLTS(),
		Constraints: cons,
	})
	c.Assert(err, gc.ErrorMatches, `invalid AWS instance type "m1.invalid" specified`)
}

func (t *localServerSuite) TestPrecheckInstanceUnsupportedArch(c *gc.C) {
	env := t.Prepare(c)
	cons := constraints.MustParse("instance-type=cc1.4xlarge arch=i386")
	err := env.PrecheckInstance(t.callCtx, environs.PrecheckInstanceParams{
		Series:      supportedversion.SupportedLTS(),
		Constraints: cons,
	})
	c.Assert(err, gc.ErrorMatches, `invalid AWS instance type "cc1.4xlarge" and arch "i386" specified`)
}

func (t *localServerSuite) TestPrecheckInstanceAvailZone(c *gc.C) {
	env := t.Prepare(c)
	placement := "zone=test-available"
	err := env.PrecheckInstance(t.callCtx, environs.PrecheckInstanceParams{
		Series:    supportedversion.SupportedLTS(),
		Placement: placement,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (t *localServerSuite) TestPrecheckInstanceAvailZoneUnavailable(c *gc.C) {
	env := t.Prepare(c)
	placement := "zone=test-unavailable"
	err := env.PrecheckInstance(t.callCtx, environs.PrecheckInstanceParams{
		Series:    supportedversion.SupportedLTS(),
		Placement: placement,
	})
	c.Assert(err, gc.ErrorMatches, `availability zone "test-unavailable" is "unavailable"`)
}

func (t *localServerSuite) TestPrecheckInstanceAvailZoneUnknown(c *gc.C) {
	env := t.Prepare(c)
	placement := "zone=test-unknown"
	err := env.PrecheckInstance(t.callCtx, environs.PrecheckInstanceParams{
		Series:    supportedversion.SupportedLTS(),
		Placement: placement,
	})
	c.Assert(err, gc.ErrorMatches, `invalid availability zone "test-unknown"`)
}

func (t *localServerSuite) TestPrecheckInstanceVolumeAvailZoneNoPlacement(c *gc.C) {
	t.testPrecheckInstanceVolumeAvailZone(c, "")
}

func (t *localServerSuite) TestPrecheckInstanceVolumeAvailZoneSameZonePlacement(c *gc.C) {
	t.testPrecheckInstanceVolumeAvailZone(c, "zone=test-available")
}

func (t *localServerSuite) testPrecheckInstanceVolumeAvailZone(c *gc.C, placement string) {
	env := t.Prepare(c)
	resp, err := t.client.CreateVolume(amzec2.CreateVolume{
		VolumeSize: 1,
		VolumeType: "gp2",
		AvailZone:  "test-available",
	})
	c.Assert(err, jc.ErrorIsNil)

	err = env.PrecheckInstance(t.callCtx, environs.PrecheckInstanceParams{
		Series:    supportedversion.SupportedLTS(),
		Placement: placement,
		VolumeAttachments: []storage.VolumeAttachmentParams{{
			AttachmentParams: storage.AttachmentParams{
				Provider: "ebs",
			},
			Volume:   names.NewVolumeTag("23"),
			VolumeId: resp.Id,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (t *localServerSuite) TestPrecheckInstanceAvailZoneVolumeConflict(c *gc.C) {
	env := t.Prepare(c)
	resp, err := t.client.CreateVolume(amzec2.CreateVolume{
		VolumeSize: 1,
		VolumeType: "gp2",
		AvailZone:  "volume-zone",
	})
	c.Assert(err, jc.ErrorIsNil)

	err = env.PrecheckInstance(t.callCtx, environs.PrecheckInstanceParams{
		Series:    supportedversion.SupportedLTS(),
		Placement: "zone=test-available",
		VolumeAttachments: []storage.VolumeAttachmentParams{{
			AttachmentParams: storage.AttachmentParams{
				Provider: "ebs",
			},
			Volume:   names.NewVolumeTag("23"),
			VolumeId: resp.Id,
		}},
	})
	c.Assert(err, gc.ErrorMatches, `cannot create instance with placement "zone=test-available", as this will prevent attaching the requested EBS volumes in zone "volume-zone"`)
}

func (t *localServerSuite) TestValidateImageMetadata(c *gc.C) {
	region := t.srv.region
	aws.Regions[region.Name] = t.srv.region
	defer delete(aws.Regions, region.Name)

	env := t.Prepare(c)
	params, err := env.(simplestreams.MetadataValidator).MetadataLookupParams("test")
	c.Assert(err, jc.ErrorIsNil)
	params.Series = supportedversion.SupportedLTS()
	params.Endpoint = region.EC2Endpoint
	params.Sources, err = environs.ImageMetadataSources(env)
	c.Assert(err, jc.ErrorIsNil)
	image_ids, _, err := imagemetadata.ValidateImageMetadata(params)
	c.Assert(err, jc.ErrorIsNil)
	sort.Strings(image_ids)
	c.Assert(image_ids, gc.DeepEquals, []string{"ami-00001133", "ami-00001135", "ami-00001139"})
}

func (t *localServerSuite) TestGetToolsMetadataSources(c *gc.C) {
	t.PatchValue(&tools.DefaultBaseURL, "")

	env := t.Prepare(c)
	sources, err := tools.GetMetadataSources(env)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sources, gc.HasLen, 0)
}

func (t *localServerSuite) TestSupportsNetworking(c *gc.C) {
	env := t.Prepare(c)
	_, supported := environs.SupportsNetworking(env)
	c.Assert(supported, jc.IsTrue)
}

func (t *localServerSuite) setUpInstanceWithDefaultVpc(c *gc.C) (environs.NetworkingEnviron, instance.Id) {
	env := t.prepareEnviron(c)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		t.callCtx, bootstrap.BootstrapParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			AdminSecret:      testing.AdminSecret,
			CAPrivateKey:     coretesting.CAKey,
		})
	c.Assert(err, jc.ErrorIsNil)

	instanceIds, err := env.ControllerInstances(t.callCtx, t.ControllerUUID)
	c.Assert(err, jc.ErrorIsNil)
	return env, instanceIds[0]
}

func (t *localServerSuite) TestNetworkInterfaces(c *gc.C) {
	env, instId := t.setUpInstanceWithDefaultVpc(c)
	interfaces, err := env.NetworkInterfaces(t.callCtx, instId)
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
	subnetId := corenetwork.Id("subnet-" + index)

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
	subnets, err := env.Subnets(t.callCtx, instId, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnets, gc.HasLen, 1)
	validateSubnets(c, subnets, "")

	interfaces, err := env.NetworkInterfaces(t.callCtx, instId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(interfaces, gc.HasLen, 1)
	c.Assert(interfaces[0].ProviderSubnetId, gc.Equals, subnets[0].ProviderId)
}

func (t *localServerSuite) TestSubnetsWithInstanceIdAndSubnetId(c *gc.C) {
	env, instId := t.setUpInstanceWithDefaultVpc(c)
	interfaces, err := env.NetworkInterfaces(t.callCtx, instId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(interfaces, gc.HasLen, 1)

	subnets, err := env.Subnets(t.callCtx, instId, []corenetwork.Id{interfaces[0].ProviderSubnetId})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnets, gc.HasLen, 1)
	c.Assert(subnets[0].ProviderId, gc.Equals, interfaces[0].ProviderSubnetId)
	validateSubnets(c, subnets, "")
}

func (t *localServerSuite) TestSubnetsWithInstanceIdMissingSubnet(c *gc.C) {
	env, instId := t.setUpInstanceWithDefaultVpc(c)
	subnets, err := env.Subnets(t.callCtx, instId, []corenetwork.Id{"missing"})
	c.Assert(err, gc.ErrorMatches, `failed to find the following subnet ids: \[missing\]`)
	c.Assert(subnets, gc.HasLen, 0)
}

func (t *localServerSuite) TestInstanceInformation(c *gc.C) {
	// TODO(macgreagoir) Where do these magic length numbers come from?
	c.Skip("Hard-coded InstanceTypes counts without explanation")
	env := t.prepareEnviron(c)
	types, err := env.InstanceTypes(t.callCtx, constraints.Value{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(types.InstanceTypes, gc.HasLen, 53)

	cons := constraints.MustParse("mem=4G")
	types, err = env.InstanceTypes(t.callCtx, cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(types.InstanceTypes, gc.HasLen, 48)
}

func validateSubnets(c *gc.C, subnets []corenetwork.SubnetInfo, vpcId corenetwork.Id) {
	// These are defined in the test server for the testing default
	// VPC.
	defaultSubnets := []corenetwork.SubnetInfo{{
		CIDR:              "10.10.0.0/24",
		ProviderId:        "subnet-0",
		ProviderNetworkId: vpcId,
		VLANTag:           0,
		AvailabilityZones: []string{"test-available2"},
	}, {
		CIDR:              "10.10.1.0/24",
		ProviderId:        "subnet-1",
		ProviderNetworkId: vpcId,
		VLANTag:           0,
		AvailabilityZones: []string{"test-available"},
	}, {
		CIDR:              "10.10.2.0/24",
		ProviderId:        "subnet-2",
		ProviderNetworkId: vpcId,
		VLANTag:           0,
		AvailabilityZones: []string{"test-impaired"},
	}, {
		CIDR:              "10.10.3.0/24",
		ProviderId:        "subnet-3",
		ProviderNetworkId: vpcId,
		VLANTag:           0,
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
		c.Check(subnet, jc.DeepEquals, defaultSubnets[index])
	}
}

func (t *localServerSuite) TestSubnets(c *gc.C) {
	env, _ := t.setUpInstanceWithDefaultVpc(c)

	subnets, err := env.Subnets(t.callCtx, instance.UnknownId, []corenetwork.Id{"subnet-0"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnets, gc.HasLen, 1)
	validateSubnets(c, subnets, "vpc-0")

	subnets, err = env.Subnets(t.callCtx, instance.UnknownId, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnets, gc.HasLen, 4)
	validateSubnets(c, subnets, "vpc-0")
}

func (t *localServerSuite) TestSubnetsMissingSubnet(c *gc.C) {
	env, _ := t.setUpInstanceWithDefaultVpc(c)

	_, err := env.Subnets(t.callCtx, "", []corenetwork.Id{"subnet-0", "Missing"})
	c.Assert(err, gc.ErrorMatches, `failed to find the following subnet ids: \[Missing\]`)
}

func (t *localServerSuite) TestInstanceTags(c *gc.C) {
	env := t.prepareAndBootstrap(c)

	instances, err := env.AllRunningInstances(t.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instances, gc.HasLen, 1)

	ec2Inst := ec2.InstanceEC2(instances[0])
	c.Assert(ec2Inst.Tags, jc.SameContents, []amzec2.Tag{
		{"Name", "juju-sample-machine-0"},
		{"juju-model-uuid", coretesting.ModelTag.Id()},
		{"juju-controller-uuid", t.ControllerUUID},
		{"juju-is-controller", "true"},
	})
}

func (t *localServerSuite) TestRootDiskTags(c *gc.C) {
	env := t.prepareAndBootstrap(c)

	instances, err := env.AllRunningInstances(t.callCtx)
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
		{"juju-controller-uuid", t.ControllerUUID},
	})
}

func (s *localServerSuite) TestBootstrapInstanceConstraints(c *gc.C) {
	env := s.prepareAndBootstrap(c)
	inst, err := env.AllRunningInstances(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(inst, gc.HasLen, 1)
	ec2inst := ec2.InstanceEC2(inst[0])
	// Controllers should be started with a burstable
	// instance if possible, and a 32 GiB disk.
	c.Assert(ec2inst.InstanceType, gc.Equals, "t3.medium")
}

func makeFilter(key string, values ...string) *amzec2.Filter {
	result := amzec2.NewFilter()
	result.Add(key, values...)
	return result
}

func (s *localServerSuite) TestAdoptResources(c *gc.C) {
	controllerEnv := s.prepareAndBootstrap(c)
	controllerInsts, err := controllerEnv.AllRunningInstances(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerInsts, gc.HasLen, 1)

	controllerVolumes, err := ec2.AllModelVolumes(controllerEnv, s.callCtx)
	c.Assert(err, jc.ErrorIsNil)

	controllerGroups, err := ec2.AllModelGroups(controllerEnv, s.callCtx)
	c.Assert(err, jc.ErrorIsNil)

	// Create a hosted model environment with an instance and a volume.
	hostedModelUUID := "7e386e08-cba7-44a4-a76e-7c1633584210"
	s.srv.ec2srv.SetInitialInstanceState(ec2test.Running)
	cfg, err := controllerEnv.Config().Apply(map[string]interface{}{
		"uuid":          hostedModelUUID,
		"firewall-mode": "global",
	})
	c.Assert(err, jc.ErrorIsNil)
	env, err := environs.New(environs.OpenParams{
		Cloud:  s.CloudSpec(),
		Config: cfg,
	})
	c.Assert(err, jc.ErrorIsNil)
	inst, _ := testing.AssertStartInstance(c, env, s.callCtx, s.ControllerUUID, "0")
	c.Assert(err, jc.ErrorIsNil)
	ebsProvider, err := env.StorageProvider(ec2.EBS_ProviderType)
	c.Assert(err, jc.ErrorIsNil)
	vs, err := ebsProvider.VolumeSource(nil)
	c.Assert(err, jc.ErrorIsNil)
	volumeResults, err := vs.CreateVolumes(s.callCtx, []storage.VolumeParams{{
		Tag:      names.NewVolumeTag("0"),
		Size:     1024,
		Provider: ec2.EBS_ProviderType,
		ResourceTags: map[string]string{
			tags.JujuController: s.ControllerUUID,
			tags.JujuModel:      hostedModelUUID,
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

	modelVolumes, err := ec2.AllModelVolumes(env, s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	allVolumes := append([]string{}, controllerVolumes...)
	allVolumes = append(allVolumes, modelVolumes...)

	modelGroups, err := ec2.AllModelGroups(env, s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	allGroups := append([]string{}, controllerGroups...)
	allGroups = append(allGroups, modelGroups...)

	ec2conn := ec2.EnvironEC2(env)

	origController := coretesting.ControllerTag.Id()

	checkInstanceTags := func(controllerUUID string, expectedIds ...string) {
		resp, err := ec2conn.Instances(
			nil, makeFilter("tag:"+tags.JujuController, controllerUUID))
		c.Assert(err, jc.ErrorIsNil)
		actualIds := set.NewStrings()
		for _, reservation := range resp.Reservations {
			for _, instance := range reservation.Instances {
				actualIds.Add(instance.InstanceId)
			}
		}
		c.Check(actualIds, gc.DeepEquals, set.NewStrings(expectedIds...))
	}

	checkVolumeTags := func(controllerUUID string, expectedIds ...string) {
		resp, err := ec2conn.Volumes(
			nil, makeFilter("tag:"+tags.JujuController, controllerUUID))
		c.Assert(err, jc.ErrorIsNil)
		actualIds := set.NewStrings()
		for _, vol := range resp.Volumes {
			actualIds.Add(vol.Id)
		}
		c.Check(actualIds, gc.DeepEquals, set.NewStrings(expectedIds...))
	}

	checkGroupTags := func(controllerUUID string, expectedIds ...string) {
		resp, err := ec2conn.SecurityGroups(
			nil, makeFilter("tag:"+tags.JujuController, controllerUUID))
		c.Assert(err, jc.ErrorIsNil)
		actualIds := set.NewStrings()
		for _, group := range resp.Groups {
			actualIds.Add(group.Id)
		}
		c.Check(actualIds, gc.DeepEquals, set.NewStrings(expectedIds...))
	}

	checkInstanceTags(origController, string(inst.Id()), string(controllerInsts[0].Id()))
	checkVolumeTags(origController, allVolumes...)
	checkGroupTags(origController, allGroups...)

	err = env.AdoptResources(s.callCtx, "new-controller", version.MustParse("0.0.1"))
	c.Assert(err, jc.ErrorIsNil)

	checkInstanceTags("new-controller", string(inst.Id()))
	checkInstanceTags(origController, string(controllerInsts[0].Id()))
	checkVolumeTags("new-controller", modelVolumes...)
	checkVolumeTags(origController, controllerVolumes...)
	checkGroupTags("new-controller", modelGroups...)
	checkGroupTags(origController, controllerGroups...)
}

// localNonUSEastSuite is similar to localServerSuite but the S3 mock server
// behaves as if it is not in the us-east region.
type localNonUSEastSuite struct {
	coretesting.BaseSuite
	sstesting.TestDataSuite

	srv localServer
	env environs.Environ
}

func (t *localNonUSEastSuite) SetUpSuite(c *gc.C) {
	t.BaseSuite.SetUpSuite(c)
	t.TestDataSuite.SetUpSuite(c)

	t.PatchValue(&imagemetadata.SimplestreamsImagesPublicKey, sstesting.SignedMetadataPublicKey)
	t.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)
	t.BaseSuite.PatchValue(ec2.DeleteSecurityGroupInsistently, deleteSecurityGroupForTestFunc)
}

func (t *localNonUSEastSuite) TearDownSuite(c *gc.C) {
	t.TestDataSuite.TearDownSuite(c)
	t.BaseSuite.TearDownSuite(c)
}

func (t *localNonUSEastSuite) SetUpTest(c *gc.C) {
	t.BaseSuite.SetUpTest(c)
	t.srv.startServer(c)

	region := t.srv.region
	credential := cloud.NewCredential(
		cloud.AccessKeyAuthType,
		map[string]string{
			"access-key": "x",
			"secret-key": "x",
		},
	)
	restoreEC2Patching := patchEC2ForTesting(c, region)
	t.AddCleanup(func(c *gc.C) { restoreEC2Patching() })

	env, err := bootstrap.PrepareController(
		false,
		envtesting.BootstrapContext(c),
		jujuclient.NewMemStore(),
		bootstrap.PrepareParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			ModelConfig:      localConfigAttrs,
			ControllerName:   localConfigAttrs["name"].(string),
			Cloud: environs.CloudSpec{
				Type:       "ec2",
				Region:     region.Name,
				Endpoint:   region.EC2Endpoint,
				Credential: &credential,
			},
			AdminSecret: testing.AdminSecret,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	t.env = env.(environs.Environ)
}

func (t *localNonUSEastSuite) TearDownTest(c *gc.C) {
	t.srv.stopServer(c)
	t.BaseSuite.TearDownTest(c)
}

func patchEC2ForTesting(c *gc.C, region aws.Region) func() {
	ec2.UseTestImageData(c, ec2.MakeTestImageStreamsData(region))
	restoreTimeouts := envtesting.PatchAttemptStrategies(ec2.ShortAttempt)
	restoreFinishBootstrap := envtesting.DisableFinishBootstrap()
	return func() {
		restoreFinishBootstrap()
		restoreTimeouts()
		ec2.UseTestImageData(c, nil)
	}
}

// If match is true, CheckScripts checks that at least one script started
// by the cloudinit data matches the given regexp pattern, otherwise it
// checks that no script matches.  It's exported so it can be used by tests
// defined in ec2_test.
func CheckScripts(c *gc.C, userDataMap map[string]interface{}, pattern string, match bool) {
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
func CheckPackage(c *gc.C, userDataMap map[string]interface{}, pkg string, match bool) {
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
