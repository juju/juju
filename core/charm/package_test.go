// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

//go:generate go run github.com/canonical/gomock/mockgen -package charm -destination charm_mock_test.go github.com/juju/juju/domain/deployment/charm CharmMeta
//go:generate go run github.com/canonical/gomock/mockgen -package charm -destination core_charm_mock_test.go github.com/juju/juju/core/charm SelectorModelConfig
