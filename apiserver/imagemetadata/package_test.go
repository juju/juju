// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata_test

import (
	stdtesting "testing"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/imagemetadata"
	"github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state/cloudimagemetadata"
	coretesting "github.com/juju/juju/testing"
)

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}

type baseImageMetadataSuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authorizer testing.FakeAuthorizer

	api   *imagemetadata.API
	state *mockState

	calls []string
}

func (s *baseImageMetadataSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.authorizer = testing.FakeAuthorizer{names.NewUserTag("testuser"), true}

	s.calls = []string{}
	s.state = s.constructState(c)

	var err error
	s.api, err = imagemetadata.CreateAPI(s.state, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *baseImageMetadataSuite) assertCalls(c *gc.C, expectedCalls []string) {
	c.Assert(s.calls, jc.SameContents, expectedCalls)
}

const (
	findMetadata = "findMetadata"
	saveMetadata = "saveMetadata"
)

func (s *baseImageMetadataSuite) constructState(c *gc.C) *mockState {
	return &mockState{
		findMetadata: func(f cloudimagemetadata.MetadataFilter) (map[cloudimagemetadata.SourceType][]cloudimagemetadata.Metadata, error) {
			s.calls = append(s.calls, findMetadata)
			return nil, nil
		},
		saveMetadata: func(m cloudimagemetadata.Metadata) error {
			s.calls = append(s.calls, saveMetadata)
			return nil
		},
	}
}

type mockState struct {
	findMetadata func(f cloudimagemetadata.MetadataFilter) (map[cloudimagemetadata.SourceType][]cloudimagemetadata.Metadata, error)
	saveMetadata func(m cloudimagemetadata.Metadata) error
}

func (st *mockState) FindMetadata(f cloudimagemetadata.MetadataFilter) (map[cloudimagemetadata.SourceType][]cloudimagemetadata.Metadata, error) {
	return st.findMetadata(f)
}

func (st *mockState) SaveMetadata(m cloudimagemetadata.Metadata) error {
	return st.saveMetadata(m)
}
