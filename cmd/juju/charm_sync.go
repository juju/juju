// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/juju/charm"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/names"
	"github.com/juju/utils"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/utils/ssh"
)

type CharmSyncCommand struct {
	envcmd.EnvCommandBase
	toUnit    string
	charmPath string
	download  bool
	apiClient *api.Client
}

const charmSyncDoc = `
charm-sync will push the contents of a charm definition in the local 
repository up to a unit, replacing the existing one.  If --pull 
is specified this process is reversed, pulling the remote charm's 
definition down to the local repository.
If --charm is not specified CWD will be used instead.
If the charm does not exist in one of both ends charm-sync will fail.
`

func (c *CharmSyncCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "charm-sync",
		Args:    "<unit name>",
		Purpose: "sync charm in unit with local charm folder or vice versa.",
		Doc:     charmSyncDoc,
	}
}

func (c *CharmSyncCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.Errorf("unit name is missing")
	case 1:
		c.toUnit = args[0]
		return nil
	default:
		return errors.Errorf("too many arguments provided.")
	}
}

func (c *CharmSyncCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.charmPath, "charm", "", "local charm repository, if not specified current working directory will be used")
	f.BoolVar(&c.download, "pull", false, "sync local copy with unit")
}

var sshCopy = ssh.Copy

// Run will sync the contents of the charm path with the charm on the unit.
// The sync can be triggered in any of both ways (up/down).
// It will return error in case of failures.
func (c *CharmSyncCommand) Run(ctx *cmd.Context) error {
	var err error
	c.apiClient, err = c.newAPIClient()
	if err != nil {
		return errors.Trace(err)
	}
	defer c.apiClient.Close()

	// Figure out local charm.
	charmDir, err := c.inferCharm()

	// Figure out remote series which is required for remote paths.
	charmSeries, err := c.remoteUnitSeries(charmDir.Meta().Name)
	if err != nil {
		return errors.Annotatef(err, "cannote determine remote machine series")
	}

	// Pull the charm to local version.
	if c.download {
		return errors.Annotatef(c.downloadCharm(charmSeries, charmDir.Path), "cannot download charm")
	}

	// Push local charm version to unit.
	return errors.Annotatef(c.uploadCharm(charmSeries, charmDir.Path), "cannot upload charm")
}

var apiRun = func(c *CharmSyncCommand, runParams params.RunParams) ([]params.RunResult, error) {
	return c.apiClient.Run(runParams)
}

// downloadCharm will scp the charm folder from the unit and overwrite the local one.
func (c *CharmSyncCommand) downloadCharm(charmSeries, charmDirPath string) error {
	remoteUrl, err := c.unitPath(charmSeries)
	if err != nil {
		return errors.Annotatef(err, "cannote determine remote machine scp url")
	}
	args := []string{"-r", remoteUrl + "/*", charmDirPath}
	return errors.Trace(ssh.Copy(args, &ssh.Options{}))
}

// uploadCharm will scp the charm folder into a temp location on the remote
// machine and then overwrite the charm with our own.
func (c *CharmSyncCommand) uploadCharm(charmSeries, charmDirPath string) error {
	unitHostPort, err := unitURL(c)
	if err != nil {
		return errors.Trace(err)
	}

	charmTransientFolder, err := remoteTempPath(c, charmSeries)
	if err != nil {
		return errors.Annotatef(err, "cannote determine remote machine temp folder")
	}

	args := []string{"-r", charmDirPath, unitHostPort + ":" + charmTransientFolder}
	if err := sshCopy(args, &ssh.Options{}); err != nil {
		return errors.Annotatef(err, "cannot copy charm to %q", unitHostPort)
	}

	unitPath, err := remoteUnitPath(c, charmSeries)
	if err != nil {
		return errors.Annotatef(err, "cannot determine remote charm path")
	}
	//TODO (perrito666) extend this to windows workloads when we know what is te equivalent.
	// sudo is used because everything under DataDir belongs to root.
	remoteRunParams := params.RunParams{
		Commands: fmt.Sprintf("sudo cp -rax %s/* %s; rm -rf %s", charmTransientFolder, unitPath, charmTransientFolder),
		Timeout:  5 * time.Minute,
		Units:    []string{c.toUnit},
	}
	_, err = apiRun(c, remoteRunParams)
	return errors.Annotatef(err, "cannot copy charm to destination")
}

var unitURL = func(c *CharmSyncCommand) (string, error) { return c.unitURL() }

// unitURL will return a string representing the ssh user@hostPort
// required to connect to the unit.
func (c *CharmSyncCommand) unitURL() (string, error) {
	if c.toUnit == "" {
		return "", errors.Errorf("the unit name must be specified")
	}
	host, err := hostFromTarget(c, c.toUnit)
	if err != nil {
		return "", errors.Trace(err)
	}
	return "ubuntu@" + host, nil
}

// newAPIClient initialises the API connection and returns
// said client.
func (c *CharmSyncCommand) newAPIClient() (*api.Client, error) {
	st, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return st.Client(), nil
}

