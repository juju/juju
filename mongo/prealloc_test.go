// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/mongo"
	coretesting "github.com/juju/juju/testing"
)

type preallocSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&preallocSuite{})

func (s *preallocSuite) TestOplogSize(c *gc.C) {
	type test struct {
		hostWordSize int
		runtimeGOOS  string
		availSpace   int
		expected     int
	}
	tests := []test{{
		hostWordSize: 64,
		runtimeGOOS:  "darwin",
		availSpace:   99999,
		expected:     183,
	}, {
		hostWordSize: 32,
		runtimeGOOS:  "linux",
		availSpace:   48,
		expected:     48,
	}, {
		hostWordSize: 64,
		runtimeGOOS:  "linux",
		availSpace:   1024,
		expected:     512,
	}, {
		hostWordSize: 64,
		runtimeGOOS:  "linux",
		availSpace:   420 * 1024,
		expected:     1024,
	}, {
		hostWordSize: 64,
		runtimeGOOS:  "linux",
		availSpace:   1024 * 1024,
		expected:     1024,
	}}
	var availSpace int
	getAvailSpace := func(dir string) (float64, error) {
		return float64(availSpace), nil
	}
	s.PatchValue(mongo.AvailSpace, getAvailSpace)
	for i, test := range tests {
		c.Logf("test %d: %+v", i, test)
		s.PatchValue(mongo.HostWordSize, test.hostWordSize)
		s.PatchValue(mongo.RuntimeGOOS, test.runtimeGOOS)
		availSpace = test.availSpace
		size, err := mongo.DefaultOplogSize("")
		c.Check(err, jc.ErrorIsNil)
		c.Check(size, gc.Equals, test.expected)
	}
}

func (s *preallocSuite) TestFsAvailSpace(c *gc.C) {
	output := `Filesystem     1K-blocks    Used Available Use% Mounted on
    /dev/vda1        8124856 1365292     12345  18% /`
	testing.PatchExecutable(c, s, "df", "#!/bin/sh\ncat<<EOF\n"+output+"\nEOF")

	mb, err := mongo.FsAvailSpace("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mb, gc.Equals, float64(12345)/1024)
}

func (s *preallocSuite) TestFsAvailSpaceErrors(c *gc.C) {
	tests := []struct {
		desc   string
		output string
		err    string
	}{{
		desc: "result is non-numeric",
		output: `Filesystem     1K-blocks    Used Available Use% Mounted on
    /dev/vda1        8124856 1365292       abc  18% /`,
		err: `strconv.(ParseInt|Atoi): parsing "abc": invalid syntax`,
	}, {
		desc:   "not enough lines",
		output: "abc",
		err:    `could not determine available space on ""`,
	}, {
		desc:   "not enough fields on second line",
		output: "abc\ndef",
		err:    `could not determine available space on ""`,
	}}
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.desc)
		testing.PatchExecutable(c, s, "df", "#!/bin/sh\ncat<<EOF\n"+test.output+"\nEOF")
		_, err := mongo.FsAvailSpace("")
		c.Check(err, gc.ErrorMatches, test.err)
	}
}
