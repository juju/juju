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
	for _, t := range []struct {
		args  model.ValidateSeriesArgs
		valid bool
	}{
		{model.ValidateSeriesArgs{model.IAAS, "test", "bionic", charm.FormatV1}, true},
		{model.ValidateSeriesArgs{model.IAAS, "test", "kubernetes", charm.FormatV1}, false},
		{model.ValidateSeriesArgs{model.CAAS, "test", "bionic", charm.FormatV1}, false},
		{model.ValidateSeriesArgs{model.CAAS, "test", "kubernetes", charm.FormatV1}, true},
	} {
		err := model.ValidateSeries(t.args)
		if t.valid {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, jc.Satisfies, errors.IsNotValid)
		}
	}
}

func (*ModelSuite) TestValidateSeriesNewCharm(c *gc.C) {
	for _, t := range []struct {
		args  model.ValidateSeriesArgs
		valid bool
	}{
		{model.ValidateSeriesArgs{model.IAAS, "test", "bionic", charm.FormatV2}, true},
		{model.ValidateSeriesArgs{model.IAAS, "test", "bionic", charm.FormatV2}, true},
		{model.ValidateSeriesArgs{model.CAAS, "test", "bionic", charm.FormatV2}, true},
		{model.ValidateSeriesArgs{model.CAAS, "test", "bionic", charm.FormatV2}, true},
	} {
		err := model.ValidateSeries(t.args)
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
