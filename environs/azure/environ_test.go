// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"

	. "launchpad.net/gocheck"
	"launchpad.net/gwacl"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/localstorage"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/testing"
	. "launchpad.net/juju-core/testing/checkers"
)

type environSuite struct {
	providerSuite
}

var _ = Suite(&environSuite{})

// makeEnviron creates a fake azureEnviron with arbitrary configuration.
func makeEnviron(c *C) *azureEnviron {
	attrs := makeAzureConfigMap(c)
	cfg, err := config.New(attrs)
	c.Assert(err, IsNil)
	env, err := NewEnviron(cfg)
	c.Assert(err, IsNil)
	// Prevent the test from trying to query for a storage-account key.
	env.storageAccountKey = "fake-storage-account-key"
	return env
}

// setDummyStorage injects the local provider's fake storage implementation
// into the given environment, so that tests can manipulate storage as if it
// were real.
// Returns a cleanup function that must be called when done with the storage.
func setDummyStorage(c *C, env *azureEnviron) func() {
	listener, err := localstorage.Serve("127.0.0.1:0", c.MkDir())
	c.Assert(err, IsNil)
	env.storage = localstorage.Client(listener.Addr().String())
	return func() { listener.Close() }
}

func (*environSuite) TestGetSnapshot(c *C) {
	original := azureEnviron{name: "this-env", ecfg: new(azureEnvironConfig)}
	snapshot := original.getSnapshot()

	// The snapshot is identical to the original.
	c.Check(*snapshot, DeepEquals, original)

	// However, they are distinct objects.
	c.Check(snapshot, Not(Equals), &original)

	// It's a shallow copy; they still share pointers.
	c.Check(snapshot.ecfg, Equals, original.ecfg)

	// Neither object is locked at the end of the copy.
	c.Check(original.Mutex, Equals, sync.Mutex{})
	c.Check(snapshot.Mutex, Equals, sync.Mutex{})
}

func (*environSuite) TestGetSnapshotLocksEnviron(c *C) {
	original := azureEnviron{}
	testing.TestLockingFunction(&original.Mutex, func() { original.getSnapshot() })
}

func (*environSuite) TestName(c *C) {
	env := azureEnviron{name: "foo"}
	c.Check(env.Name(), Equals, env.name)
}

func (*environSuite) TestConfigReturnsConfig(c *C) {
	cfg := new(config.Config)
	ecfg := azureEnvironConfig{Config: cfg}
	env := azureEnviron{ecfg: &ecfg}
	c.Check(env.Config(), Equals, cfg)
}

func (*environSuite) TestConfigLocksEnviron(c *C) {
	env := azureEnviron{name: "env", ecfg: new(azureEnvironConfig)}
	testing.TestLockingFunction(&env.Mutex, func() { env.Config() })
}

func (*environSuite) TestGetManagementAPI(c *C) {
	env := makeEnviron(c)
	context, err := env.getManagementAPI()
	c.Assert(err, IsNil)
	defer env.releaseManagementAPI(context)
	c.Check(context, NotNil)
	c.Check(context.ManagementAPI, NotNil)
	c.Check(context.certFile, NotNil)
}

func (*environSuite) TestReleaseManagementAPIAcceptsNil(c *C) {
	env := makeEnviron(c)
	env.releaseManagementAPI(nil)
	// The real test is that this does not panic.
}

func (*environSuite) TestReleaseManagementAPIAcceptsIncompleteContext(c *C) {
	env := makeEnviron(c)
	context := azureManagementContext{
		ManagementAPI: nil,
		certFile:      nil,
	}
	env.releaseManagementAPI(&context)
	// The real test is that this does not panic.
}

