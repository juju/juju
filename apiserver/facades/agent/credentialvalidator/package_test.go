// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator

//go:generate go run github.com/canonical/gomock/mockgen  -typed -package credentialvalidator -destination services_mock.go github.com/juju/juju/apiserver/facades/agent/credentialvalidator ModelCredentialService
//go:generate go run github.com/canonical/gomock/mockgen  -typed -package credentialvalidator -destination watcher_mock.go github.com/juju/juju/core/watcher NotifyWatcher
