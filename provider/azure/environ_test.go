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

	gc "launchpad.net/gocheck"
	"launchpad.net/gwacl"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
)

type environSuite struct {
	providerSuite
}

var _ = gc.Suite(&environSuite{})

// makeEnviron creates a fake azureEnviron with arbitrary configuration.
func makeEnviron(c *gc.C) *azureEnviron {
	attrs := makeAzureConfigMap(c)
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
func (s *environSuite) setDummyStorage(c *gc.C, env *azureEnviron) {
	closer, storage, _ := envtesting.CreateLocalTestStorage(c)
	env.storage = storage
	s.AddCleanup(func(c *gc.C) { closer.Close() })
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
	testing.TestLockingFunction(&original.Mutex, func() { original.getSnapshot() })
}

func (*environSuite) TestName(c *gc.C) {
	env := azureEnviron{name: "foo"}
	c.Check(env.Name(), gc.Equals, env.name)
}

func (*environSuite) TestPrecheck(c *gc.C) {
	env := azureEnviron{name: "foo"}
	var cons constraints.Value
	err := env.PrecheckInstance("saucy", cons)
	c.Check(err, gc.IsNil)
	err = env.PrecheckContainer("saucy", instance.LXC)
	c.Check(err, gc.ErrorMatches, "azure provider does not support containers")
}

func (*environSuite) TestConfigReturnsConfig(c *gc.C) {
	cfg := new(config.Config)
	ecfg := azureEnvironConfig{Config: cfg}
	env := azureEnviron{ecfg: &ecfg}
	c.Check(env.Config(), gc.Equals, cfg)
}

func (*environSuite) TestConfigLocksEnviron(c *gc.C) {
	env := azureEnviron{name: "env", ecfg: new(azureEnvironConfig)}
	testing.TestLockingFunction(&env.Mutex, func() { env.Config() })
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

func getAzureServiceListResponse(c *gc.C, services []gwacl.HostedServiceDescriptor) []gwacl.DispatcherResponse {
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

// getAzureServiceResponses returns the slice of responses
// (gwacl.DispatcherResponse) which correspond to the API requests used to
// get the properties of a Service.
func getAzureServiceResponses(c *gc.C, service gwacl.HostedService) []gwacl.DispatcherResponse {
	serviceXML, err := service.Serialize()
	c.Assert(err, gc.IsNil)
	responses := []gwacl.DispatcherResponse{gwacl.NewDispatcherResponse(
		[]byte(serviceXML),
		http.StatusOK,
		nil,
	)}
	return responses
}

func patchWithServiceListResponse(c *gc.C, services []gwacl.HostedServiceDescriptor) *[]*gwacl.X509Request {
	responses := getAzureServiceListResponse(c, services)
	return gwacl.PatchManagementAPIResponses(responses)
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
	services := []gwacl.HostedServiceDescriptor{{ServiceName: "deployment-in-another-env"}, {ServiceName: prefix + "deployment-1"}, {ServiceName: prefix + "deployment-2"}}
	requests := patchWithServiceListResponse(c, services)
	instances, err := env.AllInstances()
	c.Assert(err, gc.IsNil)
	c.Check(len(instances), gc.Equals, 2)
	c.Check(instances[0].Id(), gc.Equals, instance.Id(prefix+"deployment-1"))
	c.Check(instances[1].Id(), gc.Equals, instance.Id(prefix+"deployment-2"))
	c.Check(len(*requests), gc.Equals, 1)
}

func (suite *environSuite) TestInstancesReturnsFilteredList(c *gc.C) {
	services := []gwacl.HostedServiceDescriptor{{ServiceName: "deployment-1"}, {ServiceName: "deployment-2"}}
	requests := patchWithServiceListResponse(c, services)
	env := makeEnviron(c)
	instances, err := env.Instances([]instance.Id{"deployment-1"})
	c.Assert(err, gc.IsNil)
	c.Check(len(instances), gc.Equals, 1)
	c.Check(instances[0].Id(), gc.Equals, instance.Id("deployment-1"))
	c.Check(len(*requests), gc.Equals, 1)
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
	services := []gwacl.HostedServiceDescriptor{{ServiceName: "deployment-1"}, {ServiceName: "deployment-2"}}
	requests := patchWithServiceListResponse(c, services)
	env := makeEnviron(c)
	instances, err := env.Instances([]instance.Id{"deployment-1", "unknown-deployment"})
	c.Assert(err, gc.Equals, environs.ErrPartialInstances)
	c.Check(len(instances), gc.Equals, 1)
	c.Check(instances[0].Id(), gc.Equals, instance.Id("deployment-1"))
	c.Check(len(*requests), gc.Equals, 1)
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

	c.Check(env.ecfg.Config.DefaultSeries(), gc.Equals, "feisty")
}

func (*environSuite) TestSetConfigLocksEnviron(c *gc.C) {
	env := makeEnviron(c)
	cfg, err := config.New(config.NoDefaults, makeAzureConfigMap(c))
	c.Assert(err, gc.IsNil)

	testing.TestLockingFunction(&env.Mutex, func() { env.SetConfig(cfg) })
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
	instanceID := "my-instance"
	patchWithServiceListResponse(c, []gwacl.HostedServiceDescriptor{{
		ServiceName: instanceID,
	}})
	env := makeEnviron(c)
	s.setDummyStorage(c, env)
	err := bootstrap.SaveState(
		env.Storage(),
		&bootstrap.BootstrapState{StateInstances: []instance.Id{instance.Id(instanceID)}})
	c.Assert(err, gc.IsNil)

	stateInfo, apiInfo, err := env.StateInfo()
	c.Assert(err, gc.IsNil)

	config := env.Config()
	dnsName := "my-instance." + AZURE_DOMAIN_NAME
	stateServerAddr := fmt.Sprintf("%s:%d", dnsName, config.StatePort())
	apiServerAddr := fmt.Sprintf("%s:%d", dnsName, config.APIPort())
	c.Check(stateInfo.Addrs, gc.DeepEquals, []string{stateServerAddr})
	c.Check(apiInfo.Addrs, gc.DeepEquals, []string{apiServerAddr})
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
	location := "location"
	responses := []gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(makeAvailabilityResponse(c), http.StatusOK, nil),
		gwacl.NewDispatcherResponse(nil, http.StatusOK, nil),
	}
	requests := gwacl.PatchManagementAPIResponses(responses)
	azure, err := gwacl.NewManagementAPI("subscription", "", "West US")
	c.Assert(err, gc.IsNil)

	service, err := attemptCreateService(azure, prefix, affinityGroup, location)
	c.Assert(err, gc.IsNil)

	c.Assert(*requests, gc.HasLen, 2)
	body := parseCreateServiceRequest(c, (*requests)[1])
	c.Check(body.ServiceName, gc.Equals, service.ServiceName)
	c.Check(body.AffinityGroup, gc.Equals, affinityGroup)
	c.Check(service.ServiceName, gc.Matches, prefix+".*")
	c.Check(service.Location, gc.Equals, location)

	label, err := base64.StdEncoding.DecodeString(service.Label)
	c.Assert(err, gc.IsNil)
	c.Check(string(label), gc.Equals, service.ServiceName)
}

func (*environSuite) TestAttemptCreateServiceReturnsNilIfNameNotUnique(c *gc.C) {
	responses := []gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(makeNonAvailabilityResponse(c), http.StatusOK, nil),
	}
	gwacl.PatchManagementAPIResponses(responses)
	azure, err := gwacl.NewManagementAPI("subscription", "", "West US")
	c.Assert(err, gc.IsNil)

	service, err := attemptCreateService(azure, "service", "affinity-group", "location")
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

	_, err = attemptCreateService(azure, "service", "affinity-group", "location")
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, ".*Not Found.*")
}

