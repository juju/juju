// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package asynccharmdownloader

import (
	gc "gopkg.in/check.v1"
)

type downloadSuite struct {
	applicationService *MockApplicationService
}

var _ = gc.Suite(&downloadSuite{})
