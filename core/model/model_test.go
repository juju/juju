// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v9"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
)

type ModelSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ModelSuite{})

func (*ModelSuite) TestValidateSeries(c *gc.C) {
	for _, t := range []struct {
		modelType model.ModelType
		meta      charm.Meta
		valid     bool
	}{
		{model.IAAS, charm.Meta{Series: []string{"bionic"}}, true},
		{model.IAAS, charm.Meta{Series: []string{"kubernetes"}}, false},
		{model.IAAS, charm.Meta{Containers: map[string]charm.Container{"focal": {}}}, false},
		{model.CAAS, charm.Meta{Series: []string{"bionic"}}, false},
		{model.CAAS, charm.Meta{Series: []string{"kubernetes"}}, true},
		{model.CAAS, charm.Meta{Containers: map[string]charm.Container{"focal": {}}}, true},
	} {
		ctrl := gomock.NewController(c)
		defer ctrl.Finish()
		cm := NewMockCharmMeta(ctrl)
		cm.EXPECT().Meta().Return(&t.meta)
		if len(t.meta.Containers) > 0 {
			cm.EXPECT().Manifest().Return(&charm.Manifest{
				Bases: []charm.Base{
					{Name: "ubuntu", Channel: charm.Channel{
						Track: "20.04",
						Risk:  "stable",
					}},
				},
			}).AnyTimes()
		} else {
			cm.EXPECT().Manifest().Return(&charm.Manifest{}).AnyTimes()
		}

		err := model.ValidateModelTarget(t.modelType, cm)
		if t.valid {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, jc.Satisfies, errors.IsNotValid)
		}
	}
}

func (*ModelSuite) TestValidateBranchName(c *gc.C) {
	for _, t := range []struct {
		branchName string
		valid      bool
	}{
		{"", false},
		{model.GenerationMaster, false},
		{"something else", true},
	} {
		err := model.ValidateBranchName(t.branchName)
		if t.valid {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, jc.Satisfies, errors.IsNotValid)
		}
	}
}
