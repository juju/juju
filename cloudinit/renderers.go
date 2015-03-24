// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit

import (
	"github.com/juju/errors"
	"gopkg.in/yaml.v1"
)

func renderUnix(conf *Config) ([]byte, error) {
	data, err := yaml.Marshal(conf.attrs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return append([]byte("#cloud-config\n"), data...), nil
}

func renderWindows(conf *Config) ([]byte, error) {
	winCmds := conf.attrs["runcmd"]
	var script []byte
	newline := "\r\n"
	header := "#ps1_sysnative\r\n"
	script = append(script, header...)
	for _, value := range winCmds.([]*command) {
		script = append(script, newline...)
		script = append(script, value.literal...)

	}
	return script, nil
}
