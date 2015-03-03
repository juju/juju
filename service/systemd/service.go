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

	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/service/common"
	"github.com/juju/juju/version"
)

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
	return commands{}.listAll()
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
func NewService(name string, conf common.Conf) (*Service, error) {
	confName := name + ".service"
	dataDir, err := findDataDir()
	if err != nil {
		return nil, errors.Trace(err)
	}
	dirname := path.Join(dataDir, "init", name)

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

var findDataDir = func() (string, error) {
	return paths.DataDir(version.Current.Series)
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
}

var newConn = func() (dbusAPI, error) {
	return dbus.New()
}

var newChan = func() chan string {
	return make(chan string)
}

// Name implements service.Service.
func (s Service) Name() string {
	return s.Service.Name
}

// Conf implements service.Service.
func (s Service) Conf() common.Conf {
	return s.Service.Conf
}

// UpdateConfig implements Service.
func (s *Service) UpdateConfig(conf common.Conf) {
	s.setConf(conf) // We ignore any error (i.e. when validation fails).
}

func (s *Service) setConf(conf common.Conf) error {
	scriptPath := path.Join(s.Dirname, "exec-start.sh")

	normalConf, data := normalize(s.Service.Name, conf, scriptPath)
	if err := validate(s.Service.Name, normalConf); err != nil {
		return errors.Trace(err)
	}

	s.Service.Conf = normalConf
	s.Script = data
	return nil
}

// Installed implements Service.
func (s *Service) Installed() bool {
	names, err := ListServices()
	if err != nil {
		return false
	}
	for _, name := range names {
		if name == s.Service.Name {
			return true
		}
	}
	return false
}

// Exists implements Service.
func (s *Service) Exists() bool {
	same, err := s.check()
	if err != nil {
		return false
	}
	return same
}

func (s *Service) check() (bool, error) {
	conf, err := s.readConf()
	if err != nil {
		return false, errors.Trace(err)
	}
	return reflect.DeepEqual(s.Service.Conf, conf), nil
}

func (s *Service) readConf() (common.Conf, error) {
	var conf common.Conf

	data, err := Cmdline{}.conf(s.Service.Name)
	if err != nil {
		return conf, errors.Trace(err)
	}

	conf, err = deserialize(data)
	if err != nil {
		return conf, errors.Trace(err)
	}
	return conf, nil
}

// Running implements Service.
func (s *Service) Running() bool {
	conn, err := newConn()
	if err != nil {
		return false
	}
	defer conn.Close()

	units, err := conn.ListUnits()
	if err != nil {
		return false
	}

	for _, unit := range units {
		if unit.Name == s.UnitName {
			return unit.LoadState == "loaded" && unit.ActiveState == "active"
		}
	}
	return false
}

// Start implements Service.
func (s *Service) Start() error {
	if !s.Installed() {
		return errors.NotFoundf("service " + s.Service.Name)
	}
	if s.Running() {
		return nil
	}

	conn, err := newConn()
	if err != nil {
		return errors.Trace(err)
	}
	defer conn.Close()

	statusCh := newChan()
	_, err = conn.StartUnit(s.UnitName, "fail", statusCh)
	if err != nil {
		return errors.Trace(err)
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
		return errors.Errorf("failed to %s service %s", op, s.Service.Name)
	}
	return nil
}

// Stop implements Service.
func (s *Service) Stop() error {
	if !s.Running() {
		return nil
	}

	conn, err := newConn()
	if err != nil {
		return errors.Trace(err)
	}
	defer conn.Close()

	statusCh := newChan()
	_, err = conn.StopUnit(s.UnitName, "fail", statusCh)
	if err != nil {
		return errors.Trace(err)
	}

	if err := s.wait("stop", statusCh); err != nil {
		return errors.Trace(err)
	}

	return err
}

// StopAndRemove implements Service.
func (s *Service) StopAndRemove() error {
	if err := s.Stop(); err != nil {
		return errors.Trace(err)
	}
	err := s.Remove()
	return errors.Trace(err)
}

// Remove implements Service.
func (s *Service) Remove() error {
	if !s.Installed() {
		return nil
	}

	conn, err := newConn()
	if err != nil {
		return errors.Trace(err)
	}
	defer conn.Close()

	runtime := false
	_, err = conn.DisableUnitFiles([]string{s.UnitName}, runtime)
	if err != nil {
		return errors.Trace(err)
	}

	if err := removeAll(s.Dirname); err != nil {
		return errors.Trace(err)
	}

	return nil
}

var removeAll = func(name string) error {
	return os.RemoveAll(name)
}

// Install implements Service.
func (s *Service) Install() error {
	if s.Installed() {
		same, err := s.check()
		if err != nil {
			return errors.Trace(err)
		}
		if same {
			return nil
		}
		// An old copy is already running so stop it first.
		if err := s.StopAndRemove(); err != nil {
			return errors.Annotate(err, "systemd: could not remove old service")
		}
	}

	filename, err := s.writeConf()
	if err != nil {
		return errors.Trace(err)
	}

	conn, err := newConn()
	if err != nil {
		return errors.Trace(err)
	}
	defer conn.Close()

	runtime, force := false, true
	_, err = conn.LinkUnitFiles([]string{filename}, runtime, force)
	if err != nil {
		return errors.Trace(err)
	}

	// TODO(ericsnow) This needs SU privs...
	_, _, err = conn.EnableUnitFiles([]string{filename}, runtime, force)
	return errors.Trace(err)
}

func (s *Service) writeConf() (string, error) {
	data, err := serialize(s.UnitName, s.Service.Conf)
	if err != nil {
		return "", errors.Trace(err)
	}

	if err := mkdirAll(s.Dirname); err != nil {
		return "", errors.Trace(err)
	}
	filename := path.Join(s.Dirname, s.ConfName)

	if s.Script != nil {
		scriptPath := s.Service.Conf.ExecStart
		if err := createFile(scriptPath, s.Script, 0755); err != nil {
			return filename, errors.Trace(err)
		}
	}

	if err := createFile(filename, data, 0644); err != nil {
		return filename, errors.Trace(err)
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
	name := s.Name()
	dirname := "/tmp"

	data, err := serialize(s.UnitName, s.Service.Conf)
	if err != nil {
		return nil, errors.Trace(err)
	}

	cmds := commands{executable}
	cmdList := []string{
		cmds.writeFile(name, dirname, data),
		cmds.link(name, dirname),
		cmds.enable(name),
		cmds.start(name),
	}
	return cmdList, nil
}
