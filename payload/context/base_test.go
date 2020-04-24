// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"reflect"

	"github.com/juju/charm/v7"
	"github.com/juju/errors"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/payload"
	"github.com/juju/juju/payload/context"
	"github.com/juju/juju/worker/uniter/runner/jujuc/jujuctesting"
)

type baseSuite struct {
	jujuctesting.ContextSuite
	payload payload.Payload
}

func (s *baseSuite) SetUpTest(c *gc.C) {
	s.ContextSuite.SetUpTest(c)

	s.payload = s.newPayload("payload A", "docker", "", "")
}

func (s *baseSuite) newPayload(name, ptype, id, status string) payload.Payload {
	pl := payload.Payload{
		PayloadClass: charm.PayloadClass{
			Name: name,
			Type: ptype,
		},
		ID:     id,
		Status: status,
		Unit:   "a-application/0",
	}
	return pl
}

func (s *baseSuite) NewHookContext() (*stubHookContext, *jujuctesting.ContextInfo) {
	ctx, info := s.ContextSuite.NewHookContext()
	return &stubHookContext{ctx}, info
}

func checkPayloads(c *gc.C, payloads, expected []payload.Payload) {
	if !c.Check(payloads, gc.HasLen, len(expected)) {
		return
	}
	for _, wl := range payloads {
		matched := false
		for _, expPayload := range expected {
			if reflect.DeepEqual(wl, expPayload) {
				matched = true
				break
			}
		}
		if !matched {
			c.Errorf("%#v != %#v", payloads, expected)
			return
		}
	}
}

type stubHookContext struct {
	*jujuctesting.Context
}

func (c stubHookContext) Component(name string) (context.Component, error) {
	found, err := c.Context.Component(name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	compCtx, ok := found.(context.Component)
	if !ok && found != nil {
		return nil, errors.Errorf("wrong component context type registered: %T", found)
	}
	return compCtx, nil
}

var _ context.Component = (*stubContextComponent)(nil)

type stubContextComponent struct {
	stub     *testing.Stub
	payloads map[string]payload.Payload
	untracks map[string]struct{}
}

func newStubContextComponent(stub *testing.Stub) *stubContextComponent {
	return &stubContextComponent{
		stub:     stub,
		payloads: make(map[string]payload.Payload),
		untracks: make(map[string]struct{}),
	}
}

func (c *stubContextComponent) Get(class, id string) (*payload.Payload, error) {
	c.stub.AddCall("Get", class, id)
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	fullID := payload.BuildID(class, id)
	info, ok := c.payloads[fullID]
	if !ok {
		return nil, errors.NotFoundf(id)
	}
	return &info, nil
}

func (c *stubContextComponent) List() ([]string, error) {
	c.stub.AddCall("List")
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	var fullIDs []string
	for k := range c.payloads {
		fullIDs = append(fullIDs, k)
	}
	return fullIDs, nil
}

func (c *stubContextComponent) Track(pl payload.Payload) error {
	c.stub.AddCall("Track", pl)
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	c.payloads[pl.FullID()] = pl
	return nil
}

func (c *stubContextComponent) Untrack(class, id string) error {
	c.stub.AddCall("Untrack", class, id)

	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	fullID := payload.BuildID(class, id)
	c.untracks[fullID] = struct{}{}
	return nil
}

func (c *stubContextComponent) SetStatus(class, id, status string) error {
	c.stub.AddCall("SetStatus", class, id, status)
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	fullID := payload.BuildID(class, id)
	pl := c.payloads[fullID]
	pl.Status = status
	return nil
}

func (c *stubContextComponent) Flush() error {
	c.stub.AddCall("Flush")
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

type stubAPIClient struct {
	stub *testing.Stub
	// TODO(ericsnow) Use id for the key rather than Info.ID().
	payloads map[string]payload.Payload
}

func newStubAPIClient(stub *testing.Stub) *stubAPIClient {
	return &stubAPIClient{
		stub:     stub,
		payloads: make(map[string]payload.Payload),
	}
}

func (c *stubAPIClient) setNew(fullIDs ...string) []payload.Payload {
	var payloads []payload.Payload
	for _, id := range fullIDs {
		name, pluginID := payload.ParseID(id)
		if name == "" {
			panic("missing name")
		}
		if pluginID == "" {
			panic("missing id")
		}
		wl := payload.Payload{
			PayloadClass: charm.PayloadClass{
				Name: name,
				Type: "myplugin",
			},
			ID:     pluginID,
			Status: payload.StateRunning,
		}
		c.payloads[id] = wl
		payloads = append(payloads, wl)
	}
	return payloads
}

func (c *stubAPIClient) List(fullIDs ...string) ([]payload.Result, error) {
	c.stub.AddCall("List", fullIDs)
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	var results []payload.Result
	if fullIDs == nil {
		for id, pl := range c.payloads {
			results = append(results, payload.Result{
				ID:      id,
				Payload: &payload.FullPayloadInfo{Payload: pl},
			})
		}
	} else {
		for _, id := range fullIDs {
			pl, ok := c.payloads[id]
			if !ok {
				return nil, errors.NotFoundf("pl %q", id)
			}
			results = append(results, payload.Result{
				ID:      id,
				Payload: &payload.FullPayloadInfo{Payload: pl},
			})
		}
	}
	return results, nil
}

func (c *stubAPIClient) Track(payloads ...payload.Payload) ([]payload.Result, error) {
	c.stub.AddCall("Track", payloads)
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	var results []payload.Result
	for _, pl := range payloads {
		id := pl.FullID()
		c.payloads[id] = pl
		results = append(results, payload.Result{
			ID:      id,
			Payload: &payload.FullPayloadInfo{Payload: pl},
		})
	}
	return results, nil
}

func (c *stubAPIClient) Untrack(fullIDs ...string) ([]payload.Result, error) {
	c.stub.AddCall("Untrack", fullIDs)
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	errs := []payload.Result{}
	for _, id := range fullIDs {
		delete(c.payloads, id)
		errs = append(errs, payload.Result{ID: id})
	}
	return errs, nil
}

func (c *stubAPIClient) SetStatus(status string, fullIDs ...string) ([]payload.Result, error) {
	c.stub.AddCall("SetStatus", status, fullIDs)
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	errs := []payload.Result{}
	for _, id := range fullIDs {
		pl := c.payloads[id]
		pl.Status = status
		errs = append(errs, payload.Result{ID: id})
	}

	return errs, nil
}
