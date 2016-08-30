// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/imagemetadata"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/testing"
)

type funcMetadataSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&funcMetadataSuite{})

func (s *funcMetadataSuite) TestProcessErrorsNil(c *gc.C) {
	s.assertProcessErrorsNone(c, nil)
}

func (s *funcMetadataSuite) TestProcessErrorsEmpty(c *gc.C) {
	s.assertProcessErrorsNone(c, []params.ErrorResult{})
}

func (s *funcMetadataSuite) TestProcessErrorsNilError(c *gc.C) {
	s.assertProcessErrorsNone(c, []params.ErrorResult{{Error: nil}})
}

func (s *funcMetadataSuite) TestProcessErrorsEmptyMessageError(c *gc.C) {
	s.assertProcessErrorsNone(c, []params.ErrorResult{{Error: &params.Error{Message: ""}}})
}

func (s *funcMetadataSuite) TestProcessErrorsFullError(c *gc.C) {
	msg := "my bad"

	errs := []params.ErrorResult{{Error: &params.Error{Message: msg}}}

	expected := fmt.Sprintf(`
saving some image metadata:
%v`[1:], msg)

	s.assertProcessErrors(c, errs, expected)
}

func (s *funcMetadataSuite) TestProcessErrorsMany(c *gc.C) {
	msg1 := "my bad"
	msg2 := "my good"

	errs := []params.ErrorResult{
		{Error: &params.Error{Message: msg1}},
		{Error: &params.Error{Message: ""}},
		{Error: nil},
		{Error: &params.Error{Message: msg2}},
	}

	expected := fmt.Sprintf(`
saving some image metadata:
%v
%v`[1:], msg1, msg2)

	s.assertProcessErrors(c, errs, expected)
}

var process = imagemetadata.ProcessErrors

func (s *funcMetadataSuite) assertProcessErrorsNone(c *gc.C, errs []params.ErrorResult) {
	c.Assert(process(errs), jc.ErrorIsNil)
}

func (s *funcMetadataSuite) assertProcessErrors(c *gc.C, errs []params.ErrorResult, expected string) {
	c.Assert(process(errs), gc.ErrorMatches, expected)
}
