// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"sync"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"
	"launchpad.net/gwacl"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/instances"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	apiparams "launchpad.net/juju-core/state/api/params"
	coretesting "launchpad.net/juju-core/testing"
)

type baseEnvironSuite struct {
	providerSuite
}

type environSuite struct {
	baseEnvironSuite
}

var _ = gc.Suite(&environSuite{})
var _ = gc.Suite(&startInstanceSuite{})

// makeEnviron creates a fake azureEnviron with arbitrary configuration.
func makeEnviron(c *gc.C) *azureEnviron {
	attrs := makeAzureConfigMap(c)
	return makeEnvironWithConfig(c, attrs)
}

// makeEnvironWithConfig creates a fake azureEnviron with the specified configuration.
func makeEnvironWithConfig(c *gc.C, attrs map[string]interface{}) *azureEnviron {
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	env, err := NewEnviron(cfg)
	c.Assert(err, gc.IsNil)
	// Prevent the test from trying to query for a storage-account key.
	env.storageAccountKey = "fake-storage-account-key"
	return env
}

// setDummyStorage injects the local provider's fake storage implementation
// into the given environment, so that tests can manipulate storage as if it
// were real.
func (s *baseEnvironSuite) setDummyStorage(c *gc.C, env *azureEnviron) {
	closer, storage, _ := envtesting.CreateLocalTestStorage(c)
	env.storage = storage
	s.AddCleanup(func(c *gc.C) { closer.Close() })
}

func (*environSuite) TestGetEndpoint(c *gc.C) {
	c.Check(
		getEndpoint("West US"),
		gc.Equals,
		"https://management.core.windows.net/")
	c.Check(
		getEndpoint("China East"),
		gc.Equals,
		"https://management.core.chinacloudapi.cn/")
}

func (*environSuite) TestGetSnapshot(c *gc.C) {
	original := azureEnviron{name: "this-env", ecfg: new(azureEnvironConfig)}
	snapshot := original.getSnapshot()

	// The snapshot is identical to the original.
	c.Check(*snapshot, gc.DeepEquals, original)

	// However, they are distinct objects.
	c.Check(snapshot, gc.Not(gc.Equals), &original)

	// It's a shallow copy; they still share pointers.
	c.Check(snapshot.ecfg, gc.Equals, original.ecfg)

	// Neither object is locked at the end of the copy.
	c.Check(original.Mutex, gc.Equals, sync.Mutex{})
	c.Check(snapshot.Mutex, gc.Equals, sync.Mutex{})
}

func (*environSuite) TestGetSnapshotLocksEnviron(c *gc.C) {
	original := azureEnviron{}
	coretesting.TestLockingFunction(&original.Mutex, func() { original.getSnapshot() })
}

func (*environSuite) TestName(c *gc.C) {
	env := azureEnviron{name: "foo"}
	c.Check(env.Name(), gc.Equals, env.name)
}

func (*environSuite) TestConfigReturnsConfig(c *gc.C) {
	cfg := new(config.Config)
	ecfg := azureEnvironConfig{Config: cfg}
	env := azureEnviron{ecfg: &ecfg}
	c.Check(env.Config(), gc.Equals, cfg)
}

func (*environSuite) TestConfigLocksEnviron(c *gc.C) {
	env := azureEnviron{name: "env", ecfg: new(azureEnvironConfig)}
	coretesting.TestLockingFunction(&env.Mutex, func() { env.Config() })
}

func (*environSuite) TestGetManagementAPI(c *gc.C) {
	env := makeEnviron(c)
	context, err := env.getManagementAPI()
	c.Assert(err, gc.IsNil)
	defer env.releaseManagementAPI(context)
	c.Check(context, gc.NotNil)
	c.Check(context.ManagementAPI, gc.NotNil)
	c.Check(context.certFile, gc.NotNil)
	c.Check(context.GetRetryPolicy(), gc.DeepEquals, retryPolicy)
}

func (*environSuite) TestReleaseManagementAPIAcceptsNil(c *gc.C) {
	env := makeEnviron(c)
	env.releaseManagementAPI(nil)
	// The real test is that this does not panic.
}

func (*environSuite) TestReleaseManagementAPIAcceptsIncompleteContext(c *gc.C) {
	env := makeEnviron(c)
	context := azureManagementContext{
		ManagementAPI: nil,
		certFile:      nil,
	}
	env.releaseManagementAPI(&context)
	// The real test is that this does not panic.
}

func getAzureServiceListResponse(c *gc.C, services ...gwacl.HostedServiceDescriptor) []gwacl.DispatcherResponse {
	list := gwacl.HostedServiceDescriptorList{HostedServices: services}
	listXML, err := list.Serialize()
	c.Assert(err, gc.IsNil)
	responses := []gwacl.DispatcherResponse{gwacl.NewDispatcherResponse(
		[]byte(listXML),
		http.StatusOK,
		nil,
	)}
	return responses
}

// getAzureServiceResponse returns a gwacl.DispatcherResponse corresponding
// to the API request used to get the properties of a Service.
func getAzureServiceResponse(c *gc.C, service gwacl.HostedService) gwacl.DispatcherResponse {
	serviceXML, err := service.Serialize()
	c.Assert(err, gc.IsNil)
	return gwacl.NewDispatcherResponse([]byte(serviceXML), http.StatusOK, nil)
}

func patchWithServiceListResponse(c *gc.C, services []gwacl.HostedServiceDescriptor) *[]*gwacl.X509Request {
	responses := getAzureServiceListResponse(c, services...)
	return gwacl.PatchManagementAPIResponses(responses)
}

func prepareInstancesResponses(c *gc.C, prefix string, services ...*gwacl.HostedService) []gwacl.DispatcherResponse {
	descriptors := make([]gwacl.HostedServiceDescriptor, len(services))
	for i, service := range services {
		descriptors[i] = service.HostedServiceDescriptor
	}
	responses := getAzureServiceListResponse(c, descriptors...)
	for _, service := range services {
		if !strings.HasPrefix(service.ServiceName, prefix) {
			continue
		}
		serviceXML, err := service.Serialize()
		c.Assert(err, gc.IsNil)
		serviceGetResponse := gwacl.NewDispatcherResponse([]byte(serviceXML), http.StatusOK, nil)
		responses = append(responses, serviceGetResponse)
	}
	return responses
}

func patchInstancesResponses(c *gc.C, prefix string, services ...*gwacl.HostedService) *[]*gwacl.X509Request {
	responses := prepareInstancesResponses(c, prefix, services...)
	return gwacl.PatchManagementAPIResponses(responses)
}

func (s *environSuite) TestSupportedArchitectures(c *gc.C) {
	env := s.setupEnvWithDummyMetadata(c)
	a, err := env.SupportedArchitectures()
	c.Assert(err, gc.IsNil)
	c.Assert(a, gc.DeepEquals, []string{"amd64"})
}

func (s *environSuite) TestSupportNetworks(c *gc.C) {
	env := s.setupEnvWithDummyMetadata(c)
	c.Assert(env.SupportNetworks(), jc.IsFalse)
}

func (suite *environSuite) TestGetEnvPrefixContainsEnvName(c *gc.C) {
	env := makeEnviron(c)
	c.Check(strings.Contains(env.getEnvPrefix(), env.Name()), jc.IsTrue)
}

func (*environSuite) TestGetContainerName(c *gc.C) {
	env := makeEnviron(c)
	expected := env.getEnvPrefix() + "private"
	c.Check(env.getContainerName(), gc.Equals, expected)
}

func (suite *environSuite) TestAllInstances(c *gc.C) {
	env := makeEnviron(c)
	prefix := env.getEnvPrefix()
	service1 := makeLegacyDeployment(env, prefix+"service1")
	service2 := makeDeployment(env, prefix+"service2")
	service3 := makeDeployment(env, "not"+prefix+"service3")

	requests := patchInstancesResponses(c, prefix, service1, service2, service3)
	instances, err := env.AllInstances()
	c.Assert(err, gc.IsNil)
	c.Check(len(instances), gc.Equals, 3)
	c.Check(instances[0].Id(), gc.Equals, instance.Id(prefix+"service1"))
	service2Role1Name := service2.Deployments[0].RoleList[0].RoleName
	service2Role2Name := service2.Deployments[0].RoleList[1].RoleName
	c.Check(instances[1].Id(), gc.Equals, instance.Id(prefix+"service2-"+service2Role1Name))
	c.Check(instances[2].Id(), gc.Equals, instance.Id(prefix+"service2-"+service2Role2Name))
	c.Check(len(*requests), gc.Equals, 3)
}