func getAzureServiceListResponse(c *C, services []gwacl.HostedServiceDescriptor) []gwacl.DispatcherResponse {
	list := gwacl.HostedServiceDescriptorList{HostedServices: services}
	listXML, err := list.Serialize()
	c.Assert(err, IsNil)
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
func getAzureServiceResponses(c *C, service gwacl.HostedService) []gwacl.DispatcherResponse {
	serviceXML, err := service.Serialize()
	c.Assert(err, IsNil)
	responses := []gwacl.DispatcherResponse{gwacl.NewDispatcherResponse(
		[]byte(serviceXML),
		http.StatusOK,
		nil,
	)}
	return responses
}

func patchWithServiceListResponse(c *C, services []gwacl.HostedServiceDescriptor) *[]*gwacl.X509Request {
	responses := getAzureServiceListResponse(c, services)
	return gwacl.PatchManagementAPIResponses(responses)
}

func (suite *environSuite) TestGetEnvPrefixContainsEnvName(c *C) {
	env := makeEnviron(c)
	c.Check(strings.Contains(env.getEnvPrefix(), env.Name()), IsTrue)
}

func (*environSuite) TestGetContainerName(c *C) {
	env := makeEnviron(c)
	expected := env.getEnvPrefix() + "private"
	c.Check(env.getContainerName(), Equals, expected)
}

func (suite *environSuite) TestAllInstances(c *C) {
	env := makeEnviron(c)
	prefix := env.getEnvPrefix()
	services := []gwacl.HostedServiceDescriptor{{ServiceName: "deployment-in-another-env"}, {ServiceName: prefix + "deployment-1"}, {ServiceName: prefix + "deployment-2"}}
	requests := patchWithServiceListResponse(c, services)
	instances, err := env.AllInstances()
	c.Assert(err, IsNil)
	c.Check(len(instances), Equals, 2)
	c.Check(instances[0].Id(), Equals, instance.Id(prefix+"deployment-1"))
	c.Check(instances[1].Id(), Equals, instance.Id(prefix+"deployment-2"))
	c.Check(len(*requests), Equals, 1)
}

func (suite *environSuite) TestInstancesReturnsFilteredList(c *C) {
	services := []gwacl.HostedServiceDescriptor{{ServiceName: "deployment-1"}, {ServiceName: "deployment-2"}}
	requests := patchWithServiceListResponse(c, services)
	env := makeEnviron(c)
	instances, err := env.Instances([]instance.Id{"deployment-1"})
	c.Assert(err, IsNil)
	c.Check(len(instances), Equals, 1)
	c.Check(instances[0].Id(), Equals, instance.Id("deployment-1"))
	c.Check(len(*requests), Equals, 1)
}

func (suite *environSuite) TestInstancesReturnsErrNoInstancesIfNoInstancesRequested(c *C) {
	services := []gwacl.HostedServiceDescriptor{{ServiceName: "deployment-1"}, {ServiceName: "deployment-2"}}
	patchWithServiceListResponse(c, services)
	env := makeEnviron(c)
	instances, err := env.Instances([]instance.Id{})
	c.Check(err, Equals, environs.ErrNoInstances)
	c.Check(instances, IsNil)
}

func (suite *environSuite) TestInstancesReturnsErrNoInstancesIfNoInstanceFound(c *C) {
	services := []gwacl.HostedServiceDescriptor{}
	patchWithServiceListResponse(c, services)
	env := makeEnviron(c)
	instances, err := env.Instances([]instance.Id{"deploy-id"})
	c.Check(err, Equals, environs.ErrNoInstances)
	c.Check(instances, IsNil)
}

func (suite *environSuite) TestInstancesReturnsPartialInstancesIfSomeInstancesAreNotFound(c *C) {
	services := []gwacl.HostedServiceDescriptor{{ServiceName: "deployment-1"}, {ServiceName: "deployment-2"}}
	requests := patchWithServiceListResponse(c, services)
	env := makeEnviron(c)
	instances, err := env.Instances([]instance.Id{"deployment-1", "unknown-deployment"})
	c.Assert(err, Equals, environs.ErrPartialInstances)
	c.Check(len(instances), Equals, 1)
	c.Check(instances[0].Id(), Equals, instance.Id("deployment-1"))
	c.Check(len(*requests), Equals, 1)
}

func (*environSuite) TestStorage(c *C) {
	env := makeEnviron(c)
	baseStorage := env.Storage()
	storage, ok := baseStorage.(*azureStorage)
	c.Check(ok, Equals, true)
	c.Assert(storage, NotNil)
	c.Check(storage.storageContext.getContainer(), Equals, env.getContainerName())
	context, err := storage.getStorageContext()
	c.Assert(err, IsNil)
	c.Check(context.Account, Equals, env.ecfg.storageAccountName())
}

func (*environSuite) TestPublicStorage(c *C) {
	env := makeEnviron(c)
	baseStorage := env.PublicStorage()
	storage, ok := baseStorage.(*azureStorage)
	c.Assert(storage, NotNil)
	c.Check(ok, Equals, true)
	c.Check(storage.storageContext.getContainer(), Equals, env.ecfg.publicStorageContainerName())
	context, err := storage.getStorageContext()
	c.Assert(err, IsNil)
	c.Check(context.Account, Equals, env.ecfg.publicStorageAccountName())
	c.Check(context.Key, Equals, "")
}

func (*environSuite) TestPublicStorageReturnsEmptyStorageIfNoInfo(c *C) {
	attrs := makeAzureConfigMap(c)
	attrs["public-storage-container-name"] = ""
	attrs["public-storage-account-name"] = ""
	cfg, err := config.New(attrs)
	c.Assert(err, IsNil)
	env, err := NewEnviron(cfg)
	c.Assert(err, IsNil)
	c.Check(env.PublicStorage(), Equals, environs.EmptyStorage)
}

func (*environSuite) TestQueryStorageAccountKeyGetsKey(c *C) {
	env := makeEnviron(c)
	keysInAzure := gwacl.StorageAccountKeys{Primary: "a-key"}
	azureResponse, err := xml.Marshal(keysInAzure)
	c.Assert(err, IsNil)
	requests := gwacl.PatchManagementAPIResponses([]gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(azureResponse, http.StatusOK, nil),
	})

	returnedKey, err := env.queryStorageAccountKey()
	c.Assert(err, IsNil)

	c.Check(returnedKey, Equals, keysInAzure.Primary)
	c.Assert(*requests, HasLen, 1)
	c.Check((*requests)[0].Method, Equals, "GET")
}

func (*environSuite) TestGetStorageContextCreatesStorageContext(c *C) {
	env := makeEnviron(c)
	storage, err := env.getStorageContext()
	c.Assert(err, IsNil)
	c.Assert(storage, NotNil)
	c.Check(storage.Account, Equals, env.ecfg.storageAccountName())
	c.Check(storage.AzureEndpoint, Equals, gwacl.GetEndpoint(env.ecfg.location()))
}

func (*environSuite) TestGetStorageContextUsesKnownStorageAccountKey(c *C) {
	env := makeEnviron(c)
	env.storageAccountKey = "my-key"

	storage, err := env.getStorageContext()
	c.Assert(err, IsNil)

	c.Check(storage.Key, Equals, "my-key")
}

