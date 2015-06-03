// Copyright 2015 Cloudbase Solutions
// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package windows

import (
	"io/ioutil"
	"reflect"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"code.google.com/p/winsvc/mgr"
	"code.google.com/p/winsvc/svc"
	"code.google.com/p/winsvc/winapi"
	"github.com/juju/errors"

	"github.com/juju/juju/service/common"
	"github.com/juju/juju/service/windows/securestring"
)

//sys enumServicesStatus(h syscall.Handle, dwServiceType uint32, dwServiceState uint32, lpServices uintptr, cbBufSize uint32, pcbBytesNeeded *uint32, lpServicesReturned *uint32, lpResumeHandle *uint32) (err error) [failretval==0] = advapi32.EnumServicesStatusW

const (
	// logonProvider constants
	// https://msdn.microsoft.com/en-us/library/windows/desktop/aa378184%28v=vs.85%29.aspx
	c_LOGON32_PROVIDER_DEFAULT = 0
	c_LOGON32_PROVIDER_WINNT35 = 1
	c_LOGON32_PROVIDER_WINNT40 = 2
	c_LOGON32_PROVIDER_WINNT50 = 3

	// logonType constants
	c_LOGON32_LOGON_INTERACTIVE       = 2
	c_LOGON32_LOGON_NETWORK           = 3
	c_LOGON32_LOGON_BATCH             = 4
	c_LOGON32_LOGON_SERVICE           = 5
	c_LOGON32_LOGON_UNLOCK            = 7
	c_LOGON32_LOGON_NETWORK_CLEARTEXT = 8
	c_LOGON32_LOGON_NEW_CREDENTIALS   = 9
)

type enumService struct {
	name        *uint16
	displayName *uint16
	Status      winapi.SERVICE_STATUS
}

// Name returns the name of the service stored in enumService.
func (s *enumService) Name() string {
	if s.name != nil {
		name := make([]uint16, 0, 256)
		for p := uintptr(unsafe.Pointer(s.name)); ; p += 2 {
			u := *(*uint16)(unsafe.Pointer(p))
			if u == 0 {
				return string(utf16.Decode(name))
			}
			name = append(name, u)
		}
	}
	return ""
}

// mgrInterface exposes Mgr methods needed by the windows service package.
type mgrInterface interface {
	CreateService(name, exepath string, c mgr.Config) (svcInterface, error)
	OpenService(name string) (svcInterface, error)
}

// svcInterface exposes mgr.Service methods needed by the windows service package.
type svcInterface interface {
	Config() (mgr.Config, error)
	Control(c svc.Cmd) (svc.Status, error)
	Delete() error
	Query() (svc.Status, error)
	Start(args []string) error
}

// manager is meant to help stub out winsvc for testing
type manager struct {
	m *mgr.Mgr
}

// CreateService wraps Mgr.CreateService method.
func (m *manager) CreateService(name, exepath string, c mgr.Config) (svcInterface, error) {
	return m.m.CreateService(name, exepath, c)
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
func enumServices(h syscall.Handle) ([]enumService, error) {
	var needed uint32
	var returned uint32
	var resume uint32
	var buf [256]enumService

	for {
		err := enumServicesStatus(h, winapi.SERVICE_WIN32, winapi.SERVICE_STATE_ALL,
			uintptr(unsafe.Pointer(&buf)), uint32(unsafe.Sizeof(buf)), &needed, &returned, &resume)
		if err != nil {
			if err.(syscall.Errno) != c_ERROR_MORE_DATA {
				return []enumService{}, err
			}
			continue
		}
		return buf[:returned], nil
	}
}

// getPassword attempts to read the password for the jujud user. We define it as
// a variable to allow us to mock it out for testing
var getPassword = func() (string, error) {
	f, err := ioutil.ReadFile(jujuPasswdFile)
	if err != nil {
		return "", errors.Trace(err)
	}
	encryptedPasswd := strings.TrimSpace(string(f))
	passwd, err := securestring.Decrypt(encryptedPasswd)
	if err != nil {
		return "", errors.Trace(err)
	}
	return passwd, nil
}

// listServices returns an array of strings containing all the services on
// the current system. It is defined as a variable to allow us to mock it out
// for testing
var listServices = func() ([]string, error) {
	services := []string{}
	host := syscall.StringToUTF16Ptr(".")

	sc, err := winapi.OpenSCManager(host, nil, winapi.SC_MANAGER_ALL_ACCESS)
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

// SvcManager implements ServiceManagerInterface interface
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
	err = s.svc.Start([]string{})
	if err != nil {
		return err
	}
	return nil
}

// Exists checks whether the config of the installed service matches the
// config supplied to this function
func (s *SvcManager) Exists(name string, conf common.Conf) (bool, error) {
	passwd, err := getPassword()
	if err != nil {
		return false, err
	}
	execStart := strings.Replace(conf.ExecStart, `'`, `"`, -1)
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
	// In service definitions, single quotes make the service fail. To take
	// care of the case where spaces might exist in the path to the binary,
	// we use double quotes.
	execStart := strings.Replace(conf.ExecStart, `'`, `"`, -1)
	_, err = s.mgr.CreateService(name, execStart, cfg)
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

var newServiceManager = func() (ServiceManagerInterface, error) {
	m, err := newConn()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &SvcManager{
		mgr: m,
	}, nil
}
