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

type getSuite struct {
	store          *jujuclient.MemStore
	annotationsAPI *MockGetAnnotationsAPI
}

var _ = gc.Suite(&getSuite{})

func (s *getSuite) SetUpTest(c *gc.C) {
	store := jujuclient.NewMemStore()
	store.Controllers["mycontroller"] = jujuclient.ControllerDetails{}
	store.CurrentControllerName = "mycontroller"
	s.store = store
}

func (s *getSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.annotationsAPI = NewMockGetAnnotationsAPI(ctrl)
	return ctrl
}

func (s *getSuite) TestGetAnnotations(c *gc.C) {
	defer s.setup(c).Finish()

	s.annotationsAPI.EXPECT().Get(
		[]string{"model-e73a80f1-88d2-4d99-8e51-7a640b388399"},
	).Return([]params.AnnotationsGetResult{{
		EntityTag: "model-e73a80f1-88d2-4d99-8e51-7a640b388399",
		Annotations: map[string]string{
			"foo":  "bar",
			"pink": "floyd",
		},
	}}, nil)
	s.annotationsAPI.EXPECT().Close().Return(nil)

	ctx, err := cmdtesting.RunCommand(
		c,
		annotations.NewGetCommandForTest(
			s.store,
			s.annotationsAPI,
		),
		"model-e73a80f1-88d2-4d99-8e51-7a640b388399",
	)
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, `Resource                                    Annotations  Error
model-e73a80f1-88d2-4d99-8e51-7a640b388399  foo=bar        
                                            pink=floyd     
`)
}
