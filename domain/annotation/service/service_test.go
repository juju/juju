// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/core/annotations"
	"github.com/juju/juju/domain/annotation"
	annotationerrors "github.com/juju/juju/domain/annotation/errors"
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

var _ = tc.Suite(&serviceSuite{})

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	return ctrl
}

func (s *serviceSuite) service() *Service {
	return NewService(s.state)
}

// TestGetAnnotations is testing the happy path for getting annotations for an ID.
func (s *serviceSuite) TestGetAnnotations(c *tc.C) {
	defer s.setupMocks(c).Finish()
	id1 := annotations.ID{Kind: annotations.KindUnit, Name: "unit1"}
	id33 := annotations.ID{Kind: annotations.KindUnit, Name: "unit33"}
	id44 := annotations.ID{Kind: annotations.KindUnit, Name: "unit44"}
	idNotExist := annotations.ID{Kind: annotations.KindUnit, Name: "unitNoAnnotations"}
	mockState := map[stateAnnotationKey]string{
		{ID: id1, key: "annotationKey1"}:  "annotationValue1",
		{ID: id1, key: "annotationKey2"}:  "annotationValue2",
		{ID: id33, key: "annotationKey3"}: "annotationValue3",
		{ID: id44, key: "annotationKey4"}: "annotationValue4",
	}

	s.state.EXPECT().GetAnnotations(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, id annotations.ID) (map[string]string, error) {
			annotations := make(map[string]string)
			for annKey := range mockState {
				if annKey.ID == id {
					annotations[annKey.key] = mockState[annKey]
				}
			}
			return annotations, nil
		},
	).AnyTimes()

	annotations, err := s.service().GetAnnotations(context.Background(), id1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(annotations), tc.Equals, 2)
	c.Assert(annotations["annotationKey1"], tc.Equals, "annotationValue1")
	c.Assert(annotations["annotationKey2"], tc.Equals, "annotationValue2")

	// Assert that an empty map (not nil) is returend if no annotations
	// are associated with a given ID
	noAnnotations, err := s.service().GetAnnotations(context.Background(), idNotExist)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(noAnnotations), tc.Equals, 0)
}

func (s *serviceSuite) TestGetCharmAnnotations(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetCharmAnnotations(gomock.Any(), annotation.GetCharmArgs{
		Source:   "ch",
		Name:     "foo",
		Revision: 1,
	}).Return(map[string]string{
		"annotationKey1": "annotationValue1",
		"annotationKey2": "annotationValue2",
	}, nil)

	annotations, err := s.service().GetCharmAnnotations(context.Background(), annotation.GetCharmArgs{
		Source:   "ch",
		Name:     "foo",
		Revision: 1,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(annotations), tc.Equals, 2)
	c.Assert(annotations["annotationKey1"], tc.Equals, "annotationValue1")
	c.Assert(annotations["annotationKey2"], tc.Equals, "annotationValue2")
}

func (s *serviceSuite) TestSetAnnotations(c *tc.C) {
	defer s.setupMocks(c).Finish()
	id1 := annotations.ID{Kind: annotations.KindUnit, Name: "unit1"}
	id33 := annotations.ID{Kind: annotations.KindUnit, Name: "unit33"}
	id44 := annotations.ID{Kind: annotations.KindUnit, Name: "unit44"}
	mockState := map[stateAnnotationKey]string{
		{ID: id1, key: "annotationKey1"}:  "annotationValue1",
		{ID: id1, key: "annotationKey2"}:  "annotationValue2",
		{ID: id33, key: "annotationKey3"}: "annotationValue3",
		{ID: id44, key: "annotationKey4"}: "annotationValue4",
	}

	s.state.EXPECT().SetAnnotations(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, id annotations.ID, annotations map[string]string) error {
			for annKey, annVal := range annotations {
				if annVal == "" {
					delete(mockState, stateAnnotationKey{ID: id, key: annKey})
				} else {
					mockState[stateAnnotationKey{ID: id, key: annKey}] = annVal
				}
			}
			return nil
		},
	).AnyTimes()

	annotations := map[string]string{
		"annotationKey5": "annotationValue5",
		"annotationKey1": "annotationValue1Updated",
	}

	err := s.service().SetAnnotations(context.Background(), id1, annotations)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(mockState), tc.Equals, 5)
	c.Assert(mockState[stateAnnotationKey{ID: id1, key: "annotationKey5"}], tc.Equals, "annotationValue5")
	c.Assert(mockState[stateAnnotationKey{ID: id1, key: "annotationKey1"}], tc.Equals, "annotationValue1Updated")

	// Unset a key
	unsetAnnotations := map[string]string{"annotationKey4": ""}
	err = s.service().SetAnnotations(context.Background(), id44, unsetAnnotations)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(mockState), tc.Equals, 4)

}

func (s *serviceSuite) TestSetCharmAnnotations(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().SetCharmAnnotations(gomock.Any(), annotation.GetCharmArgs{
		Source:   "ch",
		Name:     "foo",
		Revision: 1,
	}, map[string]string{
		"annotationKey1": "annotationValue1",
		"annotationKey2": "annotationValue2",
	}).Return(nil)

	err := s.service().SetCharmAnnotations(context.Background(), annotation.GetCharmArgs{
		Source:   "ch",
		Name:     "foo",
		Revision: 1,
	}, map[string]string{
		"annotationKey1": "annotationValue1",
		"annotationKey2": "annotationValue2",
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetAnnotationsWithInvalidKeys(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service().SetAnnotations(context.Background(), annotations.ID{
		Kind: annotations.KindUnit,
		Name: "unit1",
	}, map[string]string{
		"foo.bar": "annotationValue1",
	})
	c.Assert(err, tc.ErrorIs, annotationerrors.InvalidKey)

	err = s.service().SetAnnotations(context.Background(), annotations.ID{
		Kind: annotations.KindUnit,
		Name: "unit1",
	}, map[string]string{
		"  ": "annotationValue1",
	})
	c.Assert(err, tc.ErrorIs, annotationerrors.InvalidKey)
}

func (s *serviceSuite) TestSetCharmAnnotationsWithInvalidKeys(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service().SetCharmAnnotations(context.Background(), annotation.GetCharmArgs{
		Source:   "ch",
		Name:     "foo",
		Revision: 1,
	}, map[string]string{
		"foo.bar": "annotationValue1",
	})
	c.Assert(err, tc.ErrorIs, annotationerrors.InvalidKey)

	err = s.service().SetCharmAnnotations(context.Background(), annotation.GetCharmArgs{
		Source:   "ch",
		Name:     "foo",
		Revision: 1,
	}, map[string]string{
		"  ": "annotationValue1",
	})
	c.Assert(err, tc.ErrorIs, annotationerrors.InvalidKey)
}
