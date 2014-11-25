// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

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
		hostWordSize: 64,
		runtimeGOOS:  "windows",
		availSpace:   99999,
		expected:     512,
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
		err: `strconv.ParseInt: parsing "abc": invalid syntax`,
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

func (s *preallocSuite) TestPreallocFileSizes(c *gc.C) {
	const MB = 1024 * 1024

	tests := []struct {
		desc   string
		size   int
		result []int
	}{{
		desc:   "zero size, zero files",
		size:   0,
		result: nil,
	}, {
		desc:   "exactly divides the max chunk size",
		size:   1024 * MB,
		result: []int{512 * MB, 512 * MB},
	}, {
		desc:   "remainder comes at the beginning",
		size:   1025 * MB,
		result: []int{1 * MB, 512 * MB, 512 * MB},
	}, {
		desc:   "remaining one byte must be padded out to 4096 bytes",
		size:   1024*MB + 1,
		result: []int{4096, 512 * MB, 512 * MB},
	}}

	for i, test := range tests {
		c.Logf("test %d: %s", i, test.desc)
		sizes := mongo.PreallocFileSizes(test.size)
		c.Check(sizes, gc.DeepEquals, test.result)
	}
}

func (s *preallocSuite) TestPreallocFiles(c *gc.C) {
	dir := c.MkDir()
	prefix := filepath.Join(dir, "test.")
	err := mongo.PreallocFiles(prefix, 0, 4096, 8192)
	c.Assert(err, jc.ErrorIsNil)

	zeroes := [8192]byte{}
	for i := 0; i < 3; i++ {
		filename := fmt.Sprintf("%s%d", prefix, i)
		data, err := ioutil.ReadFile(filename)
		c.Check(err, jc.ErrorIsNil)
		c.Check(data, gc.DeepEquals, zeroes[:i*4096])
	}

	_, err = os.Stat(prefix + "3")
	c.Assert(err, jc.Satisfies, os.IsNotExist)
}

func (s *preallocSuite) TestPreallocFilesErrors(c *gc.C) {
	err := mongo.PreallocFiles("", 123)
	c.Assert(err, gc.ErrorMatches, `specified size 123 for file "0" is not a multiple of 4096`)
}

func (s *preallocSuite) TestPreallocFilesWriteErrors(c *gc.C) {
	dir := c.MkDir()
	prefix := filepath.Join(dir, "test.")
	err := ioutil.WriteFile(prefix+"0", nil, 0644)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(prefix+"1", nil, 0644)
	c.Assert(err, jc.ErrorIsNil)

	var called int
	s.PatchValue(mongo.PreallocFile, func(filename string, size int) (bool, error) {
		var created bool
		var err error
		called++
		if called == 2 {
			created = true
			err = fmt.Errorf("failed to zero test.1")
		}
		return created, err
	})

	err = mongo.PreallocFiles(prefix, 4096, 8192)
	c.Assert(err, gc.ErrorMatches, "failed to zero test.1")

	// test.0 still exists because we said we didn't
	// create it (i.e. it already existed)
	_, err = os.Stat(prefix + "0")
	c.Assert(err, jc.ErrorIsNil)

	// test.1 no longer exists because we said we created
	// it, but then failed to write to it.
	_, err = os.Stat(prefix + "1")
	c.Assert(err, jc.Satisfies, os.IsNotExist)
}
