// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"github.com/altoros/gosigma"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/v2"
	"github.com/juju/utils/v2/arch"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/imagemetadata"
)

type environClient struct {
	conn *gosigma.Client
	uuid string
}

type tracer struct{}

func (tracer) Logf(format string, args ...interface{}) {
	logger.Tracef(format, args...)
}

// newClient returns an instance of the CloudSigma client.
var newClient = func(cloud environscloudspec.CloudSpec, uuid string) (client *environClient, err error) {
	logger.Debugf("creating CloudSigma client: id=%q", uuid)

	credAttrs := cloud.Credential.Attributes()
	username := credAttrs[credAttrUsername]
	password := credAttrs[credAttrPassword]

	// create connection to CloudSigma
	conn, err := gosigma.NewClient(cloud.Endpoint, username, password, nil)
	if err != nil {
		return nil, err
	}

	// configure trace logger
	if logger.LogLevel() <= loggo.TRACE {
		conn.Logger(&tracer{})
	}

	client = &environClient{
		conn: conn,
		uuid: uuid,
	}

	return client, nil
}

const (
	jujuMetaInstance           = "juju-instance"
	jujuMetaInstanceController = "controller"
	jujuMetaInstanceServer     = "server"

	jujuMetaEnvironment = "juju-model"
	jujuMetaCoudInit    = "cloudinit-user-data"
	jujuMetaBase64      = "base64_fields"
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

// isMyController is used to filter servers in the CloudSigma account
func (c environClient) isMyController(s gosigma.Server) bool {
	if v, ok := s.Get(jujuMetaInstance); ok && v == jujuMetaInstanceController {
		return c.isMyEnvironment(s)
	}
	return false
}

// instances returns a list of CloudSigma servers for this environment
func (c *environClient) instances() ([]gosigma.Server, error) {
	return c.conn.ServersFiltered(gosigma.RequestDetail, c.isMyServer)
}

// instanceMap of server ids to servers at CloudSigma account
func (c *environClient) instanceMap() (map[string]gosigma.Server, error) {
	servers, err := c.conn.ServersFiltered(gosigma.RequestDetail, c.isMyServer)
	if err != nil {
		return nil, errors.Trace(err)
	}

	m := make(map[string]gosigma.Server, len(servers))
	for _, s := range servers {
		m[s.UUID()] = s
	}

	return m, nil
}

//getControllerIds get list of ids for all controller instances
func (c *environClient) getControllerIds() (ids []instance.Id, err error) {
	logger.Tracef("query state...")

	servers, err := c.conn.ServersFiltered(gosigma.RequestDetail, c.isMyController)
	if err != nil {
		return []instance.Id{}, errors.Trace(err)
	}

	if len(servers) == 0 {
		return []instance.Id{}, environs.ErrNotBootstrapped
	}

	ids = make([]instance.Id, len(servers))

	for i, server := range servers {
		logger.Tracef("controller id: %s", server.UUID())
		ids[i] = instance.Id(server.UUID())
	}

	return ids, nil
}

//stopInstance stops the CloudSigma server corresponding to the given instance ID.
func (c *environClient) stopInstance(id instance.Id) error {
	uuid := string(id)
	if uuid == "" {
		return errors.New("invalid instance id")
	}

	s, err := c.conn.Server(uuid)
	if err != nil {
		return errors.Trace(err)
	}

	err = s.StopWait()
	logger.Tracef("environClient.StopInstance - stop server, %q = %v", uuid, err)

	err = s.Remove(gosigma.RecurseAllDrives)
	logger.Tracef("environClient.StopInstance - remove server, %q = %v", uuid, err)

	return nil
}

//newInstance creates and starts new instance.
func (c *environClient) newInstance(
	args environs.StartInstanceParams,
	img *imagemetadata.ImageMetadata,
	userData []byte,
	authorizedKeys string,
) (srv gosigma.Server, drv gosigma.Drive, ar string, err error) {

	defer func() {
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
	}()

	if args.InstanceConfig == nil {
		err = errors.New("invalid configuration for new instance: InstanceConfig is nil")
		return nil, nil, "", err
	}

	logger.Debugf("Tools: %v", args.Tools.URLs())
	logger.Debugf("Juju Constraints:" + args.Constraints.String())
	logger.Debugf("InstanceConfig: %#v", args.InstanceConfig)

	constraints := newConstraints(args.InstanceConfig.Bootstrap != nil, args.Constraints, img)
	logger.Debugf("CloudSigma Constraints: %v", constraints)

	originalDrive, err := c.conn.Drive(constraints.driveTemplate, gosigma.LibraryMedia)
	if err != nil {
		err = errors.Annotatef(err, "Failed to query drive template")
		return nil, nil, "", err
	}

	baseName := "juju-" + c.uuid + "-" + args.InstanceConfig.MachineId

	cloneParams := gosigma.CloneParams{Name: baseName}
	if drv, err = originalDrive.CloneWait(cloneParams, nil); err != nil {
		err = errors.Errorf("error cloning drive: %v", err)
		return nil, nil, "", err
	}

	if drv.Size() < constraints.driveSize {
		if err = drv.ResizeWait(constraints.driveSize); err != nil {
			err = errors.Errorf("error resizing drive: %v", err)
			return nil, nil, "", err
		}
	}

	cc, err := c.generateSigmaComponents(
		baseName, constraints, args, drv, userData, authorizedKeys,
	)
	if err != nil {
		return nil, nil, "", errors.Trace(err)
	}

	if srv, err = c.conn.CreateServer(cc); err != nil {
		err = errors.Annotatef(err, "error creating new instance")
		return nil, nil, "", err
	}

	if err = srv.Start(); err != nil {
		err = errors.Annotatef(err, "error booting new instance")
		return nil, nil, "", err
	}

	// populate root drive hardware characteristics
	switch originalDrive.Arch() {
	case "64":
		ar = arch.AMD64
	case "32":
		ar = arch.I386
	default:
		err = errors.Errorf("unknown arch: %v", ar)
		return nil, nil, "", err
	}

	return srv, drv, ar, nil
}

func (c *environClient) generateSigmaComponents(
	baseName string,
	constraints *sigmaConstraints,
	args environs.StartInstanceParams,
	drv gosigma.Drive,
	userData []byte,
	authorizedKeys string,
) (cc gosigma.Components, err error) {
	cc.SetName(baseName)
	cc.SetDescription(baseName)
	cc.SetSMP(constraints.cores)
	cc.SetCPU(constraints.power)
	cc.SetMem(constraints.mem)

	vncpass, err := utils.RandomPassword()
	if err != nil {
		err = errors.Errorf("error generating password: %v", err)
		return
	}
	cc.SetVNCPassword(vncpass)
	logger.Debugf("Setting ssh key: %s end", authorizedKeys)
	cc.SetSSHPublicKey(authorizedKeys)
	cc.AttachDrive(1, "0:0", "virtio", drv.UUID())
	cc.NetworkDHCP4(gosigma.ModelVirtio)

	if model.AnyJobNeedsState(args.InstanceConfig.Jobs...) {
		cc.SetMeta(jujuMetaInstance, jujuMetaInstanceController)
	} else {
		cc.SetMeta(jujuMetaInstance, jujuMetaInstanceServer)
	}

	cc.SetMeta(jujuMetaEnvironment, c.uuid)
	cc.SetMeta(jujuMetaCoudInit, string(userData))
	cc.SetMeta(jujuMetaBase64, jujuMetaCoudInit)

	return cc, nil
}
