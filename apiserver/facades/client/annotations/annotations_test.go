// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package annotations

import (
	"context"
	"errors"
	"fmt"

	"github.com/juju/names/v5"
	"github.com/juju/testing"
	"github.com/juju/utils/v4"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/annotations"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/domain/annotation"
	"github.com/juju/juju/rpc/params"
)

type annotationSuite struct {
	testing.IsolationSuite

	uuid string

	annotationService *MockAnnotationService
	authorizer        *MockAuthorizer

	annotationsAPI *API
}

var _ = gc.Suite(&annotationSuite{})

func (s *annotationSuite) TestGetAnnotations(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.ReadAccess, names.NewModelTag(s.uuid)).Return(nil)
	s.annotationService.EXPECT().GetAnnotations(gomock.Any(), annotations.ID{
		Kind: annotations.KindModel,
		Name: s.uuid,
	}).Return(map[string]string{"foo": "bar"}, nil)

	results := s.annotationsAPI.Get(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: names.NewModelTag(s.uuid).String()}},
	})
	c.Assert(results.Results, gc.DeepEquals, []params.AnnotationsGetResult{
		{EntityTag: names.NewModelTag(s.uuid).String(), Annotations: map[string]string{"foo": "bar"}},
	})
}

func (s *annotationSuite) TestGetAnnotationsBulk(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.ReadAccess, names.NewModelTag(s.uuid)).Return(nil)
	s.annotationService.EXPECT().GetAnnotations(gomock.Any(), annotations.ID{
		Kind: annotations.KindModel,
		Name: s.uuid,
	}).Return(map[string]string{"foo": "bar"}, nil)
	s.annotationService.EXPECT().GetAnnotations(gomock.Any(), annotations.ID{
		Kind: annotations.KindApplication,
		Name: "mysql",
	}).Return(nil, errors.New("boom"))
	s.annotationService.EXPECT().GetCharmAnnotations(gomock.Any(), annotation.GetCharmArgs{
		Source:   "ch",
		Name:     "mysql",
		Revision: 1,
	}).Return(map[string]string{"other": "one"}, nil)

	results := s.annotationsAPI.Get(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewModelTag(s.uuid).String()},
			{Tag: names.NewApplicationTag("mysql").String()},
			{Tag: names.NewCharmTag("mysql-1").String()},
		},
	})
	c.Assert(results.Results, gc.DeepEquals, []params.AnnotationsGetResult{
		{EntityTag: names.NewModelTag(s.uuid).String(), Annotations: map[string]string{"foo": "bar"}},
		{EntityTag: names.NewApplicationTag("mysql").String(),
			Error: params.ErrorResult{Error: &params.Error{
				Message: `getting annotations for "application-mysql": boom`,
			}},
		},
		{EntityTag: names.NewCharmTag("mysql-1").String(), Annotations: map[string]string{"other": "one"}},
	})
}

func (s *annotationSuite) TestGetAnnotationsNoPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.ReadAccess, names.NewModelTag(s.uuid)).Return(errors.New("boom"))

	results := s.annotationsAPI.Get(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: names.NewModelTag(s.uuid).String()}},
	})
	c.Assert(results.Results, gc.DeepEquals, []params.AnnotationsGetResult{
		{Error: params.ErrorResult{Error: &params.Error{
			Message: "boom",
		}}},
	})
}

func (s *annotationSuite) TestGetAnnotationsNoError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.ReadAccess, names.NewModelTag(s.uuid)).Return(nil)
	s.annotationService.EXPECT().GetAnnotations(context.Background(), annotations.ID{
		Kind: annotations.KindModel,
		Name: s.uuid,
	}).Return(map[string]string{"foo": "bar"}, errors.New("boom"))

	results := s.annotationsAPI.Get(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: names.NewModelTag(s.uuid).String()}},
	})
	c.Assert(results.Results, gc.DeepEquals, []params.AnnotationsGetResult{
		{
			EntityTag: names.NewModelTag(s.uuid).String(),
			Error: params.ErrorResult{Error: &params.Error{
				Message: fmt.Sprintf(`getting annotations for "model-%s": boom`, s.uuid),
			}},
		},
	})
}

