// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/storage"
)

type DirectiveSuite struct{}

var _ = gc.Suite(&DirectiveSuite{})

func (s *DirectiveSuite) TestParseDirective(c *gc.C) {
	parseStorageTests := []struct {
		arg              string
		expectSource     string
		expectName       string
		expectCount      int
		expectSize       uint64
		expectPersistent bool
		expectOptions    string
		err              string
	}{{
		arg: "",
		err: `storage name missing`,
	}, {
		arg: ":",
		err: `storage name missing`,
	}, {
		arg: "1M",
		err: "storage name missing",
	}, {
		arg: "ebs:1M",
		err: "storage name missing",
	}, {
		arg: "name=1M",
		err: "storage source missing",
	}, {
		arg:          "name=source:1M",
		expectName:   "name",
		expectSource: "source",
		expectCount:  1,
		expectSize:   1,
	}, {
		arg:          "n-a-m-e=source:1M",
		expectName:   "n-a-m-e",
		expectSource: "source",
		expectCount:  1,
		expectSize:   1,
	}, {
		arg: "name=source:1Msomejunk",
		err: `invalid trailing data "somejunk": options must be preceded by ',' when size is specified`,
	}, {
		arg:           "name=source:anyoldjunk",
		expectName:    "name",
		expectSource:  "source",
		expectCount:   0,
		expectSize:    0,
		expectOptions: "anyoldjunk",
	}, {
		arg:          "name=source:1M,",
		expectName:   "name",
		expectSource: "source",
		expectCount:  1,
		expectSize:   1,
	}, {
		arg:           "name=source:1M,whatever options that please me",
		expectName:    "name",
		expectSource:  "source",
		expectCount:   1,
		expectSize:    1,
		expectOptions: "whatever options that please me",
	}, {
		arg:          "n=s:1G",
		expectName:   "n",
		expectSource: "s",
		expectCount:  1,
		expectSize:   1024,
	}, {
		arg:          "n=s:0.5T",
		expectName:   "n",
		expectSource: "s",
		expectCount:  1,
		expectSize:   1024 * 512,
	}, {
		arg:          "n=s:3x0.125P",
		expectName:   "n",
		expectSource: "s",
		expectCount:  3,
		expectSize:   1024 * 1024 * 128,
	}, {
		arg: "n=s:0x100M",
		err: "count must be a positive integer",
	}, {
		arg: "n=s:-1x100M",
		err: "count must be a positive integer",
	}, {
		arg: "n=s:-100M",
		err: `failed to parse size: expected a non-negative number with optional multiplier suffix \(M/G/T/P\), got "-100M"`,
	}}

	for i, t := range parseStorageTests {
		c.Logf("test %d: %q", i, t.arg)
		p, err := storage.ParseDirective(t.arg)
		if t.err != "" {
			c.Check(err, gc.ErrorMatches, t.err)
			c.Check(p, gc.IsNil)
		} else {
			if !c.Check(err, gc.IsNil) {
				continue
			}
			c.Check(p, gc.DeepEquals, &storage.Directive{
				Name:    t.expectName,
				Source:  t.expectSource,
				Count:   t.expectCount,
				Size:    t.expectSize,
				Options: t.expectOptions,
			})
		}
	}
}
