// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errorutils_test

//go:generate go run github.com/canonical/gomock/mockgen -package errorutils_test -destination environs_mock_test.go github.com/juju/juju/environs CredentialInvalidator