func (suite *environSuite) TestInstancesReturnsFilteredList(c *gc.C) {
	env := makeEnviron(c)
	prefix := env.getEnvPrefix()
	service := makeDeployment(env, prefix+"service")
	requests := patchInstancesResponses(c, prefix, service)
	role1Name := service.Deployments[0].RoleList[0].RoleName
	instId := instance.Id(prefix + "service-" + role1Name)
	instances, err := env.Instances([]instance.Id{instId})
	c.Assert(err, gc.IsNil)
	c.Check(len(instances), gc.Equals, 1)
	c.Check(instances[0].Id(), gc.Equals, instId)
	c.Check(len(*requests), gc.Equals, 2)
}

func (suite *environSuite) TestInstancesReturnsErrNoInstancesIfNoInstancesRequested(c *gc.C) {
	services := []gwacl.HostedServiceDescriptor{{ServiceName: "deployment-1"}, {ServiceName: "deployment-2"}}
	patchWithServiceListResponse(c, services)
	env := makeEnviron(c)
	instances, err := env.Instances([]instance.Id{})
	c.Check(err, gc.Equals, environs.ErrNoInstances)
	c.Check(instances, gc.IsNil)
}

func (suite *environSuite) TestInstancesReturnsErrNoInstancesIfNoInstanceFound(c *gc.C) {
	services := []gwacl.HostedServiceDescriptor{}
	patchWithServiceListResponse(c, services)
	env := makeEnviron(c)
	instances, err := env.Instances([]instance.Id{"deploy-id"})
	c.Check(err, gc.Equals, environs.ErrNoInstances)
	c.Check(instances, gc.IsNil)
}

func (suite *environSuite) TestInstancesReturnsPartialInstancesIfSomeInstancesAreNotFound(c *gc.C) {
	env := makeEnviron(c)
	prefix := env.getEnvPrefix()
	service := makeDeployment(env, prefix+"service")

	role1Name := service.Deployments[0].RoleList[0].RoleName
	role2Name := service.Deployments[0].RoleList[1].RoleName
	inst1Id := instance.Id(prefix + "service-" + role1Name)
	inst2Id := instance.Id(prefix + "service-" + role2Name)
	patchInstancesResponses(c, prefix, service)

	instances, err := env.Instances([]instance.Id{inst1Id, "unknown", inst2Id})
	c.Assert(err, gc.Equals, environs.ErrPartialInstances)
	c.Check(len(instances), gc.Equals, 3)
	c.Check(instances[0].Id(), gc.Equals, inst1Id)
	c.Check(instances[1], gc.IsNil)
	c.Check(instances[2].Id(), gc.Equals, inst2Id)
}

func (*environSuite) TestStorage(c *gc.C) {
	env := makeEnviron(c)
	baseStorage := env.Storage()
	storage, ok := baseStorage.(*azureStorage)
	c.Check(ok, gc.Equals, true)
	c.Assert(storage, gc.NotNil)
	c.Check(storage.storageContext.getContainer(), gc.Equals, env.getContainerName())
	context, err := storage.getStorageContext()
	c.Assert(err, gc.IsNil)
	c.Check(context.Account, gc.Equals, env.ecfg.storageAccountName())
	c.Check(context.RetryPolicy, gc.DeepEquals, retryPolicy)
}

func (*environSuite) TestQueryStorageAccountKeyGetsKey(c *gc.C) {
	env := makeEnviron(c)
	keysInAzure := gwacl.StorageAccountKeys{Primary: "a-key"}
	azureResponse, err := xml.Marshal(keysInAzure)
	c.Assert(err, gc.IsNil)
	requests := gwacl.PatchManagementAPIResponses([]gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(azureResponse, http.StatusOK, nil),
	})

	returnedKey, err := env.queryStorageAccountKey()
	c.Assert(err, gc.IsNil)

	c.Check(returnedKey, gc.Equals, keysInAzure.Primary)
	c.Assert(*requests, gc.HasLen, 1)
	c.Check((*requests)[0].Method, gc.Equals, "GET")
}

func (*environSuite) TestGetStorageContextCreatesStorageContext(c *gc.C) {
	env := makeEnviron(c)
	stor, err := env.getStorageContext()
	c.Assert(err, gc.IsNil)
	c.Assert(stor, gc.NotNil)
	c.Check(stor.Account, gc.Equals, env.ecfg.storageAccountName())
	c.Check(stor.AzureEndpoint, gc.Equals, gwacl.GetEndpoint(env.ecfg.location()))
}

func (*environSuite) TestGetStorageContextUsesKnownStorageAccountKey(c *gc.C) {
	env := makeEnviron(c)
	env.storageAccountKey = "my-key"

	stor, err := env.getStorageContext()
	c.Assert(err, gc.IsNil)

	c.Check(stor.Key, gc.Equals, "my-key")
}

func (*environSuite) TestGetStorageContextQueriesStorageAccountKeyIfNeeded(c *gc.C) {
	env := makeEnviron(c)
	env.storageAccountKey = ""
	keysInAzure := gwacl.StorageAccountKeys{Primary: "my-key"}
	azureResponse, err := xml.Marshal(keysInAzure)
	c.Assert(err, gc.IsNil)
	gwacl.PatchManagementAPIResponses([]gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(azureResponse, http.StatusOK, nil),
	})

	stor, err := env.getStorageContext()
	c.Assert(err, gc.IsNil)

	c.Check(stor.Key, gc.Equals, keysInAzure.Primary)
	c.Check(env.storageAccountKey, gc.Equals, keysInAzure.Primary)
}

func (*environSuite) TestGetStorageContextFailsIfNoKeyAvailable(c *gc.C) {
	env := makeEnviron(c)
	env.storageAccountKey = ""
	azureResponse, err := xml.Marshal(gwacl.StorageAccountKeys{})
	c.Assert(err, gc.IsNil)
	gwacl.PatchManagementAPIResponses([]gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(azureResponse, http.StatusOK, nil),
	})

	_, err = env.getStorageContext()
	c.Assert(err, gc.NotNil)

	c.Check(err, gc.ErrorMatches, "no keys available for storage account")
}

func (*environSuite) TestUpdateStorageAccountKeyGetsFreshKey(c *gc.C) {
	env := makeEnviron(c)
	keysInAzure := gwacl.StorageAccountKeys{Primary: "my-key"}
	azureResponse, err := xml.Marshal(keysInAzure)
	c.Assert(err, gc.IsNil)
	gwacl.PatchManagementAPIResponses([]gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(azureResponse, http.StatusOK, nil),
	})

	key, err := env.updateStorageAccountKey(env.getSnapshot())
	c.Assert(err, gc.IsNil)

	c.Check(key, gc.Equals, keysInAzure.Primary)
	c.Check(env.storageAccountKey, gc.Equals, keysInAzure.Primary)
}

func (*environSuite) TestUpdateStorageAccountKeyReturnsError(c *gc.C) {
	env := makeEnviron(c)
	env.storageAccountKey = ""
	gwacl.PatchManagementAPIResponses([]gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(nil, http.StatusInternalServerError, nil),
	})

	_, err := env.updateStorageAccountKey(env.getSnapshot())
	c.Assert(err, gc.NotNil)

	c.Check(err, gc.ErrorMatches, "cannot obtain storage account keys: GET request failed.*Internal Server Error.*")
	c.Check(env.storageAccountKey, gc.Equals, "")
}

func (*environSuite) TestUpdateStorageAccountKeyDetectsConcurrentUpdate(c *gc.C) {
	env := makeEnviron(c)
	env.storageAccountKey = ""
	keysInAzure := gwacl.StorageAccountKeys{Primary: "my-key"}
	azureResponse, err := xml.Marshal(keysInAzure)
	c.Assert(err, gc.IsNil)
	gwacl.PatchManagementAPIResponses([]gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(azureResponse, http.StatusOK, nil),
	})

	// Here we use a snapshot that's different from the environment, to
	// simulate a concurrent change to the environment.
	_, err = env.updateStorageAccountKey(makeEnviron(c))
	c.Assert(err, gc.NotNil)

	// updateStorageAccountKey detects the change, and refuses to write its
	// outdated information into env.
	c.Check(err, gc.ErrorMatches, "environment was reconfigured")
	c.Check(env.storageAccountKey, gc.Equals, "")
}

