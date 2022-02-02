// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	stdcontext "context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/smithy-go"
	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/os/v2/series"
	"github.com/juju/retry"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/utils/v3/arch"
	"github.com/juju/version/v2"
	"github.com/kr/pretty"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/filestorage"
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
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/ec2"
	ec2test "github.com/juju/juju/provider/ec2/internal/testing"
	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
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

// localServer represents a fake EC2 server running within
// the test process itself.
type localServer struct {
	// createRootDisks is used to decide whether or not
	// the ec2test server will create root disks for
	// instances.
	createRootDisks bool

	ec2srv *ec2test.Server
	iamsrv *ec2test.IAMServer
	region types.Region

	defaultVPC *types.Vpc
	zones      []types.AvailabilityZone
	subnets    []types.Subnet
}

func (srv *localServer) startServer(c *gc.C) {
	var err error
	srv.ec2srv, err = ec2test.NewServer()
	if err != nil {
		c.Fatalf("cannot start ec2 test server: %v", err)
	}
	srv.iamsrv, err = ec2test.NewIAMServer()
	if err != nil {
		c.Fatalf("cannot start iam test server: %v", err)
	}
	srv.ec2srv.SetCreateRootDisks(srv.createRootDisks)
	srv.addSpice()

	srv.region = types.Region{
		RegionName: aws.String("test"),
		Endpoint:   aws.String("http://foo"),
	}
	regionName := srv.region.RegionName
	zones := make([]types.AvailabilityZone, 4)
	zones[0].RegionName = regionName
	zones[0].ZoneName = aws.String("test-available")
	zones[0].State = "available"
	zones[1].RegionName = regionName
	zones[1].ZoneName = aws.String("test-impaired")
	zones[1].State = "impaired"
	zones[2].RegionName = regionName
	zones[2].ZoneName = aws.String("test-unavailable")
	zones[2].State = "unavailable"
	zones[3].RegionName = regionName
	zones[3].ZoneName = aws.String("test-available2")
	zones[3].State = "available"
	srv.ec2srv.SetAvailabilityZones(zones)
	srv.ec2srv.SetInitialInstanceState(ec2test.Pending)
	srv.zones = zones

	defaultVPC, err := srv.ec2srv.AddDefaultVpcAndSubnets()
	c.Assert(err, jc.ErrorIsNil)
	srv.defaultVPC = &defaultVPC
}

// addSpice adds some "spice" to the local server
// by adding state that may cause tests to fail.
func (srv *localServer) addSpice() {
	states := []types.InstanceState{
		ec2test.ShuttingDown,
		ec2test.Terminated,
		ec2test.Stopped,
	}
	for _, state := range states {
		srv.ec2srv.NewInstances(1, "m1.small", "ami-a7f539ce", state, nil)
	}
}

func (srv *localServer) stopServer(c *gc.C) {
	srv.iamsrv.Reset()
	srv.ec2srv.Reset(false)
	srv.defaultVPC = nil
}

func bootstrapClientFunc(ec2Client ec2.Client) ec2.ClientFunc {
	return func(ctx stdcontext.Context, spec cloudspec.CloudSpec, options ...ec2.ClientOption) (ec2.Client, error) {
		credentialAttrs := spec.Credential.Attributes()
		accessKey := credentialAttrs["access-key"]
		secretKey := credentialAttrs["secret-key"]
		if spec.Region != "test" {
			return nil, fmt.Errorf("expected region %q, got %q",
				"test", spec.Region)
		}
		if accessKey != "x" || secretKey != "x" {
			return nil, fmt.Errorf("expected access:secret %q, got %q",
				"x:x", accessKey+":"+secretKey)
		}
		return ec2Client, nil
	}
}

func bootstrapIAMClientFunc(iamClient ec2.IAMClient) ec2.IAMClientFunc {
	return func(ctx stdcontext.Context, spec cloudspec.CloudSpec, options ...ec2.ClientOption) (ec2.IAMClient, error) {
		credentialAttrs := spec.Credential.Attributes()
		accessKey := credentialAttrs["access-key"]
		secretKey := credentialAttrs["secret-key"]
		if spec.Region != "test" {
			return nil, fmt.Errorf("expected region %q, got %q",
				"test", spec.Region)
		}
		if accessKey != "x" || secretKey != "x" {
			return nil, fmt.Errorf("expected access:secret %q, got %q",
				"x:x", accessKey+":"+secretKey)
		}
		return iamClient, nil
	}
}

func bootstrapContextWithClientFunc(
	c *gc.C,
	clientFunc ec2.ClientFunc,
	iamClientFunc ec2.IAMClientFunc,
) environs.BootstrapContext {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())

	ctx := stdcontext.TODO()
	ctx = stdcontext.WithValue(ctx, bootstrap.SimplestreamsFetcherContextKey, ss)
	if clientFunc != nil {
		ctx = stdcontext.WithValue(ctx, ec2.AWSClientContextKey, clientFunc)
	}
	if iamClientFunc != nil {
		ctx = stdcontext.WithValue(ctx, ec2.AWSIAMClientContextKey, iamClientFunc)
	}
	return envtesting.BootstrapContext(ctx, c)
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
	srv        localServer
	client     ec2.Client
	iamClient  ec2.IAMClient
	useIAMRole bool

	callCtx context.ProviderCallContext
}

var _ = gc.Suite(&localServerSuite{})

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
	t.BaseSuite.PatchValue(&series.HostSeries, func() (string, error) { return jujuversion.DefaultSupportedLTS(), nil })
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
	t.CloudRegion = aws.ToString(region.RegionName)
	t.CloudEndpoint = aws.ToString(region.Endpoint)
	t.client = t.srv.ec2srv
	t.iamClient = t.srv.iamsrv
	restoreEC2Patching := patchEC2ForTesting(c, region)
	t.AddCleanup(func(c *gc.C) { restoreEC2Patching() })
	t.Tests.SetUpTest(c)

	t.Tests.BootstrapContext = bootstrapContextWithClientFunc(c, bootstrapClientFunc(t.client), bootstrapIAMClientFunc(t.iamClient))
	t.Tests.ProviderCallContext = context.NewCloudCallContext(t.Tests.BootstrapContext.Context())
	t.callCtx = context.NewCloudCallContext(t.Tests.BootstrapContext.Context())
	t.useIAMRole = false
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
	notRecommendedVPCIDConfig := coretesting.Attrs{"vpc-id": aws.ToString(t.srv.defaultVPC.VpcId)}

	expectedError := `The given vpc-id does not meet one or more(.|\n)*Error details: VPC has unexpected state "unavailable"`
	err := t.AssertPrepareFailsWithConfig(c, notRecommendedVPCIDConfig, expectedError)
	c.Check(err, jc.Satisfies, ec2.IsVPCNotRecommendedError)
}

func (t *localServerSuite) makeTestingDefaultVPCUnavailable(c *gc.C) {
	// For simplicity, here the test server's default VPC is updated to change
	// its state to unavailable, we just verify the behavior of a "not
	// recommended VPC".
	t.srv.defaultVPC.State = "unavailable"
	err := t.srv.ec2srv.UpdateVpc(*t.srv.defaultVPC)
	c.Assert(err, jc.ErrorIsNil)
}

