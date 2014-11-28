// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju_test

import (
	"go/build"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

type dependenciesTest struct{}

var _ = gc.Suite(&dependenciesTest{})

func projectRoot(c *gc.C) string {
	p, err := build.Import("github.com/juju/juju", "", build.FindOnly)
	c.Assert(err, jc.ErrorIsNil)
	return p.Dir
}

func (*dependenciesTest) TestDependenciesTsvFormat(c *gc.C) {
	filename := filepath.Join(projectRoot(c), "dependencies.tsv")
	content, err := ioutil.ReadFile(filename)
	c.Assert(err, jc.ErrorIsNil)

	for _, line := range strings.Split(string(content), "\n") {
		if line == "" {
			continue
		}
		segments := strings.Split(line, "\t")
		c.Assert(segments, gc.HasLen, 4)
	}
}
