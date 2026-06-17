// Copyright 2025 Canonical{@re} Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package toolsversionchecker

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run github.com/canonical/gomock/mockgen -package toolsversionchecker -destination service_mock_test.go github.com/juju/juju/internal/worker/toolsversionchecker ModelConfigService,ModelAgentService,MachineService
//go:generate go run github.com/canonical/gomock/mockgen -package toolsversionchecker -destination environs_mock_test.go github.com/juju/juju/environs BootstrapEnviron

func TestManifoldSuite(t *testing.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *testing.T) {
		tc.Run(t, &ManifoldSuite{})
	})
}

func TestToolsCheckerSuite(t *testing.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *testing.T) {
		tc.Run(t, &ToolsCheckerSuite{})
	})
}