func (*environSuite) TestSetConfigValidates(c *gc.C) {
	env := makeEnviron(c)
	originalCfg := env.ecfg
	attrs := makeAzureConfigMap(c)
	// This config is not valid.  It lacks essential information.
	delete(attrs, "management-subscription-id")
	badCfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)

	err = env.SetConfig(badCfg)

	// Since the config was not valid, SetConfig returns an error.  It
	// does not update the environment's config either.
	c.Check(err, gc.NotNil)
	c.Check(
		err,
		gc.ErrorMatches,
		"management-subscription-id: expected string, got nothing")
	c.Check(env.ecfg, gc.Equals, originalCfg)
}

func (*environSuite) TestSetConfigUpdatesConfig(c *gc.C) {
	env := makeEnviron(c)
	// We're going to set a new config.  It can be recognized by its
	// unusual default Ubuntu release series: 7.04 Feisty Fawn.
	attrs := makeAzureConfigMap(c)
	attrs["default-series"] = "feisty"
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)

	err = env.SetConfig(cfg)
	c.Assert(err, gc.IsNil)

	c.Check(config.PreferredSeries(env.ecfg.Config), gc.Equals, "feisty")
}

func (*environSuite) TestSetConfigLocksEnviron(c *gc.C) {
	env := makeEnviron(c)
	cfg, err := config.New(config.NoDefaults, makeAzureConfigMap(c))
	c.Assert(err, gc.IsNil)

	coretesting.TestLockingFunction(&env.Mutex, func() { env.SetConfig(cfg) })
}

func (*environSuite) TestSetConfigWillNotUpdateName(c *gc.C) {
	// Once the environment's name has been set, it cannot be updated.
	// Global validation rejects such a change.
	// This matters because the attribute is not protected by a lock.
	env := makeEnviron(c)
	originalName := env.Name()
	attrs := makeAzureConfigMap(c)
	attrs["name"] = "new-name"
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)

	err = env.SetConfig(cfg)

	c.Assert(err, gc.NotNil)
	c.Check(
		err,
		gc.ErrorMatches,
		`cannot change name from ".*" to "new-name"`)
	c.Check(env.Name(), gc.Equals, originalName)
}

func (*environSuite) TestSetConfigClearsStorageAccountKey(c *gc.C) {
	env := makeEnviron(c)
	env.storageAccountKey = "key-for-previous-config"
	attrs := makeAzureConfigMap(c)
	attrs["default-series"] = "other"
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)

	err = env.SetConfig(cfg)
	c.Assert(err, gc.IsNil)

	c.Check(env.storageAccountKey, gc.Equals, "")
}

func (s *environSuite) TestStateInfoFailsIfNoStateInstances(c *gc.C) {
	env := makeEnviron(c)
	s.setDummyStorage(c, env)
	_, _, err := env.StateInfo()
	c.Check(err, gc.Equals, environs.ErrNotBootstrapped)
}

func (s *environSuite) TestStateInfo(c *gc.C) {
	env := makeEnviron(c)
	s.setDummyStorage(c, env)
	prefix := env.getEnvPrefix()

	service := makeDeployment(env, prefix+"myservice")
	instId := instance.Id(service.ServiceName + "-" + service.Deployments[0].RoleList[0].RoleName)
	patchInstancesResponses(c, prefix, service)
	err := bootstrap.SaveState(
		env.Storage(),
		&bootstrap.BootstrapState{StateInstances: []instance.Id{instId}},
	)
	c.Assert(err, gc.IsNil)

	responses := prepareInstancesResponses(c, prefix, service)
	responses = append(responses, prepareDeploymentInfoResponse(c, gwacl.Deployment{
		RoleInstanceList: []gwacl.RoleInstance{{
			RoleName:  service.Deployments[0].RoleList[0].RoleName,
			IPAddress: "1.2.3.4",
		}},
		VirtualNetworkName: env.getVirtualNetworkName(),
	})...)
	gwacl.PatchManagementAPIResponses(responses)

	stateInfo, apiInfo, err := env.StateInfo()
	c.Assert(err, gc.IsNil)
	config := env.Config()
	dnsName := prefix + "myservice." + AZURE_DOMAIN_NAME
	c.Check(stateInfo.Addrs, jc.SameContents, []string{
		fmt.Sprintf("1.2.3.4:%d", config.StatePort()),
		fmt.Sprintf("%s:%d", dnsName, config.StatePort()),
	})
	c.Check(apiInfo.Addrs, jc.DeepEquals, []string{
		fmt.Sprintf("1.2.3.4:%d", config.APIPort()),
		fmt.Sprintf("%s:%d", dnsName, config.APIPort()),
	})
}

// parseCreateServiceRequest reconstructs the original CreateHostedService
// request object passed to gwacl's AddHostedService method, based on the
// X509Request which the method issues.
func parseCreateServiceRequest(c *gc.C, request *gwacl.X509Request) *gwacl.CreateHostedService {
	body := gwacl.CreateHostedService{}
	err := xml.Unmarshal(request.Payload, &body)
	c.Assert(err, gc.IsNil)
	return &body
}

// getHostedServicePropertiesServiceName extracts the service name parameter
// from the GetHostedServiceProperties request URL.
func getHostedServicePropertiesServiceName(c *gc.C, request *gwacl.X509Request) string {
	url, err := url.Parse(request.URL)
	c.Assert(err, gc.IsNil)
	return path.Base(url.Path)
}

// makeNonAvailabilityResponse simulates a reply to the
// CheckHostedServiceNameAvailability call saying that a name is not available.
func makeNonAvailabilityResponse(c *gc.C) []byte {
	errorBody, err := xml.Marshal(gwacl.AvailabilityResponse{
		Result: "false",
		Reason: "he's a very naughty boy"})
	c.Assert(err, gc.IsNil)
	return errorBody
}

// makeAvailabilityResponse simulates a reply to the
// CheckHostedServiceNameAvailability call saying that a name is available.
func makeAvailabilityResponse(c *gc.C) []byte {
	errorBody, err := xml.Marshal(gwacl.AvailabilityResponse{
		Result: "true"})
	c.Assert(err, gc.IsNil)
	return errorBody
}

func (*environSuite) TestAttemptCreateServiceCreatesService(c *gc.C) {
	prefix := "myservice"
	affinityGroup := "affinity-group"

	responses := []gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(makeAvailabilityResponse(c), http.StatusOK, nil),
		gwacl.NewDispatcherResponse(nil, http.StatusOK, nil),
	}
	requests := gwacl.PatchManagementAPIResponses(responses)
	azure, err := gwacl.NewManagementAPI("subscription", "", "West US")
	c.Assert(err, gc.IsNil)

	service, err := attemptCreateService(azure, prefix, affinityGroup, "")
	c.Assert(err, gc.IsNil)

	c.Assert(*requests, gc.HasLen, 2)
	body := parseCreateServiceRequest(c, (*requests)[1])
	c.Check(body.ServiceName, gc.Equals, service.ServiceName)
	c.Check(body.AffinityGroup, gc.Equals, affinityGroup)
	c.Check(service.ServiceName, gc.Matches, prefix+".*")
	// We specify AffinityGroup, so Location should be empty.
	c.Check(service.Location, gc.Equals, "")
}

func (*environSuite) TestAttemptCreateServiceReturnsNilIfNameNotUnique(c *gc.C) {
	responses := []gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(makeNonAvailabilityResponse(c), http.StatusOK, nil),
	}
	gwacl.PatchManagementAPIResponses(responses)
	azure, err := gwacl.NewManagementAPI("subscription", "", "West US")
	c.Assert(err, gc.IsNil)

	service, err := attemptCreateService(azure, "service", "affinity-group", "")
	c.Check(err, gc.IsNil)
	c.Check(service, gc.IsNil)
}