func (*environSuite) TestNewHostedServiceCreatesService(c *gc.C) {
	prefix := "myservice"
	affinityGroup := "affinity-group"
	location := "location"
	responses := []gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(makeAvailabilityResponse(c), http.StatusOK, nil),
		gwacl.NewDispatcherResponse(nil, http.StatusOK, nil),
	}
	requests := gwacl.PatchManagementAPIResponses(responses)
	azure, err := gwacl.NewManagementAPI("subscription", "", "West US")
	c.Assert(err, gc.IsNil)

	service, err := newHostedService(azure, prefix, affinityGroup, location)
	c.Assert(err, gc.IsNil)

	c.Assert(*requests, gc.HasLen, 2)
	body := parseCreateServiceRequest(c, (*requests)[1])
	c.Check(body.ServiceName, gc.Equals, service.ServiceName)
	c.Check(body.AffinityGroup, gc.Equals, affinityGroup)
	c.Check(service.ServiceName, gc.Matches, prefix+".*")
	c.Check(service.Location, gc.Equals, location)
}

func (*environSuite) TestNewHostedServiceRetriesIfNotUnique(c *gc.C) {
	errorBody := makeNonAvailabilityResponse(c)
	okBody := makeAvailabilityResponse(c)
	// In this scenario, the first two names that we try are already
	// taken.  The third one is unique though, so we succeed.
	responses := []gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(errorBody, http.StatusOK, nil),
		gwacl.NewDispatcherResponse(errorBody, http.StatusOK, nil),
		gwacl.NewDispatcherResponse(okBody, http.StatusOK, nil),
		gwacl.NewDispatcherResponse(nil, http.StatusOK, nil),
	}
	requests := gwacl.PatchManagementAPIResponses(responses)
	azure, err := gwacl.NewManagementAPI("subscription", "", "West US")
	c.Assert(err, gc.IsNil)

	service, err := newHostedService(azure, "service", "affinity-group", "location")
	c.Check(err, gc.IsNil)

	c.Assert(*requests, gc.HasLen, 4)
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
	// last requested name.
	c.Check(
		service.ServiceName,
		gc.Equals,
		parseCreateServiceRequest(c, (*requests)[3]).ServiceName)
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

	_, err = newHostedService(azure, "service", "affinity-group", "location")
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "could not come up with a unique hosted service name.*")
}

