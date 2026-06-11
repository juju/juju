// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

//go:generate go run github.com/canonical/gomock/mockgen -package charms_test -destination charmsgetter_mock_test.go github.com/juju/juju/api/client/charms CharmGetter
//go:generate go run github.com/canonical/gomock/mockgen -package charms -destination charmsputter_mock_test.go github.com/juju/juju/api/client/charms CharmPutter
