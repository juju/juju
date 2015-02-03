// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upstart

import (
	"bytes"
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

	// TODO(ericsnow) We can do better than this!
	var buf bytes.Buffer
	if err := confT.Execute(&buf, conf); err != nil {
		return nil, errors.Trace(err)
	}
	return buf.Bytes(), nil
}

// Deserialize parses the provided data (in the init system's prefered
// format) and populates a new Conf with the result.
func Deserialize(data []byte) (*initsystems.Conf, error) {
	var conf initsystems.Conf

	// TODO(ericsnow) Is there a better way? This approach is
	// approximate at best and somewhat fragile.
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "description ") {
			start := len("description ")
			conf.Desc = strings.Trim(line[start:], `"`)
			continue
		}
		if strings.HasPrefix(line, "env ") {
			if conf.Env == nil {
				conf.Env = make(map[string]string)
			}
			start := len("env ")
			parts := strings.SplitN(line[start:], "=", 2)
			conf.Env[parts[0]] = strings.Trim(parts[1], `"`)
			continue
		}
		if strings.HasPrefix(line, "limit ") {
			if conf.Limit == nil {
				conf.Limit = make(map[string]string)
			}
			start := len("limit ")
			parts := strings.SplitN(line[start:], " ", 2)
			conf.Limit[parts[0]] = parts[1]
			continue
		}
		if strings.HasPrefix(line, "exec ") {
			start := len("exec ")
			parts := strings.SplitN(line[start:], " >> ", 2)
			conf.Cmd = parts[0]
			if len(parts) == 2 {
				conf.Out = strings.TrimSuffix(parts[1], " 2>&1")
			}
			continue
		}
	}

	return &conf, nil
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