func (*environSuite) TestGetStorageContextQueriesStorageAccountKeyIfNeeded(c *C) {
	env := makeEnviron(c)
	env.storageAccountKey = ""
	keysInAzure := gwacl.StorageAccountKeys{Primary: "my-key"}
	azureResponse, err := xml.Marshal(keysInAzure)
	c.Assert(err, IsNil)
	gwacl.PatchManagementAPIResponses([]gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(azureResponse, http.StatusOK, nil),
	})

	storage, err := env.getStorageContext()
	c.Assert(err, IsNil)

	c.Check(storage.Key, Equals, keysInAzure.Primary)
	c.Check(env.storageAccountKey, Equals, keysInAzure.Primary)
}

func (*environSuite) TestGetStorageContextFailsIfNoKeyAvailable(c *C) {
	env := makeEnviron(c)
	env.storageAccountKey = ""
	azureResponse, err := xml.Marshal(gwacl.StorageAccountKeys{})
	c.Assert(err, IsNil)
	gwacl.PatchManagementAPIResponses([]gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(azureResponse, http.StatusOK, nil),
	})

	_, err = env.getStorageContext()
	c.Assert(err, NotNil)

	c.Check(err, ErrorMatches, "no keys available for storage account")
}

func (*environSuite) TestUpdateStorageAccountKeyGetsFreshKey(c *C) {
	env := makeEnviron(c)
	keysInAzure := gwacl.StorageAccountKeys{Primary: "my-key"}
	azureResponse, err := xml.Marshal(keysInAzure)
	c.Assert(err, IsNil)
	gwacl.PatchManagementAPIResponses([]gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(azureResponse, http.StatusOK, nil),
	})

	key, err := env.updateStorageAccountKey(env.getSnapshot())
	c.Assert(err, IsNil)

	c.Check(key, Equals, keysInAzure.Primary)
	c.Check(env.storageAccountKey, Equals, keysInAzure.Primary)
}

func (*environSuite) TestUpdateStorageAccountKeyReturnsError(c *C) {
	env := makeEnviron(c)
	env.storageAccountKey = ""
	gwacl.PatchManagementAPIResponses([]gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(nil, http.StatusInternalServerError, nil),
	})

	_, err := env.updateStorageAccountKey(env.getSnapshot())
	c.Assert(err, NotNil)

	c.Check(err, ErrorMatches, "cannot obtain storage account keys: GET request failed.*Internal Server Error.*")
	c.Check(env.storageAccountKey, Equals, "")
}

func (*environSuite) TestUpdateStorageAccountKeyDetectsConcurrentUpdate(c *C) {
	env := makeEnviron(c)
	env.storageAccountKey = ""
	keysInAzure := gwacl.StorageAccountKeys{Primary: "my-key"}
	azureResponse, err := xml.Marshal(keysInAzure)
	c.Assert(err, IsNil)
	gwacl.PatchManagementAPIResponses([]gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(azureResponse, http.StatusOK, nil),
	})

	// Here we use a snapshot that's different from the environment, to
	// simulate a concurrent change to the environment.
	_, err = env.updateStorageAccountKey(makeEnviron(c))
	c.Assert(err, NotNil)

	// updateStorageAccountKey detects the change, and refuses to write its
	// outdated information into env.
	c.Check(err, ErrorMatches, "environment was reconfigured")
	c.Check(env.storageAccountKey, Equals, "")
}

func (*environSuite) TestGetPublicStorageContext(c *C) {
	env := makeEnviron(c)
	storage, err := env.getPublicStorageContext()
	c.Assert(err, IsNil)
	c.Assert(storage, NotNil)
	c.Check(storage.Account, Equals, env.ecfg.publicStorageAccountName())
	c.Check(storage.Key, Equals, "")
}

func (*environSuite) TestSetConfigValidates(c *C) {
	env := makeEnviron(c)
	originalCfg := env.ecfg
	attrs := makeAzureConfigMap(c)
	// This config is not valid.  It lacks essential information.
	delete(attrs, "management-subscription-id")
	badCfg, err := config.New(attrs)
	c.Assert(err, IsNil)

	err = env.SetConfig(badCfg)

	// Since the config was not valid, SetConfig returns an error.  It
	// does not update the environment's config either.
	c.Check(err, NotNil)
	c.Check(
		err,
		ErrorMatches,
		"management-subscription-id: expected string, got nothing")
	c.Check(env.ecfg, Equals, originalCfg)
}

func (*environSuite) TestSetConfigUpdatesConfig(c *C) {
	env := makeEnviron(c)
	// We're going to set a new config.  It can be recognized by its
	// unusual default Ubuntu release series: 7.04 Feisty Fawn.
	attrs := makeAzureConfigMap(c)
	attrs["default-series"] = "feisty"
	cfg, err := config.New(attrs)
	c.Assert(err, IsNil)

	err = env.SetConfig(cfg)
	c.Assert(err, IsNil)

	c.Check(env.ecfg.Config.DefaultSeries(), Equals, "feisty")
}

func (*environSuite) TestSetConfigLocksEnviron(c *C) {
	env := makeEnviron(c)
	cfg, err := config.New(makeAzureConfigMap(c))
	c.Assert(err, IsNil)

	testing.TestLockingFunction(&env.Mutex, func() { env.SetConfig(cfg) })
}

