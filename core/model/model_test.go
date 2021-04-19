// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"github.com/juju/charm/v8"
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
	type meta struct {
		Series     []string
		Containers map[string]charm.Container
	}
	for _, t := range []struct {
		modelType model.ModelType
		meta      meta
		valid     bool
	}{
		{model.IAAS, meta{Series: []string{"bionic"}}, true},
		{model.IAAS, meta{Series: []string{"kubernetes"}}, false},
		{model.IAAS, meta{Containers: map[string]charm.Container{"focal": {}}}, false},
		{model.CAAS, meta{Series: []string{"bionic"}}, false},
		{model.CAAS, meta{Series: []string{"kubernetes"}}, true},
		{model.CAAS, meta{Containers: map[string]charm.Container{"focal": {}}}, true},
	} {
		err := model.ValidateModelTarget(t.modelType, t.meta.Series, t.meta.Containers)
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