func (*environSuite) TestAttemptCreateServicePropagatesOtherFailure(c *gc.C) {
	responses := []gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(makeAvailabilityResponse(c), http.StatusOK, nil),
		gwacl.NewDispatcherResponse(nil, http.StatusNotFound, nil),
	}
	gwacl.PatchManagementAPIResponses(responses)
	azure, err := gwacl.NewManagementAPI("subscription", "", "West US")
	c.Assert(err, gc.IsNil)

	_, err = attemptCreateService(azure, "service", "affinity-group", "")
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, ".*Not Found.*")
}

func (*environSuite) TestNewHostedServiceCreatesService(c *gc.C) {
	prefix := "myservice"
	affinityGroup := "affinity-group"
	responses := []gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(makeAvailabilityResponse(c), http.StatusOK, nil),
		gwacl.NewDispatcherResponse(nil, http.StatusOK, nil),
		getAzureServiceResponse(c, gwacl.HostedService{
			HostedServiceDescriptor: gwacl.HostedServiceDescriptor{
				ServiceName: "anything",
			},
		}),
	}
	requests := gwacl.PatchManagementAPIResponses(responses)
	azure, err := gwacl.NewManagementAPI("subscription", "", "West US")
	c.Assert(err, gc.IsNil)

	service, err := newHostedService(azure, prefix, affinityGroup, "")
	c.Assert(err, gc.IsNil)

	c.Assert(*requests, gc.HasLen, 3)
	body := parseCreateServiceRequest(c, (*requests)[1])
	requestedServiceName := getHostedServicePropertiesServiceName(c, (*requests)[2])
	c.Check(body.ServiceName, gc.Matches, prefix+".*")
	c.Check(body.ServiceName, gc.Equals, requestedServiceName)
	c.Check(body.AffinityGroup, gc.Equals, affinityGroup)
	c.Check(service.ServiceName, gc.Equals, "anything")
	c.Check(service.Location, gc.Equals, "")
}

func (*environSuite) TestNewHostedServiceRetriesIfNotUnique(c *gc.C) {
	errorBody := makeNonAvailabilityResponse(c)
	okBody := makeAvailabilityResponse(c)
	// In this scenario, the first two names that we try are already
	// taken.  The third one is unique though, so we succeed.
	responses := []gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(errorBody, http.StatusOK, nil),
		gwacl.NewDispatcherResponse(errorBody, http.StatusOK, nil),
		gwacl.NewDispatcherResponse(okBody, http.StatusOK, nil), // name is unique
		gwacl.NewDispatcherResponse(nil, http.StatusOK, nil),    // create service
		getAzureServiceResponse(c, gwacl.HostedService{
			HostedServiceDescriptor: gwacl.HostedServiceDescriptor{
				ServiceName: "anything",
			},
		}),
	}
	requests := gwacl.PatchManagementAPIResponses(responses)
	azure, err := gwacl.NewManagementAPI("subscription", "", "West US")
	c.Assert(err, gc.IsNil)

	service, err := newHostedService(azure, "service", "affinity-group", "")
	c.Check(err, gc.IsNil)

	c.Assert(*requests, gc.HasLen, 5)
	// How many names have been attempted, and how often?
	// There is a minute chance that this tries the same name twice, and
	// then this test will fail.  If that happens, try seeding the
	// randomizer with some fixed seed that doens't produce the problem.
	attemptedNames := make(map[string]int)
	for _, request := range *requests {
		// Exit the loop if we hit the request to create the service, it comes
		// after the check calls.
		if request.Method == "POST" {
			break
		}
		// Name is the last part of the URL from the GET requests that check
		// availability.
		_, name := path.Split(strings.TrimRight(request.URL, "/"))
		attemptedNames[name] += 1
	}
	// The three attempts we just made all had different service names.
	c.Check(attemptedNames, gc.HasLen, 3)

	// Once newHostedService succeeds, we get a hosted service with the
	// name returned from GetHostedServiceProperties.
	c.Check(service.ServiceName, gc.Equals, "anything")
}

func (*environSuite) TestNewHostedServiceFailsIfUnableToFindUniqueName(c *gc.C) {
	errorBody := makeNonAvailabilityResponse(c)
	responses := []gwacl.DispatcherResponse{}
	for counter := 0; counter < 100; counter++ {
		responses = append(responses, gwacl.NewDispatcherResponse(errorBody, http.StatusOK, nil))
	}
	gwacl.PatchManagementAPIResponses(responses)
	azure, err := gwacl.NewManagementAPI("subscription", "", "West US")
	c.Assert(err, gc.IsNil)

	_, err = newHostedService(azure, "service", "affinity-group", "")
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "could not come up with a unique hosted service name.*")
}

func buildGetServicePropertiesResponses(c *gc.C, services ...*gwacl.HostedService) []gwacl.DispatcherResponse {
	responses := make([]gwacl.DispatcherResponse, len(services))
	for i, service := range services {
		serviceXML, err := service.Serialize()
		c.Assert(err, gc.IsNil)
		responses[i] = gwacl.NewDispatcherResponse([]byte(serviceXML), http.StatusOK, nil)
	}
	return responses
}

func buildStatusOKResponses(c *gc.C, n int) []gwacl.DispatcherResponse {
	responses := make([]gwacl.DispatcherResponse, n)
	for i := range responses {
		responses[i] = gwacl.NewDispatcherResponse(nil, http.StatusOK, nil)
	}
	return responses
}

func makeAzureService(name string) *gwacl.HostedService {
	return &gwacl.HostedService{
		HostedServiceDescriptor: gwacl.HostedServiceDescriptor{ServiceName: name},
	}
}

func makeRole(env *azureEnviron) *gwacl.Role {
	size := "Large"
	vhd := env.newOSDisk("source-image-name")
	userData := "example-user-data"
	return env.newRole(size, vhd, userData, false)
}

func makeLegacyDeployment(env *azureEnviron, serviceName string) *gwacl.HostedService {
	service := makeAzureService(serviceName)
	service.Deployments = []gwacl.Deployment{{
		Name:     serviceName,
		RoleList: []gwacl.Role{*makeRole(env)},
	}}
	return service
}

func makeDeployment(env *azureEnviron, serviceName string) *gwacl.HostedService {
	service := makeAzureService(serviceName)
	service.Deployments = []gwacl.Deployment{{
		Name:     serviceName + "-v2",
		RoleList: []gwacl.Role{*makeRole(env), *makeRole(env)},
	}}
	return service
}

func (s *environSuite) TestStopInstancesDestroysMachines(c *gc.C) {
	env := makeEnviron(c)
	prefix := env.getEnvPrefix()
	service1Name := "service1"
	service1 := makeLegacyDeployment(env, prefix+service1Name)
	service2Name := "service2"
	service2 := makeDeployment(env, prefix+service2Name)

	inst1, err := env.getInstance(service1, "")
	c.Assert(err, gc.IsNil)
	role2Name := service2.Deployments[0].RoleList[0].RoleName
	inst2, err := env.getInstance(service2, role2Name)
	c.Assert(err, gc.IsNil)
	role3Name := service2.Deployments[0].RoleList[1].RoleName
	inst3, err := env.getInstance(service2, role3Name)
	c.Assert(err, gc.IsNil)

	responses := buildGetServicePropertiesResponses(c, service1)
	responses = append(responses, buildStatusOKResponses(c, 1)...) // DeleteHostedService
	responses = append(responses, buildGetServicePropertiesResponses(c, service2)...)
	responses = append(responses, buildStatusOKResponses(c, 1)...) // DeleteHostedService
	requests := gwacl.PatchManagementAPIResponses(responses)
	err = env.StopInstances(inst1.Id(), inst2.Id(), inst3.Id())
	c.Check(err, gc.IsNil)

	// One GET and DELETE per service
	// (GetHostedServiceProperties and DeleteHostedService).
	c.Check(len(*requests), gc.Equals, len(responses))
	assertOneRequestMatches(c, *requests, "GET", ".*"+service1Name+".")
	assertOneRequestMatches(c, *requests, "GET", ".*"+service2Name+".*")
	assertOneRequestMatches(c, *requests, "DELETE", ".*"+service1Name+".*")
	assertOneRequestMatches(c, *requests, "DELETE", ".*"+service2Name+".*")
}

