// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"bytes"
	"os/exec"

	"github.com/juju/errors"
	"github.com/juju/utils/apt"

	"github.com/juju/juju/version"
)

func aptGetInstallMongod(numaCtl bool) error {
	// Only Quantal requires the PPA.
	if version.Current.Series == "quantal" {
		if err := addAptRepository("ppa:juju/stable"); err != nil {
			return err
		}
	}
	mongoPkg := packageForSeries(version.Current.Series)

	aptPkgs := []string{mongoPkg}
	if numaCtl {
		aptPkgs = []string{mongoPkg, numaCtlPkg}
		logger.Infof("installing %s and %s", mongoPkg, numaCtlPkg)
	} else {
		logger.Infof("installing %s", mongoPkg)
	}
	cmds := apt.GetPreparePackages(aptPkgs, version.Current.Series)
	for _, cmd := range cmds {
		if err := apt.GetInstall(cmd...); err != nil {
			return err
		}
	}
	return nil
}

func addAptRepository(name string) error {
	// add-apt-repository requires python-software-properties
	cmds := apt.GetPreparePackages(
		[]string{"python-software-properties"},
		version.Current.Series,
	)
	logger.Infof("installing python-software-properties")
	for _, cmd := range cmds {
		if err := apt.GetInstall(cmd...); err != nil {
			return err
		}
	}

	logger.Infof("adding apt repository %q", name)
	cmd := exec.Command("add-apt-repository", "-y", name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Annotatef(err, "cannot add apt repository (output %s)", bytes.TrimSpace(out))
	}
	return nil
}

// packageForSeries returns the name of the mongo package for the series
// of the machine that it is going to be running on.
func packageForSeries(series string) string {
	switch series {
	case "precise", "quantal", "raring", "saucy":
		return "mongodb-server"
	default:
		// trusty and onwards
		return "juju-mongodb"
	}
}
