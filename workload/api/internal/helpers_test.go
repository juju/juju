// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/workload"
)

type internalHelpersSuite struct{}

var _ = gc.Suite(&internalHelpersSuite{})

func (internalHelpersSuite) TestNewWorkloadResultOkay(c *gc.C) {
	id := "ce5bc2a7-65d8-4800-8199-a7c3356ab309"
	result := NewWorkloadResult(id, nil)

	c.Check(result, jc.DeepEquals, WorkloadResult{
		Entity: params.Entity{
			Tag: names.NewPayloadTag(id).String(),
		},
		Workload: nil,
		NotFound: false,
		Error:    nil,
	})
}

func (internalHelpersSuite) TestNewWorkloadResultError(c *gc.C) {
	id := "ce5bc2a7-65d8-4800-8199-a7c3356ab309"
	err := errors.New("<failure>")
	result := NewWorkloadResult(id, err)

	c.Check(result, jc.DeepEquals, WorkloadResult{
		Entity: params.Entity{
			Tag: names.NewPayloadTag(id).String(),
		},
		Workload: nil,
		NotFound: false,
		Error:    common.ServerError(err),
	})
}

func (internalHelpersSuite) TestNewWorkloadResultNotFound(c *gc.C) {
	id := "ce5bc2a7-65d8-4800-8199-a7c3356ab309"
	err := errors.NotFoundf("workload %q", id)
	result := NewWorkloadResult(id, err)

	c.Check(result, jc.DeepEquals, WorkloadResult{
		Entity: params.Entity{
			Tag: names.NewPayloadTag(id).String(),
		},
		Workload: nil,
		NotFound: true,
		Error:    common.ServerError(err),
	})
}

