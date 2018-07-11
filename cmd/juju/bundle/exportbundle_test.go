// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for info.

package bundle_test

import (
	gitjujutesting "github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type ExportBundleCommandSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	fake  fakeExportBundleClient
	store *jujuclient.MemStore
}

var _ = gc.Suite(&ExportBundleCommandSuite{})

type fakeExportBundleClient struct {
	gitjujutesting.Stub
}

func (f *fakeExportBundleClient) Close() error {
	f.MethodCall(f, "Close")
	return f.NextErr()
}
