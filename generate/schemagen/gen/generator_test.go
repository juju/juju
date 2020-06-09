// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package gen

import (
	"reflect"

	gomock "github.com/golang/mock/gomock"
	jsonschema "github.com/juju/jsonschema-gen"
	"github.com/juju/rpcreflect"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facade"
)

type GenSuite struct {
	testing.IsolationSuite

	pkgRegistry *MockPackageRegistry
	apiServer   *MockAPIServer
	registry    *MockRegistry
	linker      *MockLinker
}

var _ = gc.Suite(&GenSuite{})

func (s *GenSuite) TestResult(c *gc.C) {
	defer s.setup(c).Finish()

	s.scenario(c,
		s.expectLoadPackage,
		s.expectList,
		s.expectLinker,
		s.expectGetType,
	)
	result, err := Generate(s.pkgRegistry, s.linker, s.apiServer)
	c.Check(err, jc.ErrorIsNil)

	objtype := rpcreflect.ObjTypeOf(reflect.TypeOf(ResourcesFacade{}))
	c.Check(result, gc.DeepEquals, []FacadeSchema{
		{
			Name:        "Resources",
			Description: "",
			Version:     4,
			Schema:      jsonschema.ReflectFromObjType(objtype),
			AvailableTo: []string{},
		},
	})
}

func (s *GenSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.pkgRegistry = NewMockPackageRegistry(ctrl)
	s.apiServer = NewMockAPIServer(ctrl)
	s.registry = NewMockRegistry(ctrl)
	s.linker = NewMockLinker(ctrl)

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

func (s *GenSuite) expectLinker() {
	aExp := s.linker.EXPECT()
	aExp.Links(gomock.Any(), gomock.Any()).Return([]string{})
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
