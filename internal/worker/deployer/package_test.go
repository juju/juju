// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	coretools "github.com/juju/juju/internal/tools"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/deployer_mocks.go github.com/juju/juju/internal/worker/deployer Client,Machine,Unit

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}

type BaseSuite struct {
	testhelpers.IsolationSuite
}

func (s *BaseSuite) InitializeCurrentToolsDir(c *tc.C, dataDir string) {
	// Initialize the tools directory for the agent.
	// This should be <DataDir>/tools/<version>-<series>-<arch>.
	current := coretesting.CurrentVersion()
	toolsDir := tools.SharedToolsDir(dataDir, current)
	// Make that directory.
	err := os.MkdirAll(toolsDir, 0755)
	c.Assert(err, tc.ErrorIsNil)
	toolsPath := filepath.Join(toolsDir, "downloaded-tools.txt")
	testTools := coretools.Tools{Version: current, URL: "http://testing.invalid/tools"}
	data, err := json.Marshal(testTools)
	c.Assert(err, tc.ErrorIsNil)
	err = os.WriteFile(toolsPath, data, 0644)
	c.Assert(err, tc.ErrorIsNil)
}