func (s *environSuite) TestStopInstancesServiceSubset(c *gc.C) {
	env := makeEnviron(c)
	service := makeDeployment(env, env.getEnvPrefix()+"service")

	role1Name := service.Deployments[0].RoleList[0].RoleName
	inst1, err := env.getInstance(service, role1Name)
	c.Assert(err, gc.IsNil)

	responses := buildGetServicePropertiesResponses(c, service)
	responses = append(responses, buildStatusOKResponses(c, 1)...) // DeleteRole
	requests := gwacl.PatchManagementAPIResponses(responses)
	err = env.StopInstances(inst1.Id())
	c.Check(err, gc.IsNil)

	// One GET for the service, and one DELETE for the role.
	// The service isn't deleted because it has two roles,
	// and only one is being deleted.
	c.Check(len(*requests), gc.Equals, len(responses))
	assertOneRequestMatches(c, *requests, "GET", ".*"+service.ServiceName+".")
	assertOneRequestMatches(c, *requests, "DELETE", ".*"+role1Name+".*")
}

func (s *environSuite) TestStopInstancesWhenStoppingMachinesFails(c *gc.C) {
	env := makeEnviron(c)
	prefix := env.getEnvPrefix()
	service1 := makeDeployment(env, prefix+"service1")
	service2 := makeDeployment(env, prefix+"service2")
	service1Role1Name := service1.Deployments[0].RoleList[0].RoleName
	inst1, err := env.getInstance(service1, service1Role1Name)
	c.Assert(err, gc.IsNil)
	service2Role1Name := service2.Deployments[0].RoleList[0].RoleName
	inst2, err := env.getInstance(service2, service2Role1Name)
	c.Assert(err, gc.IsNil)

	responses := buildGetServicePropertiesResponses(c, service1)
	// Failed to delete one of the services. This will cause StopInstances to stop
	// immediately.
	responses = append(responses, gwacl.NewDispatcherResponse(nil, http.StatusConflict, nil))
	requests := gwacl.PatchManagementAPIResponses(responses)

	err = env.StopInstances(inst1.Id(), inst2.Id())
	c.Check(err, gc.ErrorMatches, ".*Conflict.*")

	c.Check(len(*requests), gc.Equals, len(responses))
	assertOneRequestMatches(c, *requests, "GET", ".*"+service1.ServiceName+".*")
	assertOneRequestMatches(c, *requests, "DELETE", service1.ServiceName)
}

func (s *environSuite) TestStopInstancesWithZeroInstance(c *gc.C) {
	env := makeEnviron(c)
	err := env.StopInstances()
	c.Check(err, gc.IsNil)
}

// getVnetAndAffinityGroupCleanupResponses returns the responses
// (gwacl.DispatcherResponse) that a fake http server should return
// when gwacl's RemoveVirtualNetworkSite() and DeleteAffinityGroup()
// are called.
func getVnetAndAffinityGroupCleanupResponses(c *gc.C) []gwacl.DispatcherResponse {
	existingConfig := &gwacl.NetworkConfiguration{
		XMLNS:               gwacl.XMLNS_NC,
		VirtualNetworkSites: nil,
	}
	body, err := existingConfig.Serialize()
	c.Assert(err, gc.IsNil)
	cleanupResponses := []gwacl.DispatcherResponse{
		// Return empty net configuration.
		gwacl.NewDispatcherResponse([]byte(body), http.StatusOK, nil),
		// Accept deletion of affinity group.
		gwacl.NewDispatcherResponse(nil, http.StatusOK, nil),
	}
	return cleanupResponses
}

func (s *environSuite) TestDestroyDoesNotCleanStorageIfError(c *gc.C) {
	env := makeEnviron(c)
	s.setDummyStorage(c, env)
	// Populate storage.
	err := bootstrap.SaveState(
		env.Storage(),
		&bootstrap.BootstrapState{StateInstances: []instance.Id{instance.Id("test-id")}})
	c.Assert(err, gc.IsNil)
	responses := []gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(nil, http.StatusBadRequest, nil),
	}
	gwacl.PatchManagementAPIResponses(responses)

	err = env.Destroy()
	c.Check(err, gc.NotNil)

	files, err := storage.List(env.Storage(), "")
	c.Assert(err, gc.IsNil)
	c.Check(files, gc.HasLen, 1)
}

func (s *environSuite) TestDestroyCleansUpStorage(c *gc.C) {
	env := makeEnviron(c)
	s.setDummyStorage(c, env)
	// Populate storage.
	err := bootstrap.SaveState(
		env.Storage(),
		&bootstrap.BootstrapState{StateInstances: []instance.Id{instance.Id("test-id")}})
	c.Assert(err, gc.IsNil)
	responses := getAzureServiceListResponse(c)
	cleanupResponses := getVnetAndAffinityGroupCleanupResponses(c)
	responses = append(responses, cleanupResponses...)
	gwacl.PatchManagementAPIResponses(responses)

	err = env.Destroy()
	c.Check(err, gc.IsNil)

	files, err := storage.List(env.Storage(), "")
	c.Assert(err, gc.IsNil)
	c.Check(files, gc.HasLen, 0)
}

func (s *environSuite) TestDestroyDeletesVirtualNetworkAndAffinityGroup(c *gc.C) {
	env := makeEnviron(c)
	s.setDummyStorage(c, env)
	responses := getAzureServiceListResponse(c)
	// Prepare a configuration with a single virtual network.
	existingConfig := &gwacl.NetworkConfiguration{
		XMLNS: gwacl.XMLNS_NC,
		VirtualNetworkSites: &[]gwacl.VirtualNetworkSite{
			{Name: env.getVirtualNetworkName()},
		},
	}
	body, err := existingConfig.Serialize()
	c.Assert(err, gc.IsNil)
	cleanupResponses := []gwacl.DispatcherResponse{
		// Return existing configuration.
		gwacl.NewDispatcherResponse([]byte(body), http.StatusOK, nil),
		// Accept upload of new configuration.
		gwacl.NewDispatcherResponse(nil, http.StatusOK, nil),
		// Accept deletion of affinity group.
		gwacl.NewDispatcherResponse(nil, http.StatusOK, nil),
	}
	responses = append(responses, cleanupResponses...)
	requests := gwacl.PatchManagementAPIResponses(responses)

	err = env.Destroy()
	c.Check(err, gc.IsNil)

	c.Assert(*requests, gc.HasLen, 4)
	// One request to get the network configuration.
	getRequest := (*requests)[1]
	c.Check(getRequest.Method, gc.Equals, "GET")
	c.Check(strings.HasSuffix(getRequest.URL, "services/networking/media"), gc.Equals, true)
	// One request to upload the new version of the network configuration.
	putRequest := (*requests)[2]
	c.Check(putRequest.Method, gc.Equals, "PUT")
	c.Check(strings.HasSuffix(putRequest.URL, "services/networking/media"), gc.Equals, true)
	// One request to delete the Affinity Group.
	agRequest := (*requests)[3]
	c.Check(strings.Contains(agRequest.URL, env.getAffinityGroupName()), jc.IsTrue)
	c.Check(agRequest.Method, gc.Equals, "DELETE")

}

var emptyListResponse = `
  <?xml version="1.0" encoding="utf-8"?>
  <EnumerationResults ContainerName="http://myaccount.blob.core.windows.net/mycontainer">
    <Prefix>prefix</Prefix>
    <Marker>marker</Marker>
    <MaxResults>maxresults</MaxResults>
    <Delimiter>delimiter</Delimiter>
    <Blobs></Blobs>
    <NextMarker />
  </EnumerationResults>`

// assertOneRequestMatches asserts that at least one request in the given slice
// contains a request with the given method and whose URL matches the given regexp.
func assertOneRequestMatches(c *gc.C, requests []*gwacl.X509Request, method string, urlPattern string) {
	for _, request := range requests {
		matched, err := regexp.MatchString(urlPattern, request.URL)
		if err == nil && request.Method == method && matched {
			return
		}
	}
	c.Error(fmt.Sprintf("none of the requests matches: Method=%v, URL pattern=%v", method, urlPattern))
}