var hostFromTarget = func(c *CharmSyncCommand, target string) (string, error) { return c.hostFromTarget(target) }

// hostFromTarget will return the host address given a unit name.
func (c *CharmSyncCommand) hostFromTarget(target string) (string, error) {
	// If the target is neither a machine nor a unit,
	// assume it's a hostname and try it directly.
	if !names.IsValidMachine(target) && !names.IsValidUnit(target) {
		return target, nil
	}
	return c.apiClient.PublicAddress(target)
}

// inferCharm will return the local charm folder physical location
// or an error if its not possible to determine it.
func (c *CharmSyncCommand) inferCharm() (*charm.Dir, error) {
	if c.charmPath != "" {
		return PathCharm(c.charmPath)
	}
	cwdCharm, err := CwdCharm()
	return cwdCharm, errors.Annotatef(err, "charm path not supplied and current working dir cannot be used")
}

// remoteUnitSeries will return a string representing the series of the
// machine where the unit is deployed or error if not possible.
func (c *CharmSyncCommand) remoteUnitSeries(charmName string) (string, error) {
	status, err := c.apiClient.Status([]string{})
	if err != nil {
		return "", errors.Annotatef(err, "cannot determine remote charm path")
	}
	charmService, ok := status.Services[charmName]
	if !ok {
		return "", errors.Errorf("cannot find service %q", charmName)
	}
	charmUnit, ok := charmService.Units[c.toUnit]
	if !ok {
		return "", errors.Errorf("cannot find unit %q in service %q", c.toUnit, charmService.Charm)
	}
	charmMachine, ok := status.Machines[charmUnit.Machine]
	if !ok {
		return "", errors.Errorf("cannot find machine %q", charmUnit.Machine)
	}
	return charmMachine.Series, nil

}

var remoteUnitPath = func(c *CharmSyncCommand, charmSeries string) (string, error) { return c.remoteUnitPath(charmSeries) }
var pathsDataDir = paths.DataDir

// remoteUnitPath will return the path of the charm folder in the given
// target or error if it cannot be determined.
func (c *CharmSyncCommand) remoteUnitPath(charmSeries string) (string, error) {
	if !names.IsValidUnit(c.toUnit) {
		return "", errors.Errorf("invalid unit name specified: %q", c.toUnit)
	}
	unitTag := names.NewUnitTag(c.toUnit)
	dataDir, err := pathsDataDir(charmSeries)
	if err != nil {
		return "", errors.Annotatef(err, "cannot determine target data directory")
	}
	dataDir = filepath.ToSlash(dataDir)
	remotePath := path.Join(dataDir, "agents", unitTag.String(), "charm")
	return remotePath, nil
}

var remoteTempPath = func(c *CharmSyncCommand, charmSeries string) (string, error) { return c.remoteTempPath(charmSeries) }
var newUUID = utils.NewUUID
var tempDir = paths.TempDir

// remoteTempPath will return a random named folder path in the
// remote machine temp folder which will be used to make a
// transient copy of the charm.
func (c *CharmSyncCommand) remoteTempPath(charmSeries string) (string, error) {
	uuid, err := newUUID()
	if err != nil {
		return "", errors.Annotatef(err, "cannot generate an UUID for the transient charm folder")
	}
	charmTransientFolderName := fmt.Sprintf("charm_sync_%s", uuid.String())
	charmTransientPath, err := tempDir(charmSeries)
	if err != nil {
		return "", errors.Annotatef(err, "cannot generate a remote temp folder name")
	}
	charmTransientPath = filepath.ToSlash(charmTransientPath)
	return path.Join(charmTransientPath, charmTransientFolderName), nil
}

var unitPath = func(c *CharmSyncCommand, charmSeries string) (string, error) { return c.unitPath(charmSeries) }

// unitPath will return the full scp path of the charm folder
// in the given unit or error if impossible to determine
// the unit host.
func (c *CharmSyncCommand) unitPath(charmSeries string) (string, error) {
	var unitHostPort string
	if hostPort, err := unitURL(c); err == nil {
		unitHostPort = hostPort
	} else {
		return "", errors.Trace(err)
	}
	remotePath, err := remoteUnitPath(c, charmSeries)
	if err != nil {
		return "", errors.Annotatef(err, "cannot determine remote path")
	}
	unitHostPort = fmt.Sprintf("%s:%s", unitHostPort, remotePath)
	return unitHostPort, nil
}

var CwdCharm = cwdCharm

// cwdCharm will return a *charm.Dir from the current work dir
// or an error if cwd is not a charm folder.
func cwdCharm() (*charm.Dir, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, errors.Errorf("cannot determine current work dir: %v", err)
	}

	charmDir, err := pathCharm(cwd)
	return charmDir, errors.Annotatef(err, "cannot use current work directory as charm")
}

var PathCharm = pathCharm

// pathCharm will try to create a *charm.Dir with the given folder
// and return it or an error in case the path is not a charm folder.
func pathCharm(charmPath string) (*charm.Dir, error) {
	charmDir, err := charm.ReadDir(charmPath)
	return charmDir, errors.Annotatef(err, "the path does not correspond to a valid charm")
}
