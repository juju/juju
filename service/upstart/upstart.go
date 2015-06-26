// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upstart

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"regexp"
	"runtime"
	"text/template"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/shell"

	"github.com/juju/juju/service/common"
)

var (
	InitDir = "/etc/init" // the default init directory name.

	logger      = loggo.GetLogger("juju.service.upstart")
	initctlPath = "/sbin/initctl"
	servicesRe  = regexp.MustCompile("^([a-zA-Z0-9-_:]+)\\.conf$")
	renderer    = &shell.BashRenderer{}
)

// IsRunning returns whether or not upstart is the local init system.
func IsRunning() (bool, error) {
	// On windows casting the error to exec.Error does not yield a os.PathError type
	// It's easyer to just return false before even trying to execute an external command
	// on windows at least
	if runtime.GOOS == "windows" {
		return false, nil
	}

	// TODO(ericsnow) This function should be fixed to precisely match
	// the equivalent shell script line in service/discovery.go.

	cmd := exec.Command(initctlPath, "--system", "list")
	_, err := cmd.CombinedOutput()
	if err == nil {
		return true, nil
	}

	msg := fmt.Sprintf("exec %q failed", initctlPath)
	if os.IsNotExist(err) {
		// Executable could not be found, go 1.3 and later
		return false, nil
	}
	if execErr, ok := err.(*exec.Error); ok {
		// Executable could not be found, go 1.2
		if os.IsNotExist(execErr.Err) || execErr.Err == exec.ErrNotFound {
			return false, nil
		}
	}
	// Note: initctl will fail if upstart is installed but not running.
	// The error message will be:
	//   Name "com.ubuntu.Upstart" does not exist
	return false, errors.Annotatef(err, msg)
}

// ListServices returns the name of all installed services on the
// local host.
func ListServices() ([]string, error) {
	fis, err := ioutil.ReadDir(InitDir)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var services []string
	for _, fi := range fis {
		if groups := servicesRe.FindStringSubmatch(fi.Name()); len(groups) > 0 {
			services = append(services, groups[1])
		}
	}
	return services, nil
}

// ListCommand returns a command that will list the services on a host.
func ListCommand() string {
	// TODO(ericsnow) Do "ls /etc/init/*.conf" instead?
	return `sudo initctl list | awk '{print $1}' | sort | uniq`
}

var startedRE = regexp.MustCompile(`^.* start/running(?:, process (\d+))?\n$`)

// Service provides visibility into and control over an upstart service.
type Service struct {
	common.Service
}

func NewService(name string, conf common.Conf) *Service {
	return &Service{
		Service: common.Service{
			Name: name,
			Conf: conf,
		},
	}
}

// Name implements service.Service.
func (s Service) Name() string {
	return s.Service.Name
}

// Conf implements service.Service.
func (s Service) Conf() common.Conf {
	return s.Service.Conf
}

// confPath returns the path to the service's configuration file.
func (s *Service) confPath() string {
	return path.Join(InitDir, s.Service.Name+".conf")
}

// Validate returns an error if the service is not adequately defined.
func (s *Service) Validate() error {
	if err := s.Service.Validate(renderer); err != nil {
		return errors.Trace(err)
	}

	if s.Service.Conf.Transient {
		if len(s.Service.Conf.Env) > 0 {
			return errors.NotSupportedf("Conf.Env (when transient)")
		}
		if len(s.Service.Conf.Limit) > 0 {
			return errors.NotSupportedf("Conf.Limit (when transient)")
		}
		if s.Service.Conf.Logfile != "" {
			return errors.NotSupportedf("Conf.Logfile (when transient)")
		}
		if s.Service.Conf.ExtraScript != "" {
			return errors.NotSupportedf("Conf.ExtraScript (when transient)")
		}
	} else {
		if s.Service.Conf.AfterStopped != "" {
			return errors.NotSupportedf("Conf.AfterStopped (when not transient)")
		}
		if s.Service.Conf.ExecStopPost != "" {
			return errors.NotSupportedf("Conf.ExecStopPost (when not transient)")
		}
	}

	return nil
}

// render returns the upstart configuration for the service as a slice of bytes.
func (s *Service) render() ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	conf := s.Conf()
	if conf.Transient {
		conf.ExecStopPost = "rm " + s.confPath()
	}
	return Serialize(s.Name(), conf)
}

// Installed returns whether the service configuration exists in the
// init directory.
func (s *Service) Installed() (bool, error) {
	_, err := os.Stat(s.confPath())
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, errors.Trace(err)
	}
	return true, nil
}

// Exists returns whether the service configuration exists in the
// init directory with the same content that this Service would have
// if installed.
func (s *Service) Exists() (bool, error) {
	// In any error case, we just say it doesn't exist with this configuration.
	// Subsequent calls into the Service will give the caller more useful errors.
	_, same, _, err := s.existsAndSame()
	if err != nil {
		return false, errors.Trace(err)
	}
	return same, nil
}

