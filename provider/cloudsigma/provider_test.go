// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"flag"
	"io"
	"testing"

	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	tt "github.com/juju/juju/testing"
)

var live = flag.Bool("live", false, "run tests on live CloudSigma account")

func TestCloudSigma(t *testing.T) {
	gc.TestingT(t)
}

type providerSuite struct {
	tt.BaseSuite
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) TestProviderBoilerplateConfig(c *gc.C) {
	cfg := providerInstance.BoilerplateConfig()
	c.Assert(cfg, gc.Not(gc.Equals), "")
}

type failReader struct {
	err error
}

func (f *failReader) Read(p []byte) (n int, err error) {
	return 0, f.err
}

type fakeStorage struct {
	call   string
	name   string
	prefix string
	err    error
	reader io.Reader
	length int64
}

func (f *fakeStorage) Get(name string) (io.ReadCloser, error) {
	f.call = "Get"
	f.name = name
	return nil, nil
}
func (f *fakeStorage) List(prefix string) ([]string, error) {
	f.call = "List"
	f.prefix = prefix
	return []string{prefix}, nil
}
func (f *fakeStorage) URL(name string) (string, error) {
	f.call = "URL"
	f.name = name
	return "", nil
}
func (f *fakeStorage) DefaultConsistencyStrategy() utils.AttemptStrategy {
	f.call = "DefaultConsistencyStrategy"
	return utils.AttemptStrategy{}
}
func (f *fakeStorage) ShouldRetry(err error) bool {
	f.call = "ShouldRetry"
	f.err = err
	return false
}
func (f *fakeStorage) Put(name string, r io.Reader, length int64) error {
	f.call = "Put"
	f.name = name
	f.reader = r
	f.length = length
	return nil
}
func (f *fakeStorage) Remove(name string) error {
	f.call = "Remove"
	f.name = name
	return nil
}
func (f *fakeStorage) RemoveAll() error {
	f.call = "RemoveAll"
	return nil
}
