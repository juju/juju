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

const (
	SC_ACTION_NONE = iota
	SC_ACTION_RESTART
	SC_ACTION_REBOOT
	SC_ACTION_RUN_COMMAND
)

type serviceAction struct {
	actionType int
	delay      uint32
}

type serviceFailureActions struct {
	dwResetPeriod uint32
	lpRebootMsg   *uint16
	lpCommand     *uint16
	cActions      uint32
	scAction      *serviceAction
}

// This is done so we can mock this function out
var WinChangeServiceConfig2 = windows.ChangeServiceConfig2

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

// windowsManager exposes Mgr methods needed by the windows service package.
type windowsManager interface {
	CreateService(name, exepath string, c mgr.Config, args ...string) (windowsService, error)
	OpenService(name string) (windowsService, error)
	GetHandle(name string) (windows.Handle, error)
	CloseHandle(handle windows.Handle) error
}

// windowsService exposes mgr.Service methods needed by the windows service package.
type windowsService interface {
	Close() error
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
func (m *manager) CreateService(name, exepath string, c mgr.Config, args ...string) (windowsService, error) {
	s, err := mgr.Connect()
	if err != nil {
		return nil, err
	}
	defer s.Disconnect()
	return s.CreateService(name, exepath, c, args...)
}

// CreateService wraps Mgr.OpenService method. It returns a windowsService object.
// This allows us to stub out this module for testing.
func (m *manager) OpenService(name string) (windowsService, error) {
	s, err := mgr.Connect()
	if err != nil {
		return nil, err
	}
	defer s.Disconnect()
	return s.OpenService(name)
}

// CreateService wraps Mgr.OpenService method but returns a windows.Handle object.
// This is used to access a lower level function not directly exposed by
// the sys/windows package.
func (m *manager) GetHandle(name string) (handle windows.Handle, err error) {
	s, err := mgr.Connect()
	if err != nil {
		return handle, err
	}
	defer s.Disconnect()
	service, err := s.OpenService(name)
	if err != nil {
		return handle, err
	}
	return service.Handle, nil
}

// CloseHandle wraps the windows.CloseServiceHandle method.
// This allows us to stub out this module for testing.
func (m *manager) CloseHandle(handle windows.Handle) error {
	return windows.CloseServiceHandle(handle)
}

var newManager = func() (windowsManager, error) {
	return &manager{}, nil
}

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
var listServices = func() (services []string, err error) {
	host := syscall.StringToUTF16Ptr(".")

	sc, err := windows.OpenSCManager(host, nil, windows.SC_MANAGER_ALL_ACCESS)
	defer func() {
		// The close service handle error is less important than others
		if err == nil {
			err = windows.CloseServiceHandle(sc)
		}
	}()
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
	svc         windowsService
	mgr         windowsManager
	serviceConf common.Conf
}

func (s *SvcManager) getService(name string) (windowsService, error) {
	service, err := s.mgr.OpenService(name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return service, nil
}

func (s *SvcManager) status(name string) (svc.State, error) {
	service, err := s.getService(name)
	if err != nil {
		return svc.Stopped, errors.Trace(err)
	}
	defer service.Close()
	status, err := service.Query()
	if err != nil {
		return svc.Stopped, errors.Trace(err)
	}
	return status.State, nil
}

func (s *SvcManager) exists(name string) (bool, error) {
	service, err := s.getService(name)
	if err == c_ERROR_SERVICE_DOES_NOT_EXIST {
		return false, nil
	} else if err != nil {
		return false, err
	}
	defer service.Close()
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
	service, err := s.getService(name)
	if err != nil {
		return errors.Trace(err)
	}
	defer service.Close()
	err = service.Start()
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
	service, err := s.getService(name)
	if err != nil {
		return errors.Trace(err)
	}
	defer service.Close()
	_, err = service.Control(svc.Stop)
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
	service, err := s.getService(name)
	if err != nil {
		return errors.Trace(err)
	}
	defer service.Close()
	err = service.Delete()
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
		ErrorControl:     mgr.ErrorSevere,
		StartType:        mgr.StartAutomatic,
		DisplayName:      conf.Desc,
		ServiceStartName: jujudUser,
		Password:         passwd,
	}
	// mgr.CreateService actually does correct argument escaping itself. There is no
	// need for quoted strings of any kind passed to this function. It takes in
	// a binary name, and an array or arguments.
	service, err := s.mgr.CreateService(name, conf.ServiceBinary, cfg, conf.ServiceArgs...)
	if err != nil {
		return errors.Trace(err)
	}
	defer service.Close()
	err = s.ensureRestartOnFailure(name)
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
	service, err := s.getService(name)
	if err != nil {
		return mgr.Config{}, errors.Trace(err)
	}
	defer service.Close()
	return service.Config()
}

func (s *SvcManager) ensureRestartOnFailure(name string) (err error) {
	handle, err := s.mgr.GetHandle(name)
	if err != nil {
		return errors.Trace(err)
	}
	defer func() {
		// The CloseHandle error is less important than another error
		if err == nil {
			err = s.mgr.CloseHandle(handle)
		}
	}()
	action := serviceAction{
		actionType: SC_ACTION_RESTART,
		delay:      1000,
	}
	failActions := serviceFailureActions{
		dwResetPeriod: 5,
		lpRebootMsg:   nil,
		lpCommand:     nil,
		cActions:      1,
		scAction:      &action,
	}
	err = WinChangeServiceConfig2(handle, windows.SERVICE_CONFIG_FAILURE_ACTIONS, (*byte)(unsafe.Pointer(&failActions)))
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

var newServiceManager = func() (ServiceManager, error) {
	m, err := newManager()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &SvcManager{
		mgr: m,
	}, nil
}
