// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package retrystrategy_test

//go:generate go run go.uber.org/mock/mockgen -package retrystrategy_test -destination service_mock_test.go github.com/juju/juju/apiserver/facades/agent/retrystrategy ModelConfigService
