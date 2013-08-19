// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state/api/params"
)

// Import the providers.
import (
	_ "launchpad.net/juju-core/provider/all"
)

var logger = loggo.GetLogger("juju.builddb")

func main() {
	if err := build(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func build() error {
	environ, err := environs.NewFromName("")
	if err != nil {
		return err
	}
	conn, err := juju.NewConn(environ)
	if err != nil {
		return err
	}
	repo := &charm.LocalRepository{filepath.Dir(os.Args[0])}
	curl := charm.MustParseURL("local:precise/builddb")
	ch, err := conn.PutCharm(curl, repo, false)
	if err != nil {
		return err
	}
	service, err := conn.State.AddService("builddb", ch)
	if err != nil {
		return err
	}
	if err := service.SetExposed(); err != nil {
		return err
	}
	units, err := conn.AddUnits(service, 1, "")
	if err != nil {
		return err
	}

	logger.Infof("Waiting for unit to reach %q status...", params.StatusStarted)
	unit := units[0]
	last, info, err := unit.Status()
	if err != nil {
		return err
	}
	logger.Infof("Unit status is %q: %s", last, info)
	for last != params.StatusStarted {
		time.Sleep(2 * time.Second)
		if err := unit.Refresh(); err != nil {
			return err
		}
		status, info, err := unit.Status()
		if err != nil {
			return err
		}
		if status != last {
			logger.Infof("Unit status is %q: %s", status, info)
			last = status
		}
	}
	addr, ok := unit.PublicAddress()
	if !ok {
		return fmt.Errorf("cannot retrieve files: build unit lacks a public-address")
	}
	logger.Infof("Built files published at http://%s", addr)
	logger.Infof("Remember to destroy the environment when you're done...")
	return nil
}