func (internalHelpersSuite) TestAPI2ResultOkay(c *gc.C) {
	id := "ce5bc2a7-65d8-4800-8199-a7c3356ab309"
	result, err := API2Result(WorkloadResult{
		Entity: params.Entity{
			Tag: names.NewPayloadTag(id).String(),
		},
		Workload: nil,
		NotFound: false,
		Error:    nil,
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(result, jc.DeepEquals, workload.Result{
		ID:       id,
		Workload: nil,
		NotFound: false,
		Error:    nil,
	})
}

func (internalHelpersSuite) TestAPI2ResultInfo(c *gc.C) {
	id := "ce5bc2a7-65d8-4800-8199-a7c3356ab309"
	result, err := API2Result(WorkloadResult{
		Entity: params.Entity{
			Tag: names.NewPayloadTag(id).String(),
		},
		NotFound: false,
		Error:    nil,
		Workload: &Workload{
			Definition: WorkloadDefinition{
				Name: "foobar",
				Type: "type",
			},
			Status: WorkloadStatus{
				State: workload.StateRunning,
			},
			Details: WorkloadDetails{
				ID: "idfoo",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(result, jc.DeepEquals, workload.Result{
		ID:       id,
		NotFound: false,
		Error:    nil,
		Workload: &workload.Info{
			PayloadClass: charm.PayloadClass{
				Name: "foobar",
				Type: "type",
			},
			Status: workload.Status{
				State: workload.StateRunning,
			},
			Details: workload.Details{
				ID: "idfoo",
			},
		},
	})
}

func (internalHelpersSuite) TestAPI2ResultError(c *gc.C) {
	id := "ce5bc2a7-65d8-4800-8199-a7c3356ab309"
	failure := errors.New("<failure>")
	result, err := API2Result(WorkloadResult{
		Entity: params.Entity{
			Tag: names.NewPayloadTag(id).String(),
		},
		Workload: nil,
		NotFound: false,
		Error:    common.ServerError(failure),
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(result, jc.DeepEquals, workload.Result{
		ID:       id,
		Workload: nil,
		NotFound: false,
		Error:    failure,
	})
}

func (internalHelpersSuite) TestAPI2ResultNotFound(c *gc.C) {
	id := "ce5bc2a7-65d8-4800-8199-a7c3356ab309"
	notFound := errors.NotFoundf("workload %q", id)
	result, err := API2Result(WorkloadResult{
		Entity: params.Entity{
			Tag: names.NewPayloadTag(id).String(),
		},
		Workload: nil,
		NotFound: false,
		Error:    common.ServerError(notFound),
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(result, jc.DeepEquals, workload.Result{
		ID:       id,
		Workload: nil,
		NotFound: false,
		Error:    notFound,
	})
}

func (internalHelpersSuite) TestResult2apiOkay(c *gc.C) {
	id := "ce5bc2a7-65d8-4800-8199-a7c3356ab309"
	result := Result2api(workload.Result{
		ID:       id,
		Workload: nil,
		NotFound: false,
		Error:    nil,
	})

	c.Check(result, jc.DeepEquals, WorkloadResult{
		Entity: params.Entity{
			Tag: names.NewPayloadTag(id).String(),
		},
		Workload: nil,
		NotFound: false,
		Error:    nil,
	})
}

func (internalHelpersSuite) TestResult2apiInfo(c *gc.C) {
	id := "ce5bc2a7-65d8-4800-8199-a7c3356ab309"
	result := Result2api(workload.Result{
		ID:       id,
		NotFound: false,
		Error:    nil,
		Workload: &workload.Info{
			PayloadClass: charm.PayloadClass{
				Name: "foobar",
				Type: "type",
			},
			Status: workload.Status{
				State: workload.StateRunning,
			},
			Details: workload.Details{
				ID: "idfoo",
			},
		},
	})

	c.Check(result, jc.DeepEquals, WorkloadResult{
		Entity: params.Entity{
			Tag: names.NewPayloadTag(id).String(),
		},
		NotFound: false,
		Error:    nil,
		Workload: &Workload{
			Definition: WorkloadDefinition{
				Name: "foobar",
				Type: "type",
			},
			Status: WorkloadStatus{
				State: workload.StateRunning,
			},
			Details: WorkloadDetails{
				ID: "idfoo",
			},
		},
	})
}

func (internalHelpersSuite) TestResult2apiError(c *gc.C) {
	id := "ce5bc2a7-65d8-4800-8199-a7c3356ab309"
	err := errors.New("<failure>")
	result := Result2api(workload.Result{
		ID:       id,
		Workload: nil,
		NotFound: false,
		Error:    err,
	})

	c.Check(result, jc.DeepEquals, WorkloadResult{
		Entity: params.Entity{
			Tag: names.NewPayloadTag(id).String(),
		},
		Workload: nil,
		NotFound: false,
		Error:    common.ServerError(err),
	})
}

func (internalHelpersSuite) TestResult2apiNotFound(c *gc.C) {
	id := "ce5bc2a7-65d8-4800-8199-a7c3356ab309"
	err := errors.NotFoundf("workload %q", id)
	result := Result2api(workload.Result{
		ID:       id,
		Workload: nil,
		NotFound: false,
		Error:    err,
	})

	c.Check(result, jc.DeepEquals, WorkloadResult{
		Entity: params.Entity{
			Tag: names.NewPayloadTag(id).String(),
		},
		Workload: nil,
		NotFound: false,
		Error:    common.ServerError(err),
	})
}

func (internalHelpersSuite) TestAPI2Workload(c *gc.C) {
	p := Workload{
		Definition: WorkloadDefinition{
			Name: "foobar",
			Type: "type",
		},
		Status: WorkloadStatus{
			State:   workload.StateRunning,
			Blocker: "",
			Message: "okay",
		},
		Labels: []string{},
		Details: WorkloadDetails{
			ID: "idfoo",
			Status: PluginStatus{
				State: "workload status",
			},
		},
	}

	wl := API2Workload(p)
	p2 := Workload2api(wl)
	c.Assert(p2, gc.DeepEquals, p)
	wl2 := API2Workload(p2)
	c.Assert(wl2, gc.DeepEquals, wl)
}

func (internalHelpersSuite) TestWorkload2API(c *gc.C) {
	wl := workload.Info{
		PayloadClass: charm.PayloadClass{
			Name: "foobar",
			Type: "type",
		},
		Status: workload.Status{
			State:   workload.StateRunning,
			Blocker: "",
			Message: "okay",
		},
		Labels: []string{},
		Details: workload.Details{
			ID: "idfoo",
			Status: workload.PluginStatus{
				State: "workload status",
			},
		},
	}

	w := Workload2api(wl)
	wl2 := API2Workload(w)
	c.Assert(wl2, gc.DeepEquals, wl)
	w2 := Workload2api(wl2)
	c.Assert(w2, gc.DeepEquals, w)
}
