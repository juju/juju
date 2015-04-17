// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
)

// AdminUser is the name of the user that is initially created in mongo.
const AdminUser = "admin"

var (
	processSignal = (*os.Process).Signal
)

type EnsureAdminUserParams struct {
	// DialInfo specifies how to connect to the mongo server.
	DialInfo *mgo.DialInfo
	// Namespace is the agent namespace, used to derive the Mongo service name.
	Namespace string
	// DataDir is the Juju data directory, used to start a --noauth server.
	DataDir string
	// Port is the listening port of the Mongo server.
	Port int
	// User holds the user to log in to the mongo server as.
	User string
	// Password holds the password for the user to log in as.
	Password string
}

// EnsureAdminUser ensures that the specified user and password
// are added to the admin database.
//
// This function will stop the Mongo service if it needs to add
// the admin user, as it must restart Mongo in --noauth mode.
func EnsureAdminUser(p EnsureAdminUserParams) (added bool, err error) {
	portStr := strconv.Itoa(p.Port)
	localIPv4Addr := net.JoinHostPort("127.0.0.1", portStr)
	localIPv6Addr := net.JoinHostPort("::1", portStr)
	if len(p.DialInfo.Addrs) > 1 {
		// Verify the addresses are for different servers.
		for _, addr := range p.DialInfo.Addrs {
			switch addr {
			case localIPv4Addr, localIPv6Addr:
				continue
			default:
				logger.Infof("more than one state server; admin user must exist")
				return false, nil
			}
		}
	}
	p.DialInfo.Addrs = []string{localIPv4Addr, localIPv6Addr}
	p.DialInfo.Direct = true

	// Attempt to login to the admin database first.
	session, err := mgo.DialWithInfo(p.DialInfo)
	if err != nil {
		return false, fmt.Errorf("can't dial mongo to ensure admin user: %v", err)
	}
	session.SetSocketTimeout(SocketTimeout)
	err = session.DB("admin").Login(p.User, p.Password)
	session.Close()
	if err == nil {
		return false, nil
	}
	logger.Debugf("admin login failed: %v", err)

	// Login failed, so we need to add the user.
	// Stop mongo, so we can start it in --noauth mode.
	mongoServiceName := ServiceName(p.Namespace)
	mongoService, err := discoverService(mongoServiceName)
	if err != nil {
		return false, errors.Annotatef(err, "failed to discover service", mongoServiceName)
	}
	if err := mongoService.Stop(); err != nil {
		return false, fmt.Errorf("failed to stop %v: %v", mongoServiceName, err)
	}

	// Start mongod in --noauth mode.
	logger.Debugf("starting mongo with --noauth")
	cmd, err := noauthCommand(p.DataDir, p.Port)
	if err != nil {
		return false, fmt.Errorf("failed to prepare mongod command: %v", err)
	}
	if err := cmd.Start(); err != nil {
		return false, fmt.Errorf("failed to start mongod: %v", err)
	}
	defer cmd.Process.Kill()

	// Add the user to the admin database.
	logger.Debugf("setting admin password")
	if session, err = mgo.DialWithInfo(p.DialInfo); err != nil {
		return false, fmt.Errorf("can't dial mongo to ensure admin user: %v", err)
	}
	err = SetAdminMongoPassword(session, p.User, p.Password)
	session.Close()
	if err != nil {
		return false, fmt.Errorf("failed to add %q to admin database: %v", p.User, err)
	}
	logger.Infof("added %q to admin database", p.User)

	// Restart mongo using the init system.
	if err := processSignal(cmd.Process, syscall.SIGTERM); err != nil {
		return false, fmt.Errorf("cannot kill mongod: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			return false, fmt.Errorf("mongod did not cleanly terminate: %v", err)
		}
	}
	if err := mongoService.Start(); err != nil {
		return false, err
	}
	return true, nil
}

// SetAdminMongoPassword sets the administrative password
// to access a mongo database. If the password is non-empty,
// all subsequent attempts to access the database must
// be authorized; otherwise no authorization is required.
func SetAdminMongoPassword(session *mgo.Session, user, password string) error {
	admin := session.DB("admin")
	if password != "" {
		if err := admin.UpsertUser(&mgo.User{
			Username: user,
			Password: password,
			Roles:    []mgo.Role{mgo.RoleDBAdminAny, mgo.RoleUserAdminAny, mgo.RoleClusterAdmin, mgo.RoleReadWriteAny},
		}); err != nil {
			return fmt.Errorf("cannot set admin password: %v", err)
		}
	} else {
		if err := admin.RemoveUser(user); err != nil && err != mgo.ErrNotFound {
			return fmt.Errorf("cannot disable admin password: %v", err)
		}
	}
	return nil
}
