// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"os"
	"path/filepath"
	"syscall"

	"github.com/juju/juju/juju/names"

	"bitbucket.org/kardianos/service"
)

func runService() {
	var name = "juju"
	var displayName = "juju service"
	var desc = "juju service"

	var s, err = service.NewService(name, displayName, desc)
	if err != nil {
		logger.Errorf("%s", err)
		os.Exit(1)
	}

	run := func() error {
		go Main(os.Args)
		return nil
	}
	stop := func() error {
		os.Exit(0)
		return nil
	}
	err = s.Run(run, stop)

	if err != nil {
		s.Error(err.Error())
	}
}

func runConsole() {
	Main(os.Args)
}

func main() {
	var mode uint32
	err := syscall.GetConsoleMode(syscall.Stdin, &mode)

	isConsole := err == nil

	commandName := filepath.Base(os.Args[0])

	if isConsole == true || commandName != names.Jujud {
		runConsole()
	} else {
		runService()
	}
}
