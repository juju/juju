// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payloads_test

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	corepayloads "github.com/juju/juju/core/payloads"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/worker/uniter/runner/context/mocks"
	"github.com/juju/juju/internal/worker/uniter/runner/context/payloads"
)

type contextSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&contextSuite{})

func (s *contextSuite) TestNewContext(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := mocks.NewMockPayloadAPIClient(ctrl)

	pl := corepayloads.Payload{
		PayloadClass: charm.PayloadClass{
			Name: "class",
		},
	}
	client.EXPECT().List(gomock.Any()).Return([]corepayloads.Result{{
		ID: "id",
		Payload: &corepayloads.FullPayloadInfo{
			Payload: pl,
			Machine: "1",
		},
	}}, nil)

	ctx, err := payloads.NewContext(context.Background(), client)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(payloads.ContextPayloads(ctx), jc.DeepEquals, map[string]corepayloads.Payload{
		"class": pl,
	})
	result, err := ctx.Payloads()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []corepayloads.Payload{pl})

}

func (s *contextSuite) TestTrackPayloads(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := mocks.NewMockPayloadAPIClient(ctrl)

	pl := corepayloads.Payload{
		ID: "id",
		PayloadClass: charm.PayloadClass{
			Name: "class",
		},
	}
	client.EXPECT().List(gomock.Any()).Return([]corepayloads.Result{{
		ID: "id",
		Payload: &corepayloads.FullPayloadInfo{
			Payload: pl,
			Machine: "1",
		},
	}}, nil)

	pl2 := corepayloads.Payload{
		ID:     "id",
		Status: "starting",
		Unit:   "a/0",
		PayloadClass: charm.PayloadClass{
			Name: "class2",
			Type: "type",
		},
	}

	ctx, err := payloads.NewContext(context.Background(), client)
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.TrackPayload(pl2)
	c.Assert(err, jc.ErrorIsNil)

	result, err := ctx.Payloads()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []corepayloads.Payload{pl, pl2})
	c.Assert(payloads.ContextPayloads(ctx), jc.DeepEquals, map[string]corepayloads.Payload{
		"class/id": pl,
	})
}

func (s *contextSuite) TestTrackPayloadsFlush(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := mocks.NewMockPayloadAPIClient(ctrl)

	pl := corepayloads.Payload{
		ID: "id",
		PayloadClass: charm.PayloadClass{
			Name: "class",
		},
	}
	client.EXPECT().List(gomock.Any()).Return([]corepayloads.Result{{
		ID: "id",
		Payload: &corepayloads.FullPayloadInfo{
			Payload: pl,
			Machine: "1",
		},
	}}, nil)

	pl2 := corepayloads.Payload{
		ID:     "id",
		Status: "starting",
		Unit:   "a/0",
		PayloadClass: charm.PayloadClass{
			Name: "class2",
			Type: "type",
		},
	}
	client.EXPECT().Track(gomock.Any(), pl2)

	ctx, err := payloads.NewContext(context.Background(), client)
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.TrackPayload(pl2)
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.FlushPayloads(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	result, err := ctx.Payloads()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []corepayloads.Payload{pl, pl2})
	c.Assert(payloads.ContextPayloads(ctx), jc.DeepEquals, map[string]corepayloads.Payload{
		"class/id":  pl,
		"class2/id": pl2,
	})
}

func (s *contextSuite) TestFlushNotDirty(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := mocks.NewMockPayloadAPIClient(ctrl)

	pl := corepayloads.Payload{
		ID: "id",
		PayloadClass: charm.PayloadClass{
			Name: "class",
		},
	}
	client.EXPECT().List(gomock.Any()).Return([]corepayloads.Result{{
		ID: "id",
		Payload: &corepayloads.FullPayloadInfo{
			Payload: pl,
			Machine: "1",
		},
	}}, nil)

	ctx, err := payloads.NewContext(context.Background(), client)
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.FlushPayloads(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	result, err := ctx.Payloads()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []corepayloads.Payload{pl})
	c.Assert(payloads.ContextPayloads(ctx), jc.DeepEquals, map[string]corepayloads.Payload{
		"class/id": pl,
	})
}

