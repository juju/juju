// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

//go:generate go run go.uber.org/mock/mockgen -typed -package charms_test -destination charmsgetter_mock_test.go github.com/juju/juju/api/client/charms CharmGetter
//go:generate go run go.uber.org/mock/mockgen -typed -package charms -destination charmsputter_mock_test.go github.com/juju/juju/api/client/charms CharmPutter
