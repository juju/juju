// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
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
		series    string
		valid     bool
	}{
		{model.IAAS, "bionic", true},
		{model.IAAS, "kubernetes", false},
		{model.CAAS, "bionic", false},
		{model.CAAS, "kubernetes", true},
	} {
		err := model.ValidateSeries(t.modelType, t.series, charm.FormatV1)
		if t.valid {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, jc.Satisfies, errors.IsNotValid)
		}
	}
}

func (*ModelSuite) TestValidateSeriesNewCharm(c *gc.C) {
	for _, t := range []struct {
		modelType model.ModelType
		series    string
		valid     bool
	}{
		{model.IAAS, "bionic", true},
		{model.IAAS, "bionic", true},
		{model.CAAS, "bionic", true},
		{model.CAAS, "bionic", true},
	} {
		err := model.ValidateSeries(t.modelType, t.series, charm.FormatV2)
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
