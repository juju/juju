// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// +build windows

package password

import (
	"syscall"
	"time"

	// https://bugs.launchpad.net/juju-core/+bug/1470820
	"github.com/gabriel-samfira/sys/windows/registry"
	"github.com/juju/errors"
	"github.com/juju/utils"

	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/service/windows"
	"github.com/juju/juju/service/windows/securestring"
)

// netUserSetInfo is used to change the password on a user.
//sys netUserSetInfo(servername *uint16, username *uint16, level uint32, buf *netUserSetPassword, parm_err *uint16) (err error) [failretval!=0] = netapi32.NetUserSetInfo

// The USER_INFO_1003 structure contains a user password. This information
// level is valid only when you call the NetUserSetInfo function.
// https://msdn.microsoft.com/en-us/library/windows/desktop/aa370963(v=vs.85).aspx
type netUserSetPassword struct {
	Password *uint16
}

const (
	alphanum = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

	// Specifies a user password. The buf parameter points to a USER_INFO_1003 structure.
	// https://msdn.microsoft.com/en-us/library/windows/desktop/aa370659(v=vs.85).aspx
	changePasswordLevel = 1003
)

var (
	ERR_REGKEY_EXIST = errors.New("Registry key already exists")
)

// EnsureJujudPassword sets a password on the jujud service and the jujud user. Writes that
// password in a registry file to be read at a later point. This should only be
// done as an initialization when starting the agent. It only does something on
// windows.
var EnsureJujudPassword = func() error {
	newPassword, err := utils.RandomPassword()
	if err != nil {
		return errors.Trace(err)
	}
	mgr, err := windows.NewServiceManager()
	if err != nil {
		return errors.Annotate(err, "could not start service manager")
	}

	err = ensureJujudPasswordHelper("jujud", newPassword, osenv.JujuRegistryKey, osenv.JujuRegistryPasswordKey, mgr, &passwordChanger{})
	if err == ERR_REGKEY_EXIST {
		return nil
	}
	return err
}

// ensureJujudPasswordHelper actually does the heavy lifting of changing the password. It checks the registry for a password. If it doesn't exist
// then it writes a new one to the registry, changes the password for the local jujud user and sets the password for all it's services.
func ensureJujudPasswordHelper(username, newPassword, regKey, regEntry string, mgr windows.ServiceManager, helpers passwordChangerHelpers) error {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, regKey[6:], registry.ALL_ACCESS)
	if err != nil {
		return errors.Annotate(err, "failed to open juju registry key")
	}
	defer k.Close()

	// Check if the password already exists in the registry
	if _, _, err := k.GetBinaryValue(regEntry); err == nil {
		return ERR_REGKEY_EXIST
	}

	enc, err := securestring.Encrypt(newPassword)
	if err != nil {
		return errors.Annotate(err, "could not encrypt password")
	}

	err = k.SetBinaryValue(regEntry, []byte(enc))
	if err != nil {
		return errors.Annotate(err, "could not write password registry key")
	}

	err = helpers.changeUserPasswordLocalhost(newPassword)
	if err != nil {
		delErr := k.DeleteValue(regEntry)
		if delErr != nil {
			return errors.Annotatef(err, "could not change user password, reverting config; could not erase entry %s at %s", regEntry, regKey)
		}
		return errors.Annotate(err, "could not change user password, reverting config")
	}

	err = helpers.changeJujudServicesPassword(newPassword, mgr, windows.ListServices)
	if err != nil {
		delErr := k.DeleteValue(regEntry)
		if delErr != nil {
			return errors.Annotatef(err, "could not change password for all jujud services; could not erase entry %s at %s", regEntry, regKey)
		}
		return errors.Annotate(err, "could not change password for all jujud services")
	}

	return nil
}

var changeServicePasswordAttempts = utils.AttemptStrategy{
	Total: 5 * time.Second,
	Delay: 6 * time.Second,
}

// passwordChangerHelpers exists only for making the testing of the ensureJujudPasswordHelper function easier
type passwordChangerHelpers interface {
	// changeUserPasswordLocalhost changes the password for the jujud user on the local computer using syscalls
	changeUserPasswordLocalhost(newPassword string) error

	// changeJujudServicesPassword changes the password for all the services created by the jujud user
	changeJujudServicesPassword(newPassword string, mgr windows.ServiceManager, listServices func() ([]string, error)) error
}

// passwordChanger implements passwordChangerHelpers
type passwordChanger struct{}

// changeUserPasswordLocalhost changes the password for username on localhost
func (c *passwordChanger) changeUserPasswordLocalhost(newPassword string) error {
	serverp, err := syscall.UTF16PtrFromString("localhost")
	if err != nil {
		return errors.Trace(err)
	}

	userp, err := syscall.UTF16PtrFromString("jujud")
	if err != nil {
		return errors.Trace(err)
	}

	passp, err := syscall.UTF16PtrFromString(newPassword)
	if err != nil {
		return errors.Trace(err)
	}

	info := netUserSetPassword{passp}

	err = netUserSetInfo(serverp, userp, changePasswordLevel, &info, nil)
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (c *passwordChanger) changeJujudServicesPassword(newPassword string, mgr windows.ServiceManager, listServices func() ([]string, error)) error {
	// Iterate through all services and change the password for those belonging
	// to jujud
	svcs, err := listServices()
	if err != nil {
		return errors.Trace(err)
	}
	for _, svc := range svcs {
		modifiedService := false
		var err error
		for attempt := changeServicePasswordAttempts.Start(); attempt.Next(); {
			err = mgr.ChangeServicePassword(svc, newPassword)
			if err != nil {
				logger.Errorf("retrying to change password on service %v; error: %v", svc, err)
			}
			if err == nil {
				modifiedService = true
				break
			}
		}
		if !modifiedService {
			return errors.Trace(err)
		}
	}

	return nil
}
