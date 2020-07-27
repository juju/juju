// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"os"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"
	"github.com/kr/pretty"

	apicloud "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/api/sshclient"
	"github.com/juju/juju/apiserver/params"
	// "github.com/juju/juju/caas/kubernetes/provider"
	k8sexec "github.com/juju/juju/caas/kubernetes/provider/exec"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/cloudspec"
	jujussh "github.com/juju/juju/network/ssh"
)

// SSHContainer implements functionality shared by sshCommand, SCPCommand
// and DebugHooksCommand.
type SSHContainer struct {
	modelcmd.ModelCommandBase
	modelcmd.CAASOnlyCommand
	proxy           bool
	remote          bool
	noHostKeyChecks bool
	Target          string
	Args            []string
	apiAddr         string
	knownHostsPath  string
	hostChecker     jujussh.ReachableChecker

	apiClient sshAPIClient
	cloudCredentialAPI
	modelAPI
	execClientGetter func(string, cloudspec.CloudSpec) (k8sexec.Executor, error)
}

type cloudCredentialAPI interface {
	Cloud(tag names.CloudTag) (jujucloud.Cloud, error)
	Close() error

	CredentialContents(cloud, credential string, withSecrets bool) ([]params.CredentialContentResult, error)
	BestAPIVersion() int
}

type modelAPI interface {
	Close() error
	ModelInfo([]names.ModelTag) ([]params.ModelInfoResult, error)
}

func (c *SSHContainer) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.remote, "remote", false, "Target to workload container")
}

func (c *SSHContainer) setHostChecker(checker jujussh.ReachableChecker) {
	if checker == nil {
		// TODO CAAS checker!!!
		checker = defaultReachableChecker()
	}
	c.hostChecker = checker
}

