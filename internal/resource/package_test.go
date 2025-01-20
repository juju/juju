// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package resource -destination charmhub_mock_test.go github.com/juju/juju/internal/resource/charmhub ResourceClient,CharmHub
//go:generate go run go.uber.org/mock/mockgen -typed -package resource -destination resource_mock_test.go github.com/juju/juju/internal/resource ResourceService,ApplicationService,DeprecatedState,DeprecatedStateApplication,DeprecatedStateUnit,ResourceDownloadLock,ResourceClientGetter

func Test(t *testing.T) {
	gc.TestingT(t)
}
