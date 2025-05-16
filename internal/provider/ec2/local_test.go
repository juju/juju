// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	stdtesting "testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/smithy-go"
	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/retry"
	"github.com/juju/tc"
	"github.com/juju/utils/v4"
	"github.com/kr/pretty"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/imagemetadata"
	imagetesting "github.com/juju/juju/environs/imagemetadata/testing"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/jujutest"
	"github.com/juju/juju/environs/models"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/environs/tags"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/provider/ec2"
	ec2test "github.com/juju/juju/internal/provider/ec2/internal/testing"
	"github.com/juju/juju/internal/storage"
	coretesting "github.com/juju/juju/internal/testing"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/juju/keys"
	"github.com/juju/juju/juju/testing"
)

var localConfigAttrs = coretesting.FakeConfig().Merge(coretesting.Attrs{
	"name":          "sample",
	"type":          "ec2",
	"agent-version": coretesting.FakeVersionNumber.String(),
})

func fakeCallback(_ context.Context, _ status.Status, _ string, _ map[string]interface{}) error {
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
}

func (srv *localServer) startServer(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
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

func (srv *localServer) stopServer(c *tc.C) {
	srv.iamsrv.Reset()
	srv.ec2srv.Reset(false)
	srv.defaultVPC = nil
}

func bootstrapClientFunc(ec2Client ec2.Client) ec2.ClientFunc {
	return func(ctx context.Context, spec cloudspec.CloudSpec, options ...ec2.ClientOption) (ec2.Client, error) {
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
	return func(ctx context.Context, spec cloudspec.CloudSpec, options ...ec2.ClientOption) (ec2.IAMClient, error) {
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
	c *tc.C,
	clientFunc ec2.ClientFunc,
	iamClientFunc ec2.IAMClientFunc,
) environs.BootstrapContext {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())

	ctx := c.Context()
	ctx = context.WithValue(ctx, bootstrap.SimplestreamsFetcherContextKey, ss)
	if clientFunc != nil {
		ctx = context.WithValue(ctx, ec2.AWSClientContextKey, clientFunc)
	}
	if iamClientFunc != nil {
		ctx = context.WithValue(ctx, ec2.AWSIAMClientContextKey, iamClientFunc)
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
}

func TestLocalServerSuite(t *stdtesting.T) { tc.Run(t, &localServerSuite{}) }
func (t *localServerSuite) SetUpSuite(c *tc.C) {
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
	t.UploadArches = []string{arch.AMD64}
	t.TestConfig = localConfigAttrs
	imagetesting.PatchOfficialDataSources(&t.BaseSuite.CleanupSuite, "test:")
	t.BaseSuite.PatchValue(&imagemetadata.SimplestreamsImagesPublicKey, sstesting.SignedMetadataPublicKey)
	t.BaseSuite.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)
	t.BaseSuite.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	t.BaseSuite.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
	t.srv.createRootDisks = true
	t.srv.startServer(c)
	// TODO(jam) I don't understand why we shouldn't do this.
	// t.Tests embeds the sstesting.TestDataSuite, but if we call this
	// SetUpSuite, then all of the tests fail because they go to access
	// "test:/streams/..." and it isn't found
	// t.Tests.SetUpSuite(c)
}

func (t *localServerSuite) TearDownSuite(c *tc.C) {
	t.Tests.TearDownSuite(c)
	t.BaseSuite.TearDownSuite(c)
}

func (t *localServerSuite) SetUpTest(c *tc.C) {
	t.BaseSuite.SetUpTest(c)
	t.srv.startServer(c)
	region := t.srv.region
	t.CloudRegion = aws.ToString(region.RegionName)
	t.CloudEndpoint = aws.ToString(region.Endpoint)
	t.client = t.srv.ec2srv
	t.iamClient = t.srv.iamsrv
	restoreEC2Patching := patchEC2ForTesting(c, region)
	t.AddCleanup(func(c *tc.C) { restoreEC2Patching() })
	t.Tests.SetUpTest(c)

	t.Tests.BootstrapContext = bootstrapContextWithClientFunc(c, bootstrapClientFunc(t.client), bootstrapIAMClientFunc(t.iamClient))
	t.useIAMRole = false
}

func (t *localServerSuite) TearDownTest(c *tc.C) {
	t.Tests.TearDownTest(c)
	t.srv.stopServer(c)
	t.BaseSuite.TearDownTest(c)
}

func (t *localServerSuite) prepareEnviron(c *tc.C) environs.NetworkingEnviron {
	env := t.Prepare(c)
	netenv, supported := environs.SupportsNetworking(env)
	c.Assert(supported, tc.IsTrue)
	return netenv
}

func (t *localServerSuite) TestPrepareForBootstrapWithInvalidVPCID(c *tc.C) {
	badVPCIDConfig := coretesting.Attrs{"vpc-id": "bad"}

	expectedError := `invalid EC2 provider config: vpc-id: "bad" is not a valid AWS VPC ID`
	t.AssertPrepareFailsWithConfig(c, badVPCIDConfig, expectedError)
}

func (t *localServerSuite) TestPrepareForBootstrapWithUnknownVPCID(c *tc.C) {
	unknownVPCIDConfig := coretesting.Attrs{"vpc-id": "vpc-unknown"}

	expectedError := `Juju cannot use the given vpc-id for bootstrapping(.|\n)*Error details: VPC "vpc-unknown" not found`
	err := t.AssertPrepareFailsWithConfig(c, unknownVPCIDConfig, expectedError)
	c.Check(err, tc.ErrorIs, ec2.ErrorVPCNotUsable)
}

func (t *localServerSuite) TestPrepareForBootstrapWithNotRecommendedVPCID(c *tc.C) {
	t.makeTestingDefaultVPCUnavailable(c)
	notRecommendedVPCIDConfig := coretesting.Attrs{"vpc-id": aws.ToString(t.srv.defaultVPC.VpcId)}

	expectedError := `The given vpc-id does not meet one or more(.|\n)*Error details: VPC ".*" has unexpected state "unavailable"`
	err := t.AssertPrepareFailsWithConfig(c, notRecommendedVPCIDConfig, expectedError)
	c.Check(err, tc.ErrorIs, ec2.ErrorVPCNotRecommended)
}

func (t *localServerSuite) makeTestingDefaultVPCUnavailable(c *tc.C) {
	// For simplicity, here the test server's default VPC is updated to change
	// its state to unavailable, we just verify the behavior of a "not
	// recommended VPC".
	t.srv.defaultVPC.State = "unavailable"
	err := t.srv.ec2srv.UpdateVpc(*t.srv.defaultVPC)
	c.Assert(err, tc.ErrorIsNil)
}

func (t *localServerSuite) TestPrepareForBootstrapWithNotRecommendedButForcedVPCID(c *tc.C) {
	t.makeTestingDefaultVPCUnavailable(c)
	params := t.PrepareParams(c)
	vpcID := aws.ToString(t.srv.defaultVPC.VpcId)
	params.ModelConfig["vpc-id"] = vpcID
	params.ModelConfig["vpc-id-force"] = true

	t.prepareWithParamsAndBootstrapWithVPCID(c, params, vpcID)
}

func (t *localServerSuite) TestPrepareForBootstrapWithEmptyVPCID(c *tc.C) {
	const emptyVPCID = ""

	params := t.PrepareParams(c)
	params.ModelConfig["vpc-id"] = emptyVPCID

	t.prepareWithParamsAndBootstrapWithVPCID(c, params, emptyVPCID)
}

func (t *localServerSuite) prepareWithParamsAndBootstrapWithVPCID(c *tc.C, params bootstrap.PrepareParams, expectedVPCID string) {
	env := t.PrepareWithParams(c, params)
	unknownAttrs := env.Config().UnknownAttrs()
	vpcID, ok := unknownAttrs["vpc-id"]
	c.Check(vpcID, tc.Equals, expectedVPCID)
	c.Check(ok, tc.IsTrue)

	err := bootstrap.Bootstrap(t.BootstrapContext, env,
		bootstrap.BootstrapParams{
			ControllerConfig:        coretesting.FakeControllerConfig(),
			AdminSecret:             testing.AdminSecret,
			CAPrivateKey:            coretesting.CAKey,
			Placement:               "zone=test-available",
			SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
		})
	c.Assert(err, tc.ErrorIsNil)
}

func (t *localServerSuite) TestPrepareForBootstrapWithVPCIDNone(c *tc.C) {
	params := t.PrepareParams(c)
	params.ModelConfig["vpc-id"] = "none"

	t.prepareWithParamsAndBootstrapWithVPCID(c, params, ec2.VPCIDNone)
}

func (t *localServerSuite) TestPrepareForBootstrapWithDefaultVPCID(c *tc.C) {
	params := t.PrepareParams(c)
	vpcID := aws.ToString(t.srv.defaultVPC.VpcId)
	params.ModelConfig["vpc-id"] = vpcID

	t.prepareWithParamsAndBootstrapWithVPCID(c, params, vpcID)
}

func (t *localServerSuite) TestSystemdBootstrapInstanceUserDataAndState(c *tc.C) {
	env := t.Prepare(c)
	err := bootstrap.Bootstrap(t.BootstrapContext, env,
		bootstrap.BootstrapParams{
			ControllerConfig:        coretesting.FakeControllerConfig(),
			BootstrapBase:           jujuversion.DefaultSupportedLTSBase(),
			AdminSecret:             testing.AdminSecret,
			CAPrivateKey:            coretesting.CAKey,
			SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
		})
	c.Assert(err, tc.ErrorIsNil)

	// check that ControllerInstances returns the id of the bootstrap machine.
	instanceIds, err := env.ControllerInstances(c.Context(), t.ControllerUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(instanceIds, tc.HasLen, 1)

	insts, err := env.AllRunningInstances(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(insts, tc.HasLen, 1)
	c.Check(insts[0].Id(), tc.Equals, instanceIds[0])

	// check that the user data is configured to and the machine and
	// provisioning agents.  check that the user data is configured to only
	// configure authorized SSH keys and set the log output; everything else
	// happens after the machine is brought up.
	inst := t.srv.ec2srv.Instance(string(insts[0].Id()))
	c.Assert(inst, tc.NotNil)
	addresses, err := insts[0].Addresses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addresses, tc.Not(tc.HasLen), 0)
	userData, err := utils.Gunzip(inst.UserData)
	c.Assert(err, tc.ErrorIsNil)

	var userDataMap map[string]interface{}
	err = goyaml.Unmarshal(userData, &userDataMap)
	c.Assert(err, tc.ErrorIsNil)
	var keys []string
	for key := range userDataMap {
		keys = append(keys, key)
	}
	c.Assert(keys, tc.SameContents, []string{"output", "users", "runcmd", "ssh_keys"})
	c.Assert(userDataMap["runcmd"], tc.DeepEquals, []interface{}{
		"set -xe",
		"install -D -m 644 /dev/null '/var/lib/juju/nonce.txt'",
		"echo 'user-admin:bootstrap' > '/var/lib/juju/nonce.txt'",
	})

	// check that a new instance will be started with a machine agent
	inst1, hc := testing.AssertStartInstance(c, env, t.ControllerUUID, "1")
	c.Check(*hc.Arch, tc.Equals, "amd64")
	c.Check(*hc.Mem, tc.Equals, uint64(8192))
	c.Check(*hc.CpuCores, tc.Equals, uint64(2))
	inst = t.srv.ec2srv.Instance(string(inst1.Id()))
	c.Assert(inst, tc.NotNil)
	userData, err = utils.Gunzip(inst.UserData)
	c.Assert(err, tc.ErrorIsNil)
	c.Logf("second instance: UserData: %q", userData)
	userDataMap = nil
	err = goyaml.Unmarshal(userData, &userDataMap)
	c.Assert(err, tc.ErrorIsNil)
	CheckPackage(c, userDataMap, "curl", true)
	CheckPackage(c, userDataMap, "mongodb-server", false)
	CheckScripts(c, userDataMap, "jujud bootstrap-state", false)
	CheckScripts(c, userDataMap, "/var/lib/juju/agents/machine-1/agent.conf", true)
	// TODO check for provisioning agent

	err = env.Destroy(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	_, err = env.ControllerInstances(c.Context(), t.ControllerUUID)
	c.Assert(err, tc.Equals, environs.ErrNotBootstrapped)
}

func (t *localServerSuite) TestTerminateInstancesIgnoresNotFound(c *tc.C) {
	env := t.Prepare(c)
	err := bootstrap.Bootstrap(t.BootstrapContext, env,
		bootstrap.BootstrapParams{
			ControllerConfig:        coretesting.FakeControllerConfig(),
			AdminSecret:             testing.AdminSecret,
			CAPrivateKey:            coretesting.CAKey,
			SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
		})
	c.Assert(err, tc.ErrorIsNil)

	insts, err := env.AllRunningInstances(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	idsToStop := make([]instance.Id, len(insts)+1)
	for i, one := range insts {
		idsToStop[i] = one.Id()
	}
	idsToStop[len(insts)] = instance.Id("i-am-not-found")

	err = env.StopInstances(c.Context(), idsToStop...)
	// NotFound should be ignored
	c.Assert(err, tc.ErrorIsNil)
}

func (t *localServerSuite) TestDestroyErr(c *tc.C) {
	env := t.prepareAndBootstrap(c)

	msg := "terminate instances error"
	t.BaseSuite.PatchValue(ec2.TerminateInstancesById, func(context.Context, ec2.Client, ...instance.Id) ([]types.InstanceStateChange, error) {
		return nil, errors.New(msg)
	})

	err := env.Destroy(c.Context())
	c.Assert(errors.Cause(err).Error(), tc.Contains, msg)
}

func (t *localServerSuite) TestIAMRoleCleanup(c *tc.C) {
	t.useIAMRole = true
	t.srv.ec2srv.SetInitialInstanceState(ec2test.Running)
	env := t.prepareAndBootstrap(c)

	res, err := t.iamClient.ListInstanceProfiles(c.Context(), &iam.ListInstanceProfilesInput{
		PathPrefix: aws.String(fmt.Sprintf("/juju/controller/%s/", t.ControllerUUID)),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(res.InstanceProfiles), tc.Equals, 1)

	res1, err := t.iamClient.ListRoles(c.Context(), &iam.ListRolesInput{
		PathPrefix: aws.String(fmt.Sprintf("/juju/controller/%s/", t.ControllerUUID)),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(res1.Roles), tc.Equals, 1)

	err = env.DestroyController(c.Context(), t.ControllerUUID)
	c.Assert(err, tc.ErrorIsNil)

	res, err = t.iamClient.ListInstanceProfiles(c.Context(), &iam.ListInstanceProfilesInput{
		PathPrefix: aws.String(fmt.Sprintf("/juju/controller/%s/", t.ControllerUUID)),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(res.InstanceProfiles), tc.Equals, 0)

	res1, err = t.iamClient.ListRoles(c.Context(), &iam.ListRolesInput{
		PathPrefix: aws.String(fmt.Sprintf("/juju/controller/%s/", t.ControllerUUID)),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(res1.Roles), tc.Equals, 0)
}

func (t *localServerSuite) TestIAMRolePermissionProblems(c *tc.C) {
	t.srv.ec2srv.SetInitialInstanceState(ec2test.Running)
	t.srv.iamsrv.ProducePermissionError(true)
	defer t.srv.iamsrv.ProducePermissionError(false)
	env := t.prepareAndBootstrap(c)

	err := env.DestroyController(c.Context(), t.ControllerUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (t *localServerSuite) TestGetTerminatedInstances(c *tc.C) {
	env := t.Prepare(c)
	err := bootstrap.Bootstrap(t.BootstrapContext, env,
		bootstrap.BootstrapParams{
			ControllerConfig:        coretesting.FakeControllerConfig(),
			AdminSecret:             testing.AdminSecret,
			CAPrivateKey:            coretesting.CAKey,
			SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
		})
	c.Assert(err, tc.ErrorIsNil)

	// create another instance to terminate
	inst1, _ := testing.AssertStartInstance(c, env, t.ControllerUUID, "1")
	inst := t.srv.ec2srv.Instance(string(inst1.Id()))
	c.Assert(inst, tc.NotNil)
	t.BaseSuite.PatchValue(ec2.TerminateInstancesById, func(ctx context.Context, client ec2.Client, ids ...instance.Id) ([]types.InstanceStateChange, error) {
		// Terminate the one destined for termination and
		// err out to ensure that one instance will be terminated, the other - not.
		_, err = client.TerminateInstances(ctx, &awsec2.TerminateInstancesInput{
			InstanceIds: []string{string(inst1.Id())},
		})
		c.Assert(err, tc.ErrorIsNil)
		return nil, errors.New("terminate instances error")
	})
	err = env.Destroy(c.Context())
	c.Assert(err, tc.NotNil)

	terminated, err := ec2.TerminatedInstances(c, env)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(terminated, tc.HasLen, 1)
	c.Assert(terminated[0].Id(), tc.DeepEquals, inst1.Id())
}

func (t *localServerSuite) TestInstanceSecurityGroupsWithInstanceStatusFilter(c *tc.C) {
	env := t.prepareAndBootstrap(c)

	insts, err := env.AllRunningInstances(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	ids := make([]instance.Id, len(insts))
	for i, one := range insts {
		ids[i] = one.Id()
	}

	groupsNoInstanceFilter, err := ec2.InstanceSecurityGroups(env, c.Context(), ids)
	c.Assert(err, tc.ErrorIsNil)
	// get all security groups for test instances
	c.Assert(groupsNoInstanceFilter, tc.HasLen, 2)

	groupsFilteredForTerminatedInstances, err := ec2.InstanceSecurityGroups(env, c.Context(), ids, "shutting-down", "terminated")
	c.Assert(err, tc.ErrorIsNil)
	// get all security groups for terminated test instances
	c.Assert(groupsFilteredForTerminatedInstances, tc.HasLen, 0)
}

func (t *localServerSuite) TestDestroyControllerModelDeleteSecurityGroupInsistentlyError(c *tc.C) {
	env := t.prepareAndBootstrap(c)
	msg := "destroy security group error"
	t.BaseSuite.PatchValue(ec2.DeleteSecurityGroupInsistently, func(
		context.Context, ec2.SecurityGroupCleaner, types.GroupIdentifier, clock.Clock,
	) error {
		return errors.New(msg)
	})
	err := env.DestroyController(c.Context(), t.ControllerUUID)
	c.Assert(err, tc.ErrorMatches, "destroying managed models: "+msg)
}

func (t *localServerSuite) TestDestroyHostedModelDeleteSecurityGroupInsistentlyError(c *tc.C) {
	env := t.prepareAndBootstrap(c)
	hostedEnv, err := environs.New(t.BootstrapContext, environs.OpenParams{
		Cloud:  t.CloudSpec(),
		Config: env.Config(),
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, tc.ErrorIsNil)

	msg := "destroy security group error"
	t.BaseSuite.PatchValue(ec2.DeleteSecurityGroupInsistently, func(
		context.Context, ec2.SecurityGroupCleaner, types.GroupIdentifier, clock.Clock,
	) error {
		return errors.New(msg)
	})
	err = hostedEnv.Destroy(c.Context())
	c.Assert(err, tc.ErrorMatches, "cannot delete model security groups: "+msg)
}

func (t *localServerSuite) TestDestroyControllerDestroysHostedModelResources(c *tc.C) {
	controllerEnv := t.prepareAndBootstrap(c)

	// Create a hosted model with an instance and a volume.
	hostedModelUUID := "7e386e08-cba7-44a4-a76e-7c1633584210"
	t.srv.ec2srv.SetInitialInstanceState(ec2test.Running)
	cfg, err := controllerEnv.Config().Apply(map[string]interface{}{
		"uuid":          hostedModelUUID,
		"firewall-mode": "global",
	})
	c.Assert(err, tc.ErrorIsNil)
	env, err := environs.New(t.BootstrapContext, environs.OpenParams{
		Cloud:  t.CloudSpec(),
		Config: cfg,
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, tc.ErrorIsNil)
	inst, _ := testing.AssertStartInstance(c, env, t.ControllerUUID, "0")
	c.Assert(err, tc.ErrorIsNil)
	ebsProvider, err := env.StorageProvider(ec2.EBS_ProviderType)
	c.Assert(err, tc.ErrorIsNil)
	vs, err := ebsProvider.VolumeSource(nil)
	c.Assert(err, tc.ErrorIsNil)
	volumeResults, err := vs.CreateVolumes(c.Context(), []storage.VolumeParams{{
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(volumeResults, tc.HasLen, 1)
	c.Assert(volumeResults[0].Error, tc.ErrorIsNil)

	assertInstances := func(expect ...instance.Id) {
		insts, err := env.AllRunningInstances(c.Context())
		c.Assert(err, tc.ErrorIsNil)
		ids := make([]instance.Id, len(insts))
		for i, inst := range insts {
			ids[i] = inst.Id()
		}
		c.Assert(ids, tc.SameContents, expect)
	}
	assertVolumes := func(expect ...string) {
		volIds, err := vs.ListVolumes(c.Context())
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(volIds, tc.SameContents, expect)
	}
	assertGroups := func(expect ...string) {
		groupsResp, err := t.client.DescribeSecurityGroups(c.Context(), nil)
		c.Assert(err, tc.ErrorIsNil)
		names := make([]string, len(groupsResp.SecurityGroups))
		for i, group := range groupsResp.SecurityGroups {
			names[i] = aws.ToString(group.GroupName)
		}
		c.Assert(names, tc.SameContents, expect)
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
	err = controllerEnv.DestroyController(c.Context(), t.ControllerUUID)
	c.Assert(err, tc.ErrorIsNil)

	assertInstances()
	assertVolumes()
	assertGroups("default")
}

func (t *localServerSuite) TestInstanceStatus(c *tc.C) {
	env := t.Prepare(c)
	err := bootstrap.Bootstrap(t.BootstrapContext, env,
		bootstrap.BootstrapParams{
			ControllerConfig:        coretesting.FakeControllerConfig(),
			AdminSecret:             testing.AdminSecret,
			CAPrivateKey:            coretesting.CAKey,
			SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
		})
	c.Assert(err, tc.ErrorIsNil)
	t.srv.ec2srv.SetInitialInstanceState(ec2test.Terminated)
	inst, _ := testing.AssertStartInstance(c, env, t.ControllerUUID, "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(inst.Status(c.Context()).Message, tc.Equals, "terminated")
}

func (t *localServerSuite) TestInstancesCreatedWithIMDSv2(c *tc.C) {
	t.srv.ec2srv.SetInitialInstanceState(ec2test.Running)
	t.prepareAndBootstrap(c)

	output, err := t.srv.ec2srv.DescribeInstances(
		c.Context(), &awsec2.DescribeInstancesInput{
			Filters: []types.Filter{makeFilter("tag:"+tags.JujuController, t.ControllerUUID)},
		})
	c.Assert(err, tc.ErrorIsNil)

	for _, res := range output.Reservations {
		for _, inst := range res.Instances {
			c.Assert(inst.MetadataOptions, tc.NotNil)
			c.Assert(inst.MetadataOptions.HttpEndpoint, tc.Equals, types.InstanceMetadataEndpointStateEnabled)
		}
	}
}

func (t *localServerSuite) TestStartInstanceHardwareCharacteristics(c *tc.C) {
	env := t.prepareAndBootstrap(c)
	_, hc := testing.AssertStartInstance(c, env, t.ControllerUUID, "1")
	c.Check(*hc.Arch, tc.Equals, "amd64")
	c.Check(*hc.Mem, tc.Equals, uint64(8192))
	c.Check(*hc.CpuCores, tc.Equals, uint64(2))
}

func (t *localServerSuite) TestStartInstanceAvailZone(c *tc.C) {
	inst, err := t.testStartInstanceAvailZone(c, "test-available")
	c.Assert(err, tc.ErrorIsNil)
	ec2Inst := ec2.InstanceSDKEC2(inst)
	c.Assert(aws.ToString(ec2Inst.Placement.AvailabilityZone), tc.Equals, "test-available")
}

func (t *localServerSuite) TestStartInstanceAvailZoneImpaired(c *tc.C) {
	_, err := t.testStartInstanceAvailZone(c, "test-impaired")
	c.Assert(err, tc.ErrorMatches, `availability zone "test-impaired" is "impaired"`)
}

func (t *localServerSuite) TestStartInstanceAvailZoneUnknown(c *tc.C) {
	_, err := t.testStartInstanceAvailZone(c, "test-unknown")
	c.Assert(err, tc.Not(tc.ErrorIs), environs.ErrAvailabilityZoneIndependent)
	c.Assert(errors.Details(err), tc.Matches, `.*availability zone \"\" not valid.*`)
}

func (t *localServerSuite) testStartInstanceAvailZone(c *tc.C, zone string) (instances.Instance, error) {
	env := t.prepareAndBootstrap(c)

	params := environs.StartInstanceParams{ControllerUUID: t.ControllerUUID, AvailabilityZone: zone, StatusCallback: fakeCallback}
	result, err := testing.StartInstanceWithParams(c, env, "1", params)
	if err != nil {
		return nil, err
	}
	return result.Instance, nil
}

func (t *localServerSuite) TestStartInstanceVolumeAttachmentsAvailZone(c *tc.C) {
	env := t.prepareAndBootstrap(c)
	resp, err := t.client.CreateVolume(c.Context(), &awsec2.CreateVolumeInput{
		Size:             aws.Int32(1),
		VolumeType:       "gp2",
		AvailabilityZone: aws.String("volume-zone"),
	})
	c.Assert(err, tc.ErrorIsNil)

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
	result, err := testing.StartInstanceWithParams(c, env, "1", args)
	c.Assert(err, tc.ErrorIsNil)
	ec2Inst := ec2.InstanceSDKEC2(result.Instance)
	c.Assert(aws.ToString(ec2Inst.Placement.AvailabilityZone), tc.Equals, "volume-zone")
}

func (t *localServerSuite) TestStartInstanceVolumeAttachmentsAvailZonePlacementConflicts(c *tc.C) {
	env := t.prepareAndBootstrap(c)
	resp, err := t.client.CreateVolume(c.Context(), &awsec2.CreateVolumeInput{
		Size:             aws.Int32(1),
		VolumeType:       "gp2",
		AvailabilityZone: aws.String("volume-zone"),
	})
	c.Assert(err, tc.ErrorIsNil)

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
	_, err = testing.StartInstanceWithParams(c, env, "1", args)
	c.Assert(err, tc.ErrorMatches, `cannot create instance with placement "zone=test-available", as this will prevent attaching the requested EBS volumes in zone "volume-zone"`)
}

func (t *localServerSuite) TestStartInstanceZoneIndependent(c *tc.C) {
	env := t.prepareAndBootstrap(c)
	params := environs.StartInstanceParams{
		ControllerUUID:   t.ControllerUUID,
		StatusCallback:   fakeCallback,
		AvailabilityZone: "test-available",
		Placement:        "nonsense",
	}
	_, err := testing.StartInstanceWithParams(c, env, "1", params)
	c.Assert(err, tc.ErrorMatches, "unknown placement directive: nonsense")
	// The returned error should indicate that it is independent
	// of the availability zone specified.
	c.Assert(err, tc.ErrorIs, environs.ErrAvailabilityZoneIndependent)
}

func (t *localServerSuite) TestStartInstanceSubnet(c *tc.C) {
	inst, err := t.testStartInstanceSubnet(c, "10.1.2.0/24")
	c.Assert(err, tc.ErrorIsNil)
	ec2Inst := ec2.InstanceSDKEC2(inst)
	c.Assert(aws.ToString(ec2Inst.Placement.AvailabilityZone), tc.Equals, "test-available")
}

func (t *localServerSuite) TestStartInstanceSubnetUnavailable(c *tc.C) {
	// See addTestingSubnets, 10.1.3.0/24 is in state "unavailable", but is in
	// an AZ that would otherwise be available
	_, err := t.testStartInstanceSubnet(c, "10.1.3.0/24")
	c.Assert(err, tc.ErrorMatches, `subnet "10.1.3.0/24" is "pending"`)
}

func (t *localServerSuite) TestStartInstanceSubnetAZUnavailable(c *tc.C) {
	// See addTestingSubnets, 10.1.4.0/24 is in an AZ that is unavailable
	_, err := t.testStartInstanceSubnet(c, "10.1.4.0/24")
	c.Assert(err, tc.ErrorMatches, `availability zone "test-unavailable" is "unavailable"`)
}

func (t *localServerSuite) testStartInstanceSubnet(c *tc.C, subnet string) (instances.Instance, error) {
	subIDs, vpcId := t.addTestingSubnets(c)
	env := t.prepareAndBootstrapWithConfig(c, coretesting.Attrs{"vpc-id": vpcId, "vpc-id-force": true})
	params := environs.StartInstanceParams{
		ControllerUUID: t.ControllerUUID,
		Placement:      fmt.Sprintf("subnet=%s", subnet),
		SubnetsToZones: []map[network.Id][]string{{
			subIDs[0]: {"test-available"},
			subIDs[1]: {"test-available"},
			subIDs[2]: {"test-unavailable"},
		}},
	}
	zonedEnviron := env.(common.ZonedEnviron)
	zones, err := zonedEnviron.DeriveAvailabilityZones(c.Context(), params)
	if err != nil {
		return nil, err
	}
	if len(zones) > 0 {
		params.AvailabilityZone = zones[0]
		result, err := testing.StartInstanceWithParams(c, env, "1", params)
		if err != nil {
			return nil, err
		}
		return result.Instance, nil
	}
	return nil, errors.Errorf("testStartInstanceSubnet failed")
}

func (t *localServerSuite) TestDeriveAvailabilityZoneSubnetWrongVPC(c *tc.C) {
	subIDs, vpcId := t.addTestingSubnets(c)
	c.Assert(vpcId, tc.Not(tc.Equals), "vpc-0")
	env := t.prepareAndBootstrapWithConfig(c, coretesting.Attrs{"vpc-id": "vpc-0", "vpc-id-force": true})
	params := environs.StartInstanceParams{
		ControllerUUID: t.ControllerUUID,
		Placement:      "subnet=10.1.2.0/24",
		SubnetsToZones: []map[network.Id][]string{{
			subIDs[0]: {"test-available"},
			subIDs[1]: {"test-available"},
			subIDs[2]: {"test-unavailable"},
		}},
	}
	zonedEnviron := env.(common.ZonedEnviron)
	_, err := zonedEnviron.DeriveAvailabilityZones(c.Context(), params)
	c.Assert(err, tc.ErrorMatches, `unable to find subnet "10.1.2.0/24" in .* for vpi-id "vpc-0"`)
}

func (t *localServerSuite) TestGetAvailabilityZones(c *tc.C) {
	var resultZones []types.AvailabilityZone
	var resultErr error
	t.PatchValue(ec2.EC2AvailabilityZones, func(c ec2.Client, ctx context.Context, params *awsec2.DescribeAvailabilityZonesInput, optFns ...func(*awsec2.Options)) (*awsec2.DescribeAvailabilityZonesOutput, error) {
		resp := &awsec2.DescribeAvailabilityZonesOutput{
			AvailabilityZones: append([]types.AvailabilityZone{}, resultZones...),
		}
		return resp, resultErr
	})
	env := t.Prepare(c).(common.ZonedEnviron)

	resultErr = fmt.Errorf("failed to get availability zones")
	zones, err := env.AvailabilityZones(c.Context())
	c.Assert(err, tc.ErrorIs, resultErr)
	c.Assert(zones, tc.IsNil)

	resultErr = nil
	resultZones = make([]types.AvailabilityZone, 1)
	resultZones[0].ZoneName = aws.String("whatever")
	zones, err = env.AvailabilityZones(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(zones, tc.HasLen, 1)
	c.Assert(zones[0].Name(), tc.Equals, "whatever")
}

func (t *localServerSuite) TestGetAvailabilityZonesCommon(c *tc.C) {
	var resultZones []types.AvailabilityZone
	t.PatchValue(ec2.EC2AvailabilityZones, func(c ec2.Client, ctx context.Context, params *awsec2.DescribeAvailabilityZonesInput, optFns ...func(*awsec2.Options)) (*awsec2.DescribeAvailabilityZonesOutput, error) {
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
	zones, err := env.AvailabilityZones(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(zones, tc.HasLen, 2)
	c.Assert(zones[0].Name(), tc.Equals, "az1")
	c.Assert(zones[1].Name(), tc.Equals, "az2")
	c.Assert(zones[0].Available(), tc.IsTrue)
	c.Assert(zones[1].Available(), tc.IsFalse)
}

func (t *localServerSuite) TestDeriveAvailabilityZones(c *tc.C) {
	var resultZones []types.AvailabilityZone
	t.PatchValue(ec2.EC2AvailabilityZones, func(c ec2.Client, ctx context.Context, params *awsec2.DescribeAvailabilityZonesInput, optFns ...func(*awsec2.Options)) (*awsec2.DescribeAvailabilityZonesOutput, error) {
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

	zones, err := env.DeriveAvailabilityZones(c.Context(), environs.StartInstanceParams{Placement: "zone=az1"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(zones, tc.DeepEquals, []string{"az1"})
}

func (t *localServerSuite) TestDeriveAvailabilityZonesImpaired(c *tc.C) {
	var resultZones []types.AvailabilityZone
	t.PatchValue(ec2.EC2AvailabilityZones, func(c ec2.Client, ctx context.Context, params *awsec2.DescribeAvailabilityZonesInput, optFns ...func(*awsec2.Options)) (*awsec2.DescribeAvailabilityZonesOutput, error) {
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

	zones, err := env.DeriveAvailabilityZones(c.Context(), environs.StartInstanceParams{Placement: "zone=az2"})
	c.Assert(err, tc.ErrorMatches, "availability zone \"az2\" is \"impaired\"")
	c.Assert(zones, tc.HasLen, 0)
}

func (t *localServerSuite) TestDeriveAvailabilityZonesConflictVolume(c *tc.C) {
	resp, err := t.client.CreateVolume(c.Context(), &awsec2.CreateVolumeInput{
		Size:             aws.Int32(1),
		VolumeType:       "gp2",
		AvailabilityZone: aws.String("volume-zone"),
	})
	c.Assert(err, tc.ErrorIsNil)

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
	zones, err := env.DeriveAvailabilityZones(c.Context(), args)
	c.Assert(err, tc.ErrorMatches, `cannot create instance with placement "zone=test-available", as this will prevent attaching the requested EBS volumes in zone "volume-zone"`)
	c.Assert(zones, tc.HasLen, 0)
}

func (t *localServerSuite) TestDeriveAvailabilityZonesVolumeNoPlacement(c *tc.C) {
	resp, err := t.client.CreateVolume(c.Context(), &awsec2.CreateVolumeInput{
		Size:             aws.Int32(1),
		VolumeType:       "gp2",
		AvailabilityZone: aws.String("volume-zone"),
	})
	c.Assert(err, tc.ErrorIsNil)

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
	zones, err := env.DeriveAvailabilityZones(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(zones, tc.DeepEquals, []string{"volume-zone"})
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

func (t *localServerSuite) TestStartInstanceAvailZoneAllConstrained(c *tc.C) {
	t.testStartInstanceAvailZoneAllConstrained(c, azConstrainedErr)
}

func (t *localServerSuite) TestStartInstanceVolumeTypeNotAvailable(c *tc.C) {
	t.testStartInstanceAvailZoneAllConstrained(c, azVolumeTypeNotAvailableInZoneErr)
}

func (t *localServerSuite) TestStartInstanceAvailZoneAllInsufficientInstanceCapacity(c *tc.C) {
	t.testStartInstanceAvailZoneAllConstrained(c, azInsufficientInstanceCapacityErr)
}

func (t *localServerSuite) TestStartInstanceAvailZoneAllNoDefaultSubnet(c *tc.C) {
	t.testStartInstanceAvailZoneAllConstrained(c, azNoDefaultSubnetErr)
}

func (t *localServerSuite) testStartInstanceAvailZoneAllConstrained(c *tc.C, runInstancesError smithy.APIError) {
	env := t.prepareAndBootstrap(c)

	t.PatchValue(ec2.RunInstances, func(ctx context.Context, e ec2.Client, ri *awsec2.RunInstancesInput, callback environs.StatusCallbackFunc) (resp *awsec2.RunInstancesOutput, err error) {
		return nil, runInstancesError
	})

	params := environs.StartInstanceParams{
		ControllerUUID:   t.ControllerUUID,
		StatusCallback:   fakeCallback,
		AvailabilityZone: "test-available",
	}

	_, err := testing.StartInstanceWithParams(c, env, "1", params)
	// All AZConstrained failures should return an error that does
	// Is(err, environs.ErrAvailabilityZoneIndependent)
	// so the caller knows to try a new zone, rather than fail.
	c.Assert(err, tc.Not(tc.ErrorIs), environs.ErrAvailabilityZoneIndependent)
	c.Assert(errors.Details(err), tc.Contains, runInstancesError.ErrorMessage())
}

// addTestingNetworkInterface adds one network interface with vpc id and
// availability zone. It will also have a private IP with no
func (t *localServerSuite) addTestingNetworkInterfaceToInstance(c *tc.C, instId instance.Id) ([]instance.Id, string) {
	vpc := t.srv.ec2srv.AddVpc(types.Vpc{
		CidrBlock: aws.String("10.1.0.0/16"),
		IsDefault: aws.Bool(true),
	})
	sub, err := t.srv.ec2srv.AddSubnet(types.Subnet{
		VpcId:            vpc.VpcId,
		CidrBlock:        aws.String("10.1.2.0/24"),
		AvailabilityZone: aws.String("test-available"),
		State:            "available",
		DefaultForAz:     aws.Bool(true),
	})
	c.Assert(err, tc.ErrorIsNil)
	results := make([]instance.Id, 1)
	iface1, err := t.srv.ec2srv.AddNetworkInterface(types.NetworkInterface{
		VpcId:              vpc.VpcId,
		AvailabilityZone:   aws.String("test-available"),
		PrivateIpAddresses: []types.NetworkInterfacePrivateIpAddress{{}},
		Attachment: &types.NetworkInterfaceAttachment{
			DeviceIndex: aws.Int32(-1),
			InstanceId:  aws.String(string(instId)),
		},
		SubnetId: sub.SubnetId,
	})
	c.Assert(err, tc.ErrorIsNil)
	results[0] = instance.Id(aws.ToString(iface1.NetworkInterfaceId))
	return results, aws.ToString(vpc.VpcId)
}

// addTestingSubnets adds a testing default VPC with 3 subnets in the EC2 test
// server: 2 of the subnets are in the "test-available" AZ, the remaining - in
// "test-unavailable". Returns a slice with the IDs of the created subnets and
// vpc id that those were added to
func (t *localServerSuite) addTestingSubnets(c *tc.C) ([]network.Id, string) {
	vpc := t.srv.ec2srv.AddVpc(types.Vpc{
		CidrBlock: aws.String("10.1.0.0/16"),
		Ipv6CidrBlockAssociationSet: []types.VpcIpv6CidrBlockAssociation{
			{
				AssociationId: aws.String("123"),
				Ipv6CidrBlock: aws.String("fdef:265c:1fdd::/48"),
			},
		},
		IsDefault: aws.Bool(true),
	})
	results := make([]network.Id, 3)
	sub1, err := t.srv.ec2srv.AddSubnet(types.Subnet{
		VpcId:                       vpc.VpcId,
		AssignIpv6AddressOnCreation: aws.Bool(true),
		CidrBlock:                   aws.String("10.1.2.0/24"),
		AvailabilityZone:            aws.String("test-available"),
		State:                       types.SubnetStateAvailable,
		DefaultForAz:                aws.Bool(true),
	})
	c.Assert(err, tc.ErrorIsNil)
	results[0] = network.Id(aws.ToString(sub1.SubnetId))
	sub2, err := t.srv.ec2srv.AddSubnet(types.Subnet{
		VpcId:            vpc.VpcId,
		CidrBlock:        aws.String("10.1.3.0/24"),
		AvailabilityZone: aws.String("test-available"),
		State:            types.SubnetStatePending,
	})
	c.Assert(err, tc.ErrorIsNil)
	results[1] = network.Id(aws.ToString(sub2.SubnetId))
	sub3, err := t.srv.ec2srv.AddSubnet(types.Subnet{
		VpcId:            vpc.VpcId,
		CidrBlock:        aws.String("10.1.4.0/24"),
		AvailabilityZone: aws.String("test-unavailable"),
		DefaultForAz:     aws.Bool(true),
		State:            types.SubnetStatePending,
	})
	c.Assert(err, tc.ErrorIsNil)
	results[2] = network.Id(aws.ToString(sub3.SubnetId))

	return results, aws.ToString(vpc.VpcId)
}

func (t *localServerSuite) addDualStackNetwork(c *tc.C) ([]network.Id, string) {
	vpc := t.srv.ec2srv.AddVpc(types.Vpc{
		CidrBlock: aws.String("10.1.0.0/16"),
		Ipv6CidrBlockAssociationSet: []types.VpcIpv6CidrBlockAssociation{
			{
				AssociationId: aws.String("123"),
				Ipv6CidrBlock: aws.String("fdef:265c:1fdd::/48"),
			},
		},
		IsDefault: aws.Bool(true),
		State:     types.VpcStateAvailable,
	})
	results := make([]network.Id, 3)
	sub1, err := t.srv.ec2srv.AddSubnet(types.Subnet{
		VpcId:                       vpc.VpcId,
		AssignIpv6AddressOnCreation: aws.Bool(true),
		CidrBlock:                   aws.String("10.1.2.0/24"),
		AvailabilityZone:            aws.String("test-available"),
		State:                       types.SubnetStateAvailable,
	})
	c.Assert(err, tc.ErrorIsNil)
	results[0] = network.Id(aws.ToString(sub1.SubnetId))
	sub2, err := t.srv.ec2srv.AddSubnet(types.Subnet{
		VpcId:                       vpc.VpcId,
		AssignIpv6AddressOnCreation: aws.Bool(true),
		CidrBlock:                   aws.String("10.1.3.0/24"),
		AvailabilityZone:            aws.String("test-available"),
		State:                       types.SubnetStateAvailable,
	})
	c.Assert(err, tc.ErrorIsNil)
	results[1] = network.Id(aws.ToString(sub2.SubnetId))
	sub3, err := t.srv.ec2srv.AddSubnet(types.Subnet{
		VpcId:                       vpc.VpcId,
		AssignIpv6AddressOnCreation: aws.Bool(true),
		CidrBlock:                   aws.String("10.1.4.0/24"),
		AvailabilityZone:            aws.String("test-available2"),
		DefaultForAz:                aws.Bool(true),
		State:                       types.SubnetStateAvailable,
	})
	c.Assert(err, tc.ErrorIsNil)
	results[2] = network.Id(aws.ToString(sub3.SubnetId))

	igw, err := t.srv.ec2srv.AddInternetGateway(types.InternetGateway{
		Attachments: []types.InternetGatewayAttachment{{
			VpcId: vpc.VpcId,
			State: "available",
		}},
	})
	c.Assert(err, tc.ErrorIsNil)

	routes := []types.Route{{
		DestinationCidrBlock: vpc.CidrBlock, // default Vpc internal traffic
		GatewayId:            aws.String("local"),
		State:                types.RouteStateActive,
	}, {
		DestinationCidrBlock: aws.String("0.0.0.0/0"), // default Vpc default egress route.
		GatewayId:            igw.InternetGatewayId,
		State:                types.RouteStateActive,
	}, {
		DestinationIpv6CidrBlock: aws.String("::/0"),
		GatewayId:                igw.InternetGatewayId,
		State:                    types.RouteStateActive,
	}}

	for _, ipv6Assoc := range vpc.Ipv6CidrBlockAssociationSet {
		routes = append(routes, types.Route{
			DestinationIpv6CidrBlock: ipv6Assoc.Ipv6CidrBlock,
			GatewayId:                aws.String("local"),
			State:                    types.RouteStateActive,
		})
	}

	_, err = t.srv.ec2srv.AddRouteTable(types.RouteTable{
		VpcId: vpc.VpcId,
		Associations: []types.RouteTableAssociation{{
			Main: aws.Bool(true),
		}},
		Routes: routes,
	})
	c.Assert(err, tc.ErrorIsNil)

	return results, aws.ToString(vpc.VpcId)
}

func (t *localServerSuite) addSingleStackIpv6Network(c *tc.C) ([]network.Id, string) {
	vpc := t.srv.ec2srv.AddVpc(types.Vpc{
		CidrBlock: aws.String("10.1.0.0/16"),
		Ipv6CidrBlockAssociationSet: []types.VpcIpv6CidrBlockAssociation{
			{
				AssociationId: aws.String("123"),
				Ipv6CidrBlock: aws.String("fdef:265c:1fdd::/48"),
			},
		},
		IsDefault: aws.Bool(true),
		State:     types.VpcStateAvailable,
	})
	results := make([]network.Id, 3)
	sub1, err := t.srv.ec2srv.AddSubnet(types.Subnet{
		VpcId:                       vpc.VpcId,
		AssignIpv6AddressOnCreation: aws.Bool(true),
		Ipv6CidrBlockAssociationSet: []types.SubnetIpv6CidrBlockAssociation{
			{
				AssociationId: aws.String("123"),
				Ipv6CidrBlock: aws.String("fdef:265c:1fdd:0001::/64"),
			},
		},
		AvailabilityZone: aws.String("test-available"),
		Ipv6Native:       aws.Bool(true),
		State:            types.SubnetStateAvailable,
	})
	c.Assert(err, tc.ErrorIsNil)
	results[0] = network.Id(aws.ToString(sub1.SubnetId))
	sub2, err := t.srv.ec2srv.AddSubnet(types.Subnet{
		VpcId:                       vpc.VpcId,
		AssignIpv6AddressOnCreation: aws.Bool(true),
		Ipv6CidrBlockAssociationSet: []types.SubnetIpv6CidrBlockAssociation{
			{
				AssociationId: aws.String("123"),
				Ipv6CidrBlock: aws.String("fdef:265c:1fdd:0002::/64"),
			},
		},
		AvailabilityZone: aws.String("test-available"),
		Ipv6Native:       aws.Bool(true),
		State:            types.SubnetStateAvailable,
	})
	c.Assert(err, tc.ErrorIsNil)
	results[1] = network.Id(aws.ToString(sub2.SubnetId))
	sub3, err := t.srv.ec2srv.AddSubnet(types.Subnet{
		VpcId:                       vpc.VpcId,
		AssignIpv6AddressOnCreation: aws.Bool(true),
		Ipv6CidrBlockAssociationSet: []types.SubnetIpv6CidrBlockAssociation{
			{
				AssociationId: aws.String("123"),
				Ipv6CidrBlock: aws.String("fdef:265c:1fdd:0003::/64"),
			},
		},
		AvailabilityZone: aws.String("test-available2"),
		Ipv6Native:       aws.Bool(true),
		DefaultForAz:     aws.Bool(true),
		State:            types.SubnetStateAvailable,
	})
	c.Assert(err, tc.ErrorIsNil)
	results[2] = network.Id(aws.ToString(sub3.SubnetId))

	igw, err := t.srv.ec2srv.AddInternetGateway(types.InternetGateway{
		Attachments: []types.InternetGatewayAttachment{{
			VpcId: vpc.VpcId,
			State: "available",
		}},
	})
	c.Assert(err, tc.ErrorIsNil)

	routes := []types.Route{{
		DestinationCidrBlock: vpc.CidrBlock, // default Vpc internal traffic
		GatewayId:            aws.String("local"),
		State:                types.RouteStateActive,
	}, {
		DestinationCidrBlock: aws.String("0.0.0.0/0"), // default Vpc default egress route.
		GatewayId:            igw.InternetGatewayId,
		State:                types.RouteStateActive,
	}, {
		DestinationIpv6CidrBlock: aws.String("::/0"),
		GatewayId:                igw.InternetGatewayId,
		State:                    types.RouteStateActive,
	}}

	for _, ipv6Assoc := range vpc.Ipv6CidrBlockAssociationSet {
		routes = append(routes, types.Route{
			DestinationIpv6CidrBlock: ipv6Assoc.Ipv6CidrBlock,
			GatewayId:                aws.String("local"),
			State:                    types.RouteStateActive,
		})
	}

	_, err = t.srv.ec2srv.AddRouteTable(types.RouteTable{
		VpcId: vpc.VpcId,
		Associations: []types.RouteTableAssociation{{
			Main: aws.Bool(true),
		}},
		Routes: routes,
	})
	c.Assert(err, tc.ErrorIsNil)

	return results, aws.ToString(vpc.VpcId)
}

func (t *localServerSuite) prepareAndBootstrap(c *tc.C) environs.Environ {
	return t.prepareAndBootstrapWithConfig(c, coretesting.Attrs{})
}

func (t *localServerSuite) prepareAndBootstrapWithConfig(c *tc.C, config coretesting.Attrs) environs.Environ {
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
		bootstrap.BootstrapParams{
			BootstrapConstraints:    constraints,
			ControllerConfig:        controllerConfig,
			AdminSecret:             testing.AdminSecret,
			CAPrivateKey:            coretesting.CAKey,
			Placement:               "zone=test-available",
			SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
		})
	c.Assert(err, tc.ErrorIsNil)
	return env
}

func (t *localServerSuite) TestSpaceConstraintsSpaceNotInPlacementZone(c *tc.C) {
	env := t.prepareAndBootstrap(c)
	subIDs, _ := t.addTestingSubnets(c)

	// Expect an error because zone test-available isn't in SubnetsToZones
	params := environs.StartInstanceParams{
		ControllerUUID: t.ControllerUUID,
		Placement:      "zone=test-available",
		Constraints:    constraints.MustParse("spaces=aaaaaaaaaa"),
		SubnetsToZones: []map[network.Id][]string{{
			subIDs[0]: {"zone2"},
			subIDs[1]: {"zone3"},
			subIDs[2]: {"zone4"},
		}},
		StatusCallback: fakeCallback,
	}
	_, err := testing.StartInstanceWithParams(c, env, "1", params)
	c.Assert(err, tc.Not(tc.ErrorIs), environs.ErrAvailabilityZoneIndependent)
	c.Assert(errors.Details(err), tc.Matches, `.*subnets in AZ "test-available" not found.*`)
}

func (t *localServerSuite) TestSpaceConstraintsSpaceInPlacementZone(c *tc.C) {
	env := t.prepareAndBootstrap(c)
	subIDs, _ := t.addTestingSubnets(c)

	// Should work - test-available is in SubnetsToZones and in myspace.
	params := environs.StartInstanceParams{
		ControllerUUID: t.ControllerUUID,
		Placement:      "zone=test-available",
		Constraints:    constraints.MustParse("spaces=aaaaaaaaaa"),
		SubnetsToZones: []map[network.Id][]string{{
			subIDs[0]: {"test-available"},
			subIDs[1]: {"zone3"},
		}},
		StatusCallback: fakeCallback,
	}
	res, err := testing.StartInstanceWithParams(c, env, "1", params)
	c.Assert(err, tc.ErrorIsNil)

	// Here we're asserting that the placement happened in the backend.
	// The post condition.
	instances, err := t.client.DescribeInstances(c.Context(), &awsec2.DescribeInstancesInput{
		InstanceIds: []string{string(res.Instance.Id())},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(instances.Reservations), tc.Equals, 1)
	c.Assert(len(instances.Reservations[0].Instances), tc.Equals, 1)

	instance := instances.Reservations[0].Instances[0]
	c.Assert(*instance.SubnetId, tc.Equals, string(subIDs[0]))
}

func (t *localServerSuite) TestIPv6SubnetSelectionWithVPC(c *tc.C) {
	subIDs, vpcId := t.addDualStackNetwork(c)

	env := t.prepareAndBootstrapWithConfig(c, coretesting.Attrs{"vpc-id": vpcId})

	instances, err := t.client.DescribeInstances(c.Context(), &awsec2.DescribeInstancesInput{
		Filters: []types.Filter{makeFilter("tag:"+tags.JujuController, t.ControllerUUID)},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(instances.Reservations), tc.Equals, 1)
	c.Assert(len(instances.Reservations[0].Instances), tc.Equals, 1)
	instance := instances.Reservations[0].Instances[0]

	inSubnet := false
	for _, subID := range subIDs {
		if inSubnet = subID == network.Id(*instance.SubnetId); inSubnet {
			break
		}
	}
	c.Assert(inSubnet, tc.IsTrue)

	// Should work - test-available is in SubnetsToZones and in myspace.
	params := environs.StartInstanceParams{
		AvailabilityZone: "test-available2",
		ControllerUUID:   t.ControllerUUID,
		StatusCallback:   fakeCallback,
	}
	res, err := testing.StartInstanceWithParams(c, env, "1", params)
	c.Assert(err, tc.ErrorIsNil)

	// Here we're asserting that the placement happened in the backend.
	// The post condition.
	instances, err = t.client.DescribeInstances(c.Context(), &awsec2.DescribeInstancesInput{
		InstanceIds: []string{string(res.Instance.Id())},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(instances.Reservations), tc.Equals, 1)
	c.Assert(len(instances.Reservations[0].Instances), tc.Equals, 1)

	instance = instances.Reservations[0].Instances[0]
	c.Assert(*instance.SubnetId, tc.Equals, string(subIDs[2]))
}

func (t *localServerSuite) TestSingleStackIPv6(c *tc.C) {
	subIDs, vpcId := t.addSingleStackIpv6Network(c)

	env := t.prepareAndBootstrapWithConfig(c, coretesting.Attrs{"vpc-id": vpcId})

	instances, err := t.client.DescribeInstances(c.Context(), &awsec2.DescribeInstancesInput{
		Filters: []types.Filter{makeFilter("tag:"+tags.JujuController, t.ControllerUUID)},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(instances.Reservations), tc.Equals, 1)
	c.Assert(len(instances.Reservations[0].Instances), tc.Equals, 1)
	instance := instances.Reservations[0].Instances[0]

	inSubnet := false
	for _, subID := range subIDs {
		if inSubnet = subID == network.Id(*instance.SubnetId); inSubnet {
			break
		}
	}
	c.Assert(inSubnet, tc.IsTrue)
	c.Assert(instance.Ipv6Address, tc.NotNil)
	c.Check(*instance.Ipv6Address != "", tc.IsTrue)

	params := environs.StartInstanceParams{
		AvailabilityZone: "test-available2",
		ControllerUUID:   t.ControllerUUID,
		StatusCallback:   fakeCallback,
	}
	res, err := testing.StartInstanceWithParams(c, env, "1", params)
	c.Assert(err, tc.ErrorIsNil)

	addresses, err := res.Instance.Addresses(c.Context())
	hasV6Address := false
	for _, address := range addresses {
		if hasV6Address = address.MachineAddress.Type == network.IPv6Address; hasV6Address {
			break
		}
	}
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hasV6Address, tc.IsTrue)
}

func (t *localServerSuite) TestSpaceConstraintsNoPlacement(c *tc.C) {
	env := t.prepareAndBootstrap(c)
	subIDs, _ := t.addTestingSubnets(c)

	params := environs.StartInstanceParams{
		ControllerUUID: t.ControllerUUID,
		Constraints:    constraints.MustParse("spaces=aaaaaaaaaa"),
		SubnetsToZones: []map[network.Id][]string{{
			subIDs[0]: {"test-available"},
			subIDs[1]: {"zone3"},
		}},
		StatusCallback: fakeCallback,
	}
	t.assertStartInstanceWithParamsFindAZ(c, env, "1", params)
}

func (t *localServerSuite) TestIPv6Subnet(c *tc.C) {

}

func (t *localServerSuite) assertStartInstanceWithParamsFindAZ(
	c *tc.C,
	env environs.Environ,
	machineId string,
	params environs.StartInstanceParams,
) {
	zonedEnviron := env.(common.ZonedEnviron)
	zones, err := zonedEnviron.DeriveAvailabilityZones(c.Context(), params)
	c.Assert(err, tc.ErrorIsNil)
	if len(zones) > 0 {
		params.AvailabilityZone = zones[0]
		_, err = testing.StartInstanceWithParams(c, env, "1", params)
		c.Assert(err, tc.ErrorIsNil)
		return
	}
	availabilityZones, err := zonedEnviron.AvailabilityZones(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	for _, zone := range availabilityZones {
		if !zone.Available() {
			continue
		}
		params.AvailabilityZone = zone.Name()
		_, err = testing.StartInstanceWithParams(c, env, "1", params)
		if err == nil {
			return
		} else if !errors.Is(err, environs.ErrAvailabilityZoneIndependent) {
			continue
		}
		c.Assert(err, tc.ErrorIsNil)
	}
}

func (t *localServerSuite) TestSpaceConstraintsNoAvailableSubnets(c *tc.C) {
	c.Skip("temporarily disabled")
	subIDs, vpcId := t.addTestingSubnets(c)
	env := t.prepareAndBootstrapWithConfig(c, coretesting.Attrs{"vpc-id": vpcId})

	// We requested a space, but there are no subnets in SubnetsToZones, so we can't resolve
	// the constraints
	params := environs.StartInstanceParams{
		ControllerUUID: t.ControllerUUID,
		Constraints:    constraints.MustParse("spaces=aaaaaaaaaa"),
		SubnetsToZones: []map[network.Id][]string{{
			subIDs[0]: {""},
		}},
		StatusCallback: fakeCallback,
	}
	// _, err := testing.StartInstanceWithParams(env, "1", params)
	zonedEnviron := env.(common.ZonedEnviron)
	_, err := zonedEnviron.DeriveAvailabilityZones(c.Context(), params)
	c.Assert(err, tc.ErrorMatches, `unable to resolve constraints: space and/or subnet unavailable in zones \[test-available\]`)
}

func (t *localServerSuite) TestStartInstanceNoPublicIP(c *tc.C) {
	env := t.prepareAndBootstrap(c)

	params := environs.StartInstanceParams{
		ControllerUUID: t.ControllerUUID,
		Constraints:    constraints.MustParse("allocate-public-ip=false"),
		StatusCallback: fakeCallback,
	}
	inst, err := testing.StartInstanceWithParams(c, env, "1", params)
	c.Assert(err, tc.ErrorIsNil)

	nics, err := t.srv.ec2srv.DescribeNetworkInterfaces(c.Context(), &awsec2.DescribeNetworkInterfacesInput{
		Filters: []types.Filter{makeFilter("attachment.instance-id", string(inst.Instance.Id()))},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(nics.NetworkInterfaces), tc.Equals, 1)
	c.Assert(nics.NetworkInterfaces[0].Association, tc.NotNil)
	c.Assert(nics.NetworkInterfaces[0].Association.PublicIp, tc.IsNil)
}

func (t *localServerSuite) TestStartInstanceAvailZoneOneConstrained(c *tc.C) {
	t.testStartInstanceAvailZoneOneConstrained(c, azConstrainedErr)
}

func (t *localServerSuite) TestStartInstanceAvailZoneOneInsufficientInstanceCapacity(c *tc.C) {
	t.testStartInstanceAvailZoneOneConstrained(c, azInsufficientInstanceCapacityErr)
}

func (t *localServerSuite) TestStartInstanceAvailZoneOneNoDefaultSubnetErr(c *tc.C) {
	t.testStartInstanceAvailZoneOneConstrained(c, azNoDefaultSubnetErr)
}

func (t *localServerSuite) testStartInstanceAvailZoneOneConstrained(c *tc.C, runInstancesError smithy.APIError) {
	env := t.prepareAndBootstrap(c)

	// The first call to RunInstances fails with an error indicating the AZ
	// is constrained. The second attempt succeeds, and so allocates to az2.
	var azArgs []string
	realRunInstances := *ec2.RunInstances

	t.PatchValue(ec2.RunInstances, func(ctx context.Context, e ec2.Client, ri *awsec2.RunInstancesInput, callback environs.StatusCallbackFunc) (resp *awsec2.RunInstancesOutput, err error) {
		azArgs = append(azArgs, aws.ToString(ri.Placement.AvailabilityZone))
		if len(azArgs) == 1 {
			return nil, runInstancesError
		}
		return realRunInstances(ctx, e, ri, fakeCallback)
	})

	params := environs.StartInstanceParams{ControllerUUID: t.ControllerUUID}
	zonedEnviron := env.(common.ZonedEnviron)
	availabilityZones, err := zonedEnviron.AvailabilityZones(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	for _, zone := range availabilityZones {
		if !zone.Available() {
			continue
		}
		params.AvailabilityZone = zone.Name()
		_, err = testing.StartInstanceWithParams(c, env, "1", params)
		if err == nil {
			break
		} else if !errors.Is(err, environs.ErrAvailabilityZoneIndependent) {
			continue
		}
		c.Assert(err, tc.ErrorIsNil)
	}
	sort.Strings(azArgs)
	c.Assert(azArgs, tc.DeepEquals, []string{"test-available", "test-available2"})
}

func (t *localServerSuite) TestStartInstanceWithImageIDErr(c *tc.C) {
	env := t.prepareAndBootstrap(c)

	expectedImageID := aws.String("ami-1234567890")
	params := environs.StartInstanceParams{
		ControllerUUID: t.ControllerUUID,
		Constraints:    constraints.Value{ImageID: expectedImageID},
	}

	t.PatchValue(ec2.RunInstances, func(ctx context.Context, e ec2.Client, ri *awsec2.RunInstancesInput, callback environs.StatusCallbackFunc) (resp *awsec2.RunInstancesOutput, err error) {
		return nil, &smithy.GenericAPIError{
			Code:    "InvalisAMIID",
			Message: fmt.Sprintf("The image id '[%s]' does not exist", *ri.ImageId),
		}
	})

	_, err := testing.StartInstanceWithParams(c, env, "1", params)
	c.Assert(err, tc.ErrorMatches, ".*The image id (.)* does not exist")
}

func (t *localServerSuite) TestStartInstanceWithImageID(c *tc.C) {
	env := t.prepareAndBootstrap(c)

	expectedImageID := aws.String("ami-1234567890")
	params := environs.StartInstanceParams{
		ControllerUUID: t.ControllerUUID,
		Constraints:    constraints.Value{ImageID: expectedImageID},
	}

	instance, err := testing.StartInstanceWithParams(c, env, "1", params)
	c.Assert(err, tc.ErrorIsNil)

	var instanceID string
	instanceID = string(instance.Instance.Id())
	instanceDesc, err := t.client.DescribeInstances(nil, &awsec2.DescribeInstancesInput{InstanceIds: []string{instanceID}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(expectedImageID, tc.DeepEquals, instanceDesc.Reservations[0].Instances[0].ImageId)
}

func (t *localServerSuite) TestAddresses(c *tc.C) {
	env := t.prepareAndBootstrap(c)
	inst, _ := testing.AssertStartInstance(c, env, t.ControllerUUID, "1")
	addrs, err := inst.Addresses(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	// Expected values use Address type but really contain a regexp for
	// the value rather than a valid ip or hostname.
	expected := network.ProviderAddresses{
		network.NewMachineAddress("8.0.0.*", network.WithScope(network.ScopePublic)).AsProviderAddress(),
		network.NewMachineAddress("127.0.0.*", network.WithScope(network.ScopeCloudLocal)).AsProviderAddress(),
	}
	expected[0].Type = network.IPv4Address
	expected[1].Type = network.IPv4Address

	c.Assert(addrs, tc.HasLen, len(expected))
	for i, addr := range addrs {
		c.Check(addr.Value, tc.Matches, expected[i].Value)
		c.Check(addr.Type, tc.Equals, expected[i].Type)
		c.Check(addr.Scope, tc.Equals, expected[i].Scope)
	}
}

func (t *localServerSuite) TestConstraintsValidatorUnsupported(c *tc.C) {
	env := t.Prepare(c)
	validator, err := env.ConstraintsValidator(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	cons := constraints.MustParse("arch=amd64 tags=foo virt-type=lxd")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unsupported, tc.SameContents, []string{"tags", "virt-type"})
}

func (t *localServerSuite) TestConstraintsValidatorVocab(c *tc.C) {
	env := t.Prepare(c)
	validator, err := env.ConstraintsValidator(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	cons := constraints.MustParse("instance-type=foo")
	_, err = validator.Validate(cons)
	c.Assert(err, tc.ErrorMatches, "invalid constraint value: instance-type=foo\nvalid values are:.*")
}

func (t *localServerSuite) TestConstraintsValidatorVocabDefaultVPC(c *tc.C) {
	env := t.Prepare(c)
	assertVPCInstanceTypeAvailable(c, env, c.Context())
}

func (t *localServerSuite) TestConstraintsValidatorVocabSpecifiedVPC(c *tc.C) {
	t.srv.defaultVPC.IsDefault = aws.Bool(false)
	err := t.srv.ec2srv.UpdateVpc(*t.srv.defaultVPC)
	c.Assert(err, tc.ErrorIsNil)

	t.TestConfig["vpc-id"] = aws.ToString(t.srv.defaultVPC.VpcId)
	defer delete(t.TestConfig, "vpc-id")

	env := t.Prepare(c)
	assertVPCInstanceTypeAvailable(c, env, c.Context())
}

func assertVPCInstanceTypeAvailable(c *tc.C, env environs.Environ, ctx context.Context) {
	validator, err := env.ConstraintsValidator(ctx)
	c.Assert(err, tc.ErrorIsNil)
	_, err = validator.Validate(constraints.MustParse("instance-type=t2.medium"))
	c.Assert(err, tc.ErrorIsNil)
}

func (t *localServerSuite) TestConstraintsMerge(c *tc.C) {
	env := t.Prepare(c)
	validator, err := env.ConstraintsValidator(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	consA := constraints.MustParse("arch=arm64 mem=1G cpu-power=10 cores=2 tags=bar")
	consB := constraints.MustParse("arch=amd64 instance-type=m1.small")
	cons, err := validator.Merge(consA, consB)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cons, tc.DeepEquals, constraints.MustParse("arch=amd64 instance-type=m1.small tags=bar"))
}

func (t *localServerSuite) TestConstraintsConflict(c *tc.C) {
	env := t.Prepare(c)
	validator, err := env.ConstraintsValidator(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	_, err = validator.Validate(constraints.MustParse("arch=amd64 instance-type=m1.small"))
	c.Assert(err, tc.ErrorIsNil)
	_, err = validator.Validate(constraints.MustParse("arch=arm64 instance-type=m1.small"))
	c.Assert(err, tc.ErrorMatches, `ambiguous constraints: "arch" overlaps with "instance-type": instance-type="m1.small" expected arch="amd64" not "arm64"`)
}

func (t *localServerSuite) TestPrecheckInstanceValidInstanceType(c *tc.C) {
	env := t.Prepare(c)
	cons := constraints.MustParse("instance-type=m1.small root-disk=1G")
	err := env.PrecheckInstance(c.Context(), environs.PrecheckInstanceParams{
		Base:        jujuversion.DefaultSupportedLTSBase(),
		Constraints: cons,
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (t *localServerSuite) TestPrecheckInstanceInvalidInstanceType(c *tc.C) {
	env := t.Prepare(c)
	cons := constraints.MustParse("instance-type=m1.invalid")
	err := env.PrecheckInstance(c.Context(), environs.PrecheckInstanceParams{
		Base:        jujuversion.DefaultSupportedLTSBase(),
		Constraints: cons,
	})
	c.Assert(err, tc.ErrorMatches, `invalid AWS instance type "m1.invalid" specified`)
}

func (t *localServerSuite) TestPrecheckInstanceUnsupportedArch(c *tc.C) {
	env := t.Prepare(c)
	cons := constraints.MustParse("instance-type=cc1.4xlarge arch=arm64")
	err := env.PrecheckInstance(c.Context(), environs.PrecheckInstanceParams{
		Base:        jujuversion.DefaultSupportedLTSBase(),
		Constraints: cons,
	})
	c.Assert(err, tc.ErrorMatches, `invalid AWS instance type "cc1.4xlarge" and arch "arm64" specified`)
}

func (t *localServerSuite) TestPrecheckInstanceAvailZone(c *tc.C) {
	env := t.Prepare(c)
	placement := "zone=test-available"
	err := env.PrecheckInstance(c.Context(), environs.PrecheckInstanceParams{
		Base:      jujuversion.DefaultSupportedLTSBase(),
		Placement: placement,
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (t *localServerSuite) TestPrecheckInstanceAvailZoneUnavailable(c *tc.C) {
	env := t.Prepare(c)
	placement := "zone=test-unavailable"
	err := env.PrecheckInstance(c.Context(), environs.PrecheckInstanceParams{
		Base:      jujuversion.DefaultSupportedLTSBase(),
		Placement: placement,
	})
	c.Assert(err, tc.ErrorMatches, `availability zone "test-unavailable" is "unavailable"`)
}

func (t *localServerSuite) TestPrecheckInstanceAvailZoneUnknown(c *tc.C) {
	env := t.Prepare(c)
	placement := "zone=test-unknown"
	err := env.PrecheckInstance(c.Context(), environs.PrecheckInstanceParams{
		Base:      jujuversion.DefaultSupportedLTSBase(),
		Placement: placement,
	})
	c.Assert(err, tc.ErrorMatches, `invalid availability zone "test-unknown"`)
}

func (t *localServerSuite) TestPrecheckInstanceVolumeAvailZoneNoPlacement(c *tc.C) {
	t.testPrecheckInstanceVolumeAvailZone(c, "")
}

func (t *localServerSuite) TestPrecheckInstanceVolumeAvailZoneSameZonePlacement(c *tc.C) {
	t.testPrecheckInstanceVolumeAvailZone(c, "zone=test-available")
}

func (t *localServerSuite) testPrecheckInstanceVolumeAvailZone(c *tc.C, placement string) {
	env := t.Prepare(c)
	resp, err := t.client.CreateVolume(c.Context(), &awsec2.CreateVolumeInput{
		Size:             aws.Int32(1),
		VolumeType:       "gp2",
		AvailabilityZone: aws.String("test-available"),
	})
	c.Assert(err, tc.ErrorIsNil)

	err = env.PrecheckInstance(c.Context(), environs.PrecheckInstanceParams{
		Base:      jujuversion.DefaultSupportedLTSBase(),
		Placement: placement,
		VolumeAttachments: []storage.VolumeAttachmentParams{{
			AttachmentParams: storage.AttachmentParams{
				Provider: "ebs",
			},
			Volume:   names.NewVolumeTag("23"),
			VolumeId: aws.ToString(resp.VolumeId),
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (t *localServerSuite) TestPrecheckInstanceAvailZoneVolumeConflict(c *tc.C) {
	env := t.Prepare(c)
	resp, err := t.client.CreateVolume(c.Context(), &awsec2.CreateVolumeInput{
		Size:             aws.Int32(1),
		VolumeType:       "gp2",
		AvailabilityZone: aws.String("volume-zone"),
	})
	c.Assert(err, tc.ErrorIsNil)

	err = env.PrecheckInstance(c.Context(), environs.PrecheckInstanceParams{
		Base:      jujuversion.DefaultSupportedLTSBase(),
		Placement: "zone=test-available",
		VolumeAttachments: []storage.VolumeAttachmentParams{{
			AttachmentParams: storage.AttachmentParams{
				Provider: "ebs",
			},
			Volume:   names.NewVolumeTag("23"),
			VolumeId: aws.ToString(resp.VolumeId),
		}},
	})
	c.Assert(err, tc.ErrorMatches, `cannot create instance with placement "zone=test-available", as this will prevent attaching the requested EBS volumes in zone "volume-zone"`)
}

func (t *localServerSuite) TestValidateImageMetadata(c *tc.C) {
	// region := t.srv.region
	// aws.Regions[region.Name] = t.srv.region
	// defer delete(aws.Regions, region.Name)

	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())

	env := t.Prepare(c)
	params, err := env.(simplestreams.ImageMetadataValidator).ImageMetadataLookupParams("test")
	c.Assert(err, tc.ErrorIsNil)
	params.Release = jujuversion.DefaultSupportedLTSBase().Channel.Track
	params.Endpoint = "http://foo"
	params.Sources, err = environs.ImageMetadataSources(env, ss)
	c.Assert(err, tc.ErrorIsNil)
	image_ids, _, err := imagemetadata.ValidateImageMetadata(c.Context(), ss, params)
	c.Assert(err, tc.ErrorIsNil)
	sort.Strings(image_ids)
	c.Assert(image_ids, tc.DeepEquals, []string{"ami-02404133", "ami-02404135", "ami-02404139"})
}

func (t *localServerSuite) TestGetToolsMetadataSources(c *tc.C) {
	t.PatchValue(&tools.DefaultBaseURL, "")

	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())

	env := t.Prepare(c)
	sources, err := tools.GetMetadataSources(env, ss)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sources, tc.HasLen, 0)
}

func (t *localServerSuite) TestSupportsNetworking(c *tc.C) {
	env := t.Prepare(c)
	_, supported := environs.SupportsNetworking(env)
	c.Assert(supported, tc.IsTrue)
}

func (t *localServerSuite) setUpInstanceWithDefaultVpc(c *tc.C) (environs.NetworkingEnviron, instance.Id) {
	env := t.prepareEnviron(c)
	err := bootstrap.Bootstrap(t.BootstrapContext, env,
		bootstrap.BootstrapParams{
			ControllerConfig:        coretesting.FakeControllerConfig(),
			AdminSecret:             testing.AdminSecret,
			CAPrivateKey:            coretesting.CAKey,
			SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
		})
	c.Assert(err, tc.ErrorIsNil)

	instanceIds, err := env.ControllerInstances(c.Context(), t.ControllerUUID)
	c.Assert(err, tc.ErrorIsNil)
	return env, instanceIds[0]
}

func (t *localServerSuite) TestNetworkInterfacesForMultipleInstances(c *tc.C) {
	// Start three instances
	env := t.prepareEnviron(c)
	testing.AssertStartInstance(c, env, t.ControllerUUID, "1")
	testing.AssertStartInstance(c, env, t.ControllerUUID, "2")
	testing.AssertStartInstance(c, env, t.ControllerUUID, "3")

	// Get a list of running instance IDs
	instances, err := env.AllInstances(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	var ids = make([]instance.Id, len(instances))
	for i, inst := range instances {
		ids[i] = inst.Id()
	}

	// Sort instance list so we always get consistent results
	sort.Slice(ids, func(l, r int) bool { return ids[l] < ids[r] })

	ifLists, err := env.NetworkInterfaces(c.Context(), ids)
	c.Assert(err, tc.ErrorIsNil)

	// Check that each entry in the list contains the right set of interfaces
	for i, id := range ids {
		c.Logf("comparing entry %d in result list with network interface list for instance %v", i, id)

		list, err := env.NetworkInterfaces(c.Context(), []instance.Id{id})
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(list, tc.HasLen, 1)
		instIfList := list[0]
		c.Assert(instIfList, tc.HasLen, 1)

		c.Assert(instIfList, tc.DeepEquals, ifLists[i], tc.Commentf("inconsistent result for entry %d in multi-instance result list", i))
		for devIdx, iface := range instIfList {
			t.assertInterfaceLooksValid(c, i+devIdx, devIdx, iface)
		}
	}
}

func (t *localServerSuite) TestPartialInterfacesForMultipleInstances(c *tc.C) {
	// Start three instances
	env := t.prepareEnviron(c)
	inst, _ := testing.AssertStartInstance(c, env, t.ControllerUUID, "1")

	infoLists, err := env.NetworkInterfaces(c.Context(), []instance.Id{inst.Id(), instance.Id("bogus")})
	c.Log(infoLists)
	c.Assert(err, tc.ErrorIs, environs.ErrPartialInstances)
	c.Assert(infoLists, tc.HasLen, 2)

	// Check interfaces for first instance
	list := infoLists[0]
	c.Assert(list, tc.HasLen, 1)
	t.assertInterfaceLooksValid(c, 0, 0, list[0])

	// Check that the slot for the second instance is nil
	c.Assert(infoLists[1], tc.IsNil, tc.Commentf("expected slot for unknown instance to be nil"))
}

func (t *localServerSuite) TestStartInstanceIsAtomic(c *tc.C) {
	env := t.prepareEnviron(c)
	_, _ = testing.AssertStartInstance(c, env, t.ControllerUUID, "1")
	callCounts := t.srv.ec2srv.GetMutatingCallCount()

	// Allow only 1 mutating call to instances and 0 to tags
	// We don't care about volumes or groups
	c.Assert(callCounts.Instances, tc.Equals, 1)
	c.Assert(callCounts.Tags, tc.Equals, 0)

}

func (t *localServerSuite) TestNetworkInterfaces(c *tc.C) {
	env, instId := t.setUpInstanceWithDefaultVpc(c)
	_, _ = t.addTestingNetworkInterfaceToInstance(c, instId)
	infoLists, err := env.NetworkInterfaces(c.Context(), []instance.Id{instId})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(infoLists, tc.HasLen, 1)

	list := infoLists[0]
	c.Assert(list, tc.HasLen, 2)

	// It's unpredictable which way around the interfaces are returned, so
	// ensure the correct one is analysed. The misc interface is given the device
	// index -1
	if list[0].DeviceIndex == -1 {
		t.assertInterfaceLooksValid(c, 0, 0, list[1])
	} else {
		t.assertInterfaceLooksValid(c, 0, 0, list[0])
	}
}

func (t *localServerSuite) assertInterfaceLooksValid(c *tc.C, expIfaceID, expDevIndex int, iface network.InterfaceInfo) {
	// The CIDR isn't predictable, but it is in the 10.10.x.0/24 format
	// The subnet ID is in the form "subnet-x", where x matches the same
	// number from the CIDR. The interfaces address is part of the CIDR.
	// For these reasons we check that the CIDR is in the expected format
	// and derive the expected values for ProviderSubnetId and Address.
	cidr := iface.PrimaryAddress().CIDR
	re := regexp.MustCompile(`10\.10\.(\d+)\.0/24`)
	c.Assert(re.Match([]byte(cidr)), tc.IsTrue)
	index := re.FindStringSubmatch(cidr)[1]
	addr := fmt.Sprintf("10.10.%s.5", index)
	subnetId := network.Id("subnet-" + index)

	expectedInterface := network.InterfaceInfo{
		DeviceIndex:      expDevIndex,
		MACAddress:       iface.MACAddress,
		ProviderId:       network.Id(fmt.Sprintf("eni-%d", expIfaceID)),
		ProviderSubnetId: subnetId,
		VLANTag:          0,
		Disabled:         false,
		NoAutoStart:      false,
		InterfaceType:    network.EthernetDevice,
		Addresses: network.ProviderAddresses{network.NewMachineAddress(
			addr,
			network.WithScope(network.ScopeCloudLocal),
			network.WithCIDR(cidr),
			network.WithConfigType(network.ConfigDHCP),
		).AsProviderAddress()},
		// Each machine is also assigned a shadow IP with the pattern:
		// 73.37.0.X where X=(provider iface ID + 1)
		ShadowAddresses: network.ProviderAddresses{network.NewMachineAddress(
			fmt.Sprintf("73.37.0.%d", expIfaceID+1),
			network.WithScope(network.ScopePublic),
			network.WithConfigType(network.ConfigDHCP),
		).AsProviderAddress()},
		Origin: network.OriginProvider,
	}
	c.Assert(iface, tc.DeepEquals, expectedInterface)
}

func (t *localServerSuite) TestSubnetsWithSubnetId(c *tc.C) {
	env, instId := t.setUpInstanceWithDefaultVpc(c)
	interfaceList, err := env.NetworkInterfaces(c.Context(), []instance.Id{instId})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(interfaceList, tc.HasLen, 1)
	interfaces := interfaceList[0]
	c.Assert(interfaces, tc.HasLen, 1)

	subnets, err := env.Subnets(c.Context(), []network.Id{interfaces[0].ProviderSubnetId})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subnets, tc.HasLen, 1)
	c.Assert(subnets[0].ProviderId, tc.Equals, interfaces[0].ProviderSubnetId)
	validateSubnets(c, subnets, "vpc-0")
}

func (t *localServerSuite) TestInstanceInformation(c *tc.C) {
	// TODO(macgreagoir) Where do these magic length numbers come from?
	c.Skip("Hard-coded InstanceTypes counts without explanation")
	env := t.prepareEnviron(c)
	types, err := env.InstanceTypes(c.Context(), constraints.Value{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(types.InstanceTypes, tc.HasLen, 53)

	cons := constraints.MustParse("mem=4G")
	types, err = env.InstanceTypes(c.Context(), cons)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(types.InstanceTypes, tc.HasLen, 48)
}

func validateSubnets(c *tc.C, subnets []network.SubnetInfo, vpcId network.Id) {
	// These are defined in the test server for the testing default
	// VPC.
	defaultSubnets := []network.SubnetInfo{{
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
		c.Assert(re.Match([]byte(subnet.CIDR)), tc.IsTrue)
		index, err := strconv.Atoi(re.FindStringSubmatch(subnet.CIDR)[1])
		c.Assert(err, tc.ErrorIsNil)
		// Don't know which AZ the subnet will end up in.
		defaultSubnets[index].AvailabilityZones = subnet.AvailabilityZones
		c.Check(subnet, tc.DeepEquals, defaultSubnets[index])
	}
}

func (t *localServerSuite) TestSubnets(c *tc.C) {
	env, _ := t.setUpInstanceWithDefaultVpc(c)

	subnets, err := env.Subnets(c.Context(), []network.Id{"subnet-0"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subnets, tc.HasLen, 1)
	validateSubnets(c, subnets, "vpc-0")

	subnets, err = env.Subnets(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subnets, tc.HasLen, 4)
	validateSubnets(c, subnets, "vpc-0")
}

func (t *localServerSuite) TestSubnetsMissingSubnet(c *tc.C) {
	env, _ := t.setUpInstanceWithDefaultVpc(c)

	_, err := env.Subnets(c.Context(), []network.Id{"subnet-0", "Missing"})
	c.Assert(err, tc.ErrorMatches, `failed to find the following subnet ids: \[Missing\]`)
}

func (t *localServerSuite) TestInstanceTags(c *tc.C) {
	env := t.prepareAndBootstrap(c)

	instances, err := env.AllRunningInstances(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(instances, tc.HasLen, 1)

	ec2Inst := ec2.InstanceSDKEC2(instances[0])
	var tags []string
	for _, t := range ec2Inst.Tags {
		tags = append(tags, *t.Key+":"+*t.Value)
	}
	namespace, err := instance.NewNamespace(coretesting.ModelTag.Id())
	c.Assert(err, tc.ErrorIsNil)

	hostname, err := namespace.Hostname("0")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(tags, tc.SameContents, []string{
		fmt.Sprintf("Name:%s", hostname),
		"juju-model-uuid:" + coretesting.ModelTag.Id(),
		"juju-controller-uuid:" + t.ControllerUUID,
		"juju-is-controller:true",
	})
}

func (t *localServerSuite) TestRootDiskTags(c *tc.C) {
	env := t.prepareAndBootstrap(c)

	instances, err := env.AllRunningInstances(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(instances, tc.HasLen, 1)

	ec2conn := ec2.EnvironEC2Client(env)
	resp, err := ec2conn.DescribeVolumes(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(resp.Volumes, tc.Not(tc.HasLen), 0)

	var found types.Volume
	for _, vol := range resp.Volumes {
		if len(vol.Tags) != 0 {
			found = vol
			break
		}
	}
	c.Assert(found, tc.NotNil)

	namespace, err := instance.NewNamespace(coretesting.ModelTag.Id())
	c.Assert(err, tc.ErrorIsNil)

	hostname, err := namespace.Hostname("0")
	c.Assert(err, tc.ErrorIsNil)

	compareTags(c, found.Tags, []tagInfo{
		{"Name", hostname + "-root"},
		{"juju-model-uuid", coretesting.ModelTag.Id()},
		{"juju-controller-uuid", t.ControllerUUID},
	})
}

func (s *localServerSuite) TestBootstrapInstanceConstraints(c *tc.C) {
	env := s.prepareAndBootstrap(c)
	inst, err := env.AllRunningInstances(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(inst, tc.HasLen, 1)
	ec2inst := ec2.InstanceSDKEC2(inst[0])
	// Controllers should be started with a burstable
	// instance if possible, and a 32 GiB disk.
	c.Assert(string(ec2inst.InstanceType), tc.Equals, "m6i.large")
}

func makeFilter(key string, values ...string) types.Filter {
	return types.Filter{Name: aws.String(key), Values: values}
}

func (s *localServerSuite) TestAdoptResources(c *tc.C) {
	controllerEnv := s.prepareAndBootstrap(c)
	controllerInsts, err := controllerEnv.AllRunningInstances(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(controllerInsts, tc.HasLen, 1)

	controllerVolumes, err := ec2.AllModelVolumes(controllerEnv, c.Context())
	c.Assert(err, tc.ErrorIsNil)

	controllerGroups, err := ec2.AllModelGroups(controllerEnv, c.Context())
	c.Assert(err, tc.ErrorIsNil)

	// Create a hosted model with an instance and a volume.
	hostedModelUUID := "7e386e08-cba7-44a4-a76e-7c1633584210"
	s.srv.ec2srv.SetInitialInstanceState(ec2test.Running)
	cfg, err := controllerEnv.Config().Apply(map[string]interface{}{
		"uuid":          hostedModelUUID,
		"firewall-mode": "global",
	})
	c.Assert(err, tc.ErrorIsNil)

	env, err := environs.New(s.BootstrapContext, environs.OpenParams{
		Cloud:          s.CloudSpec(),
		Config:         cfg,
		ControllerUUID: coretesting.ControllerTag.Id(),
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, tc.ErrorIsNil)
	inst, _ := testing.AssertStartInstance(c, env, s.ControllerUUID, "0")
	c.Assert(err, tc.ErrorIsNil)
	ebsProvider, err := env.StorageProvider(ec2.EBS_ProviderType)
	c.Assert(err, tc.ErrorIsNil)
	vs, err := ebsProvider.VolumeSource(nil)
	c.Assert(err, tc.ErrorIsNil)
	volumeResults, err := vs.CreateVolumes(c.Context(), []storage.VolumeParams{{
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(volumeResults, tc.HasLen, 1)
	c.Assert(volumeResults[0].Error, tc.ErrorIsNil)

	modelVolumes, err := ec2.AllModelVolumes(env, c.Context())
	c.Assert(err, tc.ErrorIsNil)
	allVolumes := append([]string{}, controllerVolumes...)
	allVolumes = append(allVolumes, modelVolumes...)

	modelGroups, err := ec2.AllModelGroups(env, c.Context())
	c.Assert(err, tc.ErrorIsNil)
	allGroups := append([]string{}, controllerGroups...)
	allGroups = append(allGroups, modelGroups...)

	ec2conn := ec2.EnvironEC2Client(env)

	origController := coretesting.ControllerTag.Id()

	checkInstanceTags := func(controllerUUID string, expectedIds ...string) {
		resp, err := ec2conn.DescribeInstances(
			c.Context(), &awsec2.DescribeInstancesInput{
				Filters: []types.Filter{makeFilter("tag:"+tags.JujuController, controllerUUID)},
			})
		c.Assert(err, tc.ErrorIsNil)
		actualIds := set.NewStrings()
		for _, reservation := range resp.Reservations {
			for _, instance := range reservation.Instances {
				actualIds.Add(aws.ToString(instance.InstanceId))
			}
		}
		c.Check(actualIds, tc.DeepEquals, set.NewStrings(expectedIds...))
	}

	checkVolumeTags := func(controllerUUID string, expectedIds ...string) {
		resp, err := ec2conn.DescribeVolumes(
			c.Context(), &awsec2.DescribeVolumesInput{
				Filters: []types.Filter{makeFilter("tag:"+tags.JujuController, controllerUUID)},
			})
		c.Assert(err, tc.ErrorIsNil)
		actualIds := set.NewStrings()
		for _, vol := range resp.Volumes {
			actualIds.Add(aws.ToString(vol.VolumeId))
		}
		c.Check(actualIds, tc.DeepEquals, set.NewStrings(expectedIds...))
	}

	checkGroupTags := func(controllerUUID string, expectedIds ...string) {
		resp, err := ec2conn.DescribeSecurityGroups(
			c.Context(), &awsec2.DescribeSecurityGroupsInput{
				Filters: []types.Filter{makeFilter("tag:"+tags.JujuController, controllerUUID)},
			})
		c.Assert(err, tc.ErrorIsNil)
		actualIds := set.NewStrings()
		for _, group := range resp.SecurityGroups {
			actualIds.Add(aws.ToString(group.GroupId))
		}
		c.Check(actualIds, tc.DeepEquals, set.NewStrings(expectedIds...))
	}

	checkInstanceTags(origController, string(inst.Id()), string(controllerInsts[0].Id()))
	checkVolumeTags(origController, allVolumes...)
	checkGroupTags(origController, allGroups...)

	err = env.AdoptResources(c.Context(), "new-controller", semversion.MustParse("0.0.1"))
	c.Assert(err, tc.ErrorIsNil)

	checkInstanceTags("new-controller", string(inst.Id()))
	checkInstanceTags(origController, string(controllerInsts[0].Id()))
	checkVolumeTags("new-controller", modelVolumes...)
	checkVolumeTags(origController, controllerVolumes...)
	checkGroupTags("new-controller", modelGroups...)
	checkGroupTags(origController, controllerGroups...)
}

func patchEC2ForTesting(c *tc.C, region types.Region) func() {
	ec2.UseTestImageData(c, ec2.MakeTestImageStreamsData(region))
	restoreRetryTimeouts := envtesting.PatchRetryStrategies(ec2.ShortRetryStrategy)
	restoreFinishBootstrap := envtesting.DisableFinishBootstrap(c)
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
func CheckScripts(c *tc.C, userDataMap map[string]interface{}, pattern string, match bool) {
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
func CheckPackage(c *tc.C, userDataMap map[string]interface{}, pkg string, match bool) {
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

func (t *localServerSuite) TestInstanceAttributes(c *tc.C) {
	t.Prepare(c)
	inst, hc := testing.AssertStartInstance(c, t.Env, t.ControllerUUID, "30")
	defer t.Env.StopInstances(c.Context(), inst.Id())
	// Sanity check for hardware characteristics.
	c.Assert(hc.Arch, tc.NotNil)
	c.Assert(hc.Mem, tc.NotNil)
	c.Assert(hc.RootDisk, tc.NotNil)
	c.Assert(hc.CpuCores, tc.NotNil)
	c.Assert(hc.CpuPower, tc.NotNil)
	addresses, err := testing.WaitInstanceAddresses(c, t.Env, inst.Id())
	// TODO(niemeyer): This assert sometimes fails with "no instances found"
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addresses, tc.Not(tc.HasLen), 0)

	insts, err := t.Env.Instances(c.Context(), []instance.Id{inst.Id()})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(insts), tc.Equals, 1)

	ec2inst := ec2.InstanceSDKEC2(insts[0])
	c.Assert(*ec2inst.PublicIpAddress, tc.Equals, addresses[0].Value)
	c.Assert(ec2inst.InstanceType, tc.Equals, types.InstanceType("m6i.large"))
}

func (t *localServerSuite) TestStartInstanceConstraints(c *tc.C) {
	t.Prepare(c)
	cons := constraints.MustParse("mem=4G")
	inst, hc := testing.AssertStartInstanceWithConstraints(c, t.Env, t.ControllerUUID, "30", cons)
	defer t.Env.StopInstances(c.Context(), inst.Id())
	ec2inst := ec2.InstanceSDKEC2(inst)
	c.Assert(ec2inst.InstanceType, tc.Equals, types.InstanceType("m6i.large"))
	c.Assert(*hc.Arch, tc.Equals, "amd64")
	c.Assert(*hc.Mem, tc.Equals, uint64(8*1024))
	c.Assert(*hc.RootDisk, tc.Equals, uint64(8*1024))
	c.Assert(*hc.CpuCores, tc.Equals, uint64(2))
}

func (t *localServerSuite) TestControllerInstances(c *tc.C) {
	t.prepareAndBootstrap(c)
	allInsts, err := t.Env.AllRunningInstances(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(allInsts, tc.HasLen, 1) // bootstrap instance
	bootstrapInstId := allInsts[0].Id()

	inst0, _ := testing.AssertStartInstance(c, t.Env, t.ControllerUUID, "98")
	defer t.Env.StopInstances(c.Context(), inst0.Id())

	inst1, _ := testing.AssertStartInstance(c, t.Env, t.ControllerUUID, "99")
	defer t.Env.StopInstances(c.Context(), inst1.Id())

	insts, err := t.Env.ControllerInstances(c.Context(), t.ControllerUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(insts, tc.DeepEquals, []instance.Id{bootstrapInstId})
}

func (t *localServerSuite) TestInstanceGroups(c *tc.C) {
	t.prepareAndBootstrap(c)
	allInsts, err := t.Env.AllRunningInstances(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(allInsts, tc.HasLen, 1) // bootstrap instance
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
	oldJujuGroup := createGroup(c, ec2conn, c.Context(), aws.ToString(groups[0].GroupName), "old juju group")

	// Add a permission.
	// N.B. this is unfortunately sensitive to the actual set of permissions used.
	_, err = ec2conn.AuthorizeSecurityGroupIngress(c.Context(), &awsec2.AuthorizeSecurityGroupIngressInput{
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
	c.Assert(err, tc.ErrorIsNil)

	inst0, _ := testing.AssertStartControllerInstance(c, t.Env, t.ControllerUUID, "98")
	defer t.Env.StopInstances(c.Context(), inst0.Id())

	// Create a same-named group for the second instance
	// before starting it, to check that it's reused correctly.
	oldMachineGroup := createGroup(c, ec2conn, c.Context(), aws.ToString(groups[2].GroupName), "old machine group")

	inst1, _ := testing.AssertStartControllerInstance(c, t.Env, t.ControllerUUID, "99")
	defer t.Env.StopInstances(c.Context(), inst1.Id())

	groupNames := make([]string, len(groups))
	for i, g := range groups {
		g := g
		groupNames[i] = aws.ToString(g.GroupName)
	}
	groupsResp, err := ec2conn.DescribeSecurityGroups(c.Context(), &awsec2.DescribeSecurityGroupsInput{
		GroupNames: groupNames,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(groupsResp.SecurityGroups, tc.HasLen, len(groups))

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
	c.Check(aws.ToString(groups[0].GroupId), tc.Equals, aws.ToString(oldJujuGroup.GroupId))

	// Check that it authorizes the correct ports and there
	// are no extra permissions (in particular we are checking
	// that the unneeded permission that we added earlier
	// has been deleted).
	perms := info[0].IpPermissions
	c.Assert(perms, tc.HasLen, 6)
	// SSH port
	checkPortAllowed(c, perms, 22)
	// APIServer port
	// TODO: we need a check to make sure on hosted models this port isn't open.
	checkPortAllowed(c, perms, int32(coretesting.FakeControllerConfig().APIPort()))
	checkSecurityGroupAllowed(c, perms, groups[0])

	// The old machine group should have been reused also.
	c.Check(aws.ToString(groups[2].GroupId), tc.Equals, aws.ToString(oldMachineGroup.GroupId))

	// Check that each instance is part of the correct groups.
	resp, err := ec2conn.DescribeInstances(c.Context(), &awsec2.DescribeInstancesInput{
		InstanceIds: []string{string(inst0.Id()), string(inst1.Id())},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(resp.Reservations, tc.HasLen, 2)
	for _, r := range resp.Reservations {
		c.Assert(r.Instances, tc.HasLen, 1)
		// each instance must be part of the general juju group.
		inst := r.Instances[0]
		msg := tc.Commentf("instance %#v", inst)
		c.Assert(hasSecurityGroup(inst, groups[0]), tc.Equals, true, msg)
		switch instance.Id(aws.ToString(inst.InstanceId)) {
		case inst0.Id():
			c.Assert(hasSecurityGroup(inst, groups[1]), tc.Equals, true, msg)
			c.Assert(hasSecurityGroup(inst, groups[2]), tc.Equals, false, msg)
		case inst1.Id():
			c.Assert(hasSecurityGroup(inst, groups[2]), tc.Equals, true, msg)
			c.Assert(hasSecurityGroup(inst, groups[1]), tc.Equals, false, msg)
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
	insts, err := t.Env.Instances(c.Context(), instIds)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(instIds, tc.SameContents, idsFromInsts(insts))
	allInsts, err = t.Env.AllRunningInstances(c.Context())
	c.Assert(err, tc.ErrorIsNil)
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
	c.Assert(instIds, tc.SameContents, idsFromInsts(allInsts))
}

func checkPortAllowed(c *tc.C, perms []types.IpPermission, port int32) {
	for _, perm := range perms {
		if aws.ToInt32(perm.FromPort) == port {
			c.Check(aws.ToString(perm.IpProtocol), tc.Equals, "tcp")
			c.Check(aws.ToInt32(perm.ToPort), tc.Equals, port)
			c.Check(perm.IpRanges, tc.HasLen, 1)
			c.Check(aws.ToString(perm.IpRanges[0].CidrIp), tc.DeepEquals, "0.0.0.0/0")
			c.Check(perm.UserIdGroupPairs, tc.HasLen, 0)
			return
		}
	}
	c.Errorf("ip port permission not found for %d in %#v", port, perms)
}

func checkSecurityGroupAllowed(c *tc.C, perms []types.IpPermission, g *types.SecurityGroupIdentifier) {
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

func (t *localServerSuite) TestStopInstances(c *tc.C) {
	t.Prepare(c)
	inst0, _ := testing.AssertStartInstance(c, t.Env, t.ControllerUUID, "40")
	inst1 := ec2.FabricateInstance(inst0, "i-aaaaaaaa")
	inst2, _ := testing.AssertStartInstance(c, t.Env, t.ControllerUUID, "41")

	err := t.Env.StopInstances(c.Context(), inst0.Id(), inst1.Id(), inst2.Id())
	c.Check(err, tc.ErrorIsNil)

	var insts []instances.Instance

	// We need the retry logic here because we are waiting
	// for Instances to return an error, and it will not retry
	// if it succeeds.
	retryStrategy := ec2.ShortRetryStrategy
	retryStrategy.Func = func() error {
		insts, err = t.Env.Instances(c.Context(), []instance.Id{inst0.Id(), inst2.Id()})
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

func (t *localServerSuite) TestPrechecker(c *tc.C) {
	// All implementations of InstancePrechecker should
	// return nil for empty constraints (excluding the
	// manual provider).
	t.Prepare(c)
	err := t.Env.PrecheckInstance(c.Context(),
		environs.PrecheckInstanceParams{
			Base: jujuversion.DefaultSupportedLTSBase(),
		})
	c.Assert(err, tc.ErrorIsNil)
}

func (t *localServerSuite) TestPorts(c *tc.C) {
	t.prepareAndBootstrap(c)

	inst1, _ := testing.AssertStartInstance(c, t.Env, t.ControllerUUID, "1")
	c.Assert(inst1, tc.NotNil)
	defer func() { _ = t.Env.StopInstances(c.Context(), inst1.Id()) }()
	fwInst1, ok := inst1.(instances.InstanceFirewaller)
	c.Assert(ok, tc.Equals, true)

	rules, err := fwInst1.IngressRules(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rules, tc.HasLen, 0)

	inst2, _ := testing.AssertStartInstance(c, t.Env, t.ControllerUUID, "2")
	c.Assert(inst2, tc.NotNil)
	fwInst2, ok := inst2.(instances.InstanceFirewaller)
	c.Assert(ok, tc.Equals, true)
	rules, err = fwInst2.IngressRules(c.Context(), "2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rules, tc.HasLen, 0)
	defer func() { _ = t.Env.StopInstances(c.Context(), inst2.Id()) }()

	// Open some ports and check they're there.
	err = fwInst1.OpenPorts(c.Context(),
		"1", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("67/udp")),
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("80-100/tcp")),
		})

	c.Assert(err, tc.ErrorIsNil)
	rules, err = fwInst1.IngressRules(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(
		rules, tc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("80-100/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("67/udp"), firewall.AllNetworksIPV4CIDR),
		},
	)
	rules, err = fwInst2.IngressRules(c.Context(), "2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rules, tc.HasLen, 0)

	err = fwInst2.OpenPorts(c.Context(),
		"2", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("20-30/tcp")),
		})
	c.Assert(err, tc.ErrorIsNil)

	// Check there's no crosstalk to another machine
	rules, err = fwInst2.IngressRules(c.Context(), "2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(
		rules, tc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("20-30/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR),
		},
	)
	rules, err = fwInst1.IngressRules(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(
		rules, tc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("80-100/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("67/udp"), firewall.AllNetworksIPV4CIDR),
		},
	)

	// Check that opening the same port again is ok.
	oldRules, err := fwInst2.IngressRules(c.Context(), "2")
	c.Assert(err, tc.ErrorIsNil)
	err = fwInst2.OpenPorts(c.Context(),
		"2", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp")),
		})
	c.Assert(err, tc.ErrorIsNil)
	err = fwInst2.OpenPorts(c.Context(),
		"2", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("20-30/tcp")),
		})
	c.Assert(err, tc.ErrorIsNil)
	rules, err = fwInst2.IngressRules(c.Context(), "2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rules, tc.DeepEquals, oldRules)

	// Check that opening the same port again and another port is ok.
	err = fwInst2.OpenPorts(c.Context(),
		"2", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("99/tcp")),
		})
	c.Assert(err, tc.ErrorIsNil)
	rules, err = fwInst2.IngressRules(c.Context(), "2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(
		rules, tc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("20-30/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("99/tcp"), firewall.AllNetworksIPV4CIDR),
		},
	)
	err = fwInst2.ClosePorts(c.Context(),
		"2", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("99/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("20-30/tcp")),
		})
	c.Assert(err, tc.ErrorIsNil)

	// Check that we can close ports and that there's no crosstalk.
	rules, err = fwInst2.IngressRules(c.Context(), "2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(
		rules, tc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR),
		},
	)
	rules, err = fwInst1.IngressRules(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(
		rules, tc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("80-100/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("67/udp"), firewall.AllNetworksIPV4CIDR),
		},
	)

	// Check that we can close multiple ports.
	err = fwInst1.ClosePorts(c.Context(),
		"1", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("67/udp")),
			firewall.NewIngressRule(network.MustParsePortRange("80-100/tcp")),
		})
	c.Assert(err, tc.ErrorIsNil)
	rules, err = fwInst1.IngressRules(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rules, tc.HasLen, 0)

	// Check that we can close ports that aren't there.
	err = fwInst2.ClosePorts(c.Context(),
		"2", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("111/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("222/udp")),
			firewall.NewIngressRule(network.MustParsePortRange("600-700/tcp")),
		})
	c.Assert(err, tc.ErrorIsNil)
	rules, err = fwInst2.IngressRules(c.Context(), "2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(
		rules, tc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR),
		},
	)

	// Check errors when acting on environment.
	fwEnv, ok := t.Env.(environs.Firewaller)
	c.Assert(ok, tc.Equals, true)
	err = fwEnv.OpenPorts(c.Context(), firewall.IngressRules{firewall.NewIngressRule(network.MustParsePortRange("80/tcp"))})
	c.Assert(err, tc.ErrorMatches, `invalid firewall mode "instance" for opening ports on model`)

	err = fwEnv.ClosePorts(c.Context(), firewall.IngressRules{firewall.NewIngressRule(network.MustParsePortRange("80/tcp"))})
	c.Assert(err, tc.ErrorMatches, `invalid firewall mode "instance" for closing ports on model`)

	_, err = fwEnv.IngressRules(c.Context())
	c.Assert(err, tc.ErrorMatches, `invalid firewall mode "instance" for retrieving ingress rules from model`)
}

func (t *localServerSuite) TestGlobalPorts(c *tc.C) {
	t.prepareAndBootstrap(c)

	// Change configuration.
	oldConfig := t.Env.Config()
	defer func() {
		err := t.Env.SetConfig(c.Context(), oldConfig)
		c.Assert(err, tc.ErrorIsNil)
	}()

	// So that deleteSecurityGroupInsistently succeeds. It will fail and keep
	// retrying due to StopInstances deleting the security groups, which are
	// global when firewall-mode is FwGlobal.
	t.BaseSuite.PatchValue(ec2.DeleteSecurityGroupInsistently, func(
		context.Context, ec2.SecurityGroupCleaner, types.GroupIdentifier, clock.Clock,
	) error {
		return nil
	})

	attrs := t.Env.Config().AllAttrs()
	attrs["firewall-mode"] = config.FwGlobal
	newConfig, err := t.Env.Config().Apply(attrs)
	c.Assert(err, tc.ErrorIsNil)
	err = t.Env.SetConfig(c.Context(), newConfig)
	c.Assert(err, tc.ErrorIsNil)

	// Create instances and check open ports on both instances.
	inst1, _ := testing.AssertStartInstance(c, t.Env, t.ControllerUUID, "1")
	defer func() { _ = t.Env.StopInstances(c.Context(), inst1.Id()) }()

	fwEnv, ok := t.Env.(environs.Firewaller)
	c.Assert(ok, tc.Equals, true)

	rules, err := fwEnv.IngressRules(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rules, tc.HasLen, 0)

	inst2, _ := testing.AssertStartInstance(c, t.Env, t.ControllerUUID, "2")
	rules, err = fwEnv.IngressRules(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rules, tc.HasLen, 0)
	defer func() { _ = t.Env.StopInstances(c.Context(), inst2.Id()) }()

	err = fwEnv.OpenPorts(c.Context(),
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("67/udp")),
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("99/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("100-110/tcp")),
		})
	c.Assert(err, tc.ErrorIsNil)

	rules, err = fwEnv.IngressRules(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(
		rules, tc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("99/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("100-110/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("67/udp"), firewall.AllNetworksIPV4CIDR),
		},
	)

	// Check closing some ports.
	err = fwEnv.ClosePorts(c.Context(),
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("99/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("67/udp")),
		})
	c.Assert(err, tc.ErrorIsNil)

	rules, err = fwEnv.IngressRules(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(
		rules, tc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("100-110/tcp"), firewall.AllNetworksIPV4CIDR),
		},
	)

	// Check that we can close ports that aren't there.
	err = fwEnv.ClosePorts(c.Context(),
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("111/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("222/udp")),
			firewall.NewIngressRule(network.MustParsePortRange("2000-2500/tcp")),
		})
	c.Assert(err, tc.ErrorIsNil)

	rules, err = fwEnv.IngressRules(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(
		rules, tc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("89/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("100-110/tcp"), firewall.AllNetworksIPV4CIDR),
		},
	)

	fwInst1, ok := inst1.(instances.InstanceFirewaller)
	c.Assert(ok, tc.Equals, true)
	// Check errors when acting on instances.
	err = fwInst1.OpenPorts(c.Context(),
		"1", firewall.IngressRules{firewall.NewIngressRule(network.MustParsePortRange("80/tcp"))})
	c.Assert(err, tc.ErrorMatches, `invalid firewall mode "global" for opening ports on instance`)

	err = fwInst1.ClosePorts(c.Context(),
		"1", firewall.IngressRules{firewall.NewIngressRule(network.MustParsePortRange("80/tcp"))})
	c.Assert(err, tc.ErrorMatches, `invalid firewall mode "global" for closing ports on instance`)

	_, err = fwInst1.IngressRules(c.Context(), "1")
	c.Assert(err, tc.ErrorMatches, `invalid firewall mode "global" for retrieving ingress rules from instance`)
}

func (t *localServerSuite) TestModelPorts(c *tc.C) {
	t.prepareAndBootstrap(c)

	fwModelEnv, ok := t.Env.(models.ModelFirewaller)
	c.Assert(ok, tc.Equals, true)

	rules, err := fwModelEnv.ModelIngressRules(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rules, tc.SameContents, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("22/tcp"), firewall.AllNetworksIPV4CIDR),
		// TODO: extend tests to check the api port isn't on hosted models.
		firewall.NewIngressRule(network.MustParsePortRange(strconv.Itoa(coretesting.FakeControllerConfig().APIPort())), firewall.AllNetworksIPV4CIDR),
	})

	err = fwModelEnv.OpenModelPorts(c.Context(),
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("67/udp")),
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("100-110/tcp")),
		})
	c.Assert(err, tc.ErrorIsNil)

	rules, err = fwModelEnv.ModelIngressRules(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rules, tc.SameContents, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("22/tcp"), firewall.AllNetworksIPV4CIDR),
		// TODO: extend tests to check the api port isn't on hosted models.
		firewall.NewIngressRule(network.MustParsePortRange(strconv.Itoa(coretesting.FakeControllerConfig().APIPort())), firewall.AllNetworksIPV4CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("45/tcp"), firewall.AllNetworksIPV4CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("100-110/tcp"), firewall.AllNetworksIPV4CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("67/udp"), firewall.AllNetworksIPV4CIDR),
	})

	// Check closing some ports.
	err = fwModelEnv.CloseModelPorts(c.Context(),
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("45/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("67/udp")),
		})
	c.Assert(err, tc.ErrorIsNil)

	rules, err = fwModelEnv.ModelIngressRules(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rules, tc.SameContents, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("22/tcp"), firewall.AllNetworksIPV4CIDR),
		// TODO: extend tests to check the api port isn't on hosted models.
		firewall.NewIngressRule(network.MustParsePortRange(strconv.Itoa(coretesting.FakeControllerConfig().APIPort())), firewall.AllNetworksIPV4CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("100-110/tcp"), firewall.AllNetworksIPV4CIDR),
	})

	// Check that we can close ports that aren't there.
	err = fwModelEnv.CloseModelPorts(c.Context(),
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("111/tcp")),
			firewall.NewIngressRule(network.MustParsePortRange("222/udp")),
			firewall.NewIngressRule(network.MustParsePortRange("2000-2500/tcp")),
		})
	c.Assert(err, tc.ErrorIsNil)

	rules, err = fwModelEnv.ModelIngressRules(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rules, tc.SameContents, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("22/tcp"), firewall.AllNetworksIPV4CIDR),
		// TODO: extend tests to check the api port isn't on hosted models.
		firewall.NewIngressRule(network.MustParsePortRange(strconv.Itoa(coretesting.FakeControllerConfig().APIPort())), firewall.AllNetworksIPV4CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("100-110/tcp"), firewall.AllNetworksIPV4CIDR),
	})
}

func (t *localServerSuite) TestBootstrapMultiple(c *tc.C) {
	// bootstrap.Bootstrap no longer raises errors if the environment is
	// already up, this has been moved into the bootstrap command.
	t.prepareAndBootstrap(c)

	c.Logf("destroy env")
	err := environs.Destroy(t.Env.Config().Name(), t.Env, c.Context(), t.ControllerStore)
	c.Assert(err, tc.ErrorIsNil)
	err = t.Env.Destroy(c.Context()) // Again, should work fine and do nothing.
	c.Assert(err, tc.ErrorIsNil)

	// check that we can bootstrap after destroy
	t.prepareAndBootstrap(c)
}

// Check that we get a consistent error when asking for an instance without
// a valid machine config.
func (t *localServerSuite) TestStartInstanceWithEmptyNonceFails(c *tc.C) {
	machineId := "4"
	apiInfo := testing.FakeAPIInfo(machineId)
	instanceConfig, err := instancecfg.NewInstanceConfig(coretesting.ControllerTag, machineId, "",
		"released", jujuversion.DefaultSupportedLTSBase(), apiInfo)
	c.Assert(err, tc.ErrorIsNil)

	t.Prepare(c)

	storageDir := c.MkDir()
	toolsStorage, err := filestorage.NewFileStorageWriter(storageDir)
	c.Assert(err, tc.ErrorIsNil)
	possibleTools := coretools.List(envtesting.AssertUploadFakeToolsVersions(
		c, toolsStorage, "released", semversion.MustParseBinary("5.4.5-ubuntu-amd64"),
	))
	params := environs.StartInstanceParams{
		ControllerUUID: coretesting.ControllerTag.Id(),
		Tools:          possibleTools,
		InstanceConfig: instanceConfig,
		StatusCallback: fakeCallback,
	}
	err = testing.SetImageMetadata(
		c,
		t.Env,
		simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory()),
		[]string{"24.04"},
		[]string{"amd64"},
		&params.ImageMetadata,
	)
	c.Check(err, tc.ErrorIsNil)
	result, err := t.Env.StartInstance(c.Context(), params)
	if result != nil && result.Instance != nil {
		err := t.Env.StopInstances(c.Context(), result.Instance.Id())
		c.Check(err, tc.ErrorIsNil)
	}
	c.Assert(result, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, ".*missing machine nonce")
}

func (t *localServerSuite) TestIngressRulesWithPartiallyMatchingCIDRs(c *tc.C) {
	t.prepareAndBootstrap(c)

	inst1, _ := testing.AssertStartInstance(c, t.Env, t.ControllerUUID, "1")
	c.Assert(inst1, tc.NotNil)
	defer func() { _ = t.Env.StopInstances(c.Context(), inst1.Id()) }()
	fwInst1, ok := inst1.(instances.InstanceFirewaller)
	c.Assert(ok, tc.Equals, true)

	rules, err := fwInst1.IngressRules(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rules, tc.HasLen, 0)

	// Open ports with different CIDRs. Check that rules with same port range
	// get merged.
	err = fwInst1.OpenPorts(c.Context(),
		"1", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("42/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("42/tcp"), "10.0.0.0/24"),
			firewall.NewIngressRule(network.MustParsePortRange("80/tcp")), // open to 0.0.0.0/0
		})

	c.Assert(err, tc.ErrorIsNil)
	rules, err = fwInst1.IngressRules(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(
		rules, tc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("42/tcp"), firewall.AllNetworksIPV4CIDR, "10.0.0.0/24"),
			firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
		},
	)

	// Open same port with different CIDRs and check that the CIDR gets
	// appended to the existing rule's CIDR list.
	err = fwInst1.OpenPorts(c.Context(),
		"1", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("42/tcp"), "192.168.0.0/24"),
		})

	c.Assert(err, tc.ErrorIsNil)
	rules, err = fwInst1.IngressRules(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(
		rules, tc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("42/tcp"), firewall.AllNetworksIPV4CIDR, "10.0.0.0/24", "192.168.0.0/24"),
			firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
		},
	)

	// Close port on a subset of the CIDRs and ensure that that CIDR gets
	// removed from the ingress rules
	err = fwInst1.ClosePorts(c.Context(),
		"1", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("42/tcp"), "192.168.0.0/24"),
		})

	c.Assert(err, tc.ErrorIsNil)
	rules, err = fwInst1.IngressRules(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(
		rules, tc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("42/tcp"), firewall.AllNetworksIPV4CIDR, "10.0.0.0/24"),
			firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
		},
	)

	// Remove all CIDRs from the rule and check that rules without CIDRs
	// get dropped.
	err = fwInst1.ClosePorts(c.Context(),
		"1", firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("42/tcp"), firewall.AllNetworksIPV4CIDR, "10.0.0.0/24"),
		})

	c.Assert(err, tc.ErrorIsNil)
	rules, err = fwInst1.IngressRules(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(
		rules, tc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
		},
	)
}

// createGroup creates a new EC2 group and returns it. If it already exists,
// it revokes all its permissions and returns the existing group.
func createGroup(c *tc.C, ec2conn ec2.Client, ctx context.Context, name string, descr string) types.SecurityGroupIdentifier {
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
	c.Assert(err, tc.ErrorIsNil)

	gi := gresp.SecurityGroups[0]
	if len(gi.IpPermissions) > 0 {
		_, err = ec2conn.RevokeSecurityGroupIngress(ctx, &awsec2.RevokeSecurityGroupIngressInput{
			GroupId: gi.GroupId,
		})
		c.Assert(err, tc.ErrorIsNil)
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