func (s *annotationSuite) TestSetAnnotations(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, names.NewModelTag(s.uuid)).Return(nil)
	s.annotationService.EXPECT().SetAnnotations(gomock.Any(), annotations.ID{
		Kind: annotations.KindModel,
		Name: s.uuid,
	}, map[string]string{"foo": "bar"}).Return(nil)

	results := s.annotationsAPI.Set(context.Background(), params.AnnotationsSet{
		Annotations: []params.EntityAnnotations{{
			EntityTag:   names.NewModelTag(s.uuid).String(),
			Annotations: map[string]string{"foo": "bar"},
		}},
	})
	c.Assert(results.Results, gc.DeepEquals, []params.ErrorResult{})
}

func (s *annotationSuite) TestSetAnnotationsNoPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, names.NewModelTag(s.uuid)).Return(errors.New("boom"))

	results := s.annotationsAPI.Set(context.Background(), params.AnnotationsSet{
		Annotations: []params.EntityAnnotations{{
			EntityTag:   names.NewModelTag(s.uuid).String(),
			Annotations: map[string]string{"foo": "bar"},
		}},
	})
	c.Assert(results.Results, gc.DeepEquals, []params.ErrorResult{
		{Error: &params.Error{Message: "boom"}},
	})
}

func (s *annotationSuite) TestSetAnnotationsError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, names.NewModelTag(s.uuid)).Return(nil)
	s.annotationService.EXPECT().SetAnnotations(gomock.Any(), annotations.ID{
		Kind: annotations.KindModel,
		Name: s.uuid,
	}, map[string]string{"foo": "bar"}).Return(errors.New("boom"))

	results := s.annotationsAPI.Set(context.Background(), params.AnnotationsSet{
		Annotations: []params.EntityAnnotations{{
			EntityTag:   names.NewModelTag(s.uuid).String(),
			Annotations: map[string]string{"foo": "bar"},
		}},
	})
	c.Assert(results.Results, gc.DeepEquals, []params.ErrorResult{
		{Error: &params.Error{Message: fmt.Sprintf(`setting annotations for "model-%s": boom`, s.uuid)}},
	})
}

func (s *annotationSuite) TestSetAnnotationsBrokenBehaviour(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// TODO(stickupkid): This API for set is currently broken. This test just
	// locks that in place for now, knowing that we should fix this in the
	// future..

	// Essentially, it fails on the first error and doesn't continue to set the
	// then entity annotation. There is no rollback mechanism in place.

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, names.NewModelTag(s.uuid)).Return(nil)
	s.annotationService.EXPECT().SetAnnotations(gomock.Any(), annotations.ID{
		Kind: annotations.KindModel,
		Name: s.uuid,
	}, map[string]string{"foo": "bar"}).Return(nil)
	s.annotationService.EXPECT().SetAnnotations(gomock.Any(), annotations.ID{
		Kind: annotations.KindApplication,
		Name: "mysql",
	}, map[string]string{"foo": "bar"}).Return(errors.New("boom"))

	results := s.annotationsAPI.Set(context.Background(), params.AnnotationsSet{
		Annotations: []params.EntityAnnotations{
			{
				EntityTag:   names.NewModelTag(s.uuid).String(),
				Annotations: map[string]string{"foo": "bar"},
			},
			{
				EntityTag:   names.NewApplicationTag("mysql").String(),
				Annotations: map[string]string{"foo": "bar"},
			},
		},
	})
	c.Assert(results.Results, gc.DeepEquals, []params.ErrorResult{
		{Error: &params.Error{Message: `setting annotations for "application-mysql": boom`}},
	})
}

func (s *annotationSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.uuid = utils.MustNewUUID().String()

	s.annotationService = NewMockAnnotationService(ctrl)
	s.authorizer = NewMockAuthorizer(ctrl)

	s.annotationsAPI = &API{
		modelTag:          names.NewModelTag(s.uuid),
		annotationService: s.annotationService,
		authorizer:        s.authorizer,
	}
	return ctrl
}