// buildDestroyAzureServiceResponses returns a slice containing the responses that a fake Azure server
// can use to simulate the deletion of the given list of services.
func buildDestroyAzureServiceResponses(c *gc.C, services []*gwacl.HostedService) []gwacl.DispatcherResponse {
	responses := []gwacl.DispatcherResponse{}
	for _, service := range services {
		// When destroying a hosted service, gwacl first issues a Get request
		// to fetch the properties of the services.  Then it destroys all the
		// deployments found in this service (none in this case, we make sure
		// the service does not contain deployments to keep the testing simple)
		// And it finally deletes the service itself.
		if len(service.Deployments) != 0 {
			panic("buildDestroyAzureServiceResponses does not support services with deployments!")
		}
		serviceXML, err := service.Serialize()
		c.Assert(err, gc.IsNil)
		serviceGetResponse := gwacl.NewDispatcherResponse(
			[]byte(serviceXML),
			http.StatusOK,
			nil,
		)
		responses = append(responses, serviceGetResponse)
		serviceDeleteResponse := gwacl.NewDispatcherResponse(
			nil,
			http.StatusOK,
			nil,
		)
		responses = append(responses, serviceDeleteResponse)
	}
	return responses
}

func makeAzureService(name string) (*gwacl.HostedService, *gwacl.HostedServiceDescriptor) {
	service1Desc := &gwacl.HostedServiceDescriptor{ServiceName: name}
	service1 := &gwacl.HostedService{HostedServiceDescriptor: *service1Desc}
	return service1, service1Desc
}

func (s *environSuite) setServiceDeletionConcurrency(nbGoroutines int) {
	s.PatchValue(&maxConcurrentDeletes, nbGoroutines)
}

