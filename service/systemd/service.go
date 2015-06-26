// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package systemd

import (
	"io/ioutil"
	"os"
	"path"
	"reflect"
	"strings"

	"github.com/coreos/go-systemd/dbus"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/shell"

	"github.com/juju/juju/service/common"
)

var (
	logger = loggo.GetLogger("juju.service.systemd")

	renderer = shell.BashRenderer{}
	cmds     = commands{renderer, executable}
)

// IsRunning returns whether or not systemd is the local init system.
func IsRunning() (bool, error) {
	if _, err := os.Stat("/run/systemd/system"); err == nil {
		return true, nil
	} else if os.IsNotExist(err) {
		return false, nil
	} else {
		return false, errors.Trace(err)
	}
}

// ListServices returns the list of installed service names.
func ListServices() ([]string, error) {
	// TODO(ericsnow) conn.ListUnits misses some inactive units, so we
	// would need conn.ListUnitFiles. Such a method has been requested.
	// (see https://github.com/coreos/go-systemd/issues/76). In the
	// meantime we use systemctl at the shell to list the services.
	// Once that is addressed upstread we can just call listServices here.
	names, err := Cmdline{}.ListAll()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return names, nil
}

func listServices() ([]string, error) {
	conn, err := newConn()
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer conn.Close()

	units, err := conn.ListUnits()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var services []string
	for _, unit := range units {
		name := unit.Name
		if !strings.HasSuffix(name, ".service") {
			continue
		}
		name = strings.TrimSuffix(name, ".service")
		services = append(services, name)
	}
	return services, nil
}

// ListCommand returns a command that will list the services on a host.
func ListCommand() string {
	return cmds.listAll()
}

// Service provides visibility into and control over a systemd service.
type Service struct {
	common.Service

	ConfName string
	UnitName string
	Dirname  string
	Script   []byte
}

// NewService returns a new value that implements Service for systemd.
func NewService(name string, conf common.Conf, dataDir string) (*Service, error) {
	confName := name + ".service"
	var volName string
	if conf.ExecStart != "" {
		volName = renderer.VolumeName(common.Unquote(strings.Fields(conf.ExecStart)[0]))
	}
	dirname := volName + renderer.Join(dataDir, "init", name)

	service := &Service{
		Service: common.Service{
			Name: name,
			// Conf is set in setConf.
		},
		ConfName: confName,
		UnitName: confName,
		Dirname:  dirname,
	}

	if err := service.setConf(conf); err != nil {
		return nil, errors.Trace(err)
	}

	return service, nil
}

// dbusAPI exposes all the systemd API methods needed by juju.
type dbusAPI interface {
	Close()
	ListUnits() ([]dbus.UnitStatus, error)
	StartUnit(string, string, chan<- string) (int, error)
	StopUnit(string, string, chan<- string) (int, error)
	LinkUnitFiles([]string, bool, bool) ([]dbus.LinkUnitFileChange, error)
	EnableUnitFiles([]string, bool, bool) (bool, []dbus.EnableUnitFileChange, error)
	DisableUnitFiles([]string, bool) ([]dbus.DisableUnitFileChange, error)
	GetUnitProperties(string) (map[string]interface{}, error)
	GetUnitTypeProperties(string, string) (map[string]interface{}, error)
	Reload() error
}

var newConn = func() (dbusAPI, error) {
	return dbus.New()
}

var newChan = func() chan string {
	return make(chan string)
}

func (s *Service) errorf(err error, msg string, args ...interface{}) error {
	msg += " for service %q"
	args = append(args, s.Service.Name)
	if err == nil {
		err = errors.Errorf(msg, args...)
	} else {
		err = errors.Annotatef(err, msg, args...)
	}
	err.(*errors.Err).SetLocation(1)
	logger.Errorf("%v", err)
	logger.Debugf("stack trace:\n%s", errors.ErrorStack(err))
	return err
}

// Name implements service.Service.
func (s Service) Name() string {
	return s.Service.Name
}

// Conf implements service.Service.
func (s Service) Conf() common.Conf {
	return s.Service.Conf
}

func (s *Service) serialize() ([]byte, error) {
	data, err := serialize(s.UnitName, s.Service.Conf, renderer)
	if err != nil {
		return nil, s.errorf(err, "failed to serialize conf")
	}
	return data, nil
}

func (s *Service) deserialize(data []byte) (common.Conf, error) {
	conf, err := deserialize(data, renderer)
	if err != nil {
		return conf, s.errorf(err, "failed to deserialize conf")
	}
	return conf, nil
}

func (s *Service) validate(conf common.Conf) error {
	if err := validate(s.Service.Name, conf, &renderer); err != nil {
		return s.errorf(err, "invalid conf")
	}
	return nil
}

func (s *Service) normalize(conf common.Conf) (common.Conf, []byte) {
	scriptPath := renderer.ScriptFilename("exec-start", s.Dirname)
	return normalize(s.Service.Name, conf, scriptPath, &renderer)
}

