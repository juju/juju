// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"github.com/juju/errors"

	jujuos "github.com/juju/utils/os"
)

type JoyentRenderer struct{}

func (JoyentRenderer) EncodeUserdata(udata []byte, os jujuos.OSType) ([]byte, error) {
	switch os {
	case jujuos.Ubuntu, jujuos.CentOS:
		return udata, nil
	default:
		return nil, errors.Errorf("Cannot encode userdata for OS: %s", os.String())
	}
}
