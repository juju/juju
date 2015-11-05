// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"github.com/juju/errors"
	jujuos "github.com/juju/utils/os"
)

type lxdRenderer struct{}

// EncodeUserdata implements renderers.ProviderRenderer.
func (lxdRenderer) EncodeUserdata(udata []byte, os jujuos.OSType) ([]byte, error) {
	switch os {
	case jujuos.Ubuntu, jujuos.CentOS:
		return udata, nil
	default:
		return nil, errors.Errorf("cannot encode userdata for OS %q", os)
	}
}
