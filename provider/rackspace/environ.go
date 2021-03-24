// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
	"context"
	"io/ioutil"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/v2/ssh"

	jujuos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/series"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	envcontext "github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/common"
)

type environ struct {
	environs.Environ
}

var bootstrap = common.Bootstrap

// Bootstrap implements environs.Environ.
func (e environ) Bootstrap(ctx environs.BootstrapContext, callCtx envcontext.ProviderCallContext, params environs.BootstrapParams) (*environs.BootstrapResult, error) {
	// can't redirect to openstack provider as usually, because correct environ should be passed for common.Bootstrap
	return bootstrap(ctx, e, callCtx, params)
}

var waitSSH = common.WaitSSH

// StartInstance implements environs.Environ.
func (e environ) StartInstance(ctx envcontext.ProviderCallContext, args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	osString, err := series.GetOSFromSeries(args.Tools.OneSeries())
	if err != nil {
		return nil, errors.Trace(err)
	}
	fwmode := e.Config().FirewallMode()
	if osString == jujuos.Windows && fwmode != config.FwNone {
		return nil, errors.Errorf("rackspace provider doesn't support firewalls for windows instances")

	}
	r, err := e.Environ.StartInstance(ctx, args)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if fwmode != config.FwNone {
		timeout := environs.BootstrapDialOpts{
			Timeout:        time.Minute * 5,
			RetryDelay:     time.Second * 5,
			AddressesDelay: time.Second * 20,
		}
		addr, err := waitSSH(
			context.Background(),
			ioutil.Discard,
			ssh.DefaultClient,
			common.GetCheckNonceCommand(args.InstanceConfig),
			&common.RefreshableInstance{r.Instance, e},
			ctx,
			timeout,
			common.DefaultHostSSHOptions,
		)
		if err != nil {
			return nil, errors.Trace(err)
		}
		client := newInstanceConfigurator(addr)
		apiPort := 0
		if args.InstanceConfig.Controller != nil {
			apiPort = args.InstanceConfig.Controller.Config.APIPort()
		}
		err = client.DropAllPorts([]int{apiPort, 22}, addr)
		if err != nil {
			common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
			return nil, errors.Trace(err)
		}
	}
	return r, nil
}

var newInstanceConfigurator = common.NewSshInstanceConfigurator

// Provider implements environs.Environ.
func (e environ) Provider() environs.EnvironProvider {
	return providerInstance
}
