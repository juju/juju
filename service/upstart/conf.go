// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upstart

import (
	"bytes"
	"strings"
	"text/template"

	"github.com/juju/errors"

	"github.com/juju/juju/service/common"
)

func Serialize(name string, conf common.Conf) ([]byte, error) {
	if err := validate(conf); err != nil {
		return nil, err
	}

	// TODO(ericsnow) We can do better than this!
	var buf bytes.Buffer
	if err := confT.Execute(&buf, conf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func Deserialize(data []byte) (*common.Conf, error) {
	conf := common.Conf{}

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
			conf.Env[parts[0]] = parts[1]
			continue
		}
		if strings.HasPrefix(line, "limit ") {
			if conf.Limit == nil {
				conf.Limit = make(map[string]string)
			}
			start := len("limit ")
			parts := strings.SplitN(line[start:], "=", 2)
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

// validate returns an error if the service is not adequately defined.
func validate(conf common.Conf) error {
	if conf.Desc == "" {
		return errors.New("missing Desc")
	}
	if conf.Cmd == "" {
		return errors.New("missing Cmd")
	}
	return nil
}

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
{{if .ExtraScript}}{{.ExtraScript}}{{end}}
{{if .Out}}
  # Ensure log files are properly protected
  touch {{.Out}}
  chown syslog:syslog {{.Out}}
  chmod 0600 {{.Out}}
{{end}}
  exec {{.Cmd}}{{if .Out}} >> {{.Out}} 2>&1{{end}}
end script
`[1:]))
