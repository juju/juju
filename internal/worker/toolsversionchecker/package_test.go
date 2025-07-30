// Copyright 2025 Canonical{@re} Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package toolsversionchecker

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/goleak"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package toolsversionchecker -destination service_mock_test.go github.com/juju/juju/internal/worker/toolsversionchecker ModelConfigService,ModelAgentService,MachineService
//go:generate go run go.uber.org/mock/mockgen -typed -package toolsversionchecker -destination environs_mock_test.go github.com/juju/juju/environs BootstrapEnviron

func TestManifoldSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &ManifoldSuite{})
}

func TestToolsCheckerSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &ToolsCheckerSuite{})
}
