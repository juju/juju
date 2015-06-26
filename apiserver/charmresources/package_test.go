// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmresources_test

import (
	"strings"
	stdtesting "testing"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/charmresources"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/testing"
	resources "github.com/juju/juju/charmresources"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}

type baseResourcesSuite struct {
	coretesting.BaseSuite

	apiResources *common.Resources
	authorizer   testing.FakeAuthorizer

	api          *charmresources.ResourceManagerAPI
	state        *mockState
	environOwner string

	calls []string

	resourceManager *mockResourceManager
	resources       map[string]resources.Resource

	blocks map[state.BlockType]state.Block
}

func (s *baseResourcesSuite) SetUpTest(c *gc.C) {
	s.apiResources = common.NewResources()
	s.authorizer = testing.FakeAuthorizer{names.NewUserTag("testuser"), true}
	s.calls = []string{}
	s.state = s.constructState(c)
	s.environOwner = "testuser"

	s.resources = make(map[string]resources.Resource)
	s.resourceManager = s.constructResourceManager(c)

	var err error
	s.api, err = charmresources.CreateAPI(s.state, s.apiResources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *baseResourcesSuite) assertCalls(c *gc.C, expectedCalls []string) {
	c.Assert(s.calls, jc.SameContents, expectedCalls)
}

const (
	resourceListCall    = "resourceList"
	resourceDeleteCall  = "resourceDelete"
	resourceManagerCall = "resourceManager"
	getBlockForTypeCall = "getBlockForType"
)

func (s *baseResourcesSuite) envOwner() string {
	return s.environOwner
}

func (s *baseResourcesSuite) constructState(c *gc.C) *mockState {
	s.blocks = make(map[state.BlockType]state.Block)
	return &mockState{
		resourceManager: func() resources.ResourceManager {
			s.calls = append(s.calls, resourceManagerCall)
			return s.resourceManager
		},
		getBlockForType: func(t state.BlockType) (state.Block, bool, error) {
			s.calls = append(s.calls, getBlockForTypeCall)
			val, found := s.blocks[t]
			return val, found, nil
		},
		envOwner: func() (names.UserTag, error) {
			return names.NewUserTag(s.envOwner()), nil
		},
	}
}

func (s *baseResourcesSuite) addBlock(c *gc.C, t state.BlockType, msg string) {
	s.blocks[t] = mockBlock{t: t, msg: msg}
}

func (s *baseResourcesSuite) blockAllChanges(c *gc.C, msg string) {
	s.addBlock(c, state.ChangeBlock, msg)
}

func (s *baseResourcesSuite) blockRemoveObject(c *gc.C, msg string) {
	s.addBlock(c, state.RemoveBlock, msg)
}

func (s *baseResourcesSuite) assertBlocked(c *gc.C, err error, msg string) {
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, msg)
}

func (s *baseResourcesSuite) constructResourceManager(c *gc.C) *mockResourceManager {
	return &mockResourceManager{
		resourceDelete: func(path string) error {
			s.calls = append(s.calls, resourceDeleteCall)
			delete(s.resources, path)
			return nil
		},
		resourceList: func(filter resources.ResourceAttributes) ([]resources.Resource, error) {
			s.calls = append(s.calls, resourceListCall)
			var result []resources.Resource
			for _, v := range s.resources {
				// For testing, we'll just do a very simple match (not production code).
				if filter.PathName != "" && !strings.HasSuffix(v.Path, filter.PathName) {
					continue
				}
				if filter.Series != "" && !strings.Contains(v.Path, "s/"+filter.Series) {
					continue
				}
				// Resources always have the type in their path.
				v.Path = "/blob/" + v.Path
				result = append(result, v)
			}
			return result, nil
		},
	}
}

func (s *baseResourcesSuite) addResource(res resources.Resource) {
	s.resources["/blob/"+res.Path] = res
}

type mockResourceManager struct {
	resources.ResourceManager
	resourceList   func(filter resources.ResourceAttributes) ([]resources.Resource, error)
	resourceDelete func(resourcePath string) error
}

func (m *mockResourceManager) ResourceList(filter resources.ResourceAttributes) ([]resources.Resource, error) {
	return m.resourceList(filter)
}

func (m *mockResourceManager) ResourceDelete(resourcePath string) error {
	return m.resourceDelete(resourcePath)
}

type mockState struct {
	resourceManager func() resources.ResourceManager
	getBlockForType func(t state.BlockType) (state.Block, bool, error)
	envOwner        func() (names.UserTag, error)
}

func (st *mockState) EnvOwner() (names.UserTag, error) {
	return st.envOwner()
}

func (st *mockState) ResourceManager() resources.ResourceManager {
	return st.resourceManager()
}

func (st *mockState) GetBlockForType(t state.BlockType) (state.Block, bool, error) {
	return st.getBlockForType(t)
}

type mockBlock struct {
	state.Block
	t   state.BlockType
	msg string
}

func (b mockBlock) Type() state.BlockType {
	return b.t
}

func (b mockBlock) Message() string {
	return b.msg
}
