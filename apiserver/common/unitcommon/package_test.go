// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitcommon

//go:generate go run github.com/canonical/gomock/mockgen -package unitcommon -destination service_mock_test.go github.com/juju/juju/apiserver/common/unitcommon ApplicationService