func (*environSuite) TestSetConfigWillNotUpdateName(c *C) {
	// Once the environment's name has been set, it cannot be updated.
	// Global validation rejects such a change.
	// This matters because the attribute is not protected by a lock.
	env := makeEnviron(c)
	originalName := env.Name()
	attrs := makeAzureConfigMap(c)
	attrs["name"] = "new-name"
	cfg, err := config.New(attrs)
	c.Assert(err, IsNil)

	err = env.SetConfig(cfg)

	c.Assert(err, NotNil)
	c.Check(
		err,
		ErrorMatches,
		`cannot change name from ".*" to "new-name"`)
	c.Check(env.Name(), Equals, originalName)
}

func (*environSuite) TestSetConfigClearsStorageAccountKey(c *C) {
	env := makeEnviron(c)
	env.storageAccountKey = "key-for-previous-config"
	attrs := makeAzureConfigMap(c)
	attrs["default-series"] = "other"
	cfg, err := config.New(attrs)
	c.Assert(err, IsNil)

	err = env.SetConfig(cfg)
	c.Assert(err, IsNil)

	c.Check(env.storageAccountKey, Equals, "")
}

func (*environSuite) TestStateInfoFailsIfNoStateInstances(c *C) {
	env := makeEnviron(c)
	cleanup := setDummyStorage(c, env)
	defer cleanup()
	_, _, err := env.StateInfo()
	c.Check(errors.IsNotFoundError(err), Equals, true)
}

func (*environSuite) TestStateInfo(c *C) {
	instanceID := "my-instance"
	patchWithServiceListResponse(c, []gwacl.HostedServiceDescriptor{{
		ServiceName: instanceID,
	}})
	env := makeEnviron(c)
	cleanup := setDummyStorage(c, env)
	defer cleanup()
	err := environs.SaveState(
		env.Storage(),
		&environs.BootstrapState{StateInstances: []instance.Id{instance.Id(instanceID)}})
	c.Assert(err, IsNil)

	stateInfo, apiInfo, err := env.StateInfo()
	c.Assert(err, IsNil)

	config := env.Config()
	dnsName := "my-instance." + AZURE_DOMAIN_NAME
	stateServerAddr := fmt.Sprintf("%s:%d", dnsName, config.StatePort())
	apiServerAddr := fmt.Sprintf("%s:%d", dnsName, config.APIPort())
	c.Check(stateInfo.Addrs, DeepEquals, []string{stateServerAddr})
	c.Check(apiInfo.Addrs, DeepEquals, []string{apiServerAddr})
}

// parseCreateServiceRequest reconstructs the original CreateHostedService
// request object passed to gwacl's AddHostedService method, based on the
// X509Request which the method issues.
func parseCreateServiceRequest(c *C, request *gwacl.X509Request) *gwacl.CreateHostedService {
	body := gwacl.CreateHostedService{}
	err := xml.Unmarshal(request.Payload, &body)
	c.Assert(err, IsNil)
	return &body
}

// makeServiceNameAlreadyTakenError simulates the AzureError you get when
// trying to create a hosted service with a name that's already taken.
func makeServiceNameAlreadyTakenError(c *C) []byte {
	// At the time of writing, this is the exact kind of error that Azure
	// returns in this situation.
	errorBody, err := xml.Marshal(gwacl.AzureError{
		error:      fmt.Errorf("POST request failed"),
		HTTPStatus: http.StatusConflict,
		Code:       "ConflictError",
		Message:    "The specified DNS name is already taken.",
	})
	c.Assert(err, IsNil)
	return errorBody
}

// makeNonAvailabilityResponse simulates a reply to the
// CheckHostedServiceNameAvailability call saying that a name is not available.
func makeNonAvailabilityResponse(c *C) []byte {
	errorBody, err := xml.Marshal(gwacl.AvailabilityResponse{
		Result: "false",
		Reason: "he's a very naughty boy"})
	c.Assert(err, IsNil)
	return errorBody
}

// makeAvailabilityResponse simulates a reply to the
// CheckHostedServiceNameAvailability call saying that a name is available.
func makeAvailabilityResponse(c *C) []byte {
	errorBody, err := xml.Marshal(gwacl.AvailabilityResponse{
		Result: "true"})
	c.Assert(err, IsNil)
	return errorBody
}

func (*environSuite) TestAttemptCreateServiceCreatesService(c *C) {
	prefix := "myservice"
	affinityGroup := "affinity-group"
	location := "location"
	responses := []gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(makeAvailabilityResponse(c), http.StatusOK, nil),
		gwacl.NewDispatcherResponse(nil, http.StatusOK, nil),
	}
	requests := gwacl.PatchManagementAPIResponses(responses)
	azure, err := gwacl.NewManagementAPI("subscription", "", "West US")
	c.Assert(err, IsNil)

	service, err := attemptCreateService(azure, prefix, affinityGroup, location)
	c.Assert(err, IsNil)

	c.Assert(*requests, HasLen, 2)
	body := parseCreateServiceRequest(c, (*requests)[1])
	c.Check(body.ServiceName, Equals, service.ServiceName)
	c.Check(body.AffinityGroup, Equals, affinityGroup)
	c.Check(service.ServiceName, Matches, prefix+".*")
	c.Check(service.Location, Equals, location)

	label, err := base64.StdEncoding.DecodeString(service.Label)
	c.Assert(err, IsNil)
	c.Check(string(label), Equals, service.ServiceName)
}