func (s *environSuite) TestStopInstancesDestroysMachines(c *gc.C) {
	s.setServiceDeletionConcurrency(3)
	service1Name := "service1"
	service1, service1Desc := makeAzureService(service1Name)
	service2Name := "service2"
	service2, service2Desc := makeAzureService(service2Name)
	services := []*gwacl.HostedService{service1, service2}
	responses := buildDestroyAzureServiceResponses(c, services)
	requests := gwacl.PatchManagementAPIResponses(responses)
	env := makeEnviron(c)
	instances := convertToInstances(
		[]gwacl.HostedServiceDescriptor{*service1Desc, *service2Desc},
		env)

	err := env.StopInstances(instances)
	c.Check(err, gc.IsNil)

	// It takes 2 API calls to delete each service:
	// - one GET request to fetch the service's properties;
	// - one DELETE request to delete the service.
	c.Check(len(*requests), gc.Equals, len(services)*2)
	assertOneRequestMatches(c, *requests, "GET", ".*"+service1Name+".*")
	assertOneRequestMatches(c, *requests, "DELETE", ".*"+service1Name+".*")
	assertOneRequestMatches(c, *requests, "GET", ".*"+service2Name+".")
	assertOneRequestMatches(c, *requests, "DELETE", ".*"+service2Name+".*")
}

func (s *environSuite) TestStopInstancesWhenStoppingMachinesFails(c *gc.C) {
	s.setServiceDeletionConcurrency(3)
	responses := []gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(nil, http.StatusConflict, nil),
	}
	service1Name := "service1"
	_, service1Desc := makeAzureService(service1Name)
	service2Name := "service2"
	service2, service2Desc := makeAzureService(service2Name)
	services := []*gwacl.HostedService{service2}
	destroyResponses := buildDestroyAzureServiceResponses(c, services)
	responses = append(responses, destroyResponses...)
	requests := gwacl.PatchManagementAPIResponses(responses)
	env := makeEnviron(c)
	instances := convertToInstances(
		[]gwacl.HostedServiceDescriptor{*service1Desc, *service2Desc}, env)

	err := env.StopInstances(instances)
	c.Check(err, gc.ErrorMatches, ".*Conflict.*")

	c.Check(len(*requests), gc.Equals, 3)
	assertOneRequestMatches(c, *requests, "GET", ".*"+service1Name+".")
	assertOneRequestMatches(c, *requests, "GET", ".*"+service2Name+".")
	// Only one of the services was deleted.
	assertOneRequestMatches(c, *requests, "DELETE", ".*")
}

func (s *environSuite) TestStopInstancesWithLimitedConcurrency(c *gc.C) {
	s.setServiceDeletionConcurrency(3)
	services := []*gwacl.HostedService{}
	serviceDescs := []gwacl.HostedServiceDescriptor{}
	for i := 0; i < 10; i++ {
		serviceName := fmt.Sprintf("service%d", i)
		service, serviceDesc := makeAzureService(serviceName)
		services = append(services, service)
		serviceDescs = append(serviceDescs, *serviceDesc)
	}
	responses := buildDestroyAzureServiceResponses(c, services)
	requests := gwacl.PatchManagementAPIResponses(responses)
	env := makeEnviron(c)
	instances := convertToInstances(serviceDescs, env)

	err := env.StopInstances(instances)
	c.Check(err, gc.IsNil)
	c.Check(len(*requests), gc.Equals, len(services)*2)
}