func (t *localServerSuite) TestPrepareForBootstrapWithNotRecommendedButForcedVPCID(c *gc.C) {
	t.makeTestingDefaultVPCUnavailable(c)
	params := t.PrepareParams(c)
	vpcID := aws.ToString(t.srv.defaultVPC.VpcId)
	params.ModelConfig["vpc-id"] = vpcID
	params.ModelConfig["vpc-id-force"] = true

	t.prepareWithParamsAndBootstrapWithVPCID(c, params, vpcID)
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

	err := bootstrap.Bootstrap(t.BootstrapContext, env,
		t.callCtx, bootstrap.BootstrapParams{
			ControllerConfig:         coretesting.FakeControllerConfig(),
			AdminSecret:              testing.AdminSecret,
			CAPrivateKey:             coretesting.CAKey,
			Placement:                "zone=test-available",
			SupportedBootstrapSeries: coretesting.FakeSupportedJujuSeries,
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
	vpcID := aws.ToString(t.srv.defaultVPC.VpcId)
	params.ModelConfig["vpc-id"] = vpcID

	t.prepareWithParamsAndBootstrapWithVPCID(c, params, vpcID)
}

func (t *localServerSuite) TestSystemdBootstrapInstanceUserDataAndState(c *gc.C) {
	env := t.Prepare(c)
	err := bootstrap.Bootstrap(t.BootstrapContext, env,
		t.callCtx, bootstrap.BootstrapParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			// TODO(redir): BBB: When we no longer support upstart based systems this can change to series.LatestLts()
			BootstrapSeries:          "xenial",
			AdminSecret:              testing.AdminSecret,
			CAPrivateKey:             coretesting.CAKey,
			SupportedBootstrapSeries: set.NewStrings("xenial").Union(coretesting.FakeSupportedJujuSeries),
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
	err := bootstrap.Bootstrap(t.BootstrapContext, env,
		t.callCtx, bootstrap.BootstrapParams{
			ControllerConfig:         coretesting.FakeControllerConfig(),
			BootstrapSeries:          "trusty",
			AdminSecret:              testing.AdminSecret,
			CAPrivateKey:             coretesting.CAKey,
			SupportedBootstrapSeries: coretesting.FakeSupportedJujuSeries,
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
	err := bootstrap.Bootstrap(t.BootstrapContext, env,
		t.callCtx, bootstrap.BootstrapParams{
			ControllerConfig:         coretesting.FakeControllerConfig(),
			AdminSecret:              testing.AdminSecret,
			CAPrivateKey:             coretesting.CAKey,
			SupportedBootstrapSeries: coretesting.FakeSupportedJujuSeries,
		})
	c.Assert(err, jc.ErrorIsNil)

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
	t.BaseSuite.PatchValue(ec2.TerminateInstancesById, func(ec2.Client, context.ProviderCallContext, ...instance.Id) ([]types.InstanceStateChange, error) {
		return nil, errors.New(msg)
	})

	err := env.Destroy(t.callCtx)
	c.Assert(errors.Cause(err).Error(), jc.Contains, msg)
}

func (t *localServerSuite) TestIAMRoleCleanup(c *gc.C) {
	t.useIAMRole = true
	t.srv.ec2srv.SetInitialInstanceState(ec2test.Running)
	env := t.prepareAndBootstrap(c)

	res, err := t.iamClient.ListInstanceProfiles(stdcontext.Background(), &iam.ListInstanceProfilesInput{
		PathPrefix: aws.String(fmt.Sprintf("/juju/controller/%s/", t.ControllerUUID)),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(res.InstanceProfiles), gc.Equals, 1)

	res1, err := t.iamClient.ListRoles(stdcontext.Background(), &iam.ListRolesInput{
		PathPrefix: aws.String(fmt.Sprintf("/juju/controller/%s/", t.ControllerUUID)),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(res1.Roles), gc.Equals, 1)

	err = env.DestroyController(t.callCtx, t.ControllerUUID)
	c.Assert(err, jc.ErrorIsNil)

	res, err = t.iamClient.ListInstanceProfiles(stdcontext.Background(), &iam.ListInstanceProfilesInput{
		PathPrefix: aws.String(fmt.Sprintf("/juju/controller/%s/", t.ControllerUUID)),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(res.InstanceProfiles), gc.Equals, 0)

	res1, err = t.iamClient.ListRoles(stdcontext.Background(), &iam.ListRolesInput{
		PathPrefix: aws.String(fmt.Sprintf("/juju/controller/%s/", t.ControllerUUID)),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(res1.Roles), gc.Equals, 0)
}

func (t *localServerSuite) TestIAMRolePermissionProblems(c *gc.C) {
	t.srv.ec2srv.SetInitialInstanceState(ec2test.Running)
	t.srv.iamsrv.ProducePermissionError(true)
	defer t.srv.iamsrv.ProducePermissionError(false)
	env := t.prepareAndBootstrap(c)

	err := env.DestroyController(t.callCtx, t.ControllerUUID)
	c.Assert(err, jc.ErrorIsNil)
}

func (t *localServerSuite) TestGetTerminatedInstances(c *gc.C) {
	env := t.Prepare(c)
	err := bootstrap.Bootstrap(t.BootstrapContext, env,
		t.callCtx, bootstrap.BootstrapParams{
			ControllerConfig:         coretesting.FakeControllerConfig(),
			AdminSecret:              testing.AdminSecret,
			CAPrivateKey:             coretesting.CAKey,
			SupportedBootstrapSeries: coretesting.FakeSupportedJujuSeries,
		})
	c.Assert(err, jc.ErrorIsNil)

	// create another instance to terminate
	inst1, _ := testing.AssertStartInstance(c, env, t.callCtx, t.ControllerUUID, "1")
	inst := t.srv.ec2srv.Instance(string(inst1.Id()))
	c.Assert(inst, gc.NotNil)
	t.BaseSuite.PatchValue(ec2.TerminateInstancesById, func(client ec2.Client, ctx context.ProviderCallContext, ids ...instance.Id) ([]types.InstanceStateChange, error) {
		// Terminate the one destined for termination and
		// err out to ensure that one instance will be terminated, the other - not.
		_, err = client.TerminateInstances(ctx, &awsec2.TerminateInstancesInput{
			InstanceIds: []string{string(inst1.Id())},
		})
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

func (t *localServerSuite) TestInstanceSecurityGroupsWithInstanceStatusFilter(c *gc.C) {
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
		ec2.SecurityGroupCleaner, context.ProviderCallContext, types.GroupIdentifier, clock.Clock,
	) error {
		return errors.New(msg)
	})
	err := env.DestroyController(t.callCtx, t.ControllerUUID)
	c.Assert(err, gc.ErrorMatches, "destroying managed models: "+msg)
}

func (t *localServerSuite) TestDestroyHostedModelDeleteSecurityGroupInsistentlyError(c *gc.C) {
	env := t.prepareAndBootstrap(c)
	hostedEnv, err := environs.New(t.BootstrapContext.Context(), environs.OpenParams{
		Cloud:  t.CloudSpec(),
		Config: env.Config(),
	})
	c.Assert(err, jc.ErrorIsNil)

	msg := "destroy security group error"
	t.BaseSuite.PatchValue(ec2.DeleteSecurityGroupInsistently, func(
		ec2.SecurityGroupCleaner, context.ProviderCallContext, types.GroupIdentifier, clock.Clock,
	) error {
		return errors.New(msg)
	})
	err = hostedEnv.Destroy(t.callCtx)
	c.Assert(err, gc.ErrorMatches, "cannot delete model security groups: "+msg)
}

func (t *localServerSuite) TestDestroyControllerDestroysHostedModelResources(c *gc.C) {
	controllerEnv := t.prepareAndBootstrap(c)

	// Create a hosted model with an instance and a volume.
	hostedModelUUID := "7e386e08-cba7-44a4-a76e-7c1633584210"
	t.srv.ec2srv.SetInitialInstanceState(ec2test.Running)
	cfg, err := controllerEnv.Config().Apply(map[string]interface{}{
		"uuid":          hostedModelUUID,
		"firewall-mode": "global",
	})
	c.Assert(err, jc.ErrorIsNil)
	env, err := environs.New(t.BootstrapContext.Context(), environs.OpenParams{
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
		groupsResp, err := t.client.DescribeSecurityGroups(t.callCtx, nil)
		c.Assert(err, jc.ErrorIsNil)
		names := make([]string, len(groupsResp.SecurityGroups))
		for i, group := range groupsResp.SecurityGroups {
			names[i] = aws.ToString(group.GroupName)
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
	// model too.
	err = controllerEnv.DestroyController(t.callCtx, t.ControllerUUID)
	c.Assert(err, jc.ErrorIsNil)

	assertInstances()
	assertVolumes()
	assertGroups("default")
}

func (t *localServerSuite) TestInstanceStatus(c *gc.C) {
	env := t.Prepare(c)
	err := bootstrap.Bootstrap(t.BootstrapContext, env,
		t.callCtx, bootstrap.BootstrapParams{
			ControllerConfig:         coretesting.FakeControllerConfig(),
			AdminSecret:              testing.AdminSecret,
			CAPrivateKey:             coretesting.CAKey,
			SupportedBootstrapSeries: coretesting.FakeSupportedJujuSeries,
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
	ec2Inst := ec2.InstanceSDKEC2(inst)
	c.Assert(aws.ToString(ec2Inst.Placement.AvailabilityZone), gc.Equals, "test-available")
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
	resp, err := t.client.CreateVolume(t.callCtx, &awsec2.CreateVolumeInput{
		Size:             aws.Int32(1),
		VolumeType:       "gp2",
		AvailabilityZone: aws.String("volume-zone"),
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
			VolumeId: aws.ToString(resp.VolumeId),
		}},
	}
	result, err := testing.StartInstanceWithParams(env, t.callCtx, "1", args)
	c.Assert(err, jc.ErrorIsNil)
	ec2Inst := ec2.InstanceSDKEC2(result.Instance)
	c.Assert(aws.ToString(ec2Inst.Placement.AvailabilityZone), gc.Equals, "volume-zone")
}

func (t *localServerSuite) TestStartInstanceVolumeAttachmentsAvailZonePlacementConflicts(c *gc.C) {
	env := t.prepareAndBootstrap(c)
	resp, err := t.client.CreateVolume(t.callCtx, &awsec2.CreateVolumeInput{
		Size:             aws.Int32(1),
		VolumeType:       "gp2",
		AvailabilityZone: aws.String("volume-zone"),
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
			VolumeId: aws.ToString(resp.VolumeId),
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
	ec2Inst := ec2.InstanceSDKEC2(inst)
	c.Assert(aws.ToString(ec2Inst.Placement.AvailabilityZone), gc.Equals, "test-available")
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
		SubnetsToZones: []map[corenetwork.Id][]string{{
			subIDs[0]: {"test-available"},
			subIDs[1]: {"test-available"},
			subIDs[2]: {"test-unavailable"},
		}},
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
		SubnetsToZones: []map[corenetwork.Id][]string{{
			subIDs[0]: {"test-available"},
			subIDs[1]: {"test-available"},
			subIDs[2]: {"test-unavailable"},
		}},
	}
	zonedEnviron := env.(common.ZonedEnviron)
	_, err := zonedEnviron.DeriveAvailabilityZones(t.callCtx, params)
	c.Assert(err, gc.ErrorMatches, `unknown placement directive: subnet=0.1.2.0/24`)
}

func (t *localServerSuite) TestGetAvailabilityZones(c *gc.C) {
	var resultZones []types.AvailabilityZone
	var resultErr error
	t.PatchValue(ec2.EC2AvailabilityZones, func(c ec2.Client, ctx stdcontext.Context, params *awsec2.DescribeAvailabilityZonesInput, optFns ...func(*awsec2.Options)) (*awsec2.DescribeAvailabilityZonesOutput, error) {
		resp := &awsec2.DescribeAvailabilityZonesOutput{
			AvailabilityZones: append([]types.AvailabilityZone{}, resultZones...),
		}
		return resp, resultErr
	})
	env := t.Prepare(c).(common.ZonedEnviron)

	resultErr = fmt.Errorf("failed to get availability zones")
	zones, err := env.AvailabilityZones(t.callCtx)
	c.Assert(err, gc.Equals, resultErr)
	c.Assert(zones, gc.IsNil)

	resultErr = nil
	resultZones = make([]types.AvailabilityZone, 1)
	resultZones[0].ZoneName = aws.String("whatever")
	zones, err = env.AvailabilityZones(t.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zones, gc.HasLen, 1)
	c.Assert(zones[0].Name(), gc.Equals, "whatever")

	// A successful result is cached, currently for the lifetime
	// of the Environ. This will change if/when we have long-lived
	// Environs to cut down repeated IaaS requests.
	resultErr = fmt.Errorf("failed to get availability zones")
	resultZones[0].ZoneName = aws.String("andever")
	zones, err = env.AvailabilityZones(t.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zones, gc.HasLen, 1)
	c.Assert(zones[0].Name(), gc.Equals, "whatever")
}

func (t *localServerSuite) TestGetAvailabilityZonesCommon(c *gc.C) {
	var resultZones []types.AvailabilityZone
	t.PatchValue(ec2.EC2AvailabilityZones, func(c ec2.Client, ctx stdcontext.Context, params *awsec2.DescribeAvailabilityZonesInput, optFns ...func(*awsec2.Options)) (*awsec2.DescribeAvailabilityZonesOutput, error) {
		resp := &awsec2.DescribeAvailabilityZonesOutput{
			AvailabilityZones: append([]types.AvailabilityZone{}, resultZones...),
		}
		return resp, nil
	})
	env := t.Prepare(c).(common.ZonedEnviron)
	resultZones = make([]types.AvailabilityZone, 2)
	resultZones[0].ZoneName = aws.String("az1")
	resultZones[1].ZoneName = aws.String("az2")
	resultZones[0].State = "available"
	resultZones[1].State = "impaired"
	zones, err := env.AvailabilityZones(t.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zones, gc.HasLen, 2)
	c.Assert(zones[0].Name(), gc.Equals, "az1")
	c.Assert(zones[1].Name(), gc.Equals, "az2")
	c.Assert(zones[0].Available(), jc.IsTrue)
	c.Assert(zones[1].Available(), jc.IsFalse)
}

func (t *localServerSuite) TestDeriveAvailabilityZones(c *gc.C) {
	var resultZones []types.AvailabilityZone
	t.PatchValue(ec2.EC2AvailabilityZones, func(c ec2.Client, ctx stdcontext.Context, params *awsec2.DescribeAvailabilityZonesInput, optFns ...func(*awsec2.Options)) (*awsec2.DescribeAvailabilityZonesOutput, error) {
		resp := &awsec2.DescribeAvailabilityZonesOutput{
			AvailabilityZones: append([]types.AvailabilityZone{}, resultZones...),
		}
		return resp, nil
	})
	env := t.Prepare(c).(common.ZonedEnviron)
	resultZones = make([]types.AvailabilityZone, 2)
	resultZones[0].ZoneName = aws.String("az1")
	resultZones[1].ZoneName = aws.String("az2")
	resultZones[0].State = "available"
	resultZones[1].State = "impaired"

	zones, err := env.DeriveAvailabilityZones(t.callCtx, environs.StartInstanceParams{Placement: "zone=az1"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zones, gc.DeepEquals, []string{"az1"})
}

func (t *localServerSuite) TestDeriveAvailabilityZonesImpaired(c *gc.C) {
	var resultZones []types.AvailabilityZone
	t.PatchValue(ec2.EC2AvailabilityZones, func(c ec2.Client, ctx stdcontext.Context, params *awsec2.DescribeAvailabilityZonesInput, optFns ...func(*awsec2.Options)) (*awsec2.DescribeAvailabilityZonesOutput, error) {
		resp := &awsec2.DescribeAvailabilityZonesOutput{
			AvailabilityZones: append([]types.AvailabilityZone{}, resultZones...),
		}
		return resp, nil
	})
	env := t.Prepare(c).(common.ZonedEnviron)
	resultZones = make([]types.AvailabilityZone, 2)
	resultZones[0].ZoneName = aws.String("az1")
	resultZones[1].ZoneName = aws.String("az2")
	resultZones[0].State = "available"
	resultZones[1].State = "impaired"

	zones, err := env.DeriveAvailabilityZones(t.callCtx, environs.StartInstanceParams{Placement: "zone=az2"})
	c.Assert(err, gc.ErrorMatches, "availability zone \"az2\" is \"impaired\"")
	c.Assert(zones, gc.HasLen, 0)
}

func (t *localServerSuite) TestDeriveAvailabilityZonesConflictVolume(c *gc.C) {
	resp, err := t.client.CreateVolume(t.callCtx, &awsec2.CreateVolumeInput{
		Size:             aws.Int32(1),
		VolumeType:       "gp2",
		AvailabilityZone: aws.String("volume-zone"),
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
			VolumeId: aws.ToString(resp.VolumeId),
		}},
	}
	env := t.Prepare(c).(common.ZonedEnviron)
	zones, err := env.DeriveAvailabilityZones(t.callCtx, args)
	c.Assert(err, gc.ErrorMatches, `cannot create instance with placement "zone=test-available", as this will prevent attaching the requested EBS volumes in zone "volume-zone"`)
	c.Assert(zones, gc.HasLen, 0)
}

func (t *localServerSuite) TestDeriveAvailabilityZonesVolumeNoPlacement(c *gc.C) {
	resp, err := t.client.CreateVolume(t.callCtx, &awsec2.CreateVolumeInput{
		Size:             aws.Int32(1),
		VolumeType:       "gp2",
		AvailabilityZone: aws.String("volume-zone"),
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
			VolumeId: aws.ToString(resp.VolumeId),
		}},
	}
	env := t.Prepare(c).(common.ZonedEnviron)
	zones, err := env.DeriveAvailabilityZones(t.callCtx, args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zones, gc.DeepEquals, []string{"volume-zone"})
}

var azConstrainedErr = &smithy.GenericAPIError{
	Code:    "Unsupported",
	Message: "The requested Availability Zone is currently constrained etc.",
}

var azVolumeTypeNotAvailableInZoneErr = &smithy.GenericAPIError{
	Code:    "VolumeTypeNotAvailableInZone",
	Message: "blah blah",
}

var azInsufficientInstanceCapacityErr = &smithy.GenericAPIError{
	Code: "InsufficientInstanceCapacity",
	Message: "We currently do not have sufficient m1.small capacity in the " +
		"Availability Zone you requested (us-east-1d). Our system will " +
		"be working on provisioning additional capacity. You can currently get m1.small " +
		"capacity by not specifying an Availability Zone in your request or choosing " +
		"us-east-1c, us-east-1a.",
}

var azNoDefaultSubnetErr = &smithy.GenericAPIError{
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

func (t *localServerSuite) testStartInstanceAvailZoneAllConstrained(c *gc.C, runInstancesError smithy.APIError) {
	env := t.prepareAndBootstrap(c)

	t.PatchValue(ec2.RunInstances, func(e ec2.Client, ctx context.ProviderCallContext, ri *awsec2.RunInstancesInput, callback environs.StatusCallbackFunc) (resp *awsec2.RunInstancesOutput, err error) {
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
	c.Assert(errors.Details(err), jc.Contains, runInstancesError.ErrorMessage())
}

// addTestingSubnets adds a testing default VPC with 3 subnets in the EC2 test
// server: 2 of the subnets are in the "test-available" AZ, the remaining - in
// "test-unavailable". Returns a slice with the IDs of the created subnets and
// vpc id that those were added to
func (t *localServerSuite) addTestingSubnets(c *gc.C) ([]corenetwork.Id, string) {
	vpc := t.srv.ec2srv.AddVpc(types.Vpc{
		CidrBlock: aws.String("0.1.0.0/16"),
		IsDefault: aws.Bool(true),
	})
	results := make([]corenetwork.Id, 3)
	sub1, err := t.srv.ec2srv.AddSubnet(types.Subnet{
		VpcId:            vpc.VpcId,
		CidrBlock:        aws.String("0.1.2.0/24"),
		AvailabilityZone: aws.String("test-available"),
		State:            "available",
		DefaultForAz:     aws.Bool(true),
	})
	c.Assert(err, jc.ErrorIsNil)
	results[0] = corenetwork.Id(aws.ToString(sub1.SubnetId))
	sub2, err := t.srv.ec2srv.AddSubnet(types.Subnet{
		VpcId:            vpc.VpcId,
		CidrBlock:        aws.String("0.1.3.0/24"),
		AvailabilityZone: aws.String("test-available"),
		State:            "unavailable",
	})
	c.Assert(err, jc.ErrorIsNil)
	results[1] = corenetwork.Id(aws.ToString(sub2.SubnetId))
	sub3, err := t.srv.ec2srv.AddSubnet(types.Subnet{
		VpcId:            vpc.VpcId,
		CidrBlock:        aws.String("0.1.4.0/24"),
		AvailabilityZone: aws.String("test-unavailable"),
		DefaultForAz:     aws.Bool(true),
		State:            "unavailable",
	})
	c.Assert(err, jc.ErrorIsNil)
	results[2] = corenetwork.Id(aws.ToString(sub3.SubnetId))
	return results, aws.ToString(vpc.VpcId)
}

func (t *localServerSuite) prepareAndBootstrap(c *gc.C) environs.Environ {
	return t.prepareAndBootstrapWithConfig(c, coretesting.Attrs{})
}

func (t *localServerSuite) prepareAndBootstrapWithConfig(c *gc.C, config coretesting.Attrs) environs.Environ {
	args := t.PrepareParams(c)
	args.ModelConfig = coretesting.Attrs(args.ModelConfig).Merge(config)
	env := t.PrepareWithParams(c, args)

	constraints := constraints.Value{}
	controllerConfig := coretesting.FakeControllerConfig()
	if t.useIAMRole {
		ir := "auto"
		constraints.InstanceRole = &ir
		controllerConfig["controller-name"] = "juju-test"
	}

	err := bootstrap.Bootstrap(t.BootstrapContext, env,
		t.callCtx, bootstrap.BootstrapParams{
			BootstrapConstraints:     constraints,
			ControllerConfig:         controllerConfig,
			AdminSecret:              testing.AdminSecret,
			CAPrivateKey:             coretesting.CAKey,
			Placement:                "zone=test-available",
			SupportedBootstrapSeries: coretesting.FakeSupportedJujuSeries,
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
		SubnetsToZones: []map[corenetwork.Id][]string{{
			subIDs[0]: {"zone2"},
			subIDs[1]: {"zone3"},
			subIDs[2]: {"zone4"},
		}},
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
		SubnetsToZones: []map[corenetwork.Id][]string{{
			subIDs[0]: {"test-available"},
			subIDs[1]: {"zone3"},
		}},
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
		SubnetsToZones: []map[corenetwork.Id][]string{{
			subIDs[0]: {"test-available"},
			subIDs[1]: {"zone3"},
		}},
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
		SubnetsToZones: []map[corenetwork.Id][]string{{
			subIDs[0]: {""},
		}},
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

func (t *localServerSuite) testStartInstanceAvailZoneOneConstrained(c *gc.C, runInstancesError smithy.APIError) {
	env := t.prepareAndBootstrap(c)

	// The first call to RunInstances fails with an error indicating the AZ
	// is constrained. The second attempt succeeds, and so allocates to az2.
	var azArgs []string
	realRunInstances := *ec2.RunInstances

	t.PatchValue(ec2.RunInstances, func(e ec2.Client, ctx context.ProviderCallContext, ri *awsec2.RunInstancesInput, callback environs.StatusCallbackFunc) (resp *awsec2.RunInstancesOutput, err error) {
		azArgs = append(azArgs, aws.ToString(ri.Placement.AvailabilityZone))
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
	expected := corenetwork.ProviderAddresses{
		corenetwork.NewMachineAddress("8.0.0.*", corenetwork.WithScope(corenetwork.ScopePublic)).AsProviderAddress(),
		corenetwork.NewMachineAddress("127.0.0.*", corenetwork.WithScope(corenetwork.ScopeCloudLocal)).AsProviderAddress(),
	}
	expected[0].Type = corenetwork.IPv4Address
	expected[1].Type = corenetwork.IPv4Address

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
	t.srv.defaultVPC.IsDefault = aws.Bool(false)
	err := t.srv.ec2srv.UpdateVpc(*t.srv.defaultVPC)
	c.Assert(err, jc.ErrorIsNil)

	env := t.Prepare(c)
	assertVPCInstanceTypeNotAvailable(c, env, t.callCtx)
}

func (t *localServerSuite) TestConstraintsValidatorVocabDefaultVPC(c *gc.C) {
	env := t.Prepare(c)
	assertVPCInstanceTypeAvailable(c, env, t.callCtx)
}

func (t *localServerSuite) TestConstraintsValidatorVocabSpecifiedVPC(c *gc.C) {
	t.srv.defaultVPC.IsDefault = aws.Bool(false)
	err := t.srv.ec2srv.UpdateVpc(*t.srv.defaultVPC)
	c.Assert(err, jc.ErrorIsNil)

	t.TestConfig["vpc-id"] = aws.ToString(t.srv.defaultVPC.VpcId)
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
		Series:      jujuversion.DefaultSupportedLTS(),
		Constraints: cons,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (t *localServerSuite) TestPrecheckInstanceInvalidInstanceType(c *gc.C) {
	env := t.Prepare(c)
	cons := constraints.MustParse("instance-type=m1.invalid")
	err := env.PrecheckInstance(t.callCtx, environs.PrecheckInstanceParams{
		Series:      jujuversion.DefaultSupportedLTS(),
		Constraints: cons,
	})
	c.Assert(err, gc.ErrorMatches, `invalid AWS instance type "m1.invalid" specified`)
}

func (t *localServerSuite) TestPrecheckInstanceUnsupportedArch(c *gc.C) {
	env := t.Prepare(c)
	cons := constraints.MustParse("instance-type=cc1.4xlarge arch=i386")
	err := env.PrecheckInstance(t.callCtx, environs.PrecheckInstanceParams{
		Series:      jujuversion.DefaultSupportedLTS(),
		Constraints: cons,
	})
	c.Assert(err, gc.ErrorMatches, `invalid AWS instance type "cc1.4xlarge" and arch "i386" specified`)
}

func (t *localServerSuite) TestPrecheckInstanceAvailZone(c *gc.C) {
	env := t.Prepare(c)
	placement := "zone=test-available"
	err := env.PrecheckInstance(t.callCtx, environs.PrecheckInstanceParams{
		Series:    jujuversion.DefaultSupportedLTS(),
		Placement: placement,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (t *localServerSuite) TestPrecheckInstanceAvailZoneUnavailable(c *gc.C) {
	env := t.Prepare(c)
	placement := "zone=test-unavailable"
	err := env.PrecheckInstance(t.callCtx, environs.PrecheckInstanceParams{
		Series:    jujuversion.DefaultSupportedLTS(),
		Placement: placement,
	})
	c.Assert(err, gc.ErrorMatches, `availability zone "test-unavailable" is "unavailable"`)
}

func (t *localServerSuite) TestPrecheckInstanceAvailZoneUnknown(c *gc.C) {
	env := t.Prepare(c)
	placement := "zone=test-unknown"
	err := env.PrecheckInstance(t.callCtx, environs.PrecheckInstanceParams{
		Series:    jujuversion.DefaultSupportedLTS(),
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
	resp, err := t.client.CreateVolume(t.callCtx, &awsec2.CreateVolumeInput{
		Size:             aws.Int32(1),
		VolumeType:       "gp2",
		AvailabilityZone: aws.String("test-available"),
	})
	c.Assert(err, jc.ErrorIsNil)

	err = env.PrecheckInstance(t.callCtx, environs.PrecheckInstanceParams{
		Series:    jujuversion.DefaultSupportedLTS(),
		Placement: placement,
		VolumeAttachments: []storage.VolumeAttachmentParams{{
			AttachmentParams: storage.AttachmentParams{
				Provider: "ebs",
			},
			Volume:   names.NewVolumeTag("23"),
			VolumeId: aws.ToString(resp.VolumeId),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (t *localServerSuite) TestPrecheckInstanceAvailZoneVolumeConflict(c *gc.C) {
	env := t.Prepare(c)
	resp, err := t.client.CreateVolume(t.callCtx, &awsec2.CreateVolumeInput{
		Size:             aws.Int32(1),
		VolumeType:       "gp2",
		AvailabilityZone: aws.String("volume-zone"),
	})
	c.Assert(err, jc.ErrorIsNil)

	err = env.PrecheckInstance(t.callCtx, environs.PrecheckInstanceParams{
		Series:    jujuversion.DefaultSupportedLTS(),
		Placement: "zone=test-available",
		VolumeAttachments: []storage.VolumeAttachmentParams{{
			AttachmentParams: storage.AttachmentParams{
				Provider: "ebs",
			},
			Volume:   names.NewVolumeTag("23"),
			VolumeId: aws.ToString(resp.VolumeId),
		}},
	})
	c.Assert(err, gc.ErrorMatches, `cannot create instance with placement "zone=test-available", as this will prevent attaching the requested EBS volumes in zone "volume-zone"`)
}

func (t *localServerSuite) TestValidateImageMetadata(c *gc.C) {
	//region := t.srv.region
	//aws.Regions[region.Name] = t.srv.region
	//defer delete(aws.Regions, region.Name)

	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())

	env := t.Prepare(c)
	params, err := env.(simplestreams.ImageMetadataValidator).ImageMetadataLookupParams("test")
	c.Assert(err, jc.ErrorIsNil)
	params.Release = jujuversion.DefaultSupportedLTS()
	params.Endpoint = "http://foo"
	params.Sources, err = environs.ImageMetadataSources(env, ss)
	c.Assert(err, jc.ErrorIsNil)
	image_ids, _, err := imagemetadata.ValidateImageMetadata(ss, params)
	c.Assert(err, jc.ErrorIsNil)
	sort.Strings(image_ids)
	c.Assert(image_ids, gc.DeepEquals, []string{"ami-02004133", "ami-02004135", "ami-02004139"})
}

func (t *localServerSuite) TestGetToolsMetadataSources(c *gc.C) {
	t.PatchValue(&tools.DefaultBaseURL, "")

	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())

	env := t.Prepare(c)
	sources, err := tools.GetMetadataSources(env, ss)
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
	err := bootstrap.Bootstrap(t.BootstrapContext, env,
		t.callCtx, bootstrap.BootstrapParams{
			ControllerConfig:         coretesting.FakeControllerConfig(),
			AdminSecret:              testing.AdminSecret,
			CAPrivateKey:             coretesting.CAKey,
			SupportedBootstrapSeries: coretesting.FakeSupportedJujuSeries,
		})
	c.Assert(err, jc.ErrorIsNil)

	instanceIds, err := env.ControllerInstances(t.callCtx, t.ControllerUUID)
	c.Assert(err, jc.ErrorIsNil)
	return env, instanceIds[0]
}

func (t *localServerSuite) TestNetworkInterfacesForMultipleInstances(c *gc.C) {
	// Start three instances
	env := t.prepareEnviron(c)
	testing.AssertStartInstance(c, env, t.callCtx, t.ControllerUUID, "1")
	testing.AssertStartInstance(c, env, t.callCtx, t.ControllerUUID, "2")
	testing.AssertStartInstance(c, env, t.callCtx, t.ControllerUUID, "3")

	// Get a list of running instance IDs
	instances, err := env.AllInstances(t.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	var ids = make([]instance.Id, len(instances))
	for i, inst := range instances {
		ids[i] = inst.Id()
	}

	// Sort instance list so we always get consistent results
	sort.Slice(ids, func(l, r int) bool { return ids[l] < ids[r] })

	ifLists, err := env.NetworkInterfaces(t.callCtx, ids)
	c.Assert(err, jc.ErrorIsNil)

	// Check that each entry in the list contains the right set of interfaces
	for i, id := range ids {
		c.Logf("comparing entry %d in result list with network interface list for instance %v", i, id)

		list, err := env.NetworkInterfaces(t.callCtx, []instance.Id{id})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(list, gc.HasLen, 1)
		instIfList := list[0]
		c.Assert(instIfList, gc.HasLen, 1)

		c.Assert(instIfList, gc.DeepEquals, ifLists[i], gc.Commentf("inconsistent result for entry %d in multi-instance result list", i))
		for devIdx, iface := range instIfList {
			t.assertInterfaceLooksValid(c, i+devIdx, devIdx, iface)
		}
	}
}

func (t *localServerSuite) TestPartialInterfacesForMultipleInstances(c *gc.C) {
	// Start three instances
	env := t.prepareEnviron(c)
	inst, _ := testing.AssertStartInstance(c, env, t.callCtx, t.ControllerUUID, "1")

	infoLists, err := env.NetworkInterfaces(t.callCtx, []instance.Id{inst.Id(), instance.Id("bogus")})
	c.Log(infoLists)
	c.Assert(err, gc.Equals, environs.ErrPartialInstances)
	c.Assert(infoLists, gc.HasLen, 2)

	// Check interfaces for first instance
	list := infoLists[0]
	c.Assert(list, gc.HasLen, 1)
	t.assertInterfaceLooksValid(c, 0, 0, list[0])

	// Check that the slot for the second instance is nil
	c.Assert(infoLists[1], gc.IsNil, gc.Commentf("expected slot for unknown instance to be nil"))
}

func (t *localServerSuite) TestNetworkInterfaces(c *gc.C) {
	env, instId := t.setUpInstanceWithDefaultVpc(c)
	infoLists, err := env.NetworkInterfaces(t.callCtx, []instance.Id{instId})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(infoLists, gc.HasLen, 1)

	list := infoLists[0]
	c.Assert(list, gc.HasLen, 1)

	t.assertInterfaceLooksValid(c, 0, 0, list[0])
}

func (t *localServerSuite) assertInterfaceLooksValid(c *gc.C, expIfaceID, expDevIndex int, iface corenetwork.InterfaceInfo) {
	// The CIDR isn't predictable, but it is in the 10.10.x.0/24 format
	// The subnet ID is in the form "subnet-x", where x matches the same
	// number from the CIDR. The interfaces address is part of the CIDR.
	// For these reasons we check that the CIDR is in the expected format
	// and derive the expected values for ProviderSubnetId and Address.
	cidr := iface.PrimaryAddress().CIDR
	re := regexp.MustCompile(`10\.10\.(\d+)\.0/24`)
	c.Assert(re.Match([]byte(cidr)), jc.IsTrue)
	index := re.FindStringSubmatch(cidr)[1]
	addr := fmt.Sprintf("10.10.%s.5", index)
	subnetId := corenetwork.Id("subnet-" + index)

	// AvailabilityZones will either contain "test-available",
	// "test-impaired" or "test-unavailable" depending on which subnet is
	// picked. Any of these is fine.
	zones := iface.AvailabilityZones
	c.Assert(zones, gc.HasLen, 1)
	re = regexp.MustCompile("test-available|test-unavailable|test-impaired")
	c.Assert(re.Match([]byte(zones[0])), jc.IsTrue)

	expectedInterface := corenetwork.InterfaceInfo{
		DeviceIndex:      expDevIndex,
		MACAddress:       iface.MACAddress,
		ProviderId:       corenetwork.Id(fmt.Sprintf("eni-%d", expIfaceID)),
		ProviderSubnetId: subnetId,
		VLANTag:          0,
		Disabled:         false,
		NoAutoStart:      false,
		InterfaceType:    corenetwork.EthernetDevice,
		Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
			addr,
			corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			corenetwork.WithCIDR(cidr),
			corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress()},
		// Each machine is also assigned a shadow IP with the pattern:
		// 73.37.0.X where X=(provider iface ID + 1)
		ShadowAddresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
			fmt.Sprintf("73.37.0.%d", expIfaceID+1),
			corenetwork.WithScope(corenetwork.ScopePublic),
			corenetwork.WithConfigType(corenetwork.ConfigDHCP),
		).AsProviderAddress()},
		AvailabilityZones: zones,
		Origin:            corenetwork.OriginProvider,
	}
	c.Assert(iface, gc.DeepEquals, expectedInterface)
}

func (t *localServerSuite) TestSubnetsWithInstanceId(c *gc.C) {
	env, instId := t.setUpInstanceWithDefaultVpc(c)
	subnets, err := env.Subnets(t.callCtx, instId, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnets, gc.HasLen, 1)
	validateSubnets(c, subnets, "")

	interfaceList, err := env.NetworkInterfaces(t.callCtx, []instance.Id{instId})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(interfaceList, gc.HasLen, 1)
	interfaces := interfaceList[0]
	c.Assert(interfaces, gc.HasLen, 1)
	c.Assert(interfaces[0].ProviderSubnetId, gc.Equals, subnets[0].ProviderId)
}

func (t *localServerSuite) TestSubnetsWithInstanceIdAndSubnetId(c *gc.C) {
	env, instId := t.setUpInstanceWithDefaultVpc(c)
	interfaceList, err := env.NetworkInterfaces(t.callCtx, []instance.Id{instId})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(interfaceList, gc.HasLen, 1)
	interfaces := interfaceList[0]
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

	ec2Inst := ec2.InstanceSDKEC2(instances[0])
	var tags []string
	for _, t := range ec2Inst.Tags {
		tags = append(tags, *t.Key+":"+*t.Value)
	}
	c.Assert(tags, jc.SameContents, []string{
		"Name:juju-sample-machine-0",
		"juju-model-uuid:" + coretesting.ModelTag.Id(),
		"juju-controller-uuid:" + t.ControllerUUID,
		"juju-is-controller:true",
	})
}

func (t *localServerSuite) TestRootDiskTags(c *gc.C) {
	env := t.prepareAndBootstrap(c)

	instances, err := env.AllRunningInstances(t.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instances, gc.HasLen, 1)

	ec2conn := ec2.EnvironEC2Client(env)
	resp, err := ec2conn.DescribeVolumes(t.callCtx, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.Volumes, gc.Not(gc.HasLen), 0)

	var found types.Volume
	for _, vol := range resp.Volumes {
		if len(vol.Tags) != 0 {
			found = vol
			break
		}
	}
	c.Assert(found, gc.NotNil)
	compareTags(c, found.Tags, []tagInfo{
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
	ec2inst := ec2.InstanceSDKEC2(inst[0])
	// Controllers should be started with a burstable
	// instance if possible, and a 32 GiB disk.
	c.Assert(string(ec2inst.InstanceType), gc.Equals, "t3a.medium")
}

func makeFilter(key string, values ...string) types.Filter {
	return types.Filter{Name: aws.String(key), Values: values}
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

	// Create a hosted model with an instance and a volume.
	hostedModelUUID := "7e386e08-cba7-44a4-a76e-7c1633584210"
	s.srv.ec2srv.SetInitialInstanceState(ec2test.Running)
	cfg, err := controllerEnv.Config().Apply(map[string]interface{}{
		"uuid":          hostedModelUUID,
		"firewall-mode": "global",
	})
	c.Assert(err, jc.ErrorIsNil)

	env, err := environs.New(s.BootstrapContext.Context(), environs.OpenParams{
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

	ec2conn := ec2.EnvironEC2Client(env)

	origController := coretesting.ControllerTag.Id()

	checkInstanceTags := func(controllerUUID string, expectedIds ...string) {
		resp, err := ec2conn.DescribeInstances(
			s.callCtx, &awsec2.DescribeInstancesInput{
				Filters: []types.Filter{makeFilter("tag:"+tags.JujuController, controllerUUID)},
			})
		c.Assert(err, jc.ErrorIsNil)
		actualIds := set.NewStrings()
		for _, reservation := range resp.Reservations {
			for _, instance := range reservation.Instances {
				actualIds.Add(aws.ToString(instance.InstanceId))
			}
		}
		c.Check(actualIds, gc.DeepEquals, set.NewStrings(expectedIds...))
	}

	checkVolumeTags := func(controllerUUID string, expectedIds ...string) {
		resp, err := ec2conn.DescribeVolumes(
			s.callCtx, &awsec2.DescribeVolumesInput{
				Filters: []types.Filter{makeFilter("tag:"+tags.JujuController, controllerUUID)},
			})
		c.Assert(err, jc.ErrorIsNil)
		actualIds := set.NewStrings()
		for _, vol := range resp.Volumes {
			actualIds.Add(aws.ToString(vol.VolumeId))
		}
		c.Check(actualIds, gc.DeepEquals, set.NewStrings(expectedIds...))
	}

	checkGroupTags := func(controllerUUID string, expectedIds ...string) {
		resp, err := ec2conn.DescribeSecurityGroups(
			s.callCtx, &awsec2.DescribeSecurityGroupsInput{
				Filters: []types.Filter{makeFilter("tag:"+tags.JujuController, controllerUUID)},
			})
		c.Assert(err, jc.ErrorIsNil)
		actualIds := set.NewStrings()
		for _, group := range resp.SecurityGroups {
			actualIds.Add(aws.ToString(group.GroupId))
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

func patchEC2ForTesting(c *gc.C, region types.Region) func() {
	ec2.UseTestImageData(c, ec2.MakeTestImageStreamsData(region))
	restoreRetryTimeouts := envtesting.PatchRetryStrategies(ec2.ShortRetryStrategy)
	restoreFinishBootstrap := envtesting.DisableFinishBootstrap()
	return func() {
		restoreFinishBootstrap()
		restoreRetryTimeouts()
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

func (t *localServerSuite) TestInstanceAttributes(c *gc.C) {
	t.Prepare(c)
	inst, hc := testing.AssertStartInstance(c, t.Env, t.callCtx, t.ControllerUUID, "30")
	defer t.Env.StopInstances(t.callCtx, inst.Id())
	// Sanity check for hardware characteristics.
	c.Assert(hc.Arch, gc.NotNil)
	c.Assert(hc.Mem, gc.NotNil)
	c.Assert(hc.RootDisk, gc.NotNil)
	c.Assert(hc.CpuCores, gc.NotNil)
	c.Assert(hc.CpuPower, gc.NotNil)
	addresses, err := testing.WaitInstanceAddresses(t.Env, t.callCtx, inst.Id())
	// TODO(niemeyer): This assert sometimes fails with "no instances found"
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addresses, gc.Not(gc.HasLen), 0)

	insts, err := t.Env.Instances(t.callCtx, []instance.Id{inst.Id()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(insts), gc.Equals, 1)

	ec2inst := ec2.InstanceSDKEC2(insts[0])
	c.Assert(*ec2inst.PublicIpAddress, gc.Equals, addresses[0].Value)
	c.Assert(ec2inst.InstanceType, gc.Equals, types.InstanceType("t3a.micro"))
}

func (t *localServerSuite) TestStartInstanceConstraints(c *gc.C) {
	t.Prepare(c)
	cons := constraints.MustParse("mem=4G")
	inst, hc := testing.AssertStartInstanceWithConstraints(c, t.Env, t.callCtx, t.ControllerUUID, "30", cons)
	defer t.Env.StopInstances(t.callCtx, inst.Id())
	ec2inst := ec2.InstanceSDKEC2(inst)
	c.Assert(ec2inst.InstanceType, gc.Equals, types.InstanceType("t3a.medium"))
	c.Assert(*hc.Arch, gc.Equals, "amd64")
	c.Assert(*hc.Mem, gc.Equals, uint64(4*1024))
	c.Assert(*hc.RootDisk, gc.Equals, uint64(8*1024))
	c.Assert(*hc.CpuCores, gc.Equals, uint64(2))
}

func (t *localServerSuite) TestControllerInstances(c *gc.C) {
	t.prepareAndBootstrap(c)
	allInsts, err := t.Env.AllRunningInstances(t.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allInsts, gc.HasLen, 1) // bootstrap instance
	bootstrapInstId := allInsts[0].Id()

	inst0, _ := testing.AssertStartInstance(c, t.Env, t.callCtx, t.ControllerUUID, "98")
	defer t.Env.StopInstances(t.callCtx, inst0.Id())

	inst1, _ := testing.AssertStartInstance(c, t.Env, t.callCtx, t.ControllerUUID, "99")
	defer t.Env.StopInstances(t.callCtx, inst1.Id())

	insts, err := t.Env.ControllerInstances(t.callCtx, t.ControllerUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(insts, gc.DeepEquals, []instance.Id{bootstrapInstId})
}

func (t *localServerSuite) TestInstanceGroups(c *gc.C) {
	t.prepareAndBootstrap(c)
	allInsts, err := t.Env.AllRunningInstances(t.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allInsts, gc.HasLen, 1) // bootstrap instance
	bootstrapInstId := allInsts[0].Id()

	ec2conn := ec2.EnvironEC2Client(t.Env)

	groups := []*types.SecurityGroupIdentifier{
		{GroupName: aws.String(ec2.JujuGroupName(t.Env))},
		{GroupName: aws.String(ec2.MachineGroupName(t.Env, "98"))},
		{GroupName: aws.String(ec2.MachineGroupName(t.Env, "99"))},
	}
	info := make([]types.SecurityGroup, len(groups))

	// Create a group with the same name as the juju group
	// but with different permissions, to check that it's deleted
	// and recreated correctly.
	oldJujuGroup := createGroup(c, ec2conn, t.callCtx, aws.ToString(groups[0].GroupName), "old juju group")

	// Add a permission.
	// N.B. this is unfortunately sensitive to the actual set of permissions used.
	_, err = ec2conn.AuthorizeSecurityGroupIngress(t.callCtx, &awsec2.AuthorizeSecurityGroupIngressInput{
		GroupId: oldJujuGroup.GroupId,
		IpPermissions: []types.IpPermission{
			{
				IpProtocol: aws.String("udp"),
				FromPort:   aws.Int32(4321),
				ToPort:     aws.Int32(4322),
				IpRanges:   []types.IpRange{{CidrIp: aws.String("3.4.5.6/32")}},
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	inst0, _ := testing.AssertStartControllerInstance(c, t.Env, t.callCtx, t.ControllerUUID, "98")
	defer t.Env.StopInstances(t.callCtx, inst0.Id())

	// Create a same-named group for the second instance
	// before starting it, to check that it's reused correctly.
	oldMachineGroup := createGroup(c, ec2conn, t.callCtx, aws.ToString(groups[2].GroupName), "old machine group")

	inst1, _ := testing.AssertStartControllerInstance(c, t.Env, t.callCtx, t.ControllerUUID, "99")
	defer t.Env.StopInstances(t.callCtx, inst1.Id())

	groupNames := make([]string, len(groups))
	for i, g := range groups {
		g := g
		groupNames[i] = aws.ToString(g.GroupName)
	}
	groupsResp, err := ec2conn.DescribeSecurityGroups(t.callCtx, &awsec2.DescribeSecurityGroupsInput{
		GroupNames: groupNames,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(groupsResp.SecurityGroups, gc.HasLen, len(groups))

	// For each group, check that it exists and record its id.
	for i, group := range groups {
		found := false
		for _, g := range groupsResp.SecurityGroups {
			if aws.ToString(g.GroupName) == aws.ToString(group.GroupName) {
				groups[i].GroupId = g.GroupId
				info[i] = g
				found = true
				break
			}
		}
		if !found {
			c.Fatalf("group %q not found", aws.ToString(group.GroupName))
		}
	}

	// The old juju group should have been reused.
	c.Check(aws.ToString(groups[0].GroupId), gc.Equals, aws.ToString(oldJujuGroup.GroupId))

	// Check that it authorizes the correct ports and there
	// are no extra permissions (in particular we are checking
	// that the unneeded permission that we added earlier
	// has been deleted).
	perms := info[0].IpPermissions
	c.Assert(perms, gc.HasLen, 3)
	checkPortAllowed(c, perms, 22) // SSH
	checkPortAllowed(c, perms, int32(coretesting.FakeControllerConfig().APIPort()))
	checkSecurityGroupAllowed(c, perms, groups[0])

	// The old machine group should have been reused also.
	c.Check(aws.ToString(groups[2].GroupId), gc.Equals, aws.ToString(oldMachineGroup.GroupId))

	// Check that each instance is part of the correct groups.
	resp, err := ec2conn.DescribeInstances(t.callCtx, &awsec2.DescribeInstancesInput{
		InstanceIds: []string{string(inst0.Id()), string(inst1.Id())},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.Reservations, gc.HasLen, 2)
	for _, r := range resp.Reservations {
		c.Assert(r.Instances, gc.HasLen, 1)
		// each instance must be part of the general juju group.
		inst := r.Instances[0]
		msg := gc.Commentf("instance %#v", inst)
		c.Assert(hasSecurityGroup(inst, groups[0]), gc.Equals, true, msg)
		switch instance.Id(aws.ToString(inst.InstanceId)) {
		case inst0.Id():
			c.Assert(hasSecurityGroup(inst, groups[1]), gc.Equals, true, msg)
			c.Assert(hasSecurityGroup(inst, groups[2]), gc.Equals, false, msg)
		case inst1.Id():
			c.Assert(hasSecurityGroup(inst, groups[2]), gc.Equals, true, msg)
			c.Assert(hasSecurityGroup(inst, groups[1]), gc.Equals, false, msg)
		default:
			c.Errorf("unknown instance found: %v", inst)
		}
	}

	// Check that listing those instances finds them using the groups
	instIds := []instance.Id{inst0.Id(), inst1.Id()}
	idsFromInsts := func(insts []instances.Instance) (ids []instance.Id) {
		for _, inst := range insts {
			ids = append(ids, inst.Id())
		}
		return ids
	}
	insts, err := t.Env.Instances(t.callCtx, instIds)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instIds, jc.SameContents, idsFromInsts(insts))
	allInsts, err = t.Env.AllRunningInstances(t.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	// ignore the bootstrap instance
	for i, inst := range allInsts {
		if inst.Id() == bootstrapInstId {
			if i+1 < len(allInsts) {
				copy(allInsts[i:], allInsts[i+1:])
			}
			allInsts = allInsts[:len(allInsts)-1]
			break
		}
	}
	c.Assert(instIds, jc.SameContents, idsFromInsts(allInsts))
}

func (t *localServerSuite) TestInstanceGroupsWithAutocert(c *gc.C) {
	// Prepare the controller configuration.
	t.Prepare(c)
	params := environs.StartInstanceParams{
		ControllerUUID: t.ControllerUUID,
	}
	err := testing.FillInStartInstanceParams(t.Env, "42", true, &params)
	c.Assert(err, jc.ErrorIsNil)
	config := params.InstanceConfig.Controller.Config
	config["api-port"] = 443
	config["autocert-dns-name"] = "example.com"

	// Bootstrap the controller.
	result, err := t.Env.StartInstance(t.callCtx, params)
	c.Assert(err, jc.ErrorIsNil)
	inst := result.Instance
	defer t.Env.StopInstances(t.callCtx, inst.Id())

	// Get security permissions.
	group := ec2.JujuGroupName(t.Env)
	ec2conn := ec2.EnvironEC2Client(t.Env)
	groupsResp, err := ec2conn.DescribeSecurityGroups(t.callCtx, &awsec2.DescribeSecurityGroupsInput{
		GroupNames: []string{group},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(groupsResp.SecurityGroups, gc.HasLen, 1)
	perms := groupsResp.SecurityGroups[0].IpPermissions

	// Check that the expected ports are accessible.
	checkPortAllowed(c, perms, 22)
	checkPortAllowed(c, perms, 80)
	checkPortAllowed(c, perms, 443)
}

func checkPortAllowed(c *gc.C, perms []types.IpPermission, port int32) {
	for _, perm := range perms {
		if aws.ToInt32(perm.FromPort) == port {
			c.Check(aws.ToString(perm.IpProtocol), gc.Equals, "tcp")
			c.Check(aws.ToInt32(perm.ToPort), gc.Equals, port)
			c.Check(perm.IpRanges, gc.HasLen, 1)
			c.Check(aws.ToString(perm.IpRanges[0].CidrIp), gc.DeepEquals, "0.0.0.0/0")
			c.Check(perm.UserIdGroupPairs, gc.HasLen, 0)
			return
		}
	}
	c.Errorf("ip port permission not found for %d in %#v", port, perms)
}

func checkSecurityGroupAllowed(c *gc.C, perms []types.IpPermission, g *types.SecurityGroupIdentifier) {
	for _, perm := range perms {
		if len(perm.UserIdGroupPairs) == 0 {
			continue
		}
		if aws.ToString(perm.UserIdGroupPairs[0].GroupId) == aws.ToString(g.GroupId) {
			return
		}
	}
	c.Errorf("security group permission not found for %s in %s", pretty.Sprint(g), pretty.Sprint(perms))
}

func (t *localServerSuite) TestStopInstances(c *gc.C) {
	t.Prepare(c)
	inst0, _ := testing.AssertStartInstance(c, t.Env, t.callCtx, t.ControllerUUID, "40")
	inst1 := ec2.FabricateInstance(inst0, "i-aaaaaaaa")
	inst2, _ := testing.AssertStartInstance(c, t.Env, t.callCtx, t.ControllerUUID, "41")

	err := t.Env.StopInstances(t.callCtx, inst0.Id(), inst1.Id(), inst2.Id())
	c.Check(err, jc.ErrorIsNil)

	var insts []instances.Instance

	// We need the retry logic here because we are waiting
	// for Instances to return an error, and it will not retry
	// if it succeeds.
	retryStrategy := ec2.ShortRetryStrategy
	retryStrategy.Func = func() error {
		insts, err = t.Env.Instances(t.callCtx, []instance.Id{inst0.Id(), inst2.Id()})
		if err == environs.ErrPartialInstances {
			// instances not gone yet.
			return err
		}
		if err == environs.ErrNoInstances {
			return nil
		}
		c.Fatalf("error getting instances: %v", err)
		return errors.New(fmt.Sprintf("error getting instances: %v", err))
	}
	err = retry.Call(*retryStrategy)

	if err != nil {
		c.Errorf("after termination, instances remaining: %v", insts)
	}
}

func (t *localServerSuite) TestPrechecker(c *gc.C) {
	// All implementations of InstancePrechecker should
	// return nil for empty constraints (excluding the
	// manual provider).
	t.Prepare(c)
	err := t.Env.PrecheckInstance(t.ProviderCallContext,
		environs.PrecheckInstanceParams{
			Series: "precise",
		})
	c.Assert(err, jc.ErrorIsNil)
}

func (t *localServerSuite) TestPorts(c *gc.C) {
	t.prepareAndBootstrap(c)

	inst1, _ := testing.AssertStartInstance(c, t.Env, t.ProviderCallContext, t.ControllerUUID, "1")
	c.Assert(inst1, gc.NotNil)
	defer func() { _ = t.Env.StopInstances(t.ProviderCallContext, inst1.Id()) }()
	fwInst1, ok := inst1.(instances.InstanceFirewaller)
	c.Assert(ok, gc.Equals, true)

	rules, err := fwInst1.IngressRules(t.ProviderCallContext, "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rules, gc.HasLen, 0)

	inst2, _ := testing.AssertStartInstance(c, t.Env, t.ProviderCallContext, t.ControllerUUID, "2")
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
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("80-100/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("67/udp"), firewall.AllNetworksIPV4CIDR),
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
			firewall.NewIngressRule(network.MustParsePortRange("20-30/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR),
		},
	)
	rules, err = fwInst1.IngressRules(t.ProviderCallContext, "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		rules, jc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("80-100/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("67/udp"), firewall.AllNetworksIPV4CIDR),
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
			firewall.NewIngressRule(network.MustParsePortRange("20-30/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("99/tcp"), firewall.AllNetworksIPV4CIDR),
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
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR),
		},
	)
	rules, err = fwInst1.IngressRules(t.ProviderCallContext, "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		rules, jc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("80-100/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("67/udp"), firewall.AllNetworksIPV4CIDR),
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
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR),
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

func (t *localServerSuite) TestGlobalPorts(c *gc.C) {
	t.prepareAndBootstrap(c)

	// Change configuration.
	oldConfig := t.Env.Config()
	defer func() {
		err := t.Env.SetConfig(oldConfig)
		c.Assert(err, jc.ErrorIsNil)
	}()

	// So that deleteSecurityGroupInsistently succeeds. It will fail and keep
	// retrying due to StopInstances deleting the security groups, which are
	// global when firewall-mode is FwGlobal.
	t.BaseSuite.PatchValue(ec2.DeleteSecurityGroupInsistently, func(
		ec2.SecurityGroupCleaner, context.ProviderCallContext, types.GroupIdentifier, clock.Clock,
	) error {
		return nil
	})

	attrs := t.Env.Config().AllAttrs()
	attrs["firewall-mode"] = config.FwGlobal
	newConfig, err := t.Env.Config().Apply(attrs)
	c.Assert(err, jc.ErrorIsNil)
	err = t.Env.SetConfig(newConfig)
	c.Assert(err, jc.ErrorIsNil)

	// Create instances and check open ports on both instances.
	inst1, _ := testing.AssertStartInstance(c, t.Env, t.ProviderCallContext, t.ControllerUUID, "1")
	defer func() { _ = t.Env.StopInstances(t.ProviderCallContext, inst1.Id()) }()

	fwEnv, ok := t.Env.(environs.Firewaller)
	c.Assert(ok, gc.Equals, true)

	rules, err := fwEnv.IngressRules(t.ProviderCallContext)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rules, gc.HasLen, 0)

	inst2, _ := testing.AssertStartInstance(c, t.Env, t.ProviderCallContext, t.ControllerUUID, "2")
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
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("99/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("100-110/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("67/udp"), firewall.AllNetworksIPV4CIDR),
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
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("100-110/tcp"), firewall.AllNetworksIPV4CIDR),
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
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("100-110/tcp"), firewall.AllNetworksIPV4CIDR),
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

func (t *localServerSuite) TestBootstrapMultiple(c *gc.C) {
	// bootstrap.Bootstrap no longer raises errors if the environment is
	// already up, this has been moved into the bootstrap command.
	t.prepareAndBootstrap(c)

	c.Logf("destroy env")
	err := environs.Destroy(t.Env.Config().Name(), t.Env, t.ProviderCallContext, t.ControllerStore)
	c.Assert(err, jc.ErrorIsNil)
	err = t.Env.Destroy(t.ProviderCallContext) // Again, should work fine and do nothing.
	c.Assert(err, jc.ErrorIsNil)

	// check that we can bootstrap after destroy
	t.prepareAndBootstrap(c)
}

// Check that we get a consistent error when asking for an instance without
// a valid machine config.
func (t *localServerSuite) TestStartInstanceWithEmptyNonceFails(c *gc.C) {
	machineId := "4"
	apiInfo := testing.FakeAPIInfo(machineId)
	instanceConfig, err := instancecfg.NewInstanceConfig(coretesting.ControllerTag, machineId, "", "released", "trusty", apiInfo)
	c.Assert(err, jc.ErrorIsNil)

	t.Prepare(c)

	storageDir := c.MkDir()
	toolsStorage, err := filestorage.NewFileStorageWriter(storageDir)
	c.Assert(err, jc.ErrorIsNil)
	possibleTools := coretools.List(envtesting.AssertUploadFakeToolsVersions(
		c, toolsStorage, "released", "released", version.MustParseBinary("5.4.5-ubuntu-amd64"),
	))
	params := environs.StartInstanceParams{
		ControllerUUID: coretesting.ControllerTag.Id(),
		Tools:          possibleTools,
		InstanceConfig: instanceConfig,
		StatusCallback: fakeCallback,
	}
	err = testing.SetImageMetadata(
		t.Env,
		simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory()),
		[]string{"trusty"},
		possibleTools.Arches(),
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

func (t *localServerSuite) TestIngressRulesWithPartiallyMatchingCIDRs(c *gc.C) {
	t.prepareAndBootstrap(c)

	inst1, _ := testing.AssertStartInstance(c, t.Env, t.ProviderCallContext, t.ControllerUUID, "1")
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
			firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
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
			firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
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
			firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
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
			firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
		},
	)
}

// createGroup creates a new EC2 group and returns it. If it already exists,
// it revokes all its permissions and returns the existing group.
func createGroup(c *gc.C, ec2conn ec2.Client, ctx stdcontext.Context, name string, descr string) types.SecurityGroupIdentifier {
	resp, err := ec2conn.CreateSecurityGroup(ctx, &awsec2.CreateSecurityGroupInput{
		GroupName:   aws.String(name),
		Description: aws.String(descr),
	})
	if err == nil {
		return types.SecurityGroupIdentifier{
			GroupId:   resp.GroupId,
			GroupName: aws.String(name),
		}
	}
	if err.(smithy.APIError).ErrorCode() != "InvalidGroup.Duplicate" {
		c.Fatalf("cannot make group %q: %v", name, err)
	}

	// Found duplicate group, so revoke its permissions and return it.
	gresp, err := ec2conn.DescribeSecurityGroups(ctx, &awsec2.DescribeSecurityGroupsInput{
		GroupNames: []string{name},
	})
	c.Assert(err, jc.ErrorIsNil)

	gi := gresp.SecurityGroups[0]
	if len(gi.IpPermissions) > 0 {
		_, err = ec2conn.RevokeSecurityGroupIngress(ctx, &awsec2.RevokeSecurityGroupIngressInput{
			GroupId: gi.GroupId,
		})
		c.Assert(err, jc.ErrorIsNil)
	}
	return types.SecurityGroupIdentifier{
		GroupId:   gi.GroupId,
		GroupName: gi.GroupName,
	}
}

func hasSecurityGroup(inst types.Instance, group *types.SecurityGroupIdentifier) bool {
	for _, instGroup := range inst.SecurityGroups {
		if aws.ToString(instGroup.GroupId) == aws.ToString(group.GroupId) {
			return true
		}
	}
	return false
}
