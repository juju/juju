package main

import (
	"fmt"
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
	err = environ.Bootstrap(true)
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

	log.Printf("Waiting for unit to reach started status...")
	unit := units[0]
	status, info, err := unit.Status()
	if err != nil {
		return err
	}
	for status != state.UnitStarted {
		time.Sleep(2 * time.Second)
		status, info, err = unit.Status()
		if err != nil {
			return err
		}
	}
	addr, err := unit.PublicAddress()
	if err != nil {
		return err
	}
	log.Printf("Built files published at http://%s", addr)
	log.Printf("Remember to destroy the environment when you're done...")
	return nil
}
