// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !gccgo

package vsphere

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/govmomi"
	"github.com/juju/govmomi/session"
	"github.com/juju/govmomi/vim25"
	"github.com/juju/govmomi/vim25/methods"
	"github.com/juju/govmomi/vim25/soap"
	"github.com/juju/govmomi/vim25/types"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"golang.org/x/net/context"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/testing"
)

var (
	ConfigAttrs = testing.FakeConfig().Merge(testing.Attrs{
		"type":             "vsphere",
		"uuid":             "2d02eeac-9dbb-11e4-89d3-123b93f75cba",
		"datacenter":       "/datacenter1",
		"host":             "host1",
		"user":             "user1",
		"password":         "password1",
		"external-network": "",
	})
)

type BaseSuite struct {
	gitjujutesting.IsolationSuite

	Config    *config.Config
	EnvConfig *environConfig
	Env       *environ

	ServeMux  *http.ServeMux
	ServerUrl string
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.PatchValue(&newConnection, newFakeConnection)
	s.initEnv(c)
	s.setUpHttpProxy(c)
	s.FakeMetadataServer()
	osenv.SetJujuHome(c.MkDir())
}

func (s *BaseSuite) initEnv(c *gc.C) {
	cfg, err := testing.EnvironConfig(c).Apply(ConfigAttrs)
	c.Assert(err, jc.ErrorIsNil)
	env, err := environs.New(cfg)
	c.Assert(err, jc.ErrorIsNil)
	s.Env = env.(*environ)
	s.setConfig(c, cfg)
}

func (s *BaseSuite) setConfig(c *gc.C, cfg *config.Config) {
	s.Config = cfg
	ecfg, err := newValidConfig(cfg, configDefaults)
	c.Assert(err, jc.ErrorIsNil)
	s.EnvConfig = ecfg
	s.Env.ecfg = s.EnvConfig
}

func (s *BaseSuite) UpdateConfig(c *gc.C, attrs map[string]interface{}) {
	cfg, err := s.Config.Apply(attrs)
	c.Assert(err, jc.ErrorIsNil)
	s.setConfig(c, cfg)
}

func (s *BaseSuite) setUpHttpProxy(c *gc.C) {
	s.ServeMux = http.NewServeMux()
	server := httptest.NewServer(s.ServeMux)
	s.ServerUrl = server.URL
	cfg, _ := s.Config.Apply(map[string]interface{}{"image-metadata-url": server.URL})
	s.setConfig(c, cfg)
}

type fakeApiHandler func(req, res soap.HasFault)
type fakePropertiesHandler func(req, res *methods.RetrievePropertiesBody)

type fakeApiCall struct {
	handler fakeApiHandler
	method  string
}

type fakePropertiesCall struct {
	handler fakePropertiesHandler
	object  string
}

type fakeClient struct {
	handlers         []fakeApiCall
	propertyHandlers []fakePropertiesCall
}

func (c *fakeClient) RoundTrip(ctx context.Context, req, res soap.HasFault) error {
	reqType := reflect.ValueOf(req).Elem().FieldByName("Req").Elem().Type().Name()

	if reqType == "RetrieveProperties" {
		reqBody := req.(*methods.RetrievePropertiesBody)
		resBody := res.(*methods.RetrievePropertiesBody)
		obj := reqBody.Req.SpecSet[0].ObjectSet[0].Obj.Value
		logger.Debugf("executing RetrieveProperties for object %s", obj)
		call := c.propertyHandlers[0]
		if call.object != obj {
			return errors.Errorf("expected object of type %s, got %s", obj, call.object)
		}
		call.handler(reqBody, resBody)
		c.propertyHandlers = c.propertyHandlers[1:]
	} else {
		logger.Infof("Executing RoundTrip method, type: %s", reqType)
		call := c.handlers[0]
		if call.method != reqType {
			return errors.Errorf("expected method of type %s, got %s", reqType, call.method)
		}
		call.handler(req, res)
		c.handlers = c.handlers[1:]
	}
	return nil
}

func (c *fakeClient) SetProxyHandler(method string, handler fakeApiHandler) {
	c.handlers = append(c.handlers, fakeApiCall{method: method, handler: handler})
}

func (c *fakeClient) SetPropertyProxyHandler(obj string, handler fakePropertiesHandler) {
	c.propertyHandlers = append(c.propertyHandlers, fakePropertiesCall{object: obj, handler: handler})
}

var newFakeConnection = func(url *url.URL) (*govmomi.Client, error) {
	fakeClient := &fakeClient{
		handlers:         make([]fakeApiCall, 0, 100),
		propertyHandlers: make([]fakePropertiesCall, 0, 100),
	}

	fakeClient.SetPropertyProxyHandler("FakeRootFolder", RetrieveDatacenter)

	vimClient := &vim25.Client{
		Client: &soap.Client{},
		ServiceContent: types.ServiceContent{
			RootFolder: types.ManagedObjectReference{
				Type:  "Folder",
				Value: "FakeRootFolder",
			},
			OvfManager: &types.ManagedObjectReference{
				Type:  "OvfManager",
				Value: "FakeOvfManager",
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

var CommonRetrieveProperties = func(resBody *methods.RetrievePropertiesBody, objType, objValue, propName string, propValue interface{}) {
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

var RetrieveDatacenter = func(reqBody, resBody *methods.RetrievePropertiesBody) {
	CommonRetrieveProperties(resBody, "Datacenter", "FakeDatacenter", "name", "datacenter1")
}

var RetrieveDatacenterProperties = func(reqBody, resBody *methods.RetrievePropertiesBody) {
	resBody.Res = &types.RetrievePropertiesResponse{
		Returnval: []types.ObjectContent{
			types.ObjectContent{
				Obj: types.ManagedObjectReference{
					Type:  "Datacenter",
					Value: "FakeDatacenter",
				},
				PropSet: []types.DynamicProperty{
					types.DynamicProperty{Name: "hostFolder", Val: types.ManagedObjectReference{
						Type:  "Folder",
						Value: "FakeHostFolder",
					}},
					types.DynamicProperty{Name: "vmFolder", Val: types.ManagedObjectReference{
						Type:  "Folder",
						Value: "FakeVmFolder",
					}},
				},
			},
		},
	}
}
