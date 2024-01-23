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

// stateAnnotationKey is modelling any entry in an annotation table in DB.
// In the state layer, we keep separate tables for different IDs
// (e.g. annotation_machine, annotation_unit, etc.)
// mockState in the tests below models each one.
type stateAnnotationKey struct {
	ID  annotations.ID
	key string
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

// TestGetAnnotations is testing the happy path for getting annotations for an ID.
func (s *serviceSuite) TestGetAnnotations(c *gc.C) {
	defer s.setupMocks(c).Finish()
	ID1 := annotations.ID{Kind: annotations.KindUnit, Name: "unit1"}
	ID33 := annotations.ID{Kind: annotations.KindUnit, Name: "unit33"}
	ID44 := annotations.ID{Kind: annotations.KindUnit, Name: "unit44"}
	IDNotExist := annotations.ID{Kind: annotations.KindUnit, Name: "unitNoAnnotations"}
	mockState := map[stateAnnotationKey]string{
		{ID1, "annotationKey1"}:  "annotationValue1",
		{ID1, "annotationKey2"}:  "annotationValue2",
		{ID33, "annotationKey3"}: "annotationValue3",
		{ID44, "annotationKey4"}: "annotationValue4",
	}

	s.state.EXPECT().GetAnnotations(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context,
			ID annotations.ID) (map[string]string, error) {
			annotations := make(map[string]string)
			for annKey := range mockState {
				if annKey.ID == ID {
					annotations[annKey.key] = mockState[annKey]
				}
			}
			return annotations, nil
		},
	).AnyTimes()

	annotations, err := s.service().GetAnnotations(context.Background(), ID1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(annotations), gc.Equals, 2)
	c.Assert(annotations["annotationKey1"], gc.Equals, "annotationValue1")
	c.Assert(annotations["annotationKey2"], gc.Equals, "annotationValue2")

	// Assert that an empty map (not nil) is returend if no annotations
	// are associated with a given ID
	noAnnotations, err := s.service().GetAnnotations(context.Background(), IDNotExist)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(noAnnotations), gc.Equals, 0)
}

// TestSetAnnotations is testing the happy path for setting annotations for an ID.
func (s *serviceSuite) TestSetAnnotations(c *gc.C) {
	defer s.setupMocks(c).Finish()
	ID1 := annotations.ID{Kind: annotations.KindUnit, Name: "unit1"}
	ID33 := annotations.ID{Kind: annotations.KindUnit, Name: "unit33"}
	ID44 := annotations.ID{Kind: annotations.KindUnit, Name: "unit44"}
	mockState := map[stateAnnotationKey]string{
		{ID1, "annotationKey1"}:  "annotationValue1",
		{ID1, "annotationKey2"}:  "annotationValue2",
		{ID33, "annotationKey3"}: "annotationValue3",
		{ID44, "annotationKey4"}: "annotationValue4",
	}

	s.state.EXPECT().SetAnnotations(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context,
			ID annotations.ID,
			annotations map[string]string) error {
			for annKey, annVal := range annotations {
				mockState[stateAnnotationKey{ID, annKey}] = annVal
			}
			return nil
		},
	)

	annotations := map[string]string{"annotationKey5": "annotationValue5"}

	err := s.service().SetAnnotations(context.Background(), ID1, annotations)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(mockState), gc.Equals, 5)
	c.Assert(mockState[stateAnnotationKey{ID1, "annotationKey5"}], gc.Equals, "annotationValue5")
}
