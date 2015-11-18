// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/environs"
)

var logger = loggo.GetLogger("juju.provider.rackspace")

type environProvider struct {
	environs.EnvironProvider
}

var providerInstance *environProvider

// BoilerplateConfig implements environs.EnvironProvider.
func (p environProvider) BoilerplateConfig() string {
	return `
# https://juju.ubuntu.com/docs/config-rackspace.html
rackspace:
    type: rackspace

    # network specifies the network label or uuid to bring machines up
    # on, in the case where multiple networks exist. It may be omitted
    # otherwise.
    #
    # network: <your network label or uuid>

    # agent-metadata-url specifies the location of the Juju tools and
    # metadata. It defaults to the global public tools metadata
    # location https://streams.canonical.com/tools.
    #
    # agent-metadata-url:  https://your-agent-metadata-url

    # image-metadata-url specifies the location of Ubuntu cloud image
    # metadata. It defaults to the global public image metadata
    # location https://cloud-images.ubuntu.com/releases.
    #
    # image-metadata-url:  https://your-image-metadata-url

    # image-stream chooses a simplestreams stream from which to select
    # OS images, for example daily or released images (or any other stream
    # available on simplestreams).
    #
    # image-stream: "released"

    # agent-stream chooses a simplestreams stream from which to select tools,
    # for example released or proposed tools (or any other stream available
    # on simplestreams).
    #
    # agent-stream: "released"

    # tenant-name holds the openstack tenant name. It defaults to the
    # environment variable OS_TENANT_NAME.
    #
    # tenant-name: <your tenant name>

    # region holds the openstack region. It defaults to the
    # environment variable OS_REGION_NAME.
    #
    # region: <your region>

    # The auth-mode, username and password attributes are used for
    # userpass authentication (the default).
    #
    # auth-mode holds the authentication mode. For user-password
    # authentication, auth-mode should be "userpass" and username and
    # password should be set appropriately; they default to the
    # environment variables OS_USERNAME and OS_PASSWORD respectively.
    #
    # auth-mode: userpass
    # username: <your username>
    # password: <secret>

    # For key-pair authentication, auth-mode should be "keypair" and
    # access-key and secret-key should be set appropriately; they
    # default to the environment variables OS_ACCESS_KEY and
    # OS_SECRET_KEY respectively.
    #
    # auth-mode: keypair
    # access-key: <secret>
    # secret-key: <secret>

    # Whether or not to refresh the list of available updates for an
    # OS. The default option of true is recommended for use in
    # production systems, but disabling this can speed up local
    # deployments for development or testing.
    #
    # enable-os-refresh-update: true

    # Whether or not to perform OS upgrades when machines are
    # provisioned. The default option of true is recommended for use
    # in production systems, but disabling this can speed up local
    # deployments for development or testing.
    #
    # enable-os-upgrade: true
`[1:]
}
