// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm_test

import (
	"runtime"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/container/kvm"
	"github.com/juju/juju/environs/imagedownloads"
	"github.com/juju/juju/environs/simplestreams"
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

type testSyncParams struct {
	arch, series, ftype string
	srcFunc             func() simplestreams.DataSource
	onevalErr           error
	success             bool
}

func (p testSyncParams) One() (*imagedownloads.Metadata, error) {
	if p.success {
		return &imagedownloads.Metadata{
			Arch:    p.arch,
			Release: p.series,
		}, nil

	}
	return nil, p.onevalErr
}

func (p testSyncParams) sourceURL() (string, error) {
	return p.srcFunc().URL("")
}

// Test that the call to SyncImages utilizes the defined source
func (s *LibVertSuite) TestSyncImagesUtilizesSimpleStreamsSource(c *gc.C) {

	const (
		series = "mocked-series"
		arch   = "mocked-arch"
		source = "mocked-url"
	)
	p := testSyncParams{
		arch:    arch,
		series:  series,
		srcFunc: func() simplestreams.DataSource { return imagedownloads.NewDataSource(source) },
		success: true,
	}
	err := kvm.Sync(p, fakeUpdater{})
	c.Assert(err, jc.ErrorIsNil)

	url, err := p.sourceURL()
	c.Check(err, jc.ErrorIsNil)
	c.Check(url, jc.DeepEquals, source+"/")

	res, err := p.One()
	c.Check(err, jc.ErrorIsNil)

	c.Check(res.Arch, jc.DeepEquals, arch)
	c.Check(res.Release, jc.DeepEquals, series)
}
