// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger_test

//go:generate go run go.uber.org/mock/mockgen -typed -package logger_test -destination service_mock_test.go github.com/juju/juju/apiserver/facades/agent/logger ModelConfigService
