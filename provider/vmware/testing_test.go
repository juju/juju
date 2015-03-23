// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vmware

import (
	"net/url"
	"reflect"

	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
	"golang.org/x/net/context"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/testing"
)

var (
	ConfigAttrs = testing.FakeConfig().Merge(testing.Attrs{
		"type":          "vmware",
		"uuid":          "2d02eeac-9dbb-11e4-89d3-123b93f75cba",
		"datacenter":    "/datacenter1",
		"datastore":     "datastore1",
		"resource-pool": "resource-pool1",
		"host":          "host1",
		"user":          "user1",
		"password":      "password1",
	})
)

type BaseSuiteUnpatched struct {
	gitjujutesting.IsolationSuite

	Config    *config.Config
	EnvConfig *environConfig
	Env       *environ
	Prefix    string
}

func (s *BaseSuiteUnpatched) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.initEnv(c)
	//s.initInst(c)
}

func (s *BaseSuiteUnpatched) initEnv(c *gc.C) {
	s.Env = &environ{
		name: "vmware",
	}
	cfg := s.NewConfig(c, nil)
	s.setConfig(c, cfg)
}

func (s *BaseSuiteUnpatched) setConfig(c *gc.C, cfg *config.Config) {
	s.Config = cfg
	ecfg, err := newValidConfig(cfg, configDefaults)
	c.Assert(err, jc.ErrorIsNil)
	s.EnvConfig = ecfg
	uuid, _ := cfg.UUID()
	s.Env.uuid = uuid
	s.Env.ecfg = s.EnvConfig
	s.Prefix = "juju-" + uuid + "-"
}

