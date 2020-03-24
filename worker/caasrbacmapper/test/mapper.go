// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package test

import (
	"github.com/juju/errors"
	"k8s.io/apimachinery/pkg/types"
)

// Mapper is a dummy rbac mapper used in testing
type Mapper struct {
	// AppNameForServiceAccountFunc is the function called when
	// AppNameForServiceAccount is used.
	AppNameForServiceAccountFunc func(types.UID) (string, error)
}

// AppNameForServiceAccount implements the Mapper interface and calls the
// AppNameForServiceAccountFunc field when defined. Otherwise returns a not
// found error
func (m *Mapper) AppNameForServiceAccount(t types.UID) (string, error) {
	if m.AppNameForServiceAccountFunc == nil {
		return "", errors.NotFoundf("no service account for app found with id %v", t)
	}
	return m.AppNameForServiceAccountFunc(t)
}
