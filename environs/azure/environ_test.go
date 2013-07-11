// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	. "launchpad.net/gocheck"
	"launchpad.net/gwacl"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/localstorage"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/testing"
	. "launchpad.net/juju-core/testing/checkers"
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

func (*EnvironSuite) TestGetSnapshot(c *C) {
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

func (*EnvironSuite) TestGetSnapshotLocksEnviron(c *C) {
	original := azureEnviron{}
	testing.TestLockingFunction(&original.Mutex, func() { original.getSnapshot() })
}

func (*EnvironSuite) TestName(c *C) {
	env := azureEnviron{name: "foo"}
	c.Check(env.Name(), Equals, env.name)
}

func (*EnvironSuite) TestConfigReturnsConfig(c *C) {
	cfg := new(config.Config)
	ecfg := azureEnvironConfig{Config: cfg}
	env := azureEnviron{ecfg: &ecfg}
	c.Check(env.Config(), Equals, cfg)
}

func (*EnvironSuite) TestConfigLocksEnviron(c *C) {
	env := azureEnviron{name: "env", ecfg: new(azureEnvironConfig)}
	testing.TestLockingFunction(&env.Mutex, func() { env.Config() })
}

func (*EnvironSuite) TestGetManagementAPI(c *C) {
	env := makeEnviron(c)
	context, err := env.getManagementAPI()
	c.Assert(err, IsNil)
	defer env.releaseManagementAPI(context)
	c.Check(context, NotNil)
	c.Check(context.ManagementAPI, NotNil)
	c.Check(context.certFile, NotNil)
}

func (*EnvironSuite) TestReleaseManagementAPIAcceptsNil(c *C) {
	env := makeEnviron(c)
	env.releaseManagementAPI(nil)
	// The real test is that this does not panic.
}

func (*EnvironSuite) TestReleaseManagementAPIAcceptsIncompleteContext(c *C) {
	env := makeEnviron(c)
	context := azureManagementContext{
		ManagementAPI: nil,
		certFile:      nil,
	}
	env.releaseManagementAPI(&context)
	// The real test is that this does not panic.
}

func buildAzureServiceListResponse(c *C, services []gwacl.HostedServiceDescriptor) []gwacl.DispatcherResponse {
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

// buildAzureServiceResponses returns the slice of responses
// (gwacl.DispatcherResponse) which correspond to the API request used to
// get the properties of a Service.
func buildAzureServiceResponses(c *C, service gwacl.HostedService) []gwacl.DispatcherResponse {
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
	responses := buildAzureServiceListResponse(c, services)
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

func (*EnvironSuite) TestStorage(c *C) {
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

func (*EnvironSuite) TestPublicStorage(c *C) {
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

func (*EnvironSuite) TestPublicStorageReturnsEmptyStorageIfNoInfo(c *C) {
	attrs := makeAzureConfigMap(c)
	attrs["public-storage-container-name"] = ""
	attrs["public-storage-account-name"] = ""
	cfg, err := config.New(attrs)
	c.Assert(err, IsNil)
	env, err := NewEnviron(cfg)
	c.Assert(err, IsNil)
	c.Check(env.PublicStorage(), Equals, environs.EmptyStorage)
}

func (*EnvironSuite) TestGetStorageContext(c *C) {
	env := makeEnviron(c)
	storage, err := env.getStorageContext()
	c.Assert(err, IsNil)
	c.Assert(storage, NotNil)
	c.Check(storage.Account, Equals, env.ecfg.StorageAccountName())
	c.Check(storage.Key, Equals, env.ecfg.StorageAccountKey())
}

func (*EnvironSuite) TestGetPublicStorageContext(c *C) {
	env := makeEnviron(c)
	storage, err := env.getPublicStorageContext()
	c.Assert(err, IsNil)
	c.Assert(storage, NotNil)
	c.Check(storage.Account, Equals, env.ecfg.PublicStorageAccountName())
	c.Check(storage.Key, Equals, "")
}

func (*EnvironSuite) TestSetConfigValidates(c *C) {
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

func (*EnvironSuite) TestSetConfigUpdatesConfig(c *C) {
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

func (*EnvironSuite) TestSetConfigLocksEnviron(c *C) {
	env := makeEnviron(c)
	cfg, err := config.New(makeAzureConfigMap(c))
	c.Assert(err, IsNil)

	testing.TestLockingFunction(&env.Mutex, func() { env.SetConfig(cfg) })
}

func (*EnvironSuite) TestSetConfigWillNotUpdateName(c *C) {
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

func (*EnvironSuite) TestStateInfoFailsIfNoStateInstances(c *C) {
	env := makeEnviron(c)
	cleanup := setDummyStorage(c, env)
	defer cleanup()
	_, _, err := env.StateInfo()
	c.Check(errors.IsNotFoundError(err), Equals, true)
}

func (*EnvironSuite) TestStateInfo(c *C) {
	instanceID := "my-instance"
	label := fmt.Sprintf("my-label.%s", AZURE_DOMAIN_NAME)
	// In the Azure provider, the DNS name of the instance is the
	// service's label (instance==service).
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
	c.Check(stateInfo.Addrs, DeepEquals, []string{label + statePortSuffix})
	c.Check(apiInfo.Addrs, DeepEquals, []string{label + apiPortSuffix})
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

func (*EnvironSuite) TestAttemptCreateServiceCreatesService(c *C) {
	responses := []gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(nil, http.StatusOK, nil),
	}
	requests := gwacl.PatchManagementAPIResponses(responses)
	azure, err := gwacl.NewManagementAPI("subscription", "certfile.pem")
	c.Assert(err, IsNil)

	service, err := attemptCreateService(azure)
	c.Assert(err, IsNil)

	c.Assert(*requests, HasLen, 1)
	body := parseCreateServiceRequest(c, (*requests)[0])
	c.Check(body.ServiceName, Equals, service.ServiceName)
}

func (*EnvironSuite) TestAttemptCreateServiceReturnsNilIfNameNotUnique(c *C) {
	errorBody := makeServiceNameAlreadyTakenError(c)
	responses := []gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(errorBody, http.StatusConflict, nil),
	}
	gwacl.PatchManagementAPIResponses(responses)
	azure, err := gwacl.NewManagementAPI("subscription", "certfile.pem")
	c.Assert(err, IsNil)

	service, err := attemptCreateService(azure)
	c.Check(err, IsNil)
	c.Check(service, IsNil)
}

func (*EnvironSuite) TestAttemptCreateServiceRecognizesChangedConflictError(c *C) {
	// Even if Azure or gwacl makes slight changes to the error they
	// return (e.g. to translate output), attemptCreateService can still
	// recognize the error that means "this service name is not unique."
	errorBody, err := xml.Marshal(gwacl.AzureError{
		error:      fmt.Errorf("broken HTTP request"),
		HTTPStatus: http.StatusConflict,
		Code:       "ServiceNameTaken",
		Message:    "De aangevraagde naam is al in gebruik.",
	})
	c.Assert(err, IsNil)
	responses := []gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(errorBody, http.StatusConflict, nil),
	}
	gwacl.PatchManagementAPIResponses(responses)
	azure, err := gwacl.NewManagementAPI("subscription", "certfile.pem")
	c.Assert(err, IsNil)

	service, err := attemptCreateService(azure)
	c.Check(err, IsNil)
	c.Check(service, IsNil)
}

func (*EnvironSuite) TestAttemptCreateServicePropagatesOtherFailure(c *C) {
	responses := []gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(nil, http.StatusNotFound, nil),
	}
	gwacl.PatchManagementAPIResponses(responses)
	azure, err := gwacl.NewManagementAPI("subscription", "certfile.pem")
	c.Assert(err, IsNil)

	_, err = attemptCreateService(azure)
	c.Assert(err, NotNil)
	c.Check(err, ErrorMatches, ".*Not Found.*")
}

func (*EnvironSuite) TestExtractDeploymentDNSExtractsHost(c *C) {
	// Example taken from Azure documentation:
	// http://msdn.microsoft.com/en-us/library/windowsazure/ee460804.aspx
	instanceURL := "http://MyService.cloudapp.net"
	instanceDNS, err := extractDeploymentDNS(instanceURL)
	c.Assert(err, IsNil)
	c.Check(instanceDNS, Equals, "MyService.cloudapp.net")
}

func (*EnvironSuite) TestNewHostedServiceCreatesService(c *C) {
	responses := []gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(nil, http.StatusOK, nil),
	}
	requests := gwacl.PatchManagementAPIResponses(responses)
	azure, err := gwacl.NewManagementAPI("subscription", "certfile.pem")
	c.Assert(err, IsNil)

	service, err := newHostedService(azure)
	c.Assert(err, IsNil)

	c.Assert(*requests, HasLen, 1)
	body := parseCreateServiceRequest(c, (*requests)[0])
	c.Check(body.ServiceName, Equals, service.ServiceName)
}

func (*EnvironSuite) TestNewHostedServiceRetriesIfNotUnique(c *C) {
	errorBody := makeServiceNameAlreadyTakenError(c)
	// In this scenario, the first two names that we try are already
	// taken.  The third one is unique though, so we succeed.
	responses := []gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(errorBody, http.StatusConflict, nil),
		gwacl.NewDispatcherResponse(errorBody, http.StatusConflict, nil),
		gwacl.NewDispatcherResponse(nil, http.StatusOK, nil),
	}
	requests := gwacl.PatchManagementAPIResponses(responses)
	azure, err := gwacl.NewManagementAPI("subscription", "certfile.pem")
	c.Assert(err, IsNil)

	service, err := newHostedService(azure)
	c.Check(err, IsNil)

	c.Assert(*requests, HasLen, 3)
	// How many names have been attempted, and how often?
	// There is a minute chance that this tries the same name twice, and
	// then this test will fail.  If that happens, try seeding the
	// randomizer with some fixed seed that doens't produce the problem.
	attemptedNames := make(map[string]int)
	for _, request := range *requests {
		name := parseCreateServiceRequest(c, request).ServiceName
		attemptedNames[name] += 1
	}
	// The three attempts we just made all had different service names.
	c.Check(attemptedNames, HasLen, 3)

	// Once newHostedService succeeds, we get a hosted service with the
	// last requested name.
	c.Check(
		service.ServiceName,
		Equals,
		parseCreateServiceRequest(c, (*requests)[2]).ServiceName)
}

func (*EnvironSuite) TestNewHostedServiceFailsIfUnableToFindUniqueName(c *C) {
	errorBody := makeServiceNameAlreadyTakenError(c)
	responses := []gwacl.DispatcherResponse{}
	for counter := 0; counter < 100; counter++ {
		responses = append(responses, gwacl.NewDispatcherResponse(errorBody, http.StatusConflict, nil))
	}
	gwacl.PatchManagementAPIResponses(responses)
	azure, err := gwacl.NewManagementAPI("subscription", "certfile.pem")
	c.Assert(err, IsNil)

	_, err = newHostedService(azure)
	c.Assert(err, NotNil)
	c.Check(err, ErrorMatches, "could not come up with a unique hosted service name.*")
}

func (*EnvironSuite) TestExtractDeploymentDNSPropagatesError(c *C) {
	_, err := extractDeploymentDNS(":x:THIS BREAKS:x:")
	c.Check(err, NotNil)
	c.Check(err, ErrorMatches, "parse error in instance URL: .*")
}

func (*EnvironSuite) TestSetServiceDNSNameReadsDeploymentAndUpdatesService(c *C) {
	serviceName := "fub"
	deploymentName := "default"
	instanceDNS := fmt.Sprintf("foobar.%s", AZURE_DOMAIN_NAME)
	deploymentBody, err := xml.Marshal(gwacl.Deployment{
		Name: deploymentName,
		URL:  fmt.Sprintf("http://%s", instanceDNS),
	})
	c.Assert(err, IsNil)
	// setServiceDNSName reads the Deployment to obtain the instance URL,
	// then updates the Hosted Service by setting its label to the DNS
	// name of the instance.
	responses := []gwacl.DispatcherResponse{
		gwacl.NewDispatcherResponse(deploymentBody, http.StatusOK, nil),
		gwacl.NewDispatcherResponse(nil, http.StatusOK, nil),
	}
	requests := gwacl.PatchManagementAPIResponses(responses)

	azure, err := gwacl.NewManagementAPI("subscription", "certfile.pem")
	c.Assert(err, IsNil)
	err = setServiceDNSName(azure, serviceName, deploymentName)
	c.Assert(err, IsNil)

	c.Assert(*requests, HasLen, 2)
	getDeploymentReq := (*requests)[0]
	updateServiceReq := (*requests)[1]

	c.Check(getDeploymentReq.URL, Equals, fmt.Sprintf(
		"https://management.core.windows.net/%s/services/hostedservices/%s/deployments/%s",
		"subscription", serviceName, deploymentName))
	updateServiceBody := &gwacl.UpdateHostedService{}
	err = xml.Unmarshal(updateServiceReq.Payload, updateServiceBody)
	c.Assert(err, IsNil)
	newLabel, err := base64.StdEncoding.DecodeString(updateServiceBody.Label)
	c.Check(string(newLabel), Equals, instanceDNS)
}

func (*EnvironSuite) TestMakeProvisionalServiceLabelIsConsistent(c *C) {
	c.Check(makeProvisionalServiceLabel("foo"), Equals, makeProvisionalServiceLabel("foo"))
}

func (*EnvironSuite) TestMakeProvisionalServiceLabelIncludesName(c *C) {
	c.Check(makeProvisionalServiceLabel("splyz"), Matches, ".*splyz.*")
}

func (*EnvironSuite) TestIsProvisionalServiceLabelRecognizesProvisionalLabel(c *C) {
	c.Check(isProvisionalServiceLabel(makeProvisionalServiceLabel("x")), Equals, true)
}

func (*EnvironSuite) TestIsProvisionalServiceLabelRecognizesPermanentLabel(c *C) {
	c.Check(isProvisionalServiceLabel("label"), Equals, false)
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

func (EnvironSuite) TestStopInstancesDestroysMachines(c *C) {
	service1Name := "service1"
	service1, service1Desc := makeAzureService(service1Name)
	service2Name := "service2"
	service2, service2Desc := makeAzureService(service2Name)
	services := []*gwacl.HostedService{service1, service2}
	responses := buildDestroyAzureServiceResponses(c, services)
	requests := gwacl.PatchManagementAPIResponses(responses)
	env := makeEnviron(c)
	instances := convertToInstances([]gwacl.HostedServiceDescriptor{*service1Desc, *service2Desc})

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

func (EnvironSuite) TestDestroyCleansUpStorage(c *C) {
	env := makeEnviron(c)
	cleanup := setDummyStorage(c, env)
	defer cleanup()
	services := []gwacl.HostedServiceDescriptor{}
	patchWithServiceListResponse(c, services)
	instances := convertToInstances([]gwacl.HostedServiceDescriptor{})

	err := env.Destroy(instances)
	c.Check(err, IsNil)

	files, err := env.Storage().List("")
	c.Assert(err, IsNil)
	c.Check(files, HasLen, 0)
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
	cleanup := setDummyStorage(c, env)
	defer cleanup()

	// Simulate 2 nodes corresponding to two Azure services.
	prefix := env.getEnvPrefix()
	service1Name := prefix + "service1"
	service2Name := prefix + "service2"
	service1, service1Desc := makeAzureService(service1Name)
	service2, service2Desc := makeAzureService(service2Name)
	services := []*gwacl.HostedService{service1, service2}
	// The call to AllInstances() will return only one service (service1).
	listInstancesResponses := buildAzureServiceListResponse(c, []gwacl.HostedServiceDescriptor{*service1Desc})
	destroyResponses := buildDestroyAzureServiceResponses(c, services)
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
	c.Check(strings.Contains((*requests)[1].URL, service1Name), IsTrue)
	c.Check((*requests)[2].Method, Equals, "DELETE")
	c.Check(strings.Contains((*requests)[2].URL, service1Name), IsTrue)
	c.Check((*requests)[3].Method, Equals, "GET")
	c.Check(strings.Contains((*requests)[3].URL, service2Name), IsTrue)
	c.Check((*requests)[4].Method, Equals, "DELETE")
	c.Check(strings.Contains((*requests)[4].URL, service2Name), IsTrue)
}

func (EnvironSuite) TestGetInstance(c *C) {
	env := makeEnviron(c)
	prefix := env.getEnvPrefix()
	serviceName := prefix + "instance-name"
	serviceDesc := gwacl.HostedServiceDescriptor{ServiceName: serviceName}
	service := gwacl.HostedService{HostedServiceDescriptor: serviceDesc}
	responses := buildAzureServiceResponses(c, service)
	gwacl.PatchManagementAPIResponses(responses)

	instance, err := env.getInstance("serviceName")
	c.Check(err, IsNil)

	c.Check(string(instance.Id()), Equals, serviceName)
}

func (EnvironSuite) TestNewOSVirtualDisk(c *C) {
	env := makeEnviron(c)

	vhd := env.newOSVirtualDisk()

	mediaLinkUrl, err := url.Parse(vhd.MediaLink)
	c.Check(err, IsNil)
	st := env.Storage().(*azureStorage)
	storageAccount := st.getContainer()
	c.Check(mediaLinkUrl.Host, Equals, fmt.Sprintf("%s.blob.core.windows.net", storageAccount))
	// TODO: check vhd's sourceImageName when we will use simplestreams to
	// to get the image name to use.
}

func (EnvironSuite) TestNewRole(c *C) {
	env := makeEnviron(c)
	vhd := env.newOSVirtualDisk()
	userData := "example-user-data"

	role := env.newRole(vhd, userData)

	configs := role.ConfigurationSets
	linuxConfig := configs[0]
	networkConfig := configs[1]
	c.Check(linuxConfig.UserData, Equals, userData)
	c.Check(linuxConfig.DisableSSHPasswordAuthentication, Equals, "true")
	firstEndpoint := (*networkConfig.InputEndpoints)[0]
	c.Check(firstEndpoint.LocalPort, Equals, 22)
	c.Check(firstEndpoint.Port, Equals, 22)
	c.Check(firstEndpoint.Protocol, Equals, "TCP")
	c.Check(role.OSVirtualHardDisk[0], Equals, *vhd)
}

func (EnvironSuite) TestNewDeployment(c *C) {
	env := makeEnviron(c)
	userData := "example-user-data"
	deploymentLabel := "deployment-label"

	deployment := env.newDeployment(deploymentLabel, userData)

	c.Check(deployment.RoleList[0].ConfigurationSets[0].UserData, Equals, userData)
	base64Label := base64.StdEncoding.EncodeToString([]byte(deploymentLabel))
	c.Check(deployment.Label, Equals, base64Label)
	c.Check(deployment.RoleList, HasLen, 1)
}
