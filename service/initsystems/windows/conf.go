// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package windows

import (
	"github.com/juju/errors"

	"github.com/juju/juju/service/initsystems"
)

// Validate returns an error if the service is not adequately defined.
func Validate(name string, conf initsystems.Conf) (string, error) {
	confName := name + ".conf"

	if err := conf.Validate(name); err != nil {
		return confName, errors.Trace(err)
	}

	if len(conf.Env) > 0 {
		return confName, initsystems.NewUnsupportedField("Env")
	}
	if len(conf.Limit) > 0 {
		return confName, initsystems.NewUnsupportedField("Limit")
	}
	if len(conf.Out) > 0 {
		return confName, initsystems.NewUnsupportedField("Out")
	}
	return confName, nil
}

// Serialize serializes the provided Conf for the named service. The
// resulting data will be in the prefered format for consumption by
// the init system.
func Serialize(name string, conf initsystems.Conf) ([]byte, error) {
	if _, err := Validate(name, conf); err != nil {
		return nil, errors.Trace(err)
	}

	data, err := initsystems.SerializeJSON(conf)
	return data, errors.Trace(err)
}

// Deserialize parses the provided data (in the init system's prefered
// format) and populates a new Conf with the result.
func Deserialize(data []byte, name string) (initsystems.Conf, error) {
	conf, err := initsystems.DeserializeJSON(data)
	if err != nil {
		return conf, errors.Trace(err)
	}

	if name == "" {
		name = "<>"
	}
	_, err = Validate(name, conf)
	return conf, errors.Trace(err)
}
