// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/mongo"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
)

type preallocSuite struct {
	coretesting.BaseSuite
}

func TestPreallocSuite(t *stdtesting.T) {
	tc.Run(t, &preallocSuite{})
}

func (s *preallocSuite) TestOplogSize(c *tc.C) {
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
		s.PatchValue(mongo.RuntimeGOOS, test.runtimeGOOS)
		availSpace = test.availSpace
		size, err := mongo.DefaultOplogSize("")
		c.Check(err, tc.ErrorIsNil)
		c.Check(size, tc.Equals, test.expected)
	}
}

func (s *preallocSuite) TestFsAvailSpace(c *tc.C) {
	output := `Filesystem     1K-blocks    Used Available Use% Mounted on
    /dev/vda1        8124856 1365292     12345  18% /`
	testhelpers.PatchExecutable(c, s, "df", "#!/bin/sh\ncat<<EOF\n"+output+"\nEOF")

	mb, err := mongo.FsAvailSpace("")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mb, tc.Equals, float64(12345)/1024)
}

func (s *preallocSuite) TestFsAvailSpaceErrors(c *tc.C) {
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
		testhelpers.PatchExecutable(c, s, "df", "#!/bin/sh\ncat<<EOF\n"+test.output+"\nEOF")
		_, err := mongo.FsAvailSpace("")
		c.Check(err, tc.ErrorMatches, test.err)
	}
}