func (s *environSuite) TestStopInstancesWithZeroInstance(c *gc.C) {
	s.setServiceDeletionConcurrency(3)
	env := makeEnviron(c)
	instances := []instance.Instance{}

	err := env.StopInstances(instances)
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
	services := []gwacl.HostedServiceDescriptor{}
	responses := getAzureServiceListResponse(c, services)
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
	services := []gwacl.HostedServiceDescriptor{}
	responses := getAzureServiceListResponse(c, services)
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
	s.setServiceDeletionConcurrency(3)
	env := makeEnviron(c)
	s.setDummyStorage(c, env)

	// Simulate 2 instances corresponding to two Azure services.
	prefix := env.getEnvPrefix()
	service1Name := prefix + "service1"
	service1, service1Desc := makeAzureService(service1Name)
	services := []*gwacl.HostedService{service1}
	// The call to AllInstances() will return only one service (service1).
	listInstancesResponses := getAzureServiceListResponse(c, []gwacl.HostedServiceDescriptor{*service1Desc})
	destroyResponses := buildDestroyAzureServiceResponses(c, services)
	responses := append(listInstancesResponses, destroyResponses...)
	cleanupResponses := getVnetAndAffinityGroupCleanupResponses(c)
	responses = append(responses, cleanupResponses...)
	requests := gwacl.PatchManagementAPIResponses(responses)

	err := env.Destroy()
	c.Check(err, gc.IsNil)

	// One request to get the list of all the environment's instances.
	// Then two requests per destroyed machine (one to fetch the
	// service's information, one to delete it) and two requests to delete
	// the Virtual Network and the Affinity Group.
	c.Check((*requests), gc.HasLen, 1+len(services)*2+2)
	c.Check((*requests)[0].Method, gc.Equals, "GET")
	assertOneRequestMatches(c, *requests, "GET", ".*"+service1Name+".*")
	assertOneRequestMatches(c, *requests, "DELETE", ".*"+service1Name+".*")
}

