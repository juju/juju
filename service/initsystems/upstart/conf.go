// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build linux

package upstart

import (
	"bytes"
	"regexp"
	"strings"
	"text/template"

	"github.com/juju/errors"

	"github.com/juju/juju/service/initsystems"
)

// Validate returns an error if the service is not adequately defined.
func Validate(name string, conf initsystems.Conf) error {
	err := conf.Validate(name)
	return errors.Trace(err)
}

// Serialize serializes the provided Conf for the named service. The
// resulting data will be in the prefered format for consumption by
// the init system.
func Serialize(name string, conf initsystems.Conf) ([]byte, error) {
	if err := Validate(name, conf); err != nil {
		return nil, errors.Trace(err)
	}

	// TODO(ericsnow) While the template approach is sufficient for use
	// in juju, it is insufficient for more general upstart usage.
	var buf bytes.Buffer
	if err := confT.Execute(&buf, conf); err != nil {
		return nil, errors.Trace(err)
	}
	return buf.Bytes(), nil
}

var confRegex = regexp.MustCompile(`` +
	`description "(.*)"|` +
	`env (\w+)="(.*)"|` +
	`limit (\w*) (\w+)|` +
	`exec ([^ ]+)(?: >> (.+) 2>&1)?`)

// Deserialize parses the provided data (in the init system's prefered
// format) and populates a new Conf with the result.
func Deserialize(data []byte, name string) (*initsystems.Conf, error) {
	var conf initsystems.Conf

	// TODO(ericsnow) Is there a better way? This approach is
	// approximate at best and somewhat fragile.
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		groups := confRegex.FindSubmatch([]byte(line))
		if groups == nil {
			continue
		}

		switch {
		case groups[1] != nil:
			conf.Desc = string(groups[1])
		case groups[2] != nil:
			if conf.Env == nil {
				conf.Env = make(map[string]string)
			}
			name := string(groups[2])
			conf.Env[name] = string(groups[3])
		case groups[4] != nil:
			if conf.Limit == nil {
				conf.Limit = make(map[string]string)
			}
			name := string(groups[4])
			conf.Limit[name] = string(groups[5])
		case groups[6] != nil:
			conf.Cmd = string(groups[6])
			conf.Out = string(groups[7])
		}
	}

	if name == "" {
		name = "<>"
	}
	err := Validate(name, conf)
	return &conf, errors.Trace(err)
}

// TODO(ericsnow) Do not hard-code the author in the template (use Conf.Meta).
// TODO(ericsnow) Eliminate the blank lines due to templating (e.g. env).
// TODO(ericsnow) Move the Out touch/chown/chmod part to Conf.PreStart.

// BUG: %q quoting does not necessarily match libnih quoting rules
// (as used by upstart); this may become an issue in the future.
var confT = template.Must(template.New("").Parse(`
description "{{.Desc}}"
author "Juju Team <juju@lists.ubuntu.com>"
start on runlevel [2345]
stop on runlevel [!2345]
respawn
normal exit 0
{{range $k, $v := .Env}}env {{$k}}={{$v|printf "%q"}}
{{end}}
{{range $k, $v := .Limit}}limit {{$k}} {{$v}}
{{end}}
script
{{if .Out}}
  # Ensure log files are properly protected
  touch {{.Out}}
  chown syslog:syslog {{.Out}}
  chmod 0600 {{.Out}}
{{end}}
  exec {{.Cmd}}{{if .Out}} >> {{.Out}} 2>&1{{end}}
end script
`[1:]))