func (*environSuite) TestAttemptCreateServiceReturnsNilIfNameNotUnique(c *C) {
	responses := []gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(makeNonAvailabilityResponse(c), http.StatusOK, nil),
	}
	gwacl.PatchManagementAPIResponses(responses)
	azure, err := gwacl.NewManagementAPI("subscription", "", "West US")
	c.Assert(err, IsNil)

	service, err := attemptCreateService(azure, "service", "affinity-group", "location")
	c.Check(err, IsNil)
	c.Check(service, IsNil)
}

func (*environSuite) TestAttemptCreateServicePropagatesOtherFailure(c *C) {
	responses := []gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(makeAvailabilityResponse(c), http.StatusOK, nil),
		gwacl.NewDispatcherResponse(nil, http.StatusNotFound, nil),
	}
	gwacl.PatchManagementAPIResponses(responses)
	azure, err := gwacl.NewManagementAPI("subscription", "", "West US")
	c.Assert(err, IsNil)

	_, err = attemptCreateService(azure, "service", "affinity-group", "location")
	c.Assert(err, NotNil)
	c.Check(err, ErrorMatches, ".*Not Found.*")
}

func (*environSuite) TestNewHostedServiceCreatesService(c *C) {
	prefix := "myservice"
	affinityGroup := "affinity-group"
	location := "location"
	responses := []gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(makeAvailabilityResponse(c), http.StatusOK, nil),
		gwacl.NewDispatcherResponse(nil, http.StatusOK, nil),
	}
	requests := gwacl.PatchManagementAPIResponses(responses)
	azure, err := gwacl.NewManagementAPI("subscription", "", "West US")
	c.Assert(err, IsNil)

	service, err := newHostedService(azure, prefix, affinityGroup, location)
	c.Assert(err, IsNil)

	c.Assert(*requests, HasLen, 2)
	body := parseCreateServiceRequest(c, (*requests)[1])
	c.Check(body.ServiceName, Equals, service.ServiceName)
	c.Check(body.AffinityGroup, Equals, affinityGroup)
	c.Check(service.ServiceName, Matches, prefix+".*")
	c.Check(service.Location, Equals, location)
}

func (*environSuite) TestNewHostedServiceRetriesIfNotUnique(c *C) {
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
	c.Assert(err, IsNil)

	service, err := newHostedService(azure, "service", "affinity-group", "location")
	c.Check(err, IsNil)

	c.Assert(*requests, HasLen, 4)
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
	c.Check(attemptedNames, HasLen, 3)

	// Once newHostedService succeeds, we get a hosted service with the
	// last requested name.
	c.Check(
		service.ServiceName,
		Equals,
		parseCreateServiceRequest(c, (*requests)[3]).ServiceName)
}

func (*environSuite) TestNewHostedServiceFailsIfUnableToFindUniqueName(c *C) {
	errorBody := makeNonAvailabilityResponse(c)
	responses := []gwacl.DispatcherResponse{}
	for counter := 0; counter < 100; counter++ {
		responses = append(responses, gwacl.NewDispatcherResponse(errorBody, http.StatusOK, nil))
	}
	gwacl.PatchManagementAPIResponses(responses)
	azure, err := gwacl.NewManagementAPI("subscription", "", "West US")
	c.Assert(err, IsNil)

	_, err = newHostedService(azure, "service", "affinity-group", "location")
	c.Assert(err, NotNil)
	c.Check(err, ErrorMatches, "could not come up with a unique hosted service name.*")
}

