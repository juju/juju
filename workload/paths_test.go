// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workload_test

import (
	"path/filepath"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/workload"
)

type pathsSuite struct{}

var _ = gc.Suite(&pathsSuite{})

func (*pathsSuite) TestDataDir(c *gc.C) {
	base := filepath.Join("some", "base", "path")
	dataDir := workload.DataDir(base)

	c.Check(dataDir, gc.Equals, filepath.Join(base, "workloads"))
}