func (s *Service) existsAndSame() (exists, same bool, conf []byte, err error) {
	expected, err := s.render()
	if err != nil {
		return false, false, nil, errors.Trace(err)
	}
	current, err := ioutil.ReadFile(s.confPath())
	if err != nil {
		if os.IsNotExist(err) {
			// no existing config
			return false, false, expected, nil
		}
		return false, false, nil, errors.Trace(err)
	}
	return true, bytes.Equal(current, expected), expected, nil
}

// Running returns true if the Service appears to be running.
func (s *Service) Running() (bool, error) {
	cmd := exec.Command("status", "--system", s.Service.Name)
	out, err := cmd.CombinedOutput()
	logger.Tracef("Running \"status --system %s\": %q", s.Service.Name, out)
	if err == nil {
		return startedRE.Match(out), nil
	}
	if err.Error() != "exit status 1" {
		return false, errors.Trace(err)
	}
	return false, nil
}

// Start starts the service.
func (s *Service) Start() error {
	running, err := s.Running()
	if err != nil {
		return errors.Trace(err)
	}
	if running {
		return nil
	}
	err = runCommand("start", "--system", s.Service.Name)
	if err != nil {
		// Double check to see if we were started before our command ran.
		// If this fails then we simply trust it's okay.
		if running, _ := s.Running(); running {
			return nil
		}
	}
	return err
}

func runCommand(args ...string) error {
	out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
	if err == nil {
		return nil
	}
	out = bytes.TrimSpace(out)
	if len(out) > 0 {
		return fmt.Errorf("exec %q: %v (%s)", args, err, out)
	}
	return fmt.Errorf("exec %q: %v", args, err)
}

// Stop stops the service.
func (s *Service) Stop() error {
	running, err := s.Running()
	if err != nil {
		return errors.Trace(err)
	}
	if !running {
		return nil
	}
	return runCommand("stop", "--system", s.Service.Name)
}

// Restart restarts the service.
func (s *Service) Restart() error {
	return runCommand("restart", s.Service.Name)
}

// Remove deletes the service configuration from the init directory.
func (s *Service) Remove() error {
	installed, err := s.Installed()
	if err != nil {
		return errors.Trace(err)
	}
	if !installed {
		return nil
	}
	return os.Remove(s.confPath())
}

// Install installs and starts the service.
func (s *Service) Install() error {
	exists, same, conf, err := s.existsAndSame()
	if err != nil {
		return errors.Trace(err)
	}
	if same {
		return nil
	}
	if exists {
		if err := s.Stop(); err != nil {
			return errors.Annotate(err, "upstart: could not stop installed service")
		}
		if err := s.Remove(); err != nil {
			return errors.Annotate(err, "upstart: could not remove installed service")
		}
	}
	if err := ioutil.WriteFile(s.confPath(), conf, 0644); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// InstallCommands returns shell commands to install the service.
func (s *Service) InstallCommands() ([]string, error) {
	conf, err := s.render()
	if err != nil {
		return nil, err
	}
	cmd := fmt.Sprintf("cat > %s << 'EOF'\n%sEOF\n", s.confPath(), conf)
	return []string{cmd}, nil
}

// StartCommands returns shell commands to start the service.
func (s *Service) StartCommands() ([]string, error) {
	// TODO(ericsnow) Add clarification about why transient services are not started.
	if s.Service.Conf.Transient {
		return nil, nil
	}
	return []string{"start " + s.Service.Name}, nil
}

// Serialize renders the conf as raw bytes.
func Serialize(name string, conf common.Conf) ([]byte, error) {
	var buf bytes.Buffer
	if conf.Transient {
		if err := transientConfT.Execute(&buf, conf); err != nil {
			return nil, err
		}
	} else {
		if err := confT.Execute(&buf, conf); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// TODO(ericsnow) Use a different solution than templates?

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
{{range $k, $v := .Limit}}limit {{$k}} {{$v}} {{$v}}
{{end}}
script
{{if .ExtraScript}}{{.ExtraScript}}{{end}}
{{if .Logfile}}
  # Ensure log files are properly protected
  touch {{.Logfile}}
  chown syslog:syslog {{.Logfile}}
  chmod 0600 {{.Logfile}}
{{end}}
  exec {{.ExecStart}}{{if .Logfile}} >> {{.Logfile}} 2>&1{{end}}
end script
`[1:]))

var transientConfT = template.Must(template.New("").Parse(`
description "{{.Desc}}"
author "Juju Team <juju@lists.ubuntu.com>"
start on stopped {{.AfterStopped}}

script
  {{.ExecStart}}
end script
{{if .ExecStopPost}}
post-stop script
  {{.ExecStopPost}}
end script
{{end}}
`[1:]))

// CleanShutdownJob is added to machines to ensure DHCP-assigned IP
// addresses are released on shutdown, reboot, or halt. See bug
// http://pad.lv/1348663 for more info.
const CleanShutdownJob = `
author "Juju Team <juju@lists.ubuntu.com>"
description "Stop all network interfaces on shutdown"
start on runlevel [016]
task
console output

exec /sbin/ifdown -a -v --force
`

// CleanShutdownJobPath is the full file path where CleanShutdownJob
// is created.
const CleanShutdownJobPath = "/etc/init/juju-clean-shutdown.conf"
