// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package windows

import (
	"github.com/juju/errors"

	"github.com/juju/juju/service/initsystems"
)

// Validate returns an error if the service is not adequately defined.
func Validate(name string, conf initsystems.Conf) error {
	if err := conf.Validate(name); err != nil {
		return errors.Trace(err)
	}

	if len(conf.Env) > 0 {
		return initsystems.NewUnsupportedField("Env")
	}
	if len(conf.Limit) > 0 {
		return initsystems.NewUnsupportedField("Limit")
	}
	if len(conf.Out) > 0 {
		return initsystems.NewUnsupportedField("Out")
	}
	return nil
}

// Serialize serializes the provided Conf for the named service. The
// resulting data will be in the prefered format for consumption by
// the init system.
func Serialize(name string, conf initsystems.Conf) ([]byte, error) {
	// TODO(ericsnow) Finish!
	return nil, nil
}

// Deserialize parses the provided data (in the init system's prefered
// format) and populates a new Conf with the result.
func Deserialize(data []byte) (*initsystems.Conf, error) {
	// TODO(ericsnow) Finish!
	return nil, nil
}
