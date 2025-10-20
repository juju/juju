// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"github.com/juju/tc"
	macaroon "gopkg.in/macaroon.v2"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package crossmodelrelations -destination package_mock_test.go github.com/juju/juju/apiserver/facades/controller/crossmodelrelations CrossModelRelationService,ModelConfigService,SecretService,StatusService,RelationService,RelationUnitsWatcherService,ApplicationService,RemovalService
//go:generate go run go.uber.org/mock/mockgen -typed -package crossmodelrelations -destination auth_mock_test.go github.com/juju/juju/apiserver/facade CrossModelAuthContext,MacaroonAuthenticator
//go:generate go run go.uber.org/mock/mockgen -typed -package crossmodelrelations -destination watcher_mock_test.go github.com/juju/juju/core/watcher NotifyWatcher,StringsWatcher

func newMacaroon(c *tc.C, id string) *macaroon.Macaroon {
	mac, err := macaroon.New(nil, []byte(id), "", macaroon.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)
	return mac
}

func ptr[T any](v T) *T {
	return &v
}
