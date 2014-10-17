// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"fmt"
	"strings"

	"github.com/Altoros/gosigma"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/instance"
	"github.com/juju/loggo"
	"github.com/juju/utils"
)

// This file contains implementation of CloudSigma client.
type environClient struct {
	conn    *gosigma.Client
	uuid    string
	storage *environStorage
}

type tracer struct{}

func (tracer) Logf(format string, args ...interface{}) {
	logger.Tracef(format, args...)
}

// newClient returns an instance of the CloudSigma client.
var newClient = func(cfg *environConfig) (client *environClient, err error) {
	uuid, ok := cfg.UUID()
	if !ok {
		return nil, fmt.Errorf("Environ uuid must not be empty")
	}

	logger.Debugf("creating CloudSigma client: region=%s, user=%s, password=%s, name=%q",
		cfg.region(), cfg.username(), strings.Repeat("*", len(cfg.password())), uuid)

	// create connection to CloudSigma
	conn, err := gosigma.NewClient(cfg.region(), cfg.username(), cfg.password(), nil)
	if err != nil {
		return
	}

	// configure trace logger
	if logger.LogLevel() <= loggo.TRACE {
		conn.Logger(&tracer{})
	}

	client = &environClient{
		conn: conn,
		uuid: uuid,
	}

	return
}

const (
	jujuMetaInstance            = "juju-instance"
	jujuMetaInstanceStateServer = "state-server"
	jujuMetaInstanceServer      = "server"

	jujuMetaEnvironment = "juju-environment"
)

func (c *environClient) isMyEnvironment(s gosigma.Server) bool {
	if v, ok := s.Get(jujuMetaEnvironment); ok && c.uuid == v {
		return true
	}
	return false
}

func (c *environClient) isMyServer(s gosigma.Server) bool {
	if _, ok := s.Get(jujuMetaInstance); ok {
		return c.isMyEnvironment(s)
	}
	return false
}

// this function is used to filter servers in the CloudSigma account
func (c environClient) isMyStateServer(s gosigma.Server) bool {
	if v, ok := s.Get(jujuMetaInstance); ok && v == jujuMetaInstanceStateServer {
		return c.isMyEnvironment(s)
	}
	return false
}

// instances returns a list of CloudSigma servers
func (c *environClient) instances() ([]gosigma.Server, error) {
	return c.conn.ServersFiltered(gosigma.RequestDetail, c.isMyServer)
}

// instanceMap of server ids to servers at CloudSigma account
func (c *environClient) instanceMap() (map[string]gosigma.Server, error) {
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

// returns address of the state server.
// this function is used when we move storage from local temporary storage to remote storage, that is located on the state server
func (c *environClient) stateServerAddress() (string, string, bool) {
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
func (c *environClient) stopInstance(id instance.Id) error {
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
func (c *environClient) newInstance(args environs.StartInstanceParams, img *imagemetadata.ImageMetadata) (srv gosigma.Server, drv gosigma.Drive, err error) {

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

	constraints, err := newConstraints(args.MachineConfig.Bootstrap,
		args.Constraints, img)
	if err != nil {
		return
	}
	logger.Debugf("CloudSigma Constraints: %v", constraints)

	originalDrive, err := c.conn.Drive(constraints.driveTemplate, gosigma.LibraryMedia)
	if err != nil {
		err = fmt.Errorf("query drive template: %v", err)
		return
	}

	baseName := "juju-" + c.uuid + "-" + args.MachineConfig.MachineId

	cloneParams := gosigma.CloneParams{Name: baseName}
	if drv, err = originalDrive.CloneWait(cloneParams, nil); err != nil {
		err = fmt.Errorf("error cloning drive: %v", err)
		return
	}

	if drv.Size() < constraints.driveSize {
		if err = drv.ResizeWait(constraints.driveSize); err != nil {
			err = fmt.Errorf("error resizing drive: %v", err)
			return
		}
	}

	cc, err := c.generateSigmaComponents(baseName, constraints, args, drv)
	if err != nil {
		return nil, drv, err
	}

	if srv, err = c.conn.CreateServer(cc); err != nil {
		err = fmt.Errorf("error creating new instance: %v", err)
		return
	}

	if err = srv.Start(); err != nil {
		err = fmt.Errorf("error booting new instance: %v", err)
	}

	return
}

func (c *environClient) generateSigmaComponents(baseName string, constraints *sigmaConstraints, args environs.StartInstanceParams, drv gosigma.Drive) (cc gosigma.Components, err error) {
	cc.SetName(baseName)
	cc.SetDescription(baseName)
	cc.SetSMP(constraints.cores)
	cc.SetCPU(constraints.power)
	cc.SetMem(constraints.mem)

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

	cc.SetMeta(jujuMetaEnvironment, c.uuid)

	return
}
