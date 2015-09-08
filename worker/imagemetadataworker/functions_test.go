// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadataworker_test

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/imagemetadataworker"
)

type funcMetadataSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&funcMetadataSuite{})

func (s *funcMetadataSuite) TestVersionSeriesValid(c *gc.C) {
	s.assertSeriesForVersion(c, "14.04", "trusty")
}

func (s *funcMetadataSuite) TestVersionSeriesEmpty(c *gc.C) {
	s.assertSeriesForVersion(c, "", "")
}

func (s *funcMetadataSuite) TestVersionSeriesInvalid(c *gc.C) {
	s.assertSeriesForVersion(c, "73655", "73655")
}

func (s *funcMetadataSuite) assertSeriesForVersion(c *gc.C, version, series string) {
	c.Assert(series, gc.DeepEquals, imagemetadataworker.VersionSeries(version))
}

func (s *funcMetadataSuite) TestVersionSeriesError(c *gc.C) {
	// Patch to return err
	patchErr := func(series string) (string, error) {
		return "", errors.New("oops")
	}
	s.PatchValue(imagemetadataworker.SeriesVersion, patchErr)

	s.assertSeriesForVersion(c, "73655", "73655")
	// warning displayed
	logOutputText := strings.Replace(c.GetTestLog(), "\n", "", -1)
	c.Assert(logOutputText, gc.Matches, ".*cannot determine version for series.*")
}

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

var process = imagemetadataworker.ProcessErrors

func (s *funcMetadataSuite) assertProcessErrorsNone(c *gc.C, errs []params.ErrorResult) {
	c.Assert(process(errs), jc.ErrorIsNil)
}

func (s *funcMetadataSuite) assertProcessErrors(c *gc.C, errs []params.ErrorResult, expected string) {
	c.Assert(process(errs), gc.ErrorMatches, expected)
}