// buildDestroyAzureServiceResponses returns a slice containing the responses that a fake Azure server
// can use to simulate the deletion of the given list of services.
func buildDestroyAzureServiceResponses(c *C, services []*gwacl.HostedService) []gwacl.DispatcherResponse {
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
		c.Assert(err, IsNil)
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

func (*environSuite) TestStopInstancesDestroysMachines(c *C) {
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
	c.Check(err, IsNil)

	// It takes 2 API calls to delete each service:
	// - one GET request to fetch the service's properties;
	// - one DELETE request to delete the service.
	c.Check(len(*requests), Equals, len(services)*2)
	c.Check((*requests)[0].Method, Equals, "GET")
	c.Check((*requests)[1].Method, Equals, "DELETE")
	c.Check((*requests)[2].Method, Equals, "GET")
	c.Check((*requests)[3].Method, Equals, "DELETE")
}

// getVnetAndAffinityGroupCleanupResponses returns the responses
// (gwacl.DispatcherResponse) that a fake http server should return
// when gwacl's RemoveVirtualNetworkSite() and DeleteAffinityGroup()
// are called.
func getVnetAndAffinityGroupCleanupResponses(c *C) []gwacl.DispatcherResponse {
	existingConfig := &gwacl.NetworkConfiguration{
		XMLNS:               gwacl.XMLNS_NC,
		VirtualNetworkSites: nil,
	}
	body, err := existingConfig.Serialize()
	c.Assert(err, IsNil)
	cleanupResponses := []gwacl.DispatcherResponse{
		// Return empty net configuration.
		gwacl.NewDispatcherResponse([]byte(body), http.StatusOK, nil),
		// Accept deletion of affinity group.
		gwacl.NewDispatcherResponse(nil, http.StatusOK, nil),
	}
	return cleanupResponses
}

func (*environSuite) TestDestroyCleansUpStorage(c *C) {
	env := makeEnviron(c)
	cleanup := setDummyStorage(c, env)
	defer cleanup()
	services := []gwacl.HostedServiceDescriptor{}
	responses := getAzureServiceListResponse(c, services)
	cleanupResponses := getVnetAndAffinityGroupCleanupResponses(c)
	responses = append(responses, cleanupResponses...)
	gwacl.PatchManagementAPIResponses(responses)
	instances := convertToInstances([]gwacl.HostedServiceDescriptor{}, env)

	err := env.Destroy(instances)
	c.Check(err, IsNil)

	files, err := env.Storage().List("")
	c.Assert(err, IsNil)
	c.Check(files, HasLen, 0)
}

func (*environSuite) TestDestroyDeletesVirtualNetworkAndAffinityGroup(c *C) {
	env := makeEnviron(c)
	cleanup := setDummyStorage(c, env)
	defer cleanup()
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
	c.Assert(err, IsNil)
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
	instances := convertToInstances([]gwacl.HostedServiceDescriptor{}, env)

	err = env.Destroy(instances)
	c.Check(err, IsNil)

	c.Assert(*requests, HasLen, 4)
	// One request to get the network configuration.
	getRequest := (*requests)[1]
	c.Check(getRequest.Method, Equals, "GET")
	c.Check(strings.HasSuffix(getRequest.URL, "services/networking/media"), Equals, true)
	// One request to upload the new version of the network configuration.
	putRequest := (*requests)[2]
	c.Check(putRequest.Method, Equals, "PUT")
	c.Check(strings.HasSuffix(putRequest.URL, "services/networking/media"), Equals, true)
	// One request to delete the Affinity Group.
	agRequest := (*requests)[3]
	c.Check(strings.Contains(agRequest.URL, env.getAffinityGroupName()), IsTrue)
	c.Check(agRequest.Method, Equals, "DELETE")

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

func (*environSuite) TestDestroyStopsAllInstances(c *C) {
	env := makeEnviron(c)
	cleanup := setDummyStorage(c, env)
	defer cleanup()

	// Simulate 2 instances corresponding to two Azure services.
	prefix := env.getEnvPrefix()
	service1Name := prefix + "service1"
	service2Name := prefix + "service2"
	service1, service1Desc := makeAzureService(service1Name)
	service2, service2Desc := makeAzureService(service2Name)
	services := []*gwacl.HostedService{service1, service2}
	// The call to AllInstances() will return only one service (service1).
	listInstancesResponses := getAzureServiceListResponse(c, []gwacl.HostedServiceDescriptor{*service1Desc})
	destroyResponses := buildDestroyAzureServiceResponses(c, services)
	responses := append(listInstancesResponses, destroyResponses...)
	cleanupResponses := getVnetAndAffinityGroupCleanupResponses(c)
	responses = append(responses, cleanupResponses...)
	requests := gwacl.PatchManagementAPIResponses(responses)

	// Call Destroy with service1 and service2.
	instances := convertToInstances(
		[]gwacl.HostedServiceDescriptor{*service1Desc, *service2Desc},
		env)
	err := env.Destroy(instances)
	c.Check(err, IsNil)

	// One request to get the list of all the environment's instances.
	// Then two requests per destroyed machine (one to fetch the
	// service's information, one to delete it) and two requests to delete
	// the Virtual Network and the Affinity Group.
	c.Check((*requests), HasLen, 1+len(services)*2+2)
	c.Check((*requests)[0].Method, Equals, "GET")
	c.Check((*requests)[1].Method, Equals, "GET")
	c.Check(strings.Contains((*requests)[1].URL, service1Name), IsTrue)
	c.Check((*requests)[2].Method, Equals, "DELETE")
	c.Check(strings.Contains((*requests)[2].URL, service1Name), IsTrue)
	c.Check((*requests)[3].Method, Equals, "GET")
	c.Check(strings.Contains((*requests)[3].URL, service2Name), IsTrue)
	c.Check((*requests)[4].Method, Equals, "DELETE")
	c.Check(strings.Contains((*requests)[4].URL, service2Name), IsTrue)
}

func (*environSuite) TestGetInstance(c *C) {
	env := makeEnviron(c)
	prefix := env.getEnvPrefix()
	serviceName := prefix + "instance-name"
	serviceDesc := gwacl.HostedServiceDescriptor{ServiceName: serviceName}
	service := gwacl.HostedService{HostedServiceDescriptor: serviceDesc}
	responses := getAzureServiceResponses(c, service)
	gwacl.PatchManagementAPIResponses(responses)

	instance, err := env.getInstance("serviceName")
	c.Check(err, IsNil)

	c.Check(string(instance.Id()), Equals, serviceName)
	c.Check(instance, FitsTypeOf, &azureInstance{})
	azInstance := instance.(*azureInstance)
	c.Check(azInstance.environ, Equals, env)
}

func (*environSuite) TestNewOSVirtualDisk(c *C) {
	env := makeEnviron(c)
	sourceImageName := "source-image-name"

	vhd := env.newOSDisk(sourceImageName)

	mediaLinkUrl, err := url.Parse(vhd.MediaLink)
	c.Check(err, IsNil)
	storageAccount := env.ecfg.storageAccountName()
	c.Check(mediaLinkUrl.Host, Equals, fmt.Sprintf("%s.blob.core.windows.net", storageAccount))
	c.Check(vhd.SourceImageName, Equals, sourceImageName)
}

// mapInputEndpointsByPort takes a slice of input endpoints, and returns them
// as a map keyed by their (external) ports.  This makes it easier to query
// individual endpoints from an array whose ordering you don't know.
// Multiple input endpoints for the same port are treated as an error.
func mapInputEndpointsByPort(c *C, endpoints []gwacl.InputEndpoint) map[int]gwacl.InputEndpoint {
	mapping := make(map[int]gwacl.InputEndpoint)
	for _, endpoint := range endpoints {
		_, have := mapping[endpoint.Port]
		c.Assert(have, Equals, false)
		mapping[endpoint.Port] = endpoint
	}
	return mapping
}

func (*environSuite) TestNewRole(c *C) {
	env := makeEnviron(c)
	size := "Large"
	vhd := env.newOSDisk("source-image-name")
	userData := "example-user-data"
	hostname := "hostname"

	role := env.newRole(size, vhd, userData, hostname)

	configs := role.ConfigurationSets
	linuxConfig := configs[0]
	networkConfig := configs[1]
	c.Check(linuxConfig.CustomData, Equals, userData)
	c.Check(linuxConfig.Hostname, Equals, hostname)
	c.Check(linuxConfig.Username, Not(Equals), "")
	c.Check(linuxConfig.Password, Not(Equals), "")
	c.Check(linuxConfig.DisableSSHPasswordAuthentication, Equals, "true")
	c.Check(role.RoleSize, Equals, size)
	c.Check(role.OSVirtualHardDisk[0], Equals, *vhd)

	endpoints := mapInputEndpointsByPort(c, *networkConfig.InputEndpoints)

	// The network config contains an endpoint for ssh communication.
	sshEndpoint, ok := endpoints[22]
	c.Assert(ok, Equals, true)
	c.Check(sshEndpoint.LocalPort, Equals, 22)
	c.Check(sshEndpoint.Protocol, Equals, "TCP")

	// There's also an endpoint for the state (mongodb) port.
	// TODO: Ought to have this only for state servers.
	stateEndpoint, ok := endpoints[env.Config().StatePort()]
	c.Assert(ok, Equals, true)
	c.Check(stateEndpoint.LocalPort, Equals, env.Config().StatePort())
	c.Check(stateEndpoint.Protocol, Equals, "TCP")

	// And one for the API port.
	// TODO: Ought to have this only for API servers.
	apiEndpoint, ok := endpoints[env.Config().APIPort()]
	c.Assert(ok, Equals, true)
	c.Check(apiEndpoint.LocalPort, Equals, env.Config().APIPort())
	c.Check(apiEndpoint.Protocol, Equals, "TCP")
}

func (*environSuite) TestNewDeployment(c *C) {
	env := makeEnviron(c)
	deploymentName := "deployment-name"
	deploymentLabel := "deployment-label"
	virtualNetworkName := "virtual-network-name"
	vhd := env.newOSDisk("source-image-name")
	role := env.newRole("Small", vhd, "user-data", "hostname")

	deployment := env.newDeployment(role, deploymentName, deploymentLabel, virtualNetworkName)

	base64Label := base64.StdEncoding.EncodeToString([]byte(deploymentLabel))
	c.Check(deployment.Label, Equals, base64Label)
	c.Check(deployment.Name, Equals, deploymentName)
	c.Check(deployment.RoleList, HasLen, 1)
}

func (*environSuite) TestProviderReturnsAzureEnvironProvider(c *C) {
	prov := makeEnviron(c).Provider()
	c.Assert(prov, NotNil)
	azprov, ok := prov.(azureEnvironProvider)
	c.Assert(ok, Equals, true)
	c.Check(azprov, NotNil)
}

func (*environSuite) TestCreateVirtualNetwork(c *C) {
	env := makeEnviron(c)
	responses := []gwacl.DispatcherResponse{
		// No existing configuration found.
		gwacl.NewDispatcherResponse(nil, http.StatusNotFound, nil),
		// Accept upload of new configuration.
		gwacl.NewDispatcherResponse(nil, http.StatusOK, nil),
	}
	requests := gwacl.PatchManagementAPIResponses(responses)

	env.createVirtualNetwork()

	c.Assert(*requests, HasLen, 2)
	request := (*requests)[1]
	body := gwacl.NetworkConfiguration{}
	err := xml.Unmarshal(request.Payload, &body)
	c.Assert(err, IsNil)
	networkConf := (*body.VirtualNetworkSites)[0]
	c.Check(networkConf.Name, Equals, env.getVirtualNetworkName())
	c.Check(networkConf.AffinityGroup, Equals, env.getAffinityGroupName())
}

func (*environSuite) TestDestroyVirtualNetwork(c *C) {
	env := makeEnviron(c)
	// Prepare a configuration with a single virtual network.
	existingConfig := &gwacl.NetworkConfiguration{
		XMLNS: gwacl.XMLNS_NC,
		VirtualNetworkSites: &[]gwacl.VirtualNetworkSite{
			{Name: env.getVirtualNetworkName()},
		},
	}
	body, err := existingConfig.Serialize()
	c.Assert(err, IsNil)
	responses := []gwacl.DispatcherResponse{
		// Return existing configuration.
		gwacl.NewDispatcherResponse([]byte(body), http.StatusOK, nil),
		// Accept upload of new configuration.
		gwacl.NewDispatcherResponse(nil, http.StatusOK, nil),
	}
	requests := gwacl.PatchManagementAPIResponses(responses)

	env.deleteVirtualNetwork()

	c.Assert(*requests, HasLen, 2)
	// One request to get the existing network configuration.
	getRequest := (*requests)[0]
	c.Check(getRequest.Method, Equals, "GET")
	// One request to update the network configuration.
	putRequest := (*requests)[1]
	c.Check(putRequest.Method, Equals, "PUT")
	newConfig := gwacl.NetworkConfiguration{}
	err = xml.Unmarshal(putRequest.Payload, &newConfig)
	c.Assert(err, IsNil)
	// The new configuration has no VirtualNetworkSites.
	c.Check(newConfig.VirtualNetworkSites, IsNil)
}

func (*environSuite) TestGetVirtualNetworkNameContainsEnvName(c *C) {
	env := makeEnviron(c)
	c.Check(strings.Contains(env.getVirtualNetworkName(), env.Name()), IsTrue)
}

func (*environSuite) TestGetVirtualNetworkNameIsConstant(c *C) {
	env := makeEnviron(c)
	c.Check(env.getVirtualNetworkName(), Equals, env.getVirtualNetworkName())
}

func (*environSuite) TestCreateAffinityGroup(c *C) {
	env := makeEnviron(c)
	responses := []gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(nil, http.StatusCreated, nil),
	}
	requests := gwacl.PatchManagementAPIResponses(responses)

	env.createAffinityGroup()

	c.Assert(*requests, HasLen, 1)
	request := (*requests)[0]
	body := gwacl.CreateAffinityGroup{}
	err := xml.Unmarshal(request.Payload, &body)
	c.Assert(err, IsNil)
	c.Check(body.Name, Equals, env.getAffinityGroupName())
	// This is a testing antipattern, the expected data comes from
	// config defaults.  Fix it sometime.
	c.Check(body.Location, Equals, "location")
}

func (*environSuite) TestDestroyAffinityGroup(c *C) {
	env := makeEnviron(c)
	responses := []gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(nil, http.StatusOK, nil),
	}
	requests := gwacl.PatchManagementAPIResponses(responses)

	env.deleteAffinityGroup()

	c.Assert(*requests, HasLen, 1)
	request := (*requests)[0]
	c.Check(strings.Contains(request.URL, env.getAffinityGroupName()), IsTrue)
	c.Check(request.Method, Equals, "DELETE")
}

