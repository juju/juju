// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payloads_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v9"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/payload"
	"github.com/juju/juju/worker/uniter/runner/context/mocks"
	"github.com/juju/juju/worker/uniter/runner/context/payloads"
)

type contextSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&contextSuite{})

func (s *contextSuite) TestNewContext(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := mocks.NewMockPayloadAPIClient(ctrl)

	pl := payload.Payload{
		PayloadClass: charm.PayloadClass{
			Name: "class",
		},
	}
	client.EXPECT().List().Return([]payload.Result{{
		ID: "id",
		Payload: &payload.FullPayloadInfo{
			Payload: pl,
			Machine: "1",
		},
	}}, nil)

	ctx, err := payloads.NewContext(client)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(payloads.ContextPayloads(ctx), jc.DeepEquals, map[string]payload.Payload{
		"class": pl,
	})
	result, err := ctx.Payloads()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []payload.Payload{pl})

}

func (s *contextSuite) TestTrackPayloads(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := mocks.NewMockPayloadAPIClient(ctrl)

	pl := payload.Payload{
		ID: "id",
		PayloadClass: charm.PayloadClass{
			Name: "class",
		},
	}
	client.EXPECT().List().Return([]payload.Result{{
		ID: "id",
		Payload: &payload.FullPayloadInfo{
			Payload: pl,
			Machine: "1",
		},
	}}, nil)

	pl2 := payload.Payload{
		ID:     "id",
		Status: "starting",
		Unit:   "a/0",
		PayloadClass: charm.PayloadClass{
			Name: "class2",
			Type: "type",
		},
	}

	ctx, err := payloads.NewContext(client)
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.TrackPayload(pl2)
	c.Assert(err, jc.ErrorIsNil)

	result, err := ctx.Payloads()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []payload.Payload{pl, pl2})
	c.Assert(payloads.ContextPayloads(ctx), jc.DeepEquals, map[string]payload.Payload{
		"class/id": pl,
	})
}

func (s *contextSuite) TestTrackPayloadsFlush(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := mocks.NewMockPayloadAPIClient(ctrl)

	pl := payload.Payload{
		ID: "id",
		PayloadClass: charm.PayloadClass{
			Name: "class",
		},
	}
	client.EXPECT().List().Return([]payload.Result{{
		ID: "id",
		Payload: &payload.FullPayloadInfo{
			Payload: pl,
			Machine: "1",
		},
	}}, nil)

	pl2 := payload.Payload{
		ID:     "id",
		Status: "starting",
		Unit:   "a/0",
		PayloadClass: charm.PayloadClass{
			Name: "class2",
			Type: "type",
		},
	}
	client.EXPECT().Track(pl2)

	ctx, err := payloads.NewContext(client)
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.TrackPayload(pl2)
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.FlushPayloads()
	c.Assert(err, jc.ErrorIsNil)

	result, err := ctx.Payloads()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []payload.Payload{pl, pl2})
	c.Assert(payloads.ContextPayloads(ctx), jc.DeepEquals, map[string]payload.Payload{
		"class/id":  pl,
		"class2/id": pl2,
	})
}

func (s *contextSuite) TestFlushNotDirty(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := mocks.NewMockPayloadAPIClient(ctrl)

	pl := payload.Payload{
		ID: "id",
		PayloadClass: charm.PayloadClass{
			Name: "class",
		},
	}
	client.EXPECT().List().Return([]payload.Result{{
		ID: "id",
		Payload: &payload.FullPayloadInfo{
			Payload: pl,
			Machine: "1",
		},
	}}, nil)

	ctx, err := payloads.NewContext(client)
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.FlushPayloads()
	c.Assert(err, jc.ErrorIsNil)

	result, err := ctx.Payloads()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []payload.Payload{pl})
	c.Assert(payloads.ContextPayloads(ctx), jc.DeepEquals, map[string]payload.Payload{
		"class/id": pl,
	})
}

func (s *contextSuite) TestTrackOverwritePayloads(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := mocks.NewMockPayloadAPIClient(ctrl)

	pl := payload.Payload{
		ID:     "id",
		Status: "starting",
		PayloadClass: charm.PayloadClass{
			Name: "class",
			Type: "type",
		},
		Unit: "a/0",
	}
	client.EXPECT().List().Return([]payload.Result{{
		ID: "id",
		Payload: &payload.FullPayloadInfo{
			Payload: pl,
			Machine: "1",
		},
	}}, nil)

	pl.Status = "stopping"
	client.EXPECT().Track(pl)

	ctx, err := payloads.NewContext(client)
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.TrackPayload(pl)
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.FlushPayloads()
	c.Assert(err, jc.ErrorIsNil)

	result, err := ctx.Payloads()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []payload.Payload{pl})
	c.Assert(payloads.ContextPayloads(ctx), jc.DeepEquals, map[string]payload.Payload{
		"class/id": pl,
	})
}

func (s *contextSuite) TestUnTrackPayloads(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := mocks.NewMockPayloadAPIClient(ctrl)

	pl := payload.Payload{
		ID: "id",
		PayloadClass: charm.PayloadClass{
			Name: "class",
		},
	}
	client.EXPECT().List().Return([]payload.Result{{
		ID: "id",
		Payload: &payload.FullPayloadInfo{
			Payload: pl,
			Machine: "1",
		},
	}}, nil)

	client.EXPECT().Untrack("class/id")

	ctx, err := payloads.NewContext(client)
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.UntrackPayload("class", "id")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(payloads.ContextPayloads(ctx), jc.DeepEquals, map[string]payload.Payload{})
}

func (s *contextSuite) TestSetPayloadStatus(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := mocks.NewMockPayloadAPIClient(ctrl)

	pl := payload.Payload{
		ID: "id",
		PayloadClass: charm.PayloadClass{
			Name: "class",
		},
	}
	client.EXPECT().List().Return([]payload.Result{{
		ID: "id",
		Payload: &payload.FullPayloadInfo{
			Payload: pl,
			Machine: "1",
		},
	}}, nil)

	client.EXPECT().SetStatus("stopping", "class/id")

	ctx, err := payloads.NewContext(client)
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.SetPayloadStatus("class", "id", "stopping")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *contextSuite) TestGetPayload(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := mocks.NewMockPayloadAPIClient(ctrl)

	pl := payload.Payload{
		ID: "id",
		PayloadClass: charm.PayloadClass{
			Name: "class",
		},
	}
	client.EXPECT().List().Return([]payload.Result{{
		ID: "id",
		Payload: &payload.FullPayloadInfo{
			Payload: pl,
			Machine: "1",
		},
	}}, nil)

	ctx, err := payloads.NewContext(client)
	c.Assert(err, jc.ErrorIsNil)
	result, err := ctx.GetPayload("class", "id")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, &pl)
}

func (s *contextSuite) TestTrackedPayload(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := mocks.NewMockPayloadAPIClient(ctrl)

	pl := payload.Payload{
		ID:     "id",
		Status: "starting",
		PayloadClass: charm.PayloadClass{
			Name: "class",
			Type: "type",
		},
		Unit: "a/0",
	}
	client.EXPECT().List().Return([]payload.Result{{
		ID: "id",
		Payload: &payload.FullPayloadInfo{
			Payload: pl,
			Machine: "1",
		},
	}}, nil)

	pl.Status = "stopping"

	ctx, err := payloads.NewContext(client)
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.TrackPayload(pl)
	c.Assert(err, jc.ErrorIsNil)
	result, err := ctx.GetPayload("class", "id")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, &pl)
}
