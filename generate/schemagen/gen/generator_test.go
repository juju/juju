// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gen

import (
	"reflect"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facade"
	jsonschema "github.com/juju/juju/generate/schemagen/jsonschema-gen"
	"github.com/juju/juju/internal/rpcreflect"
)

type GenSuite struct {
	testing.IsolationSuite

	pkgRegistry *MockPackageRegistry
	apiServer   *MockAPIServer
	registry    *MockRegistry
}

var _ = gc.Suite(&GenSuite{})

func (s *GenSuite) TestResult(c *gc.C) {
	defer s.setup(c).Finish()

	s.scenario(c,
		s.expectLoadPackage,
		s.expectList,
		s.expectGetType,
	)
	result, err := Generate(s.pkgRegistry, s.apiServer)
	c.Check(err, jc.ErrorIsNil)

	objtype := rpcreflect.ObjTypeOf(reflect.TypeOf(ResourcesFacade{}))
	c.Check(result, gc.DeepEquals, []FacadeSchema{
		{
			Name:        "Resources",
			Description: "",
			Version:     4,
			Schema:      jsonschema.ReflectFromObjType(objtype),
		},
	})
}

func (s *GenSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.pkgRegistry = NewMockPackageRegistry(ctrl)
	s.apiServer = NewMockAPIServer(ctrl)
	s.registry = NewMockRegistry(ctrl)

	return ctrl
}

func (s *GenSuite) scenario(c *gc.C, behaviours ...func()) {
	for _, b := range behaviours {
		b()
	}
}

func (s *GenSuite) expectList() {
	aExp := s.apiServer.EXPECT()
	aExp.AllFacades().Return(s.registry)

	rExp := s.registry.EXPECT()
	rExp.ListDetails().Return([]facade.Details{
		{
			Name:    "Resources",
			Version: 4,
		},
	})
}

func (s *GenSuite) expectLoadPackage() {
	aExp := s.pkgRegistry.EXPECT()
	aExp.LoadPackage().Return(nil, nil)
}

type ResourcesFacade struct{}

func (ResourcesFacade) Resources(params []string) ([]string, error) {
	return nil, nil
}

func (s *GenSuite) expectGetType() {
	rExp := s.registry.EXPECT()
	rExp.GetType("Resources", 4).Return(reflect.TypeOf(ResourcesFacade{}), nil)
}