func (s *Service) setConf(conf common.Conf) error {
	if conf.IsZero() {
		s.Service.Conf = conf
		return nil
	}

	normalConf, data := s.normalize(conf)
	if err := s.validate(normalConf); err != nil {
		return errors.Trace(err)
	}

	s.Script = data
	s.Service.Conf = normalConf
	return nil
}

// Installed implements Service.
func (s *Service) Installed() (bool, error) {
	names, err := ListServices()
	if err != nil {
		return false, s.errorf(err, "failed to list services")
	}
	for _, name := range names {
		if name == s.Service.Name {
			return true, nil
		}
	}
	return false, nil
}

// Exists implements Service.
func (s *Service) Exists() (bool, error) {
	if s.NoConf() {
		return false, s.errorf(nil, "no conf expected")
	}

	same, err := s.check()
	if err != nil {
		return false, errors.Trace(err)
	}
	return same, nil
}

func (s *Service) check() (bool, error) {
	conf, err := s.readConf()
	if err != nil {
		return false, errors.Trace(err)
	}
	normalConf, _ := s.normalize(s.Service.Conf)
	return reflect.DeepEqual(normalConf, conf), nil
}

func (s *Service) readConf() (common.Conf, error) {
	var conf common.Conf

	data, err := Cmdline{}.conf(s.Service.Name, s.Dirname)
	if err != nil {
		return conf, s.errorf(err, "failed to read conf from systemd")
	}

	conf, err = s.deserialize(data)
	if err != nil {
		return conf, errors.Trace(err)
	}
	return conf, nil
}

func (s Service) newConn() (dbusAPI, error) {
	conn, err := newConn()
	if err != nil {
		logger.Errorf("failed to connect to dbus for service %q: %v", s.Service.Name, err)
	}
	return conn, err
}

// Running implements Service.
func (s *Service) Running() (bool, error) {
	conn, err := s.newConn()
	if err != nil {
		return false, errors.Trace(err)
	}
	defer conn.Close()

	units, err := conn.ListUnits()
	if err != nil {
		return false, s.errorf(err, "failed to query services from dbus")
	}

	for _, unit := range units {
		if unit.Name == s.UnitName {
			running := unit.LoadState == "loaded" && unit.ActiveState == "active"
			return running, nil
		}
	}
	return false, nil
}

// Start implements Service.
func (s *Service) Start() error {
	err := s.start()
	if errors.IsAlreadyExists(err) {
		logger.Debugf("service %q already running", s.Name())
		return nil
	} else if err != nil {
		logger.Errorf("service %q failed to start: %v", s.Name(), err)
		return err
	}
	logger.Debugf("service %q successfully started", s.Name())
	return nil
}

func (s *Service) start() error {
	installed, err := s.Installed()
	if err != nil {
		return errors.Trace(err)
	}
	if !installed {
		return errors.NotFoundf("service " + s.Service.Name)
	}
	running, err := s.Running()
	if err != nil {
		return errors.Trace(err)
	}
	if running {
		return errors.AlreadyExistsf("running service %s", s.Service.Name)
	}

	conn, err := s.newConn()
	if err != nil {
		return errors.Trace(err)
	}
	defer conn.Close()

	statusCh := newChan()
	_, err = conn.StartUnit(s.UnitName, "fail", statusCh)
	if err != nil {
		return s.errorf(err, "dbus start request failed")
	}

	if err := s.wait("start", statusCh); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *Service) wait(op string, statusCh chan string) error {
	status := <-statusCh

	// TODO(ericsnow) Other status values *may* be okay. See:
	//  https://godoc.org/github.com/coreos/go-systemd/dbus#Conn.StartUnit
	if status != "done" {
		return s.errorf(nil, "failed to %s (API status %q)", op, status)
	}
	return nil
}

// Stop implements Service.
func (s *Service) Stop() error {
	err := s.stop()
	if errors.IsNotFound(err) {
		logger.Debugf("service %q not running", s.Name())
		return nil
	} else if err != nil {
		logger.Errorf("service %q failed to stop: %v", s.Name(), err)
		return err
	}
	logger.Debugf("service %q successfully stopped", s.Name())
	return nil
}

func (s *Service) stop() error {
	running, err := s.Running()
	if err != nil {
		return errors.Trace(err)
	}
	if !running {
		return errors.NotFoundf("running service %s", s.Service.Name)
	}

	conn, err := s.newConn()
	if err != nil {
		return errors.Trace(err)
	}
	defer conn.Close()

	statusCh := newChan()
	_, err = conn.StopUnit(s.UnitName, "fail", statusCh)
	if err != nil {
		return s.errorf(err, "dbus stop request failed")
	}

	if err := s.wait("stop", statusCh); err != nil {
		return errors.Trace(err)
	}

	return err
}

// Remove implements Service.
func (s *Service) Remove() error {
	err := s.remove()
	if errors.IsNotFound(err) {
		logger.Debugf("service %q not installed", s.Name())
		return nil
	} else if err != nil {
		logger.Errorf("failed to remove service %q: %v", s.Name(), err)
		return err
	}
	logger.Debugf("service %q successfully removed", s.Name())
	return nil
}

