// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing_test

import (
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common/testing"
)

var _ service.Service = (*testing.FakeService)(nil)
