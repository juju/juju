// Copyright 2012-2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/juju/loggo"

	components "github.com/juju/juju/component/all"
)

var logger = loggo.GetLogger("juju.cmd.k8sagent")

func init() {
	if err := components.RegisterForServer(); err != nil {
		logger.Criticalf("unable to register server components: %v", err)
		os.Exit(1)
	}
}

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

func cmd(args []string) int {
	fmt.Println("Hello World!")
	return 0
}