func (s *Service) remove() error {
	installed, err := s.Installed()
	if err != nil {
		return errors.Trace(err)
	}
	if !installed {
		return errors.NotFoundf("service %s", s.Service.Name)
	}

	conn, err := s.newConn()
	if err != nil {
		return errors.Trace(err)
	}
	defer conn.Close()

	runtime := false
	_, err = conn.DisableUnitFiles([]string{s.UnitName}, runtime)
	if err != nil {
		return s.errorf(err, "dbus disable request failed")
	}

	if err := conn.Reload(); err != nil {
		return s.errorf(err, "dbus post-disable daemon reload request failed")
	}

	if err := removeAll(s.Dirname); err != nil {
		return s.errorf(err, "failed to delete juju-managed conf dir")
	}

	return nil
}

var removeAll = func(name string) error {
	return os.RemoveAll(name)
}

// Install implements Service.
func (s *Service) Install() error {
	if s.NoConf() {
		return s.errorf(nil, "missing conf")
	}

	err := s.install()
	if errors.IsAlreadyExists(err) {
		logger.Debugf("service %q already installed", s.Name())
		return nil
	} else if err != nil {
		logger.Errorf("failed to install service %q: %v", s.Name(), err)
		return err
	}
	logger.Debugf("service %q successfully installed", s.Name())
	return nil
}

func (s *Service) install() error {
	installed, err := s.Installed()
	if err != nil {
		return errors.Trace(err)
	}
	if installed {
		same, err := s.check()
		if err != nil {
			return errors.Trace(err)
		}
		if same {
			return errors.AlreadyExistsf("service %s", s.Service.Name)
		}
		// An old copy is already running so stop it first.
		if err := s.Stop(); err != nil {
			return errors.Annotate(err, "systemd: could not stop old service")
		}
		if err := s.Remove(); err != nil {
			return errors.Annotate(err, "systemd: could not remove old service")
		}
	}

	filename, err := s.writeConf()
	if err != nil {
		return errors.Trace(err)
	}

	conn, err := s.newConn()
	if err != nil {
		return errors.Trace(err)
	}
	defer conn.Close()

	runtime, force := false, true
	_, err = conn.LinkUnitFiles([]string{filename}, runtime, force)
	if err != nil {
		return s.errorf(err, "dbus link request failed")
	}

	if err := conn.Reload(); err != nil {
		return s.errorf(err, "dbus post-link daemon reload request failed")
	}

	_, _, err = conn.EnableUnitFiles([]string{filename}, runtime, force)
	if err != nil {
		return s.errorf(err, "dbus enable request failed")
	}
	return nil
}

func (s *Service) writeConf() (string, error) {
	data, err := s.serialize()
	if err != nil {
		return "", errors.Trace(err)
	}

	if err := mkdirAll(s.Dirname); err != nil {
		return "", s.errorf(err, "failed to create juju-managed service dir %q", s.Dirname)
	}
	filename := path.Join(s.Dirname, s.ConfName)

	if s.Script != nil {
		scriptPath := renderer.ScriptFilename("exec-start", s.Dirname)
		if scriptPath != s.Service.Conf.ExecStart {
			err := errors.Errorf("wrong script path: expected %q, got %q", scriptPath, s.Service.Conf.ExecStart)
			return filename, s.errorf(err, "failed to write script at %q", scriptPath)
		}
		// TODO(ericsnow) Use the renderer here for the perms.
		if err := createFile(scriptPath, s.Script, 0755); err != nil {
			return filename, s.errorf(err, "failed to write script at %q", scriptPath)
		}
	}

	if err := createFile(filename, data, 0644); err != nil {
		return filename, s.errorf(err, "failed to write conf file %q", filename)
	}

	return filename, nil
}

var mkdirAll = func(dirname string) error {
	return os.MkdirAll(dirname, 0755)
}

var createFile = func(filename string, data []byte, perm os.FileMode) error {
	return ioutil.WriteFile(filename, data, perm)
}

// InstallCommands implements Service.
func (s *Service) InstallCommands() ([]string, error) {
	if s.NoConf() {
		return nil, s.errorf(nil, "missing conf")
	}

	name := s.Name()
	dirname := s.Dirname

	data, err := s.serialize()
	if err != nil {
		return nil, errors.Trace(err)
	}

	cmdList := []string{
		cmds.mkdirs(dirname),
	}
	if s.Script != nil {
		scriptName := renderer.Base(renderer.ScriptFilename("exec-start", ""))
		cmdList = append(cmdList, []string{
			// TODO(ericsnow) Use the renderer here.
			cmds.writeFile(scriptName, dirname, s.Script),
			cmds.chmod(scriptName, dirname, 0755),
		}...)
	}
	cmdList = append(cmdList, []string{
		cmds.writeConf(name, dirname, data),
		cmds.link(name, dirname),
		cmds.reload(),
		cmds.enableLinked(name, dirname),
	}...)
	return cmdList, nil
}

// StartCommands implements Service.
func (s *Service) StartCommands() ([]string, error) {
	name := s.Name()
	cmdList := []string{
		cmds.start(name),
	}
	return cmdList, nil
}
