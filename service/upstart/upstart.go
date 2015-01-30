// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upstart

import (
	"bytes"
	"fmt"
	"path"
	"text/template"

	"github.com/juju/errors"
	"github.com/juju/utils"

	"github.com/juju/juju/service"
	"github.com/juju/juju/service/initsystems"
	"github.com/juju/juju/service/initsystems/upstart"
)

// TODO(ericsnow) Eliminate MachineAgentUpstartService (use NewAgentService).

const maxAgentFiles = 20000

// MachineAgentUpstartService returns the upstart config for a machine agent
// based on the tag and machineId passed in.
func MachineAgentUpstartService(name, toolsDir, dataDir, logDir, tag, machineId string, env map[string]string) *Service {
	logFile := path.Join(logDir, tag+".log")
	// The machine agent always starts with debug turned on.  The logger worker
	// will update this to the system logging environment as soon as it starts.
	conf := service.Conf{Conf: initsystems.Conf{
		Desc: fmt.Sprintf("juju %s agent", tag),
		Limit: map[string]string{
			"nofile": fmt.Sprintf("%d %d", maxAgentFiles, maxAgentFiles),
		},
		Cmd: path.Join(toolsDir, "jujud") +
			" machine" +
			" --data-dir " + utils.ShQuote(dataDir) +
			" --machine-id " + machineId +
			" --debug",
		Out: logFile,
		Env: env,
	}}
	svc := &Service{
		Name: name,
		Conf: conf,
	}
	return svc
}

// Service provides visibility into and control over an upstart service.
type Service struct {
	Name string
	Conf service.Conf
}

// InstallCommands returns shell commands to install and start the service.
func (s *Service) InstallCommands() ([]string, error) {
	if err := upstart.Validate(s.Name, s.Conf.Conf); err != nil {
		return nil, errors.Trace(err)
	}

	var buf bytes.Buffer
	if err := confT.Execute(&buf, s.Conf); err != nil {
		return nil, errors.Trace(err)
	}
	conf := buf.Bytes()
	confPath := path.Join(upstart.ConfDir, s.Name+".conf")

	return []string{
		fmt.Sprintf("cat >> %s << 'EOF'\n%sEOF\n", confPath, conf),
		"start " + s.Name,
	}, nil
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
