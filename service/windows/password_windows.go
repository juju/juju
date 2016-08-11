// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// +build windows

package windows

import (
	"syscall"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils"
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
	// Specifies a user password. The buf parameter points to a USER_INFO_1003 structure.
	// https://msdn.microsoft.com/en-us/library/windows/desktop/aa370659(v=vs.85).aspx
	changePasswordLevel = 1003
)

// resetJujudPassword sets a password on the jujud service and the jujud user
// and returns it. This should only be done when we're deploying new units.
// The reason is that there isn't a completely secure way of storing the user's password
// and we do not *really* need it except when deploying new units.
var resetJujudPassword = func() (string, error) {
	newPassword, err := utils.RandomPassword()
	if err != nil {
		return "", errors.Trace(err)
	}
	mgr, err := NewServiceManager()
	if err != nil {
		return "", errors.Annotate(err, "could not start service manager")
	}

	err = ensureJujudPasswordHelper("jujud", newPassword, mgr, &PasswordChanger{})
	if err != nil {
		return "", errors.Annotate(err, "could not change password")
	}
	return newPassword, nil
}

// ensureJujudPasswordHelper actually does the heavy lifting of changing the password. It checks the registry for a password. If it doesn't exist
// then it writes a new one to the registry, changes the password for the local jujud user and sets the password for all it's services.
func ensureJujudPasswordHelper(username, newPassword string, mgr ServiceManager, helpers PasswordChangerHelpers) error {
	err := helpers.ChangeUserPasswordLocalhost(newPassword)
	if err != nil {
		return errors.Annotate(err, "could not change user password")
	}

	err = helpers.ChangeJujudServicesPassword(newPassword, mgr, ListServices)
	if err != nil {
		return errors.Annotate(err, "could not change password for all jujud services")
	}

	return nil
}

// TODO(katco): 2016-08-09: lp:1611427
var changeServicePasswordAttempts = utils.AttemptStrategy{
	Total: 5 * time.Second,
	Delay: 6 * time.Second,
}

// passwordChangerHelpers exists only for making the testing of the ensureJujudPasswordHelper function easier
type PasswordChangerHelpers interface {
	// ChangeUserPasswordLocalhost changes the password for the jujud user on the local computer using syscalls
	ChangeUserPasswordLocalhost(newPassword string) error

	// changeJujudServicesPassword changes the password for all the services created by the jujud user
	ChangeJujudServicesPassword(newPassword string, mgr ServiceManager, listServices func() ([]string, error)) error
}

// passwordChanger implements passwordChangerHelpers
type PasswordChanger struct{}

// changeUserPasswordLocalhost changes the password for username on localhost
func (c *PasswordChanger) ChangeUserPasswordLocalhost(newPassword string) error {
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

func (c *PasswordChanger) ChangeJujudServicesPassword(newPassword string, mgr ServiceManager, listServices func() ([]string, error)) error {
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
