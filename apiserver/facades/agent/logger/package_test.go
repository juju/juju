// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger_test

//go:generate go run github.com/canonical/gomock/mockgen -package logger_test -destination service_mock_test.go github.com/juju/juju/apiserver/facades/agent/logger ModelConfigService
