// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
)

type removeSuite struct{}

var _ = Suite(&removeSuite{})

func (*removeSuite) TestRemove(c *C) {
	st := &fakeRemoverState{
		entities: map[string]*fakeRemover{
			"x0": &fakeRemover{errEnsureDead: fmt.Errorf("x0 EnsureDead fails")},
			"x1": &fakeRemover{errRemove: fmt.Errorf("x1 Remove fails")},
			"x2": &fakeRemover{},
			"x3": &fakeRemover{},
			"x4": &fakeRemover{err: fmt.Errorf("x4 error")},
		},
	}
	getCanModify := func() (common.AuthFunc, error) {
		return func(tag string) bool {
			switch tag {
			case "x0", "x1", "x2", "x4":
				return true
			}
			return false
		}, nil
	}
	r := common.NewRemover(st, getCanModify)
	entities := params.Entities{[]params.Entity{
		{"x0"}, {"x1"}, {"x2"}, {"x3"}, {"x4"}, {"x5"},
	}}
	result, err := r.Remove(entities)
	c.Assert(err, IsNil)
	unauth := &params.Error{
		Message: "permission denied",
		Code:    params.CodeUnauthorized,
	}
	c.Assert(result, DeepEquals, params.ErrorResults{
		Errors: []*params.Error{
			&params.Error{Message: "x0 EnsureDead fails"},
			&params.Error{Message: "x1 Remove fails"},
			nil,
			unauth,
			&params.Error{Message: "x4 error"},
			unauth,
		},
	})
}

func (*removeSuite) TestRemoveError(c *C) {
	getCanModify := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	r := common.NewRemover(&fakeRemoverState{}, getCanModify)
	_, err := r.Remove(params.Entities{[]params.Entity{{"x0"}}})
	c.Assert(err, ErrorMatches, "pow")
}

func (*removeSuite) TestRemoveNoArgsNoError(c *C) {
	getCanModify := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	r := common.NewRemover(&fakeRemoverState{}, getCanModify)
	result, err := r.Remove(params.Entities{})
	c.Assert(err, IsNil)
	c.Assert(result.Errors, HasLen, 0)
}

type fakeRemoverState struct {
	entities map[string]*fakeRemover
}

func (st *fakeRemoverState) Remover(tag string) (state.Remover, error) {
	if remover, ok := st.entities[tag]; ok {
		if remover.err != nil {
			return nil, remover.err
		}
		return remover, nil
	}
	return nil, errors.NotFoundf("entity %q", tag)
}

type fakeRemover struct {
	err           error
	errEnsureDead error
	errRemove     error
}

func (r *fakeRemover) Tag() string {
	panic("not needed")
}

func (r *fakeRemover) EnsureDead() error {
	return r.errEnsureDead
}

func (r *fakeRemover) Remove() error {
	return r.errRemove
}