func (*environSuite) TestGetInstance(c *gc.C) {
	env := makeEnviron(c)
	prefix := env.getEnvPrefix()
	serviceName := prefix + "instance-name"
	serviceDesc := gwacl.HostedServiceDescriptor{ServiceName: serviceName}
	service := gwacl.HostedService{HostedServiceDescriptor: serviceDesc}
	responses := getAzureServiceResponses(c, service)
	gwacl.PatchManagementAPIResponses(responses)

	instance, err := env.getInstance("serviceName")
	c.Check(err, gc.IsNil)

	c.Check(string(instance.Id()), gc.Equals, serviceName)
	c.Check(instance, gc.FitsTypeOf, &azureInstance{})
	azInstance := instance.(*azureInstance)
	c.Check(azInstance.environ, gc.Equals, env)
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

func (*environSuite) TestNewRole(c *gc.C) {
	env := makeEnviron(c)
	size := "Large"
	vhd := env.newOSDisk("source-image-name")
	userData := "example-user-data"
	hostname := "hostname"

	role := env.newRole(size, vhd, userData, hostname)

	configs := role.ConfigurationSets
	linuxConfig := configs[0]
	networkConfig := configs[1]
	c.Check(linuxConfig.CustomData, gc.Equals, userData)
	c.Check(linuxConfig.Hostname, gc.Equals, hostname)
	c.Check(linuxConfig.Username, gc.Not(gc.Equals), "")
	c.Check(linuxConfig.Password, gc.Not(gc.Equals), "")
	c.Check(linuxConfig.DisableSSHPasswordAuthentication, gc.Equals, "true")
	c.Check(role.RoleSize, gc.Equals, size)
	c.Check(role.OSVirtualHardDisk[0], gc.Equals, *vhd)

	endpoints := mapInputEndpointsByPort(c, *networkConfig.InputEndpoints)

	// The network config contains an endpoint for ssh communication.
	sshEndpoint, ok := endpoints[22]
	c.Assert(ok, gc.Equals, true)
	c.Check(sshEndpoint.LocalPort, gc.Equals, 22)
	c.Check(sshEndpoint.Protocol, gc.Equals, "tcp")

	// There's also an endpoint for the state (mongodb) port.
	// TODO: Ought to have this only for state servers.
	stateEndpoint, ok := endpoints[env.Config().StatePort()]
	c.Assert(ok, gc.Equals, true)
	c.Check(stateEndpoint.LocalPort, gc.Equals, env.Config().StatePort())
	c.Check(stateEndpoint.Protocol, gc.Equals, "tcp")

	// And one for the API port.
	// TODO: Ought to have this only for API servers.
	apiEndpoint, ok := endpoints[env.Config().APIPort()]
	c.Assert(ok, gc.Equals, true)
	c.Check(apiEndpoint.LocalPort, gc.Equals, env.Config().APIPort())
	c.Check(apiEndpoint.Protocol, gc.Equals, "tcp")
}

func (*environSuite) TestNewDeployment(c *gc.C) {
	env := makeEnviron(c)
	deploymentName := "deployment-name"
	deploymentLabel := "deployment-label"
	virtualNetworkName := "virtual-network-name"
	vhd := env.newOSDisk("source-image-name")
	role := env.newRole("Small", vhd, "user-data", "hostname")

	deployment := env.newDeployment(role, deploymentName, deploymentLabel, virtualNetworkName)

	base64Label := base64.StdEncoding.EncodeToString([]byte(deploymentLabel))
	c.Check(deployment.Label, gc.Equals, base64Label)
	c.Check(deployment.Name, gc.Equals, deploymentName)
	c.Check(deployment.RoleList, gc.HasLen, 1)
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

func (*environSuite) TestGetImageStreamDefaultsToBlank(c *gc.C) {
	env := makeEnviron(c)
	// Hard-coded to default for now.
	c.Check(env.getImageStream(), gc.Equals, "")
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

	instanceType, image, err := env.selectInstanceTypeAndImage(cons, "precise", "West US")
	c.Assert(err, gc.IsNil)

	c.Check(instanceType, gc.Equals, aim.Name)
	c.Check(image, gc.Equals, forcedImage)
}

func (*environSuite) TestSelectInstanceTypeAndImageUsesSimplestreamsByDefault(c *gc.C) {
	env := makeEnviron(c)

	// We'll tailor our constraints so as to get a specific instance type.
	aim := gwacl.RoleNameMap["ExtraSmall"]
	cons := constraints.Value{
		CpuCores: &aim.CpuCores,
		Mem:      &aim.Mem,
	}

	// We have one image available.
	images := []*imagemetadata.ImageMetadata{
		{
			Id:          "image",
			VType:       "Hyper-V",
			Arch:        "amd64",
			RegionAlias: "North Europe",
			RegionName:  "North Europe",
			Endpoint:    "http://localhost/",
		},
	}
	cleanup := patchFetchImageMetadata(images, nil)
	defer cleanup()

	instanceType, image, err := env.selectInstanceTypeAndImage(cons, "precise", "West US")
	c.Assert(err, gc.IsNil)

	c.Check(instanceType, gc.Equals, aim.Name)
	c.Check(image, gc.Equals, "image")
}

func (*environSuite) TestConvertToInstances(c *gc.C) {
	services := []gwacl.HostedServiceDescriptor{
		{ServiceName: "foo"}, {ServiceName: "bar"},
	}
	env := makeEnviron(c)
	instances := convertToInstances(services, env)
	c.Check(instances, gc.DeepEquals, []instance.Instance{
		&azureInstance{services[0], env},
		&azureInstance{services[1], env},
	})
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

func (s *environSuite) TestGetImageMetadataSources(c *gc.C) {
	env := makeEnviron(c)
	s.setDummyStorage(c, env)

	data := []byte{1, 2, 3, 4}
	env.Storage().Put("images/filename", bytes.NewReader(data), int64(len(data)))

	sources, err := imagemetadata.GetMetadataSources(env)
	c.Assert(err, gc.IsNil)
	c.Assert(len(sources), gc.Equals, 2)
	assertSourceContents(c, sources[0], "filename", data)
	url, err := sources[1].URL("")
	c.Assert(err, gc.IsNil)
	c.Assert(url, gc.Equals, imagemetadata.DefaultBaseURL+"/")
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
