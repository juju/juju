package main

import (
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	corelog "log"
	"os"
	"path/filepath"
	"time"

	// Register the provider
	_ "launchpad.net/juju-core/environs/ec2"
)

func main() {
	log.Target = corelog.New(os.Stdout, "", corelog.LstdFlags)
	if err := build(); err != nil {
		corelog.Fatalf("error: %v", err)
	}
}

func build() error {
	environ, err := environs.NewFromName("")
	if err != nil {
		return err
	}
	err = juju.Bootstrap(true, nil)
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
	service, err := conn.AddService("builddb", ch)
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

	log.Printf("builddb: Waiting for unit to reach %q status...", state.UnitStarted)
	unit := units[0]
	last, info, err := unit.Status()
	if err != nil {
		return err
	}
	logStatus(last, info)
	for last != state.UnitStarted {
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
	addr, err := unit.PublicAddress()
	if err != nil {
		return err
	}
	log.Printf("builddb: Built files published at http://%s", addr)
	log.Printf("builddb: Remember to destroy the environment when you're done...")
	return nil
}

func logStatus(status state.UnitStatus, info string) {
	if info == "" {
		log.Printf("builddb: Unit status is %q", status)
	} else {
		log.Printf("builddb: Unit status is %q: %s", status, info)
	}
}
