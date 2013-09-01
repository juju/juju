// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"strings"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/agent/tools"
	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/log/syslog"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/upstart"
	"launchpad.net/juju-core/version"
)

// SimpleContext is a Context that manages unit deployments via upstart
// jobs on the local system.
type SimpleContext struct {

	// addresser is used to get the current state server addresses at the time
	// the given unit is deployed.
	addresser Addresser

	// caCert holds the CA certificate that will be used
	// to validate the state server's certificate, in PEM format.
	caCert []byte

	// initDir specifies the directory used by upstart on the local system.
	// It is typically set to "/etc/init".
	initDir string

	// dataDir specifies the directory used by juju to store its state. It
	// is typically set to "/var/lib/juju".
	dataDir string

	// logDir specifies the directory to which installed units will write
	// their log files. It is typically set to "/var/log/juju".
	logDir string

	// sysLogConfigDir specifies the directory to which the syslog conf file
	// will be written. It is set for testing and left empty for production, in
	// which case the system default is used, typically /etc/rsyslog.d
	syslogConfigDir string

	// syslogConfigPath is the full path name of the syslog conf file.
	syslogConfigPath string
}

var _ Context = (*SimpleContext)(nil)

// NewSimpleContext returns a new SimpleContext, acting on behalf of the
// specified deployer, that deploys unit agents as upstart jobs in
// "/etc/init" logging to "/var/log/juju". Paths to which agents and tools
// are installed are relative to dataDir; if dataDir is empty, it will be
// set to "/var/lib/juju".
func NewSimpleContext(dataDir string, caCert []byte, addresser Addresser) *SimpleContext {
	if dataDir == "" {
		dataDir = "/var/lib/juju"
	}
	return &SimpleContext{
		addresser: addresser,
		caCert:    caCert,
		initDir:   "/etc/init",
		dataDir:   dataDir,
		logDir:    "/var/log/juju",
	}
}

func (ctx *SimpleContext) DeployUnit(unitName, initialPassword string) (err error) {
	// Check sanity.
	svc := ctx.upstartService(unitName)
	if svc.Installed() {
		return fmt.Errorf("unit %q is already deployed", unitName)
	}

	// Link the current tools for use by the new agent.
	tag := names.UnitTag(unitName)
	_, err = tools.ChangeAgentTools(ctx.dataDir, tag, version.Current)
	toolsDir := tools.ToolsDir(ctx.dataDir, tag)
	defer removeOnErr(&err, toolsDir)

	// Retrieve the state addresses.
	// TODO: remove the state addresses when unit agent is API only.
	stateAddrs, err := ctx.addresser.StateAddresses()
	if err != nil {
		return err
	}
	logger.Debugf("state addresses: %q", stateAddrs)
	apiAddrs, err := ctx.addresser.APIAddresses()
	if err != nil {
		return err
	}
	logger.Debugf("API addresses: %q", apiAddrs)
	conf, err := agent.NewAgentConfig(
		agent.AgentConfigParams{
			DataDir:        ctx.dataDir,
			Tag:            tag,
			Password:       initialPassword,
			Nonce:          "unused",
			StateAddresses: stateAddrs,
			APIAddresses:   apiAddrs,
			CACert:         ctx.caCert,
		})
	if err != nil {
		return err
	}
	if err := conf.Write(); err != nil {
		return err
	}
	defer removeOnErr(&err, conf.Dir())

	// Install an upstart job that runs the unit agent.
	logPath := path.Join(ctx.logDir, tag+".log")
	syslogConfigRenderer := syslog.NewForwardConfig(tag, stateAddrs)
	syslogConfigRenderer.ConfigDir = ctx.syslogConfigDir
	syslogConfigRenderer.ConfigFileName = fmt.Sprintf("26-juju-%s.conf", tag)
	if err := syslogConfigRenderer.Write(); err != nil {
		return err
	}
	ctx.syslogConfigPath = syslogConfigRenderer.ConfigFilePath()
	if err := syslog.Restart(); err != nil {
		logger.Warningf("installer: cannot restart syslog daemon: %v", err)
	}
	defer removeOnErr(&err, ctx.syslogConfigPath)

	cmd := strings.Join([]string{
		path.Join(toolsDir, "jujud"), "unit",
		"--data-dir", conf.DataDir(),
		"--unit-name", unitName,
		"--debug", // TODO: propagate debug state sensibly
	}, " ")
	uconf := &upstart.Conf{
		Service: *svc,
		Desc:    "juju unit agent for " + unitName,
		Cmd:     cmd,
		Out:     logPath,
		// Propagate the provider type and container type enviroment variables.
		Env: map[string]string{
			osenv.JujuProviderType:  os.Getenv(osenv.JujuProviderType),
			osenv.JujuContainerType: os.Getenv(osenv.JujuContainerType),
		},
	}
	return uconf.Install()
}

