package main

import (
	"fmt"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state/api/params"
	stdlog "log"
	"os"
	"path/filepath"
	"time"
)

// Import the providers.
import (
	_ "launchpad.net/juju-core/environs/all"
)

func main() {
	log.SetTarget(stdlog.New(os.Stdout, "", stdlog.LstdFlags))
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
	units, err := conn.AddUnits(service, 1)
	if err != nil {
		return err
	}

	log.Infof("builddb: Waiting for unit to reach %q status...", params.StatusStarted)
	unit := units[0]
	last, info, err := unit.Status()
	if err != nil {
		return err
	}
	logStatus(last, info)
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
			logStatus(status, info)
			last = status
		}
	}
	addr, ok := unit.PublicAddress()
	if !ok {
		return fmt.Errorf("cannot retrieve files: build unit lacks a public-address")
	}
	log.Noticef("builddb: Built files published at http://%s", addr)
	log.Noticef("builddb: Remember to destroy the environment when you're done...")
	return nil
}

func logStatus(status params.Status, info string) {
	if info == "" {
		log.Infof("builddb: Unit status is %q", status)
	} else {
		log.Infof("builddb: Unit status is %q: %s", status, info)
	}
}