func (s *environSuite) TestDestroyStopsAllInstances(c *gc.C) {
	env := makeEnviron(c)
	s.setDummyStorage(c, env)
	prefix := env.getEnvPrefix()
	service1 := makeDeployment(env, prefix+"service1")
	service2 := makeDeployment(env, prefix+"service2")

	// The call to AllInstances() will return only one service (service1).
	responses := getAzureServiceListResponse(c, service1.HostedServiceDescriptor, service2.HostedServiceDescriptor)
	responses = append(responses, buildStatusOKResponses(c, 2)...) // DeleteHostedService
	responses = append(responses, getVnetAndAffinityGroupCleanupResponses(c)...)
	requests := gwacl.PatchManagementAPIResponses(responses)

	err := env.Destroy()
	c.Check(err, gc.IsNil)

	// One request to get the list of all the environment's instances.
	// One delete request per destroyed service, and two additional
	// requests to delete the Virtual Network and the Affinity Group.
	c.Check((*requests), gc.HasLen, 5)
	c.Check((*requests)[0].Method, gc.Equals, "GET")
	assertOneRequestMatches(c, *requests, "DELETE", ".*"+service1.ServiceName+".*")
	assertOneRequestMatches(c, *requests, "DELETE", ".*"+service2.ServiceName+".*")
}

func (s *environSuite) TestGetInstance(c *gc.C) {
	env := makeEnviron(c)
	service1 := makeLegacyDeployment(env, "service1")
	service2 := makeDeployment(env, "service1")

	// azureEnviron.Instances will call getInstance with roleName==""
	// for legacy instances. This will cause getInstance to get the
	// one and only role (or error if there is more than one).
	inst1, err := env.getInstance(service1, "")
	c.Assert(err, gc.IsNil)
	c.Check(inst1.Id(), gc.Equals, instance.Id("service1"))
	c.Assert(inst1, gc.FitsTypeOf, &azureInstance{})
	c.Check(inst1.(*azureInstance).environ, gc.Equals, env)
	c.Check(inst1.(*azureInstance).roleName, gc.Equals, service1.Deployments[0].RoleList[0].RoleName)
	service1.Deployments[0].RoleList = service2.Deployments[0].RoleList
	inst1, err = env.getInstance(service1, "")
	c.Check(err, gc.ErrorMatches, `expected one role for "service1", got 2`)

	inst2, err := env.getInstance(service2, service2.Deployments[0].RoleList[0].RoleName)
	c.Assert(err, gc.IsNil)
	c.Check(inst2.Id(), gc.Equals, instance.Id("service1-"+service2.Deployments[0].RoleList[0].RoleName))
}

func (s *environSuite) TestInitialPorts(c *gc.C) {
	env := makeEnviron(c)
	service1 := makeLegacyDeployment(env, "service1")
	service2 := makeDeployment(env, "service2")
	service3 := makeDeployment(env, "service3")
	service3.Label = base64.StdEncoding.EncodeToString([]byte(stateServerLabel))

	role1 := &service1.Deployments[0].RoleList[0]
	inst1, err := env.getInstance(service1, role1.RoleName)
	c.Assert(err, gc.IsNil)
	c.Assert(inst1.(*azureInstance).maskStateServerPorts, jc.IsTrue)
	role2 := &service2.Deployments[0].RoleList[0]
	inst2, err := env.getInstance(service2, role2.RoleName)
	c.Assert(err, gc.IsNil)
	role3 := &service3.Deployments[0].RoleList[0]
	inst3, err := env.getInstance(service3, role3.RoleName)
	c.Assert(err, gc.IsNil)

	// Only role2 should report opened state server ports via the Ports method.
	dummyRole := *role1
	configSetNetwork(&dummyRole).InputEndpoints = &[]gwacl.InputEndpoint{{
		LocalPort: env.Config().StatePort(),
		Protocol:  "tcp",
		Name:      "stateserver",
		Port:      env.Config().StatePort(),
	}, {
		LocalPort: env.Config().APIPort(),
		Protocol:  "tcp",
		Name:      "apiserver",
		Port:      env.Config().APIPort(),
	}}
	reportsStateServerPorts := func(inst instance.Instance) bool {
		responses := preparePortChangeConversation(c, &dummyRole)
		gwacl.PatchManagementAPIResponses(responses)
		ports, err := inst.Ports("")
		c.Assert(err, gc.IsNil)
		portmap := make(map[int]bool)
		for _, port := range ports {
			portmap[port.Number] = true
		}
		return portmap[env.Config().StatePort()] && portmap[env.Config().APIPort()]
	}
	c.Check(inst1, gc.Not(jc.Satisfies), reportsStateServerPorts)
	c.Check(inst2, jc.Satisfies, reportsStateServerPorts)
	c.Check(inst3, gc.Not(jc.Satisfies), reportsStateServerPorts)
}

func (*environSuite) TestNewOSVirtualDisk(c *gc.C) {
	env := makeEnviron(c)
	sourceImageName := "source-image-name"

	vhd := env.newOSDisk(sourceImageName)

	mediaLinkUrl, err := url.Parse(vhd.MediaLink)
	c.Check(err, gc.IsNil)
	storageAccount := env.ecfg.storageAccountName()
	c.Check(mediaLinkUrl.Host, gc.Equals, fmt.Sprintf("%s.blob.core.windows.net", storageAccount))
	c.Check(vhd.SourceImageName, gc.Equals, sourceImageName)
}

// mapInputEndpointsByPort takes a slice of input endpoints, and returns them
// as a map keyed by their (external) ports.  This makes it easier to query
// individual endpoints from an array whose ordering you don't know.
// Multiple input endpoints for the same port are treated as an error.
func mapInputEndpointsByPort(c *gc.C, endpoints []gwacl.InputEndpoint) map[int]gwacl.InputEndpoint {
	mapping := make(map[int]gwacl.InputEndpoint)
	for _, endpoint := range endpoints {
		_, have := mapping[endpoint.Port]
		c.Assert(have, gc.Equals, false)
		mapping[endpoint.Port] = endpoint
	}
	return mapping
}

func (s *environSuite) TestNewRole(c *gc.C) {
	s.testNewRole(c, false)
}

func (s *environSuite) TestNewRoleStateServer(c *gc.C) {
	s.testNewRole(c, true)
}

func (*environSuite) testNewRole(c *gc.C, stateServer bool) {
	env := makeEnviron(c)
	size := "Large"
	vhd := env.newOSDisk("source-image-name")
	userData := "example-user-data"

	role := env.newRole(size, vhd, userData, stateServer)

	configs := role.ConfigurationSets
	linuxConfig := configs[0]
	networkConfig := configs[1]
	c.Check(linuxConfig.CustomData, gc.Equals, userData)
	c.Check(linuxConfig.Hostname, gc.Equals, role.RoleName)
	c.Check(linuxConfig.Username, gc.Not(gc.Equals), "")
	c.Check(linuxConfig.Password, gc.Not(gc.Equals), "")
	c.Check(linuxConfig.DisableSSHPasswordAuthentication, gc.Equals, "true")
	c.Check(role.RoleSize, gc.Equals, size)
	c.Check(role.OSVirtualHardDisk, gc.DeepEquals, vhd)

	endpoints := mapInputEndpointsByPort(c, *networkConfig.InputEndpoints)

	// The network config contains an endpoint for ssh communication.
	sshEndpoint, ok := endpoints[22]
	c.Assert(ok, gc.Equals, true)
	c.Check(sshEndpoint.LocalPort, gc.Equals, 22)
	c.Check(sshEndpoint.Protocol, gc.Equals, "tcp")

	if stateServer {
		// There's also an endpoint for the state (mongodb) port.
		stateEndpoint, ok := endpoints[env.Config().StatePort()]
		c.Assert(ok, gc.Equals, true)
		c.Check(stateEndpoint.LocalPort, gc.Equals, env.Config().StatePort())
		c.Check(stateEndpoint.Protocol, gc.Equals, "tcp")

		// And one for the API port.
		apiEndpoint, ok := endpoints[env.Config().APIPort()]
		c.Assert(ok, gc.Equals, true)
		c.Check(apiEndpoint.LocalPort, gc.Equals, env.Config().APIPort())
		c.Check(apiEndpoint.Protocol, gc.Equals, "tcp")
	}
}

