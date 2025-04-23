// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coremodel "github.com/juju/juju/core/model"
	corerelation "github.com/juju/juju/core/relation"
	relationtesting "github.com/juju/juju/core/relation/testing"
	"github.com/juju/juju/domain/relation"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importSuite struct {
	testing.IsolationSuite

	service *MockImportService
}

var _ = gc.Suite(&importSuite{})

func (s *importSuite) TestImport(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	id1 := 3
	key1 := relationtesting.GenNewKey(c, "ubuntu:peer")
	id2 := 7
	key2 := relationtesting.GenNewKey(c, "ubuntu:juju-info ntp:juju-info")
	model := s.expectExecute(c, []int{id1, id2}, []corerelation.Key{key1, key2})

	importOp := importOperation{
		service: s.service,
		logger:  loggertesting.WrapCheckLog(c),
	}

	// Act
	err := importOp.Execute(context.Background(), model)

	// Assert
	c.Assert(err, gc.IsNil)
}

func (s *importSuite) TestImportNoRelations(c *gc.C) {
	// Arrange
	defer s.setupMocks(c)

	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})

	importOp := importOperation{
		service: s.service,
		logger:  loggertesting.WrapCheckLog(c),
	}

	// Act
	err := importOp.Execute(context.Background(), model)

	// Assert
	c.Assert(err, gc.IsNil)
}

func (s *importSuite) TestImportBadKey(c *gc.C) {
	// Arrange
	defer s.setupMocks(c)

	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})
	model.AddRelation(description.RelationArgs{
		Id:  32,
		Key: "failme",
	})

	importOp := importOperation{
		service: s.service,
		logger:  loggertesting.WrapCheckLog(c),
	}

	// Act
	err := importOp.Execute(context.Background(), model)

	// Assert
	c.Assert(err, gc.Not(jc.ErrorIsNil))
}

func (s *importSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.service = NewMockImportService(ctrl)
	return ctrl
}

func (s *importSuite) expectExecute(c *gc.C, ids []int, keys []corerelation.Key) description.Model {
	model := description.NewModel(description.ModelArgs{
		Type: coremodel.IAAS.String(),
	})
	args := make(relation.ImportRelationsArgs, len(ids))
	for i, id := range ids {
		rel := model.AddRelation(description.RelationArgs{
			Id:  id,
			Key: keys[i].String(),
		})
		args[i] = relation.ImportRelationArg{
			ID:  id,
			Key: keys[i],
		}
		eps := keys[i].EndpointIdentifiers()
		args[i].Endpoints = make([]relation.ImportEndpoint, len(eps))
		for j, ep := range eps {
			rel.AddEndpoint(description.EndpointArgs{
				ApplicationName: ep.ApplicationName,
				Name:            ep.EndpointName,
				Role:            string(ep.Role),
			})

			args[i].Endpoints[j] = relation.ImportEndpoint{
				ApplicationName:     ep.ApplicationName,
				EndpointName:        ep.EndpointName,
				ApplicationSettings: map[string]interface{}{},
				UnitSettings:        map[string]map[string]interface{}{},
			}
		}

	}

	s.service.EXPECT().ImportRelations(gomock.Any(), relationArgMatcher{c: c, expected: args}).Return(nil)
	return model
}

type relationArgMatcher struct {
	c        *gc.C
	expected relation.ImportRelationsArgs
}

func (m relationArgMatcher) Matches(x interface{}) bool {
	obtained, ok := x.(relation.ImportRelationsArgs)
	if !ok {
		return false
	}
	return m.c.Check(obtained, jc.SameContents, m.expected)
}

func (relationArgMatcher) String() string {
	return "matches charm upload requests"
}