// findUpstartJob tries to find an upstart job matching the
// given unit name in one of these formats:
//   jujud-<deployer-tag>:<unit-tag>.conf (for compatibility)
//   jujud-<unit-tag>.conf (default)
func (ctx *SimpleContext) findUpstartJob(unitName string) *upstart.Service {
	unitsAndJobs, err := ctx.deployedUnitsUpstartJobs()
	if err != nil {
		return nil
	}
	if job, ok := unitsAndJobs[unitName]; ok {
		svc := upstart.NewService(job)
		svc.InitDir = ctx.initDir
		return svc
	}
	return nil
}

func (ctx *SimpleContext) RecallUnit(unitName string) error {
	svc := ctx.findUpstartJob(unitName)
	if svc == nil || !svc.Installed() {
		return fmt.Errorf("unit %q is not deployed", unitName)
	}
	if err := svc.StopAndRemove(); err != nil {
		return err
	}
	tag := names.UnitTag(unitName)
	agentDir := agent.Dir(ctx.dataDir, tag)
	if err := os.RemoveAll(agentDir); err != nil {
		return err
	}
	if err := os.Remove(ctx.syslogConfigPath); err != nil && !os.IsNotExist(err) {
		logger.Warningf("installer: cannot remove %q: %v", ctx.syslogConfigPath, err)
	}
	// Defer this so a failure here does not impede the cleanup (as in tests).
	defer func() {
		if err := syslog.Restart(); err != nil {
			logger.Warningf("installer: cannot restart syslog daemon: %v", err)
		}
	}()
	toolsDir := tools.ToolsDir(ctx.dataDir, tag)
	return os.Remove(toolsDir)
}

var deployedRe = regexp.MustCompile("^(jujud-.*unit-([a-z0-9-]+)-([0-9]+))\\.conf$")

func (ctx *SimpleContext) deployedUnitsUpstartJobs() (map[string]string, error) {
	fis, err := ioutil.ReadDir(ctx.initDir)
	if err != nil {
		return nil, err
	}
	installed := make(map[string]string)
	for _, fi := range fis {
		if groups := deployedRe.FindStringSubmatch(fi.Name()); len(groups) == 4 {
			unitName := groups[2] + "/" + groups[3]
			if !names.IsUnit(unitName) {
				continue
			}
			installed[unitName] = groups[1]
		}
	}
	return installed, nil
}

func (ctx *SimpleContext) DeployedUnits() ([]string, error) {
	unitsAndJobs, err := ctx.deployedUnitsUpstartJobs()
	if err != nil {
		return nil, err
	}
	var installed []string
	for unitName := range unitsAndJobs {
		installed = append(installed, unitName)
	}
	return installed, nil
}

// upstartService returns an upstart.Service corresponding to the specified
// unit.
func (ctx *SimpleContext) upstartService(unitName string) *upstart.Service {
	tag := names.UnitTag(unitName)
	svcName := "jujud-" + tag
	svc := upstart.NewService(svcName)
	svc.InitDir = ctx.initDir
	return svc
}

func removeOnErr(err *error, path string) {
	if *err != nil {
		if err := os.Remove(path); err != nil {
			logger.Warningf("installer: cannot remove %q: %v", path, err)
		}
	}
}
