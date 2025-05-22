// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular

//go:generate go run go.uber.org/mock/mockgen -typed -package singular -destination lease_mock_test.go github.com/juju/juju/core/lease Manager,Claimer
//go:generate go run go.uber.org/mock/mockgen -typed -package singular -destination clock_mock_test.go github.com/juju/clock Clock
//go:generate go run go.uber.org/mock/mockgen -typed -package singular -destination agent_mock_test.go github.com/juju/juju/agent Agent,Config
