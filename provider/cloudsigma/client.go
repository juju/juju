// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"fmt"
	"strings"

	"github.com/Altoros/gosigma"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/loggo"
	"github.com/juju/utils"
)

// This file contains implementation of CloudSigma client.
type environClient struct {
	conn     *gosigma.Client
	region   string
	username string
	password string
	name     string
	storage  *environStorage
}

type tracer struct{}

func (tracer) Logf(format string, args ...interface{}) {
	logger.Tracef(format, args...)
}

// newClient creates new CloudSigma client connection.
var newClient = func(cfg *environConfig) (*environClient, error) {

	// fetch and validate configuration
	region, err := gosigma.ResolveEndpoint(cfg.region())
	if err != nil {
		region = cfg.region()
	}
	username := cfg.username()
	password := cfg.password()
	name := cfg.Name()

	logger.Debugf("creating CloudSigma client: region=%s, user=%s, password=%s, name=%q",
		region, username, strings.Repeat("*", len(password)), name)

	// create connection to CloudSigma
	conn, err := gosigma.NewClient(region, username, password, nil)
	if err != nil {
		return nil, err
	}

	// configure trace logger
	if logger.LogLevel() <= loggo.TRACE {
		conn.Logger(&tracer{})
	}

	c := &environClient{
		conn:     conn,
		region:   region,
		name:     name,
		username: username,
		password: password,
	}

	return c, nil
}

// configChanged checks if CloudSigma client environment configuration is changed
func (c environClient) configChanged(cfg *environConfig) bool {
	// fetch configuration
	region, err := gosigma.ResolveEndpoint(cfg.region())
	if err != nil {
		return true
	}
	username := cfg.username()
	password := cfg.password()
	name := cfg.Name()

	// compare
	if region != c.region || username != c.username ||
		password != c.password || name != c.name {
		return true
	}

	return false
}

const (
	jujuMetaInstance            = "juju-instance"
	jujuMetaInstanceStateServer = "state-server"
	jujuMetaInstanceServer      = "server"

	jujuMetaEnvironment = "juju-environment"
)

func (c environClient) isMyEnvironment(s gosigma.Server) bool {
	if v, _ := s.Get(jujuMetaEnvironment); c.name == v {
		return true
	}
	return false
}

func (c environClient) isMyServer(s gosigma.Server) bool {
	if _, ok := s.Get(jujuMetaInstance); ok {
		return c.isMyEnvironment(s)
	}
	return false
}

func (c environClient) isMyStateServer(s gosigma.Server) bool {
	if v, ok := s.Get(jujuMetaInstance); ok && v == jujuMetaInstanceStateServer {
		return c.isMyEnvironment(s)
	}
	return false
}

// instances of servers at CloudSigma account
func (c environClient) instances() ([]gosigma.Server, error) {
	return c.conn.ServersFiltered(gosigma.RequestDetail, c.isMyServer)
}

// instanceMap of server ids to servers at CloudSigma account
func (c environClient) instanceMap() (map[string]gosigma.Server, error) {
	servers, err := c.conn.ServersFiltered(gosigma.RequestDetail, c.isMyServer)
	if err != nil {
		return nil, err
	}

	m := make(map[string]gosigma.Server, len(servers))
	for _, s := range servers {
		m[s.UUID()] = s
	}

	return m, nil
}

func (c environClient) stateServerAddress() (string, string, bool) {
	logger.Debugf("query state...")

	servers, err := c.conn.ServersFiltered(gosigma.RequestDetail, c.isMyStateServer)
	if err != nil {
		return "", "", false
	}

	logger.Debugf("...servers count: %d", len(servers))
	if logger.LogLevel() <= loggo.TRACE {
		for _, s := range servers {
			logger.Tracef("... %s", s)
		}
	}

	for _, s := range servers {
		if s.Status() != gosigma.ServerRunning {
			continue
		}
		if addrs := s.IPv4(); len(addrs) != 0 {
			return s.UUID(), addrs[0], true
		}
	}

	return "", "", false
}