func (*environSuite) TestGetAffinityGroupName(c *C) {
	env := makeEnviron(c)
	c.Check(strings.Contains(env.getAffinityGroupName(), env.Name()), IsTrue)
}

func (*environSuite) TestGetAffinityGroupNameIsConstant(c *C) {
	env := makeEnviron(c)
	c.Check(env.getAffinityGroupName(), Equals, env.getAffinityGroupName())
}

func (*environSuite) TestGetImageBaseURLs(c *C) {
	env := makeEnviron(c)
	urls, err := env.getImageBaseURLs()
	c.Assert(err, IsNil)
	// At the moment this is not configurable.  It returns a fixed URL for
	// the central simplestreams database.
	c.Check(urls, DeepEquals, []string{imagemetadata.DefaultBaseURL})
}

func (*environSuite) TestGetImageStreamDefaultsToBlank(c *C) {
	env := makeEnviron(c)
	// Hard-coded to default for now.
	c.Check(env.getImageStream(), Equals, "")
}

func (*environSuite) TestGetImageMetadataSigningRequiredDefaultsToTrue(c *C) {
	env := makeEnviron(c)
	// Hard-coded to true for now.  Once we support other base URLs, this
	// may have to become configurable.
	c.Check(env.getImageMetadataSigningRequired(), Equals, true)
}

