// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

//go:generate go run github.com/canonical/gomock/mockgen -package config -destination agent_config_mock_test.go github.com/juju/juju/agent/config AgentConfigReader