func (s *BaseSuiteUnpatched) NewConfig(c *gc.C, updates testing.Attrs) *config.Config {
	var err error
	cfg := testing.EnvironConfig(c)
	cfg, err = cfg.Apply(ConfigAttrs)
	c.Assert(err, jc.ErrorIsNil)
	cfg, err = cfg.Apply(updates)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

func (s *BaseSuiteUnpatched) UpdateConfig(c *gc.C, attrs map[string]interface{}) {
	cfg, err := s.Config.Apply(attrs)
	c.Assert(err, jc.ErrorIsNil)
	s.setConfig(c, cfg)
}

type BaseSuite struct {
	BaseSuiteUnpatched
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.BaseSuiteUnpatched.SetUpTest(c)

	s.PatchValue(&newConnection, newFakeConnection)

	/*s.FakeConn = &fakeConn{}
	s.FakeCommon = &fakeCommon{}
	s.FakeEnviron = &fakeEnviron{}
	s.FakeImages = &fakeImages{}

	// Patch out all expensive external deps.
	s.Env.gce = s.FakeConn
	s.PatchValue(&newConnection, func(*environConfig) gceConnection {
		return s.FakeConn
	})
	s.PatchValue(&supportedArchitectures, s.FakeCommon.SupportedArchitectures)
	s.PatchValue(&bootstrap, s.FakeCommon.Bootstrap)
	s.PatchValue(&destroyEnv, s.FakeCommon.Destroy)
	s.PatchValue(&availabilityZoneAllocations, s.FakeCommon.AvailabilityZoneAllocations)
	s.PatchValue(&buildInstanceSpec, s.FakeEnviron.BuildInstanceSpec)
	s.PatchValue(&getHardwareCharacteristics, s.FakeEnviron.GetHardwareCharacteristics)
	s.PatchValue(&newRawInstance, s.FakeEnviron.NewRawInstance)
	s.PatchValue(&findInstanceSpec, s.FakeEnviron.FindInstanceSpec)
	s.PatchValue(&getInstances, s.FakeEnviron.GetInstances)
	s.PatchValue(&imageMetadataFetch, s.FakeImages.ImageMetadataFetch)*/
}

type fakeApiHandler func(req, res soap.HasFault)
type fakePropertiesHandler func(req, res *methods.RetrievePropertiesBody)

type fakeClient struct {
	handlers         map[string]fakeApiHandler
	propertyHandlers map[string]fakePropertiesHandler
}

func (c *fakeClient) RoundTrip(ctx context.Context, req, res soap.HasFault) error {
	reqType := reflect.ValueOf(req).Elem().FieldByName("Req").Elem().Type().Name()

	if reqType == "RetrieveProperties" {
		reqBody := req.(*methods.RetrievePropertiesBody)
		resBody := res.(*methods.RetrievePropertiesBody)
		obj := reqBody.Req.SpecSet[0].ObjectSet[0].Obj.Value
		logger.Debugf("executing RetrieveProperties for object %s", obj)
		handler := c.propertyHandlers[obj]
		handler(reqBody, resBody)
	} else {
		logger.Infof("Executing RoundTrip method, type: %s", reqType)
		handler := c.handlers[reqType]
		handler(req, res)
	}
	return nil
}

func (c *fakeClient) SetProxyHandler(method string, handler fakeApiHandler) {
	c.handlers[method] = handler
}

func (c *fakeClient) SetPropertyProxyHandler(obj string, handler fakePropertiesHandler) {
	c.propertyHandlers[obj] = handler
}

var newFakeConnection = func(url *url.URL) (*govmomi.Client, error) {
	fakeClient := &fakeClient{
		handlers:         make(map[string]fakeApiHandler),
		propertyHandlers: make(map[string]fakePropertiesHandler),
	}

	fakeClient.SetPropertyProxyHandler("FakeRootFolder", retrieveDatacenter)
	fakeClient.SetPropertyProxyHandler("FakeDatacenter", retrieveDatacenterProperties)
	fakeClient.SetPropertyProxyHandler("FakeDatastoreFolder", retrieveDatastore)
	fakeClient.SetPropertyProxyHandler("FakeHostFolder", retrieveResourcePool)

	vimClient := &vim25.Client{
		//Client:         soapClient,
		ServiceContent: types.ServiceContent{
			RootFolder: types.ManagedObjectReference{
				Type:  "Folder",
				Value: "FakeRootFolder",
			},
		},
		RoundTripper: fakeClient,
	}

	c := &govmomi.Client{
		Client:         vimClient,
		SessionManager: session.NewManager(vimClient),
	}
	return c, nil
}

var commonRetrieveProperties = func(resBody *methods.RetrievePropertiesBody, objType, objValue, propName string, propValue interface{}) {
	resBody.Res = &types.RetrievePropertiesResponse{
		Returnval: []types.ObjectContent{
			types.ObjectContent{
				Obj: types.ManagedObjectReference{
					Type:  objType,
					Value: objValue,
				},
				PropSet: []types.DynamicProperty{
					types.DynamicProperty{Name: propName, Val: propValue},
				},
			},
		},
	}
}

var retrieveDatacenter = func(reqBody, resBody *methods.RetrievePropertiesBody) {
	commonRetrieveProperties(resBody, "Datacenter", "FakeDatacenter", "name", "datacenter1")
}

var retrieveDatastore = func(reqBody, resBody *methods.RetrievePropertiesBody) {
	commonRetrieveProperties(resBody, "Datastore", "FakeDatastore", "name", "datastore1")
}

var retrieveResourcePool = func(reqBody, resBody *methods.RetrievePropertiesBody) {
	commonRetrieveProperties(resBody, "ResourcePool", "FakeResourcePool", "name", "resource-pool1")
}

var retrieveDatacenterProperties = func(reqBody, resBody *methods.RetrievePropertiesBody) {
	resBody.Res = &types.RetrievePropertiesResponse{
		Returnval: []types.ObjectContent{
			types.ObjectContent{
				Obj: types.ManagedObjectReference{
					Type:  "Datacenter",
					Value: "FakeDatacenter",
				},
				PropSet: []types.DynamicProperty{
					types.DynamicProperty{Name: "datastoreFolder", Val: types.ManagedObjectReference{
						Type:  "Folder",
						Value: "FakeDatastoreFolder",
					}},
					types.DynamicProperty{Name: "hostFolder", Val: types.ManagedObjectReference{
						Type:  "Folder",
						Value: "FakeHostFolder",
					}},
				},
			},
		},
	}
}
