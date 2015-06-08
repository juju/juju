// Copyright 2015 Cloudbase Solutions
// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package windows

import (
	"io/ioutil"
	"reflect"
	"strings"
	"syscall"
	"unsafe"

	"github.com/gabriel-samfira/sys/windows"
	"github.com/gabriel-samfira/sys/windows/svc"
	"github.com/gabriel-samfira/sys/windows/svc/mgr"
	"github.com/juju/errors"

	"github.com/juju/juju/service/common"
	"github.com/juju/juju/service/windows/securestring"
)

//sys enumServicesStatus(h windows.Handle, dwServiceType uint32, dwServiceState uint32, lpServices uintptr, cbBufSize uint32, pcbBytesNeeded *uint32, lpServicesReturned *uint32, lpResumeHandle *uint32) (err error) [failretval==0] = advapi32.EnumServicesStatusW

type enumService struct {
	name        *uint16
	displayName *uint16
	Status      windows.SERVICE_STATUS
}

// Name returns the name of the service stored in enumService.
func (s *enumService) Name() string {
	if s.name != nil {
		return syscall.UTF16ToString((*[1 << 16]uint16)(unsafe.Pointer(s.name))[:])
	}
	return ""
}

// mgrInterface exposes Mgr methods needed by the windows service package.
type mgrInterface interface {
	CreateService(name, exepath string, c mgr.Config, args ...string) (svcInterface, error)
	OpenService(name string) (svcInterface, error)
}

// svcInterface exposes mgr.Service methods needed by the windows service package.
type svcInterface interface {
	Config() (mgr.Config, error)
	Control(c svc.Cmd) (svc.Status, error)
	Delete() error
	Query() (svc.Status, error)
	Start(...string) error
}

// manager is meant to help stub out winsvc for testing
type manager struct {
	m *mgr.Mgr
}

// CreateService wraps Mgr.CreateService method.
func (m *manager) CreateService(name, exepath string, c mgr.Config, args ...string) (svcInterface, error) {
	return m.m.CreateService(name, exepath, c, args...)
}

// CreateService wraps Mgr.OpenService method. It returns a svcInterface object.
// This allows us to stub out this module for testing.
func (m *manager) OpenService(name string) (svcInterface, error) {
	return m.m.OpenService(name)
}

func newManagerConn() (mgrInterface, error) {
	s, err := mgr.Connect()
	if err != nil {
		return nil, err
	}
	return &manager{m: s}, nil
}

var newConn = newManagerConn

// enumServices casts the bytes returned by enumServicesStatus into an array of
// enumService with all the services on the current system
func enumServices(h windows.Handle) ([]enumService, error) {
	var needed uint32
	var returned uint32
	var resume uint32
	var buf [2048]enumService

	for {
		err := enumServicesStatus(h, windows.SERVICE_WIN32,
			windows.SERVICE_STATE_ALL, uintptr(unsafe.Pointer(&buf)), uint32(unsafe.Sizeof(buf)), &needed, &returned, &resume)
		if err != nil {
			if err == windows.ERROR_MORE_DATA {
				continue
			}
			return []enumService{}, err
		}
		return buf[:returned], nil
	}
}

// getPassword attempts to read the password for the jujud user. We define it as
// a variable to allow us to mock it out for testing
var getPassword = func() (string, error) {
	f, err := ioutil.ReadFile(jujuPasswdFile)
	if err != nil {
		return "", errors.Annotate(err, "Failed to read password file")
	}
	encryptedPasswd := strings.TrimSpace(string(f))
	passwd, err := securestring.Decrypt(encryptedPasswd)
	if err != nil {
		return "", errors.Annotate(err, "Failed to decrypt password")
	}
	return passwd, nil
}

// listServices returns an array of strings containing all the services on
// the current system. It is defined as a variable to allow us to mock it out
// for testing
var listServices = func() ([]string, error) {
	services := []string{}
	host := syscall.StringToUTF16Ptr(".")

	sc, err := windows.OpenSCManager(host, nil, windows.SC_MANAGER_ALL_ACCESS)
	if err != nil {
		return services, err
	}
	enum, err := enumServices(sc)
	if err != nil {
		return services, err
	}
	for _, v := range enum {
		services = append(services, v.Name())
	}
	return services, nil
}

// SvcManager implements ServiceManager interface
type SvcManager struct {
	svc         svcInterface
	mgr         mgrInterface
	serviceConf common.Conf
}