func (s *contextSuite) TestTrackOverwritePayloads(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := mocks.NewMockPayloadAPIClient(ctrl)

	pl := corepayloads.Payload{
		ID:     "id",
		Status: "starting",
		PayloadClass: charm.PayloadClass{
			Name: "class",
			Type: "type",
		},
		Unit: "a/0",
	}
	client.EXPECT().List(gomock.Any()).Return([]corepayloads.Result{{
		ID: "id",
		Payload: &corepayloads.FullPayloadInfo{
			Payload: pl,
			Machine: "1",
		},
	}}, nil)

	pl.Status = "stopping"
	client.EXPECT().Track(gomock.Any(), pl)

	ctx, err := payloads.NewContext(context.Background(), client)
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.TrackPayload(pl)
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.FlushPayloads(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	result, err := ctx.Payloads()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []corepayloads.Payload{pl})
	c.Assert(payloads.ContextPayloads(ctx), jc.DeepEquals, map[string]corepayloads.Payload{
		"class/id": pl,
	})
}

func (s *contextSuite) TestUnTrackPayloads(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := mocks.NewMockPayloadAPIClient(ctrl)

	pl := corepayloads.Payload{
		ID: "id",
		PayloadClass: charm.PayloadClass{
			Name: "class",
		},
	}
	client.EXPECT().List(gomock.Any()).Return([]corepayloads.Result{{
		ID: "id",
		Payload: &corepayloads.FullPayloadInfo{
			Payload: pl,
			Machine: "1",
		},
	}}, nil)

	client.EXPECT().Untrack(gomock.Any(), "class/id")

	ctx, err := payloads.NewContext(context.Background(), client)
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.UntrackPayload(context.Background(), "class", "id")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(payloads.ContextPayloads(ctx), jc.DeepEquals, map[string]corepayloads.Payload{})
}

func (s *contextSuite) TestSetPayloadStatus(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := mocks.NewMockPayloadAPIClient(ctrl)

	pl := corepayloads.Payload{
		ID: "id",
		PayloadClass: charm.PayloadClass{
			Name: "class",
		},
	}
	client.EXPECT().List(gomock.Any()).Return([]corepayloads.Result{{
		ID: "id",
		Payload: &corepayloads.FullPayloadInfo{
			Payload: pl,
			Machine: "1",
		},
	}}, nil)

	client.EXPECT().SetStatus(gomock.Any(), "stopping", "class/id")

	ctx, err := payloads.NewContext(context.Background(), client)
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.SetPayloadStatus(context.Background(), "class", "id", "stopping")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *contextSuite) TestGetPayload(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := mocks.NewMockPayloadAPIClient(ctrl)

	pl := corepayloads.Payload{
		ID: "id",
		PayloadClass: charm.PayloadClass{
			Name: "class",
		},
	}
	client.EXPECT().List(gomock.Any()).Return([]corepayloads.Result{{
		ID: "id",
		Payload: &corepayloads.FullPayloadInfo{
			Payload: pl,
			Machine: "1",
		},
	}}, nil)

	ctx, err := payloads.NewContext(context.Background(), client)
	c.Assert(err, jc.ErrorIsNil)
	result, err := ctx.GetPayload("class", "id")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, &pl)
}

func (s *contextSuite) TestTrackedPayload(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := mocks.NewMockPayloadAPIClient(ctrl)

	pl := corepayloads.Payload{
		ID:     "id",
		Status: "starting",
		PayloadClass: charm.PayloadClass{
			Name: "class",
			Type: "type",
		},
		Unit: "a/0",
	}
	client.EXPECT().List(gomock.Any()).Return([]corepayloads.Result{{
		ID: "id",
		Payload: &corepayloads.FullPayloadInfo{
			Payload: pl,
			Machine: "1",
		},
	}}, nil)

	pl.Status = "stopping"

	ctx, err := payloads.NewContext(context.Background(), client)
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.TrackPayload(pl)
	c.Assert(err, jc.ErrorIsNil)
	result, err := ctx.GetPayload("class", "id")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, &pl)
}
