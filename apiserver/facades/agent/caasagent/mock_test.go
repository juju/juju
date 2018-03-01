// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasagent_test

import (
	"github.com/juju/testing"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/facades/agent/caasagent"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type mockState struct {
	testing.Stub
	model mockModel
}

func (st *mockState) Model() (caasagent.Model, error) {
	st.MethodCall(st, "Model")
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	return &st.model, nil
}

type mockModel struct {
	testing.Stub
}

func (st *mockModel) Name() string {
	return "some-model"
}

func (st *mockModel) UUID() string {
	return coretesting.ModelTag.Id()
}

func (st *mockModel) Type() state.ModelType {
	return state.ModelTypeCAAS
}

func (st *mockModel) Owner() names.UserTag {
	return names.NewUserTag("fred")
}

func (st *mockModel) ModelTag() names.ModelTag {
	return coretesting.ModelTag
}