// stop instance
func (c environClient) stopInstance(id instance.Id) error {
	uuid := string(id)
	if uuid == "" {
		return fmt.Errorf("invalid instance id")
	}

	var err error

	s, err := c.conn.Server(uuid)
	if err != nil {
		return err
	}

	if c.storage != nil && c.isMyStateServer(s) {
		c.storage.onStateInstanceStop(uuid)
	}

	err = s.StopWait()
	logger.Tracef("environClient.StopInstance - stop server, %q = %v", uuid, err)

	err = s.Remove(gosigma.RecurseAllDrives)
	logger.Tracef("environClient.StopInstance - remove server, %q = %v", uuid, err)

	return nil
}

// start new instance
func (c environClient) newInstance(args environs.StartInstanceParams) (srv gosigma.Server, drv gosigma.Drive, err error) {

	cleanup := func() {
		if err == nil {
			return
		}
		if srv != nil {
			srv.Remove(gosigma.RecurseAllDrives)
		} else if drv != nil {
			drv.Remove()
		}
		srv = nil
		drv = nil
	}
	defer cleanup()

	if args.MachineConfig == nil {
		err = fmt.Errorf("invalid configuration for new instance")
		return
	}

	logger.Tracef("Tools: %v", args.Tools.URLs())
	logger.Tracef("Juju Constraints:" + args.Constraints.String())
	logger.Tracef("MachineConfig: %#v", args.MachineConfig)

	cs, err := newConstraints(args.MachineConfig.Bootstrap,
		args.Constraints, args.MachineConfig.Tools.Version.Series)
	if err != nil {
		return
	}
	logger.Debugf("CloudSigma Constraints: %v", cs)

	originalDrive, err := c.conn.Drive(cs.driveTemplate, gosigma.LibraryMedia)
	if err != nil {
		err = fmt.Errorf("query drive template: %v", err)
		return
	}

	baseName := "juju-" + c.name + "-" + args.MachineConfig.MachineId

	cloneParams := gosigma.CloneParams{Name: baseName}
	if drv, err = originalDrive.CloneWait(cloneParams, nil); err != nil {
		err = fmt.Errorf("error cloning drive: %v", err)
		return
	}

	if drv.Size() < cs.driveSize {
		if err = drv.ResizeWait(cs.driveSize); err != nil {
			err = fmt.Errorf("error resizing drive: %v", err)
			return
		}
	}

	var cc gosigma.Components
	cc.SetName(baseName)
	cc.SetDescription(baseName)

	if cs.cores != 0 {
		cc.SetSMP(cs.cores)
	}

	if cs.power != 0 {
		cc.SetCPU(cs.power)
	}

	if cs.mem != 0 {
		cc.SetMem(cs.mem)
	}

	vncpass, err := utils.RandomPassword()
	if err != nil {
		err = fmt.Errorf("error generating password: %v", err)
		return
	}
	cc.SetVNCPassword(vncpass)

	cc.SetSSHPublicKey(args.MachineConfig.AuthorizedKeys)
	cc.AttachDrive(1, "0:0", "virtio", drv.UUID())
	cc.NetworkDHCP4(gosigma.ModelVirtio)

	if args.MachineConfig.Bootstrap {
		cc.SetMeta(jujuMetaInstance, jujuMetaInstanceStateServer)
	} else {
		cc.SetMeta(jujuMetaInstance, jujuMetaInstanceServer)
	}

	if c.name != "" {
		cc.SetMeta(jujuMetaEnvironment, c.name)
	}

	if srv, err = c.conn.CreateServer(cc); err != nil {
		err = fmt.Errorf("error creating new instance: %v", err)
		return
	}

	if err = srv.StartWait(); err != nil {
		err = fmt.Errorf("error booting new instance: %v", err)
		return
	}

	var ipaddr string
	instanceNetworkAvailable := func(s gosigma.Server) bool {
		ipaddr = sigmaInstance{s}.findIPv4()
		return ipaddr != ""
	}
	if err = srv.Wait(instanceNetworkAvailable); err != nil {
		err = fmt.Errorf("error waiting for instance IP address: %v", err)
		return
	}

	logger.Tracef("instance ip %q", ipaddr)

	return
}