// initRun initializes the API connection if required, and determines
// if SSH proxying is required. It must be called at the top of the
// command's Run method.
//
// The apiClient, apiAddr and proxy fields are initialized after this call.
func (c *SSHContainer) initRun() error {
	if err := c.ensureAPIClient(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// cleanupRun removes the temporary SSH known_hosts file (if one was
// created) and closes the API connection. It must be called at the
// end of the command's Run (i.e. as a defer).
func (c *SSHContainer) cleanupRun() {
	if c.apiClient != nil {
		c.apiClient.Close()
		c.apiClient = nil
	}

	if c.cloudCredentialAPI != nil {
		c.cloudCredentialAPI.Close()
		c.cloudCredentialAPI = nil
	}
	if c.modelAPI != nil {
		c.modelAPI.Close()
		c.modelAPI = nil
	}

}

func (c *SSHContainer) ensureAPIClient() error {
	if c.apiClient != nil {
		return nil
	}
	return errors.Trace(c.initAPIClient())
}

// initAPIClient initialises the API connection.
func (c *SSHContainer) initAPIClient() error {
	conn, err := c.NewAPIRoot()
	if err != nil {
		return errors.Trace(err)
	}
	c.apiClient = sshclient.NewFacade(conn)
	c.apiAddr = conn.Addr()

	cAPI, err := c.NewControllerAPIRoot()
	if err != nil {
		return errors.Trace(err)
	}
	c.cloudCredentialAPI = apicloud.NewClient(cAPI)
	c.modelAPI = modelmanager.NewClient(cAPI)
	// TODO: move to constructor for better test!!
	c.execClientGetter = k8sexec.NewForJujuCloudCloudSpec
	return nil
}

func (c *SSHContainer) resolveTarget(target string) (*resolvedTarget, error) {
	// out, ok := c.resolveAsAgent(target)
	// if !ok {
	// 	// Not a machine or unit agent target - use directly.
	// 	return out, nil
	// }

	// getAddress := c.reachableAddressGetter
	// if c.apiClient.BestAPIVersion() < 2 || c.forceAPIv1 {
	// 	logger.Debugf("using legacy SSHClient API v1: no support for AllAddresses()")
	// 	getAddress = c.legacyAddressGetter
	// } else if c.proxy {
	// 	// Ideally a reachability scan would be done from the
	// 	// controller's perspective but that isn't possible yet, so
	// 	// fall back to the legacy mode (i.e. use the instance's
	// 	// "private" address).
	// 	//
	// 	// This is in some ways better anyway as a both the external
	// 	// and internal addresses of an instance (if it has both) are
	// 	// likely to be accessible from the controller. With a
	// 	// reachability scan juju ssh could inadvertently end up using
	// 	// the public address when it really should be using the
	// 	// internal/private address.
	// 	logger.Debugf("proxy-ssh enabled so not doing reachability scan")
	// 	getAddress = c.legacyAddressGetter
	// }

	// return c.resolveWithRetry(*out, getAddress)
	logger.Criticalf("target -> %q", target)
	return nil, nil
}

func (c *SSHContainer) ssh(ctx *cmd.Context, enablePty bool, target *resolvedTarget) error {
	execClient, err := c.getExecClient(ctx)
	if err != nil {
		return err
	}
	ch := make(chan os.Signal, 1)
	cancel := make(chan struct{})
	ctx.InterruptNotify(ch)
	defer ctx.StopInterruptNotify(ch)
	defer close(ch)

	go func() {
		for range ch {
			select {
			case _, ok := <-cancel:
				if ok {
					close(cancel)
				}
			default:
			}
		}
	}()
	return execClient.Exec(
		k8sexec.ExecParams{
			PodName:  "mariadb-k8s-operator-0",
			Commands: []string{"bash"},
			Stdout:   ctx.GetStdout(),
			Stderr:   ctx.GetStdout(),
			Stdin:    ctx.GetStdin(),
			Tty:      true,
		},
		cancel,
	)
}

func (c *SSHContainer) getExecClient(ctxt *cmd.Context) (k8sexec.Executor, error) {
	if v := c.cloudCredentialAPI.BestAPIVersion(); v < 2 {
		return nil, errors.NotSupportedf("credential content lookup on the controller in Juju v%d", v)
	}
	_, mDetails, err := c.ModelDetails()
	if err != nil {
		return nil, err
	}

	modelTag := names.NewModelTag(mDetails.ModelUUID)
	results, err := c.modelAPI.ModelInfo([]names.ModelTag{modelTag})
	if err != nil {
		return nil, err
	}
	mInfo := results[0]
	if mInfo.Error != nil {
		return nil, errors.Annotatef(mInfo.Error, "getting model information")
	}

	credentialTag, err := names.ParseCloudCredentialTag(mInfo.Result.CloudCredentialTag)
	remoteContents, err := c.cloudCredentialAPI.CredentialContents(credentialTag.Cloud().Id(), credentialTag.Name(), true)
	if err != nil {
		return nil, err
	}
	cred := remoteContents[0]
	if cred.Error != nil {
		return nil, errors.Annotatef(cred.Error, "getting credential")
	}
	if cred.Result.Content.Valid != nil && !*cred.Result.Content.Valid {
		return nil, errors.NotValidf("model credential %q", cred.Result.Content.Name)
	}
	jujuCred := jujucloud.NewCredential(jujucloud.AuthType(cred.Result.Content.AuthType), cred.Result.Content.Attributes)

	cloud, err := c.cloudCredentialAPI.Cloud(names.NewCloudTag(cred.Result.Content.Cloud))
	if err != nil {
		return nil, err
	}
	if !jujucloud.CloudIsCAAS(cloud) {
		return nil, errors.NotValidf("cloud %q is not kubernetes cloud type", cloud.Name)
	}
	cloudSpec, err := cloudspec.MakeCloudSpec(cloud, "", &jujuCred)
	if err != nil {
		return nil, err
	}
	return c.execClientGetter(mInfo.Result.Name, cloudSpec)
}