func (*environSuite) TestSelectInstanceTypeAndImageUsesForcedImage(c *C) {
	env := makeEnviron(c)
	forcedImage := "my-image"
	env.ecfg.attrs["force-image-name"] = forcedImage

	// We'll tailor our constraints so as to get a specific instance type.
	aim := gwacl.RoleNameMap["ExtraLarge"]
	cons := constraints.Value{
		CpuCores: &aim.CpuCores,
		Mem:      &aim.Mem,
	}

	instanceType, image, err := env.selectInstanceTypeAndImage(cons, "precise", "West US")
	c.Assert(err, IsNil)

	c.Check(instanceType, Equals, aim.Name)
	c.Check(image, Equals, forcedImage)
}

func (*environSuite) TestSelectInstanceTypeAndImageUsesSimplestreamsByDefault(c *C) {
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
	c.Assert(err, IsNil)

	c.Check(instanceType, Equals, aim.Name)
	c.Check(image, Equals, "image")
}

func (*environSuite) TestConvertToInstances(c *C) {
	services := []gwacl.HostedServiceDescriptor{
		{ServiceName: "foo"}, {ServiceName: "bar"},
	}
	env := makeEnviron(c)
	instances := convertToInstances(services, env)
	c.Check(instances, DeepEquals, []instance.Instance{
		&azureInstance{services[0], env},
		&azureInstance{services[1], env},
	})
}

func (*environSuite) TestExtractStorageKeyPicksPrimaryKeyIfSet(c *C) {
	keys := gwacl.StorageAccountKeys{
		Primary:   "mainkey",
		Secondary: "otherkey",
	}
	c.Check(extractStorageKey(&keys), Equals, "mainkey")
}

func (*environSuite) TestExtractStorageKeyFallsBackToSecondaryKey(c *C) {
	keys := gwacl.StorageAccountKeys{
		Secondary: "sparekey",
	}
	c.Check(extractStorageKey(&keys), Equals, "sparekey")
}

func (*environSuite) TestExtractStorageKeyReturnsBlankIfNoneSet(c *C) {
	c.Check(extractStorageKey(&gwacl.StorageAccountKeys{}), Equals, "")
}
