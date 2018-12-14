# Copyright 2014-2015 Canonical Limited.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#  http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

from charmhelpers.core.hookenv import (
    config,
    unit_get,
    service_name,
    network_get_primary_address,
)
from charmhelpers.contrib.network.ip import (
    get_address_in_network,
    is_address_in_network,
    is_ipv6,
    get_ipv6_addr,
    resolve_network_cidr,
)
from charmhelpers.contrib.hahelpers.cluster import is_clustered

PUBLIC = 'public'
INTERNAL = 'int'
ADMIN = 'admin'
ACCESS = 'access'

ADDRESS_MAP = {
    PUBLIC: {
        'binding': 'public',
        'config': 'os-public-network',
        'fallback': 'public-address',
        'override': 'os-public-hostname',
    },
    INTERNAL: {
        'binding': 'internal',
        'config': 'os-internal-network',
        'fallback': 'private-address',
        'override': 'os-internal-hostname',
    },
    ADMIN: {
        'binding': 'admin',
        'config': 'os-admin-network',
        'fallback': 'private-address',
        'override': 'os-admin-hostname',
    },
    ACCESS: {
        'binding': 'access',
        'config': 'access-network',
        'fallback': 'private-address',
        'override': 'os-access-hostname',
    },
}


def canonical_url(configs, endpoint_type=PUBLIC):
    """Returns the correct HTTP URL to this host given the state of HTTPS
    configuration, hacluster and charm configuration.

    :param configs: OSTemplateRenderer config templating object to inspect
                    for a complete https context.
    :param endpoint_type: str endpoint type to resolve.
    :param returns: str base URL for services on the current service unit.
    """
    scheme = _get_scheme(configs)

    address = resolve_address(endpoint_type)
    if is_ipv6(address):
        address = "[{}]".format(address)

    return '%s://%s' % (scheme, address)


def _get_scheme(configs):
    """Returns the scheme to use for the url (either http or https)
    depending upon whether https is in the configs value.

    :param configs: OSTemplateRenderer config templating object to inspect
                    for a complete https context.
    :returns: either 'http' or 'https' depending on whether https is
              configured within the configs context.
    """
    scheme = 'http'
    if configs and 'https' in configs.complete_contexts():
        scheme = 'https'
    return scheme


def _get_address_override(endpoint_type=PUBLIC):
    """Returns any address overrides that the user has defined based on the
    endpoint type.

    Note: this function allows for the service name to be inserted into the
    address if the user specifies {service_name}.somehost.org.

    :param endpoint_type: the type of endpoint to retrieve the override
                          value for.
    :returns: any endpoint address or hostname that the user has overridden
              or None if an override is not present.
    """
    override_key = ADDRESS_MAP[endpoint_type]['override']
    addr_override = config(override_key)
    if not addr_override:
        return None
    else:
        return addr_override.format(service_name=service_name())


def resolve_address(endpoint_type=PUBLIC, override=True):
    """Return unit address depending on net config.

    If unit is clustered with vip(s) and has net splits defined, return vip on
    correct network. If clustered with no nets defined, return primary vip.

    If not clustered, return unit address ensuring address is on configured net
    split if one is configured, or a Juju 2.0 extra-binding has been used.

    :param endpoint_type: Network endpoing type
    :param override: Accept hostname overrides or not
    """
    resolved_address = None
    if override:
        resolved_address = _get_address_override(endpoint_type)
        if resolved_address:
            return resolved_address

    vips = config('vip')
    if vips:
        vips = vips.split()

    net_type = ADDRESS_MAP[endpoint_type]['config']
    net_addr = config(net_type)
    net_fallback = ADDRESS_MAP[endpoint_type]['fallback']
    binding = ADDRESS_MAP[endpoint_type]['binding']
    clustered = is_clustered()

    if clustered and vips:
        if net_addr:
            for vip in vips:
                if is_address_in_network(net_addr, vip):
                    resolved_address = vip
                    break
        else:
            # NOTE: endeavour to check vips against network space
            #       bindings
            try:
                bound_cidr = resolve_network_cidr(
                    network_get_primary_address(binding)
                )
                for vip in vips:
                    if is_address_in_network(bound_cidr, vip):
                        resolved_address = vip
                        break
            except NotImplementedError:
                # If no net-splits configured and no support for extra
                # bindings/network spaces so we expect a single vip
                resolved_address = vips[0]
    else:
        if config('prefer-ipv6'):
            fallback_addr = get_ipv6_addr(exc_list=vips)[0]
        else:
            fallback_addr = unit_get(net_fallback)

        if net_addr:
            resolved_address = get_address_in_network(net_addr, fallback_addr)
        else:
            # NOTE: only try to use extra bindings if legacy network
            #       configuration is not in use
            try:
                resolved_address = network_get_primary_address(binding)
            except NotImplementedError:
                resolved_address = fallback_addr

    if resolved_address is None:
        raise ValueError("Unable to resolve a suitable IP address based on "
                         "charm state and configuration. (net_type=%s, "
                         "clustered=%s)" % (net_type, clustered))

    return resolved_address


def get_vip_in_network(network):
    matching_vip = None
    vips = config('vip')
    if vips:
        for vip in vips.split():
            if is_address_in_network(network, vip):
                matching_vip = vip
    return matching_vip