func (s *SvcManager) query(name string) (svc.State, error) {
	var err error
	s.svc, err = s.mgr.OpenService(name)
	if err != nil {
		return svc.Stopped, errors.Trace(err)
	}
	status, err := s.svc.Query()
	if err != nil {
		return svc.Stopped, errors.Trace(err)
	}
	return status.State, nil
}

func (s *SvcManager) status(name string) (svc.State, error) {
	status, err := s.query(name)
	if err != nil {
		return svc.Stopped, errors.Trace(err)
	}
	return status, nil
}

func (s *SvcManager) exists(name string) (bool, error) {
	_, err := s.query(name)
	if err == c_ERROR_SERVICE_DOES_NOT_EXIST {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

// Start starts a service.
func (s *SvcManager) Start(name string) error {
	running, err := s.Running(name)
	if err != nil {
		return errors.Trace(err)
	}
	if running {
		return nil
	}
	err = s.svc.Start()
	if err != nil {
		return err
	}
	return nil
}

func (s *SvcManager) escapeExecPath(exePath string, args []string) string {
	ret := syscall.EscapeArg(exePath)
	for _, v := range args {
		ret += " " + syscall.EscapeArg(v)
	}
	return ret
}

// Exists checks whether the config of the installed service matches the
// config supplied to this function
func (s *SvcManager) Exists(name string, conf common.Conf) (bool, error) {
	passwd, err := getPassword()
	if err != nil {
		return false, err
	}

	// We escape and compose BinaryPathName the same way mgr.CreateService does.
	execStart := s.escapeExecPath(conf.ServiceBinary, conf.ServiceArgs)
	cfg := mgr.Config{
		// make this service dependent on WMI service. WMI is needed for almost
		// all installers to work properly, and is needed for all of the advanced windows
		// instrumentation bits (powershell included). Juju agents must start after this
		// service to ensure hooks run properly.
		Dependencies:     []string{"Winmgmt"},
		StartType:        mgr.StartAutomatic,
		DisplayName:      conf.Desc,
		ServiceStartName: jujudUser,
		Password:         passwd,
		BinaryPathName:   execStart,
	}
	currentConfig, err := s.Config(name)
	if err != nil {
		return false, err
	}

	if reflect.DeepEqual(cfg, currentConfig) {
		return true, nil
	}
	return false, nil
}

// Stop stops a service.
func (s *SvcManager) Stop(name string) error {
	running, err := s.Running(name)
	if err != nil {
		return errors.Trace(err)
	}
	if !running {
		return nil
	}
	_, err = s.svc.Control(svc.Stop)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Delete deletes a service.
func (s *SvcManager) Delete(name string) error {
	exists, err := s.exists(name)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	err = s.svc.Delete()
	if err == c_ERROR_SERVICE_DOES_NOT_EXIST {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Create creates a service with the given config.
func (s *SvcManager) Create(name string, conf common.Conf) error {
	passwd, err := getPassword()
	if err != nil {
		return errors.Trace(err)
	}
	cfg := mgr.Config{
		Dependencies:     []string{"Winmgmt"},
		StartType:        mgr.StartAutomatic,
		DisplayName:      conf.Desc,
		ServiceStartName: jujudUser,
		Password:         passwd,
	}
	// mgr.CreateService actually does correct argument escaping itself. There is no
	// need for quoted strings of any kind passed to this function. It takes in
	// a binary name, and an array or arguments.
	_, err = s.mgr.CreateService(name, conf.ServiceBinary, cfg, conf.ServiceArgs...)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Running returns the status of a service.
func (s *SvcManager) Running(name string) (bool, error) {
	status, err := s.status(name)
	if err != nil {
		return false, errors.Trace(err)
	}
	logger.Infof("Service %q Status %v", name, status)
	if status == svc.Running {
		return true, nil
	}
	return false, nil
}

// Config returns the mgr.Config of the service. This config reflects the actual
// service configuration in Windows.
func (s *SvcManager) Config(name string) (mgr.Config, error) {
	exists, err := s.exists(name)
	if err != nil {
		return mgr.Config{}, err
	}
	if !exists {
		return mgr.Config{}, c_ERROR_SERVICE_DOES_NOT_EXIST
	}
	return s.svc.Config()
}

var newServiceManager = func() (ServiceManager, error) {
	m, err := newConn()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &SvcManager{
		mgr: m,
	}, nil
}