func (*environSuite) TestProviderReturnsAzureEnvironProvider(c *gc.C) {
	prov := makeEnviron(c).Provider()
	c.Assert(prov, gc.NotNil)
	azprov, ok := prov.(azureEnvironProvider)
	c.Assert(ok, gc.Equals, true)
	c.Check(azprov, gc.NotNil)
}

func (*environSuite) TestCreateVirtualNetwork(c *gc.C) {
	env := makeEnviron(c)
	responses := []gwacl.DispatcherResponse{
		// No existing configuration found.
		gwacl.NewDispatcherResponse(nil, http.StatusNotFound, nil),
		// Accept upload of new configuration.
		gwacl.NewDispatcherResponse(nil, http.StatusOK, nil),
	}
	requests := gwacl.PatchManagementAPIResponses(responses)

	env.createVirtualNetwork()

	c.Assert(*requests, gc.HasLen, 2)
	request := (*requests)[1]
	body := gwacl.NetworkConfiguration{}
	err := xml.Unmarshal(request.Payload, &body)
	c.Assert(err, gc.IsNil)
	networkConf := (*body.VirtualNetworkSites)[0]
	c.Check(networkConf.Name, gc.Equals, env.getVirtualNetworkName())
	c.Check(networkConf.AffinityGroup, gc.Equals, env.getAffinityGroupName())
}

func (*environSuite) TestDestroyVirtualNetwork(c *gc.C) {
	env := makeEnviron(c)
	// Prepare a configuration with a single virtual network.
	existingConfig := &gwacl.NetworkConfiguration{
		XMLNS: gwacl.XMLNS_NC,
		VirtualNetworkSites: &[]gwacl.VirtualNetworkSite{
			{Name: env.getVirtualNetworkName()},
		},
	}
	body, err := existingConfig.Serialize()
	c.Assert(err, gc.IsNil)
	responses := []gwacl.DispatcherResponse{
		// Return existing configuration.
		gwacl.NewDispatcherResponse([]byte(body), http.StatusOK, nil),
		// Accept upload of new configuration.
		gwacl.NewDispatcherResponse(nil, http.StatusOK, nil),
	}
	requests := gwacl.PatchManagementAPIResponses(responses)

	env.deleteVirtualNetwork()

	c.Assert(*requests, gc.HasLen, 2)
	// One request to get the existing network configuration.
	getRequest := (*requests)[0]
	c.Check(getRequest.Method, gc.Equals, "GET")
	// One request to update the network configuration.
	putRequest := (*requests)[1]
	c.Check(putRequest.Method, gc.Equals, "PUT")
	newConfig := gwacl.NetworkConfiguration{}
	err = xml.Unmarshal(putRequest.Payload, &newConfig)
	c.Assert(err, gc.IsNil)
	// The new configuration has no VirtualNetworkSites.
	c.Check(newConfig.VirtualNetworkSites, gc.IsNil)
}

func (*environSuite) TestGetVirtualNetworkNameContainsEnvName(c *gc.C) {
	env := makeEnviron(c)
	c.Check(strings.Contains(env.getVirtualNetworkName(), env.Name()), jc.IsTrue)
}

func (*environSuite) TestGetVirtualNetworkNameIsConstant(c *gc.C) {
	env := makeEnviron(c)
	c.Check(env.getVirtualNetworkName(), gc.Equals, env.getVirtualNetworkName())
}

func (*environSuite) TestCreateAffinityGroup(c *gc.C) {
	env := makeEnviron(c)
	responses := []gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(nil, http.StatusCreated, nil),
	}
	requests := gwacl.PatchManagementAPIResponses(responses)

	env.createAffinityGroup()

	c.Assert(*requests, gc.HasLen, 1)
	request := (*requests)[0]
	body := gwacl.CreateAffinityGroup{}
	err := xml.Unmarshal(request.Payload, &body)
	c.Assert(err, gc.IsNil)
	c.Check(body.Name, gc.Equals, env.getAffinityGroupName())
	// This is a testing antipattern, the expected data comes from
	// config defaults.  Fix it sometime.
	c.Check(body.Location, gc.Equals, "location")
}

func (*environSuite) TestDestroyAffinityGroup(c *gc.C) {
	env := makeEnviron(c)
	responses := []gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(nil, http.StatusOK, nil),
	}
	requests := gwacl.PatchManagementAPIResponses(responses)

	env.deleteAffinityGroup()

	c.Assert(*requests, gc.HasLen, 1)
	request := (*requests)[0]
	c.Check(strings.Contains(request.URL, env.getAffinityGroupName()), jc.IsTrue)
	c.Check(request.Method, gc.Equals, "DELETE")
}

func (*environSuite) TestGetAffinityGroupName(c *gc.C) {
	env := makeEnviron(c)
	c.Check(strings.Contains(env.getAffinityGroupName(), env.Name()), jc.IsTrue)
}

func (*environSuite) TestGetAffinityGroupNameIsConstant(c *gc.C) {
	env := makeEnviron(c)
	c.Check(env.getAffinityGroupName(), gc.Equals, env.getAffinityGroupName())
}

func (*environSuite) TestGetImageMetadataSigningRequiredDefaultsToTrue(c *gc.C) {
	env := makeEnviron(c)
	// Hard-coded to true for now.  Once we support other base URLs, this
	// may have to become configurable.
	c.Check(env.getImageMetadataSigningRequired(), gc.Equals, true)
}

func (*environSuite) TestSelectInstanceTypeAndImageUsesForcedImage(c *gc.C) {
	env := makeEnviron(c)
	forcedImage := "my-image"
	env.ecfg.attrs["force-image-name"] = forcedImage

	aim := gwacl.RoleNameMap["ExtraLarge"]
	cons := constraints.Value{
		CpuCores: &aim.CpuCores,
		Mem:      &aim.Mem,
	}

	instanceType, image, err := env.selectInstanceTypeAndImage(&instances.InstanceConstraint{
		Region:      "West US",
		Series:      "precise",
		Constraints: cons,
	})
	c.Assert(err, gc.IsNil)

	c.Check(instanceType, gc.Equals, aim.Name)
	c.Check(image, gc.Equals, forcedImage)
}

func (s *environSuite) setupEnvWithDummyMetadata(c *gc.C) *azureEnviron {
	envAttrs := makeAzureConfigMap(c)
	envAttrs["location"] = "North Europe"
	env := makeEnvironWithConfig(c, envAttrs)
	s.setDummyStorage(c, env)
	s.PatchValue(&imagemetadata.DefaultBaseURL, "")
	s.PatchValue(&signedImageDataOnly, false)
	images := []*imagemetadata.ImageMetadata{
		{
			Id:         "image-id",
			VirtType:   "Hyper-V",
			Arch:       "amd64",
			RegionName: "North Europe",
			Endpoint:   "https://management.core.windows.net/",
		},
	}
	makeTestMetadata(c, env, "precise", "North Europe", images)
	return env
}

func (s *environSuite) TestSelectInstanceTypeAndImageUsesSimplestreamsByDefault(c *gc.C) {
	env := s.setupEnvWithDummyMetadata(c)
	// We'll tailor our constraints so as to get a specific instance type.
	aim := gwacl.RoleNameMap["ExtraSmall"]
	cons := constraints.Value{
		CpuCores: &aim.CpuCores,
		Mem:      &aim.Mem,
	}
	instanceType, image, err := env.selectInstanceTypeAndImage(&instances.InstanceConstraint{
		Region:      "North Europe",
		Series:      "precise",
		Constraints: cons,
	})
	c.Assert(err, gc.IsNil)
	c.Assert(instanceType, gc.Equals, aim.Name)
	c.Assert(image, gc.Equals, "image-id")
}

func (*environSuite) TestExtractStorageKeyPicksPrimaryKeyIfSet(c *gc.C) {
	keys := gwacl.StorageAccountKeys{
		Primary:   "mainkey",
		Secondary: "otherkey",
	}
	c.Check(extractStorageKey(&keys), gc.Equals, "mainkey")
}

func (*environSuite) TestExtractStorageKeyFallsBackToSecondaryKey(c *gc.C) {
	keys := gwacl.StorageAccountKeys{
		Secondary: "sparekey",
	}
	c.Check(extractStorageKey(&keys), gc.Equals, "sparekey")
}

func (*environSuite) TestExtractStorageKeyReturnsBlankIfNoneSet(c *gc.C) {
	c.Check(extractStorageKey(&gwacl.StorageAccountKeys{}), gc.Equals, "")
}

