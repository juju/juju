// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

//go:generate mockgen -package common -destination context_mock_test.go github.com/juju/juju/environs/context ProviderCallContext
//go:generate mockgen -package common -destination environs_mock_test.go github.com/juju/juju/environs BootstrapEnviron,NetworkingEnviron
//go:generate mockgen -package common -destination common_mock_test.go github.com/juju/juju/apiserver/common ReloadSpacesState
