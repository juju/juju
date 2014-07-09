// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"labix.org/v2/mgo"

	"github.com/juju/juju/upstart"
)

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
	if len(p.DialInfo.Addrs) > 1 {
		logger.Infof("more than one state server; admin user must exist")
		return false, nil
	}
	p.DialInfo.Addrs = []string{fmt.Sprintf("127.0.0.1:%d", p.Port)}
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
	mongoService := upstart.NewService(mongoServiceName)
	if err := upstartServiceStop(mongoService); err != nil {
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

	// Restart mongo using upstart.
	if err := processSignal(cmd.Process, syscall.SIGTERM); err != nil {
		return false, fmt.Errorf("cannot kill mongod: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			return false, fmt.Errorf("mongod did not cleanly terminate: %v", err)
		}
	}
	if err := upstartServiceStart(mongoService); err != nil {
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
		if err := admin.Login(user, password); err != nil {
			return fmt.Errorf("cannot login after setting password: %v", err)
		}
	} else {
		if err := admin.RemoveUser(user); err != nil && err != mgo.ErrNotFound {
			return fmt.Errorf("cannot disable admin password: %v", err)
		}
	}
	return nil
}

// SetMongoPassword sets the mongo password in the specified databases for the given user name.
// Previous passwords are invalidated.
func SetMongoPassword(name, password string, dbs ...*mgo.Database) error {
	user := &mgo.User{
		Username: name,
		Password: password,
		Roles:    []mgo.Role{mgo.RoleReadWriteAny, mgo.RoleUserAdmin, mgo.RoleClusterAdmin},
	}
	for _, db := range dbs {
		if err := db.UpsertUser(user); err != nil {
			return fmt.Errorf("cannot set password in juju db %q for %q: %v", db.Name, name, err)
		}
	}
	return nil
}
