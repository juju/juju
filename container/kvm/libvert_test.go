// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm_test

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/container/kvm"
	coretesting "github.com/juju/juju/testing"
)

type LibVertSuite struct {
	coretesting.BaseSuite
	ContainerDir string
	RemovedDir   string
}

var _ = gc.Suite(&LibVertSuite{})

func (s *LibVertSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	// Skip if not linux
	if runtime.GOOS != "linux" {
		c.Skip("not running linux")
	}
}

// Test that the call to SyncImages utilizes the defined source
func (s *LibVertSuite) TestSyncImagesUtilizesSimpleStreamsSource(c *gc.C) {

	const simpStreamsBinName = "uvt-simplestreams-libvirt"
	testing.PatchExecutableAsEchoArgs(c, s, simpStreamsBinName)

	const (
		series = "mocked-series"
		arch   = "mocked-arch"
		source = "mocked-url"
	)
	err := kvm.SyncImages(series, arch, source)
	c.Assert(err, jc.ErrorIsNil)

	expectedArgs := strings.Split(
		fmt.Sprintf(
			"sync arch=%s release=%s --source=%s",
			arch,
			series,
			source,
		),
		" ",
	)

	testing.AssertEchoArgs(c, simpStreamsBinName, expectedArgs...)
}
