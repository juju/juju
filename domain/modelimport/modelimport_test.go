// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelimport

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/export"
	"github.com/juju/juju/domain/export/types/v4_0_12"
	"github.com/juju/juju/domain/export/types/v4_1_0"
	"github.com/juju/juju/domain/modelimport/transformer"
)

type modelimportSuite struct{}

func TestModelimport(t *testing.T) {
	tc.Run(t, &modelimportSuite{})
}

func (s *modelimportSuite) TestNewTransformerTargetsLatestSupportedPayloadVersion(c *tc.C) {
	tr, err := NewTransformer()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(tr.Target(), tc.Equals, export.LatestSupportedPayloadVersion())
}

// TestTransformSameVersionIsNoOp asserts a payload already at the target
// version passes through unchanged.
func (s *modelimportSuite) TestTransformSameVersionIsNoOp(c *tc.C) {
	tr, err := NewTransformer()
	c.Assert(err, tc.ErrorIsNil)

	payload := v4_1_0.ModelExport{
		RelationUnitSetting: []v4_1_0.RelationUnitSetting{{
			RelationUnitUUID: "ru-1", Key: "k", Value: "v",
		}},
	}
	got, err := tr.Transform(c.Context(), semversion.MustParse("4.1.0"), payload)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, payload)
}

// TestTransformAppliesChainFromFloor asserts a payload stamped with the
// window floor version is walked through the registered chain, including
// the relation setting delta: values become NOT NULL in 4.1.0, and rows
// with NULL (or empty) values are dropped because the 4.1.0 column
// forbids empty strings.
func (s *modelimportSuite) TestTransformAppliesChainFromFloor(c *tc.C) {
	tr, err := NewTransformer()
	c.Assert(err, tc.ErrorIsNil)

	value := "v"
	payload := v4_0_12.ModelExport{
		RelationUnitSetting: []v4_0_12.RelationUnitSetting{{
			RelationUnitUUID: "ru-1", Key: "k", Value: &value,
		}, {
			RelationUnitUUID: "ru-2", Key: "k2", Value: nil,
		}},
	}
	got, err := tr.Transform(c.Context(), semversion.MustParse("4.0.12"), payload)
	c.Assert(err, tc.ErrorIsNil)

	transformed, ok := got.(v4_1_0.ModelExport)
	c.Assert(ok, tc.IsTrue)
	c.Assert(transformed.RelationUnitSetting, tc.HasLen, 1)
	c.Check(transformed.RelationUnitSetting[0], tc.DeepEquals, v4_1_0.RelationUnitSetting{
		RelationUnitUUID: "ru-1", Key: "k", Value: "v",
	})
}

// TestTransformRejectsBelowFloor asserts a payload from an older patch of a
// listed line is rejected: only the latest patch of each window line is
// supported, older sources must upgrade their controller in place first.
func (s *modelimportSuite) TestTransformRejectsBelowFloor(c *tc.C) {
	tr, err := NewTransformer()
	c.Assert(err, tc.ErrorIsNil)

	_, err = tr.Transform(c.Context(), semversion.MustParse("4.0.6"), v4_0_12.ModelExport{})
	c.Assert(err, tc.ErrorIs, transformer.ErrUnknownSourceVersion)
}
