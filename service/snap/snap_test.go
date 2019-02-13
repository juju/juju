// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package snap_test

import (
	"os/exec"
	"strings"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service/common"
	"github.com/juju/juju/service/snap"
)

type validationSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&validationSuite{})

func (*validationSuite) TestBackgroundServiceNeedsNonZeroName(c *gc.C) {
	empty := snap.BackgroundService{}
	fail := empty.Validate()
	c.Check(fail, gc.ErrorMatches, "backgroundService.Name must be non-empty.*")
}

func (*validationSuite) TestBackgroundServiceNeedsLegalName(c *gc.C) {
	illegal := snap.BackgroundService{Name: "23-==+++"}
	fail := illegal.Validate()
	c.Check(fail, gc.ErrorMatches, ".* fails validation check - not valid")
}

func (*validationSuite) TestValidateJujuDbDaemon(c *gc.C) {
	service := snap.BackgroundService{
		Name:            "daemon",
		EnableAtStartup: true,
	}
	err := service.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (*validationSuite) TestValidateJujuDbSnap(c *gc.C) {
	// manually
	jujudb := snap.App{
		Name:               "juju-db",
		Channel:            "edge",
		ConfinementPolicy:  "jailmode",
		BackgroundServices: []snap.BackgroundService{{Name: "daemon"}},
		Prerequisites:      []snap.App{{Name: "core", Channel: "stable", ConfinementPolicy: "jailmode"}},
	}
	err := jujudb.Validate()
	c.Check(err, jc.ErrorIsNil)

	// via NewService
	jujudbService, err := snap.NewService("juju-db", common.Conf{Desc: "juju-db snap"}, snap.Command, "edge", "jailmode", []snap.BackgroundService{}, []snap.App{})
	c.Check(err, jc.ErrorIsNil)
	c.Check(jujudbService.Validate(), jc.ErrorIsNil)

}

func (*validationSuite) TestNewApp(c *gc.C) {
	app := snap.NewApp("core")
	c.Check(app, jc.DeepEquals, snap.App{
		Name:               "core",
		ConfinementPolicy:  snap.DefaultConfinementPolicy,
		Channel:            snap.DefaultChannel,
		BackgroundServices: []snap.BackgroundService{},
		Prerequisites:      []snap.App{},
	})
}

type externalCommandsSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&externalCommandsSuite{})

func (*externalCommandsSuite) TestSnapCommandIsAValidCommand(c *gc.C) {
	_, err := exec.LookPath(snap.Command)
	c.Check(err, gc.NotNil)
}

func (*externalCommandsSuite) TestSnapListCommandreValidShellCommand(c *gc.C) {
	listCommand := snap.ListCommand()
	listCommandParts := strings.Fields(listCommand)

	// check that we refer to valid commands
	executable := listCommandParts[0]
	_, err := exec.LookPath(executable)

	for i, token := range listCommandParts {
		// we've found a pipe, next token should be executable
		if token == "|" {
			_, err = exec.LookPath(listCommandParts[i+1])
		}
	}
	c.Check(err, gc.NotNil)
}
