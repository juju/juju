// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/annotations"
)

type serviceSuite struct {
	state *MockState
}

// stateAnnotationEntity is modelling any entry in an annotation table in DB.
// In the state layer, we keep separate tables for different entities
// (e.g. annotation_machine, annotation_unit, etc.)
// mockState in the tests below models each one.
type stateAnnotationKey struct {
	entity annotations.ID
	key    string
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	return ctrl
}

func (s *serviceSuite) service() *Service {
	return NewService(s.state)
}

// TestGetAnnotations is testing the happy path for getting annotations for an entity.
func (s *serviceSuite) TestGetAnnotations(c *gc.C) {
	defer s.setupMocks(c).Finish()
	entity1 := annotations.ID{Kind: annotations.KindUnit, Name: "unit1"}
	entity33 := annotations.ID{Kind: annotations.KindUnit, Name: "unit33"}
	entity44 := annotations.ID{Kind: annotations.KindUnit, Name: "unit44"}
	mockState := map[stateAnnotationKey]string{
		{entity1, "annotationKey1"}:  "annotationValue1",
		{entity1, "annotationKey2"}:  "annotationValue2",
		{entity33, "annotationKey3"}: "annotationValue3",
		{entity44, "annotationKey4"}: "annotationValue4",
	}

	s.state.EXPECT().GetAnnotations(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context,
			entity annotations.ID) (map[string]string, error) {
			annotations := make(map[string]string)
			for annKey := range mockState {
				if annKey.entity == entity {
					annotations[annKey.key] = mockState[annKey]
				}
			}
			return annotations, nil
		},
	)

	annotations, err := s.service().GetAnnotations(context.Background(), entity1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(annotations), gc.Equals, 2)
	c.Assert(annotations["annotationKey1"], gc.Equals, "annotationValue1")
	c.Assert(annotations["annotationKey2"], gc.Equals, "annotationValue2")
}

// TestSetAnnotations is testing the happy path for setting annotations for an entity.
func (s *serviceSuite) TestSetAnnotations(c *gc.C) {
	defer s.setupMocks(c).Finish()
	entity1 := annotations.ID{Kind: annotations.KindUnit, Name: "unit1"}
	entity33 := annotations.ID{Kind: annotations.KindUnit, Name: "unit33"}
	entity44 := annotations.ID{Kind: annotations.KindUnit, Name: "unit44"}
	mockState := map[stateAnnotationKey]string{
		{entity1, "annotationKey1"}:  "annotationValue1",
		{entity1, "annotationKey2"}:  "annotationValue2",
		{entity33, "annotationKey3"}: "annotationValue3",
		{entity44, "annotationKey4"}: "annotationValue4",
	}

	s.state.EXPECT().SetAnnotations(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context,
			entity annotations.ID,
			annotations map[string]string) error {
			for annKey, annVal := range annotations {
				mockState[stateAnnotationKey{entity, annKey}] = annVal
			}
			return nil
		},
	)

	annotations := map[string]string{"annotationKey5": "annotationValue5"}

	err := s.service().SetAnnotations(context.Background(), entity1, annotations)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(mockState), gc.Equals, 5)
	c.Assert(mockState[stateAnnotationKey{entity1, "annotationKey5"}], gc.Equals, "annotationValue5")
}