func assertSourceContents(c *gc.C, source simplestreams.DataSource, filename string, content []byte) {
	rc, _, err := source.Fetch(filename)
	c.Assert(err, gc.IsNil)
	defer rc.Close()
	retrieved, err := ioutil.ReadAll(rc)
	c.Assert(err, gc.IsNil)
	c.Assert(retrieved, gc.DeepEquals, content)
}

func (s *environSuite) assertGetImageMetadataSources(c *gc.C, stream, officialSourcePath string) {
	envAttrs := makeAzureConfigMap(c)
	if stream != "" {
		envAttrs["image-stream"] = stream
	}
	env := makeEnvironWithConfig(c, envAttrs)
	s.setDummyStorage(c, env)

	data := []byte{1, 2, 3, 4}
	env.Storage().Put("images/filename", bytes.NewReader(data), int64(len(data)))

	sources, err := imagemetadata.GetMetadataSources(env)
	c.Assert(err, gc.IsNil)
	c.Assert(len(sources), gc.Equals, 2)
	assertSourceContents(c, sources[0], "filename", data)
	url, err := sources[1].URL("")
	c.Assert(err, gc.IsNil)
	c.Assert(url, gc.Equals, fmt.Sprintf("http://cloud-images.ubuntu.com/%s/", officialSourcePath))
}

func (s *environSuite) TestGetImageMetadataSources(c *gc.C) {
	s.assertGetImageMetadataSources(c, "", "releases")
	s.assertGetImageMetadataSources(c, "released", "releases")
	s.assertGetImageMetadataSources(c, "daily", "daily")
}

func (s *environSuite) TestGetToolsMetadataSources(c *gc.C) {
	env := makeEnviron(c)
	s.setDummyStorage(c, env)

	data := []byte{1, 2, 3, 4}
	env.Storage().Put("tools/filename", bytes.NewReader(data), int64(len(data)))

	sources, err := tools.GetMetadataSources(env)
	c.Assert(err, gc.IsNil)
	c.Assert(len(sources), gc.Equals, 1)
	assertSourceContents(c, sources[0], "filename", data)
}

func (s *environSuite) TestCheckUnitAssignment(c *gc.C) {
	// If availability-sets-enabled is true, then placement is disabled.
	attrs := makeAzureConfigMap(c)
	attrs["availability-sets-enabled"] = true
	env := environs.Environ(makeEnvironWithConfig(c, attrs))
	err := env.SupportsUnitPlacement()
	c.Assert(err, gc.ErrorMatches, "unit placement is not supported with availability-sets-enabled")

	// If the user disables availability sets, they can do what they want.
	attrs["availability-sets-enabled"] = false
	env = environs.Environ(makeEnvironWithConfig(c, attrs))
	err = env.SupportsUnitPlacement()
	c.Assert(err, gc.IsNil)
}

type startInstanceSuite struct {
	baseEnvironSuite
	env    *azureEnviron
	params environs.StartInstanceParams
}

func (s *startInstanceSuite) SetUpTest(c *gc.C) {
	s.baseEnvironSuite.SetUpTest(c)
	s.env = makeEnviron(c)
	s.setDummyStorage(c, s.env)
	s.env.ecfg.attrs["force-image-name"] = "my-image"
	stateInfo := &state.Info{
		Addrs:    []string{"localhost:123"},
		CACert:   coretesting.CACert,
		Password: "password",
		Tag:      "machine-1",
	}
	apiInfo := &api.Info{
		Addrs:    []string{"localhost:124"},
		CACert:   coretesting.CACert,
		Password: "admin",
		Tag:      "machine-1",
	}
	s.params = environs.StartInstanceParams{
		Tools: envtesting.AssertUploadFakeToolsVersions(
			c, s.env.storage, envtesting.V120p...,
		),
		MachineConfig: environs.NewMachineConfig(
			"1", "yanonce", nil, nil, stateInfo, apiInfo,
		),
	}
}

func (s *startInstanceSuite) startInstance(c *gc.C) (serviceName string, stateServer bool) {
	var called bool
	restore := testing.PatchValue(&createInstance, func(env *azureEnviron, azure *gwacl.ManagementAPI, role *gwacl.Role, serviceNameArg string, stateServerArg bool) (instance.Instance, error) {
		serviceName = serviceNameArg
		stateServer = stateServerArg
		called = true
		return nil, nil
	})
	defer restore()
	_, _, _, err := s.env.StartInstance(s.params)
	c.Assert(err, gc.IsNil)
	c.Assert(called, jc.IsTrue)
	return serviceName, stateServer
}

func (s *startInstanceSuite) TestStartInstanceDistributionGroupError(c *gc.C) {
	s.params.DistributionGroup = func() ([]instance.Id, error) {
		return nil, fmt.Errorf("DistributionGroupError")
	}
	s.env.ecfg.attrs["availability-sets-enabled"] = true
	_, _, _, err := s.env.StartInstance(s.params)
	c.Assert(err, gc.ErrorMatches, "DistributionGroupError")
	// DistributionGroup should not be called if availability-sets-enabled=false.
	s.env.ecfg.attrs["availability-sets-enabled"] = false
	s.startInstance(c)
}

func (s *startInstanceSuite) TestStartInstanceDistributionGroupEmpty(c *gc.C) {
	// serviceName will be empty if DistributionGroup is nil or returns nothing.
	s.env.ecfg.attrs["availability-sets-enabled"] = true
	serviceName, _ := s.startInstance(c)
	c.Assert(serviceName, gc.Equals, "")
	s.params.DistributionGroup = func() ([]instance.Id, error) { return nil, nil }
	serviceName, _ = s.startInstance(c)
	c.Assert(serviceName, gc.Equals, "")
}

func (s *startInstanceSuite) TestStartInstanceDistributionGroup(c *gc.C) {
	s.params.DistributionGroup = func() ([]instance.Id, error) {
		return []instance.Id{
			instance.Id(s.env.getEnvPrefix() + "whatever-role0"),
		}, nil
	}
	// DistributionGroup will only have an effect if
	// availability-sets-enabled=true.
	s.env.ecfg.attrs["availability-sets-enabled"] = false
	serviceName, _ := s.startInstance(c)
	c.Assert(serviceName, gc.Equals, "")
	s.env.ecfg.attrs["availability-sets-enabled"] = true
	serviceName, _ = s.startInstance(c)
	c.Assert(serviceName, gc.Equals, "juju-testenv-whatever")
}

func (s *startInstanceSuite) TestStartInstanceStateServerJobs(c *gc.C) {
	// If the machine has the JobManagesEnviron job,
	// we should see stateServer==true.
	s.params.MachineConfig.Jobs = []apiparams.MachineJob{
		apiparams.JobHostUnits,
	}
	_, stateServer := s.startInstance(c)
	c.Assert(stateServer, jc.IsFalse)
	s.params.MachineConfig.Jobs = []apiparams.MachineJob{
		apiparams.JobHostUnits, apiparams.JobManageEnviron,
	}
	_, stateServer = s.startInstance(c)
	c.Assert(stateServer, jc.IsTrue)
}

func (s *environSuite) TestConstraintsValidator(c *gc.C) {
	env := s.setupEnvWithDummyMetadata(c)
	validator, err := env.ConstraintsValidator()
	c.Assert(err, gc.IsNil)
	cons := constraints.MustParse("arch=amd64 tags=bar cpu-power=10")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, gc.IsNil)
	c.Assert(unsupported, jc.SameContents, []string{"cpu-power", "tags"})
}

func (s *environSuite) TestConstraintsValidatorVocab(c *gc.C) {
	env := s.setupEnvWithDummyMetadata(c)
	validator, err := env.ConstraintsValidator()
	c.Assert(err, gc.IsNil)
	cons := constraints.MustParse("arch=ppc64")
	_, err = validator.Validate(cons)
	c.Assert(err, gc.ErrorMatches, "invalid constraint value: arch=ppc64\nvalid values are:.*")
	cons = constraints.MustParse("instance-type=foo")
	_, err = validator.Validate(cons)
	c.Assert(err, gc.ErrorMatches, "invalid constraint value: instance-type=foo\nvalid values are:.*")
}
