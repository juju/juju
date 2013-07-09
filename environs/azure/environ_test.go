// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/gwacl"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/localstorage"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/testing"
	. "launchpad.net/juju-core/testing/checkers"
	"net/http"
	"strings"
	"sync"
)

type EnvironSuite struct {
	ProviderSuite
}

var _ = Suite(new(EnvironSuite))

func makeEnviron(c *C) *azureEnviron {
	attrs := makeAzureConfigMap(c)
	cfg, err := config.New(attrs)
	c.Assert(err, IsNil)
	env, err := NewEnviron(cfg)
	c.Assert(err, IsNil)
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

func (EnvironSuite) TestGetSnapshot(c *C) {
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

func (EnvironSuite) TestGetSnapshotLocksEnviron(c *C) {
	original := azureEnviron{}
	testing.TestLockingFunction(&original.Mutex, func() { original.getSnapshot() })
}

func (EnvironSuite) TestName(c *C) {
	env := azureEnviron{name: "foo"}
	c.Check(env.Name(), Equals, env.name)
}

func (EnvironSuite) TestConfigReturnsConfig(c *C) {
	cfg := new(config.Config)
	ecfg := azureEnvironConfig{Config: cfg}
	env := azureEnviron{ecfg: &ecfg}
	c.Check(env.Config(), Equals, cfg)
}

func (EnvironSuite) TestConfigLocksEnviron(c *C) {
	env := azureEnviron{name: "env", ecfg: new(azureEnvironConfig)}
	testing.TestLockingFunction(&env.Mutex, func() { env.Config() })
}

func (EnvironSuite) TestGetManagementAPI(c *C) {
	env := makeEnviron(c)
	context, err := env.getManagementAPI()
	c.Assert(err, IsNil)
	defer env.releaseManagementAPI(context)
	c.Check(context, NotNil)
	c.Check(context.ManagementAPI, NotNil)
	c.Check(context.certFile, NotNil)
}

func (EnvironSuite) TestReleaseManagementAPIAcceptsNil(c *C) {
	env := makeEnviron(c)
	env.releaseManagementAPI(nil)
	// The real test is that this does not panic.
}

func (EnvironSuite) TestReleaseManagementAPIAcceptsIncompleteContext(c *C) {
	env := makeEnviron(c)
	context := azureManagementContext{
		ManagementAPI: nil,
		certFile:      nil,
	}
	env.releaseManagementAPI(&context)
	// The real test is that this does not panic.
}

func buildServiceListResponse(c *C, services []gwacl.HostedServiceDescriptor) []gwacl.DispatcherResponse {
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

func patchWithServiceListResponse(c *C, services []gwacl.HostedServiceDescriptor) *[]*gwacl.X509Request {
	responses := buildServiceListResponse(c, services)
	return gwacl.PatchManagementAPIResponses(responses)
}

func (suite EnvironSuite) TestGetEnvPrefixContainsEnvName(c *C) {
	env := makeEnviron(c)
	c.Check(strings.Contains(env.getEnvPrefix(), env.Name()), IsTrue)
}

func (suite EnvironSuite) TestAllInstances(c *C) {
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

func (suite EnvironSuite) TestInstancesReturnsFilteredList(c *C) {
	services := []gwacl.HostedServiceDescriptor{{ServiceName: "deployment-1"}, {ServiceName: "deployment-2"}}
	requests := patchWithServiceListResponse(c, services)
	env := makeEnviron(c)
	instances, err := env.Instances([]instance.Id{"deployment-1"})
	c.Assert(err, IsNil)
	c.Check(len(instances), Equals, 1)
	c.Check(instances[0].Id(), Equals, instance.Id("deployment-1"))
	c.Check(len(*requests), Equals, 1)
}

func (suite EnvironSuite) TestInstancesReturnsErrNoInstancesIfNoInstancesRequested(c *C) {
	services := []gwacl.HostedServiceDescriptor{{ServiceName: "deployment-1"}, {ServiceName: "deployment-2"}}
	patchWithServiceListResponse(c, services)
	env := makeEnviron(c)
	instances, err := env.Instances([]instance.Id{})
	c.Check(err, Equals, environs.ErrNoInstances)
	c.Check(instances, IsNil)
}

func (suite EnvironSuite) TestInstancesReturnsErrNoInstancesIfNoInstanceFound(c *C) {
	services := []gwacl.HostedServiceDescriptor{}
	patchWithServiceListResponse(c, services)
	env := makeEnviron(c)
	instances, err := env.Instances([]instance.Id{"deploy-id"})
	c.Check(err, Equals, environs.ErrNoInstances)
	c.Check(instances, IsNil)
}

func (suite EnvironSuite) TestInstancesReturnsPartialInstancesIfSomeInstancesAreNotFound(c *C) {
	services := []gwacl.HostedServiceDescriptor{{ServiceName: "deployment-1"}, {ServiceName: "deployment-2"}}
	requests := patchWithServiceListResponse(c, services)
	env := makeEnviron(c)
	instances, err := env.Instances([]instance.Id{"deployment-1", "unknown-deployment"})
	c.Assert(err, Equals, environs.ErrPartialInstances)
	c.Check(len(instances), Equals, 1)
	c.Check(instances[0].Id(), Equals, instance.Id("deployment-1"))
	c.Check(len(*requests), Equals, 1)
}

func (EnvironSuite) TestStorage(c *C) {
	env := makeEnviron(c)
	baseStorage := env.Storage()
	storage, ok := baseStorage.(*azureStorage)
	c.Check(ok, Equals, true)
	c.Assert(storage, NotNil)
	c.Check(storage.storageContext.getContainer(), Equals, env.ecfg.StorageContainerName())
	context, err := storage.getStorageContext()
	c.Assert(err, IsNil)
	c.Check(context.Account, Equals, env.ecfg.StorageAccountName())
	c.Check(context.Key, Equals, env.ecfg.StorageAccountKey())
}

func (EnvironSuite) TestPublicStorage(c *C) {
	env := makeEnviron(c)
	baseStorage := env.PublicStorage()
	storage, ok := baseStorage.(*azureStorage)
	c.Assert(storage, NotNil)
	c.Check(ok, Equals, true)
	c.Check(storage.storageContext.getContainer(), Equals, env.ecfg.PublicStorageContainerName())
	context, err := storage.getStorageContext()
	c.Assert(err, IsNil)
	c.Check(context.Account, Equals, env.ecfg.PublicStorageAccountName())
	c.Check(context.Key, Equals, "")
}

func (EnvironSuite) TestPublicStorageReturnsEmptyStorageIfNoInfo(c *C) {
	attrs := makeAzureConfigMap(c)
	attrs["public-storage-container-name"] = ""
	attrs["public-storage-account-name"] = ""
	cfg, err := config.New(attrs)
	c.Assert(err, IsNil)
	env, err := NewEnviron(cfg)
	c.Assert(err, IsNil)
	c.Check(env.PublicStorage(), Equals, environs.EmptyStorage)
}

func (EnvironSuite) TestGetStorageContext(c *C) {
	env := makeEnviron(c)
	storage, err := env.getStorageContext()
	c.Assert(err, IsNil)
	c.Assert(storage, NotNil)
	c.Check(storage.Account, Equals, env.ecfg.StorageAccountName())
	c.Check(storage.Key, Equals, env.ecfg.StorageAccountKey())
}

func (EnvironSuite) TestGetPublicStorageContext(c *C) {
	env := makeEnviron(c)
	storage, err := env.getPublicStorageContext()
	c.Assert(err, IsNil)
	c.Assert(storage, NotNil)
	c.Check(storage.Account, Equals, env.ecfg.PublicStorageAccountName())
	c.Check(storage.Key, Equals, "")
}

func (EnvironSuite) TestSetConfigValidates(c *C) {
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

func (EnvironSuite) TestSetConfigUpdatesConfig(c *C) {
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

func (EnvironSuite) TestSetConfigLocksEnviron(c *C) {
	env := makeEnviron(c)
	cfg, err := config.New(makeAzureConfigMap(c))
	c.Assert(err, IsNil)

	testing.TestLockingFunction(&env.Mutex, func() { env.SetConfig(cfg) })
}

func (EnvironSuite) TestSetConfigWillNotUpdateName(c *C) {
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

func (EnvironSuite) TestStateInfoFailsIfNoStateInstances(c *C) {
	env := makeEnviron(c)
	cleanup := setDummyStorage(c, env)
	defer cleanup()
	_, _, err := env.StateInfo()
	c.Check(errors.IsNotFoundError(err), Equals, true)
}

func (EnvironSuite) TestStateInfo(c *C) {
	instanceID := "my-instance"
	label := "my-label"
	// In the Azure provider, the hostname of the instance is the
	// service's label.
	expectedDNSName := fmt.Sprintf("%s.%s", label, AZURE_DOMAIN_NAME)
	encodedLabel := base64.StdEncoding.EncodeToString([]byte(label))
	patchWithServiceListResponse(c, []gwacl.HostedServiceDescriptor{{
		ServiceName: instanceID,
		Label:       encodedLabel,
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
	statePortSuffix := fmt.Sprintf(":%d", config.StatePort())
	apiPortSuffix := fmt.Sprintf(":%d", config.APIPort())
	c.Check(stateInfo.Addrs, DeepEquals, []string{expectedDNSName + statePortSuffix})
	c.Check(apiInfo.Addrs, DeepEquals, []string{expectedDNSName + apiPortSuffix})
}

func buildDestroyServiceResponses(c *C, services []*gwacl.HostedService) []gwacl.DispatcherResponse {
	responses := []gwacl.DispatcherResponse{}
	for _, service := range services {
		// When destroying a hosted service, gwacl first issues a Get reques
		// to fetch the properties of the services.  Then it destroys all the
		// deployments found in this service (none in this case, we make sure
		// the service does not contain deployments to keep the testing simple)
		// And it finally deletes the service itself.
		if len(service.Deployments) != 0 {
			panic("buildDestroyServiceResponses does not support services with deployments!")
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
			[]byte(""),
			http.StatusOK,
			nil,
		)
		responses = append(responses, serviceDeleteResponse)
	}
	return responses
	//	return requests
}

func makeService(name string) (*gwacl.HostedService, *gwacl.HostedServiceDescriptor) {
	service1 := &gwacl.HostedService{ServiceName: name}
	service1Desc := &gwacl.HostedServiceDescriptor{ServiceName: name}
	return service1, service1Desc
}

func (EnvironSuite) TestStopInstancesDestroysMachines(c *C) {
	service1Name := "service1"
	service1, service1Desc := makeService(service1Name)
	service2Name := "service2"
	service2, service2Desc := makeService(service2Name)
	services := []*gwacl.HostedService{service1, service2}
	responses := buildDestroyServiceResponses(c, services)
	requests := gwacl.PatchManagementAPIResponses(responses)
	env := makeEnviron(c)
	instances := convertToInstances([]gwacl.HostedServiceDescriptor{*service1Desc, *service2Desc})
	err := env.StopInstances(instances)
	c.Check(err, IsNil)
	// It takes 2 API calls to delete each service:
	// - one GET request to fetch the infos about the service;
	// - one DELETE request to delete the service.
	c.Check(len(*requests), Equals, len(services)*2)
	c.Check((*requests)[0].Method, Equals, "GET")
	c.Check((*requests)[1].Method, Equals, "DELETE")
	c.Check((*requests)[2].Method, Equals, "GET")
	c.Check((*requests)[3].Method, Equals, "DELETE")
}

// makeAzureStorage creates a test azureStorage object that will talk to a
// fake http server set up to always return the given http.Response object.
func makeAzureStorageMocking(transport http.RoundTripper, container string, account string) azureStorage {
	client := &http.Client{Transport: transport}
	storageContext := gwacl.NewTestStorageContext(client)
	storageContext.Account = account
	context := &testStorageContext{container: container, storageContext: storageContext}
	azStorage := azureStorage{context}
	return azStorage
}

var blobListWith2BlobsResponse = `
<?xml version="1.0" encoding="utf-8"?>
<EnumerationResults ContainerName="http://myaccount.blob.core.windows.net/mycontainer">
  <Prefix>prefix</Prefix>
  <Marker>marker</Marker>
  <MaxResults>maxresults</MaxResults>
  <Delimiter>delimiter</Delimiter>
  <Blobs>
    <Blob>
      <Name>blob-1</Name>
      <Url>blob-url1</Url>
    </Blob>
    <Blob>
      <Name>blob-2</Name>
      <Url>blob-url2</Url>
   </Blob>
  </Blobs>
  <NextMarker />
</EnumerationResults>`

func (EnvironSuite) TestDestroyCleansUpStorage(c *C) {
	env := makeEnviron(c)
	container := "container"
	transport := &gwacl.MockingTransport{}
	transport.AddExchange(&http.Response{StatusCode: http.StatusOK, Body: ioutil.NopCloser(strings.NewReader(blobListWith2BlobsResponse))}, nil)
	transport.AddExchange(&http.Response{StatusCode: http.StatusAccepted, Body: nil}, nil)
	transport.AddExchange(&http.Response{StatusCode: http.StatusAccepted, Body: nil}, nil)
	azStorage := makeAzureStorageMocking(transport, container, "account")
	env.storage = &azStorage
	services := []gwacl.HostedServiceDescriptor{}
	patchWithServiceListResponse(c, services)
	instances := convertToInstances([]gwacl.HostedServiceDescriptor{})

	err := env.Destroy(instances)

	c.Check(err, IsNil)
	c.Check(transport.Exchanges[1].Request.Method, Equals, "DELETE")
	c.Check(strings.HasSuffix(transport.Exchanges[1].Request.URL.String(), "blob-1"), IsTrue)
	c.Check(transport.Exchanges[2].Request.Method, Equals, "DELETE")
	c.Check(strings.HasSuffix(transport.Exchanges[2].Request.URL.String(), "blob-2"), IsTrue)
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

func (EnvironSuite) TestDestroyStopsAllInstances(c *C) {
	env := makeEnviron(c)
	// Setup an empty storage.
	container := "container"
	response := makeResponse(emptyListResponse, http.StatusOK)
	azStorage, _ := makeAzureStorage(response, container, "account")
	env.storage = &azStorage
	// Simulate 2 nodes corresponding to two Azure services.
	prefix := env.getEnvPrefix()
	service1, service1Desc := makeService(prefix + "service1")
	service2, service2Desc := makeService(prefix + "service2")
	services := []*gwacl.HostedService{service1, service2}
	// The call to AllInstances() will return only one service (service1).
	listInstancesResponses := buildServiceListResponse(c, []gwacl.HostedServiceDescriptor{*service1Desc})
	destroyResponses := buildDestroyServiceResponses(c, services)
	responses := append(listInstancesResponses, destroyResponses...)
	requests := gwacl.PatchManagementAPIResponses(responses)

	// Call Destroy with service1 and service2.
	instances := convertToInstances([]gwacl.HostedServiceDescriptor{*service1Desc, *service2Desc})
	err := env.Destroy(instances)
	c.Check(err, IsNil)

	// One request to get the list of all the environment's instances.
	// Then two requests per destroyed machine (one to fetch the
	// service's information, one to delete it).
	c.Check((*requests), HasLen, 1+len(services)*2)
	c.Check((*requests)[0].Method, Equals, "GET")
	c.Check((*requests)[1].Method, Equals, "GET")
	c.Check((*requests)[2].Method, Equals, "DELETE")
	c.Check((*requests)[3].Method, Equals, "GET")
	c.Check((*requests)[4].Method, Equals, "DELETE")
}
