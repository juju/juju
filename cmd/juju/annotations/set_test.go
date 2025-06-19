// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package annotations_test

import (
	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/annotations"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

type setSuite struct {
	store          *jujuclient.MemStore
	annotationsAPI *MockSetAnnotationsAPI
}

var _ = gc.Suite(&setSuite{})

func (s *setSuite) SetUpTest(c *gc.C) {
	store := jujuclient.NewMemStore()
	store.Controllers["mycontroller"] = jujuclient.ControllerDetails{}
	store.CurrentControllerName = "mycontroller"
	s.store = store
}

func (s *setSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.annotationsAPI = NewMockSetAnnotationsAPI(ctrl)
	return ctrl
}

func (s *setSuite) TestSetAnnotations(c *gc.C) {
	defer s.setup(c).Finish()

	s.annotationsAPI.EXPECT().Set(
		map[string]map[string]string{
			"model-e73a80f1-88d2-4d99-8e51-7a640b388399": {
				"foo": "bar",
			}}).Return([]params.ErrorResult{}, nil)
	s.annotationsAPI.EXPECT().Close().Return(nil)

	_, err := cmdtesting.RunCommand(
		c,
		annotations.NewSetCommandForTest(
			s.store,
			s.annotationsAPI,
		),
		"model-e73a80f1-88d2-4d99-8e51-7a640b388399",
		"foo=bar",
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *setSuite) TestSetAnnotationsError(c *gc.C) {
	defer s.setup(c).Finish()

	s.annotationsAPI.EXPECT().Set(
		map[string]map[string]string{
			"model-e73a80f1-88d2-4d99-8e51-7a640b388399": {
				"foo": "bar",
			}}).Return([]params.ErrorResult{{
		Error: &params.Error{
			Message: "test error",
			Code:    "test code",
		},
	}}, nil)
	s.annotationsAPI.EXPECT().Close().Return(nil)

	_, err := cmdtesting.RunCommand(
		c,
		annotations.NewSetCommandForTest(
			s.store,
			s.annotationsAPI,
		),
		"model-e73a80f1-88d2-4d99-8e51-7a640b388399",
		"foo=bar",
	)
	c.Assert(err, jc.DeepEquals, &params.Error{
		Message: "test error",
		Code:    "test code",
	})
}
