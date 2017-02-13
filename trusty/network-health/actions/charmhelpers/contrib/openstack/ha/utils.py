# Copyright 2014-2016 Canonical Limited.
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

#
# Copyright 2016 Canonical Ltd.
#
# Authors:
#  Openstack Charmers <
#

"""
Helpers for high availability.
"""

import re

from charmhelpers.core.hookenv import (
    log,
    relation_set,
    charm_name,
    config,
    status_set,
    DEBUG,
)

from charmhelpers.core.host import (
    lsb_release
)

from charmhelpers.contrib.openstack.ip import (
    resolve_address,
)


class DNSHAException(Exception):
    """Raised when an error occurs setting up DNS HA
    """

    pass


def update_dns_ha_resource_params(resources, resource_params,
                                  relation_id=None,
                                  crm_ocf='ocf:maas:dns'):
    """ Check for os-*-hostname settings and update resource dictionaries for
    the HA relation.

    @param resources: Pointer to dictionary of resources.
                      Usually instantiated in ha_joined().
    @param resource_params: Pointer to dictionary of resource parameters.
                            Usually instantiated in ha_joined()
    @param relation_id: Relation ID of the ha relation
    @param crm_ocf: Corosync Open Cluster Framework resource agent to use for
                    DNS HA
    """

    # Validate the charm environment for DNS HA
    assert_charm_supports_dns_ha()

    settings = ['os-admin-hostname', 'os-internal-hostname',
                'os-public-hostname', 'os-access-hostname']

    # Check which DNS settings are set and update dictionaries
    hostname_group = []
    for setting in settings:
        hostname = config(setting)
        if hostname is None:
            log('DNS HA: Hostname setting {} is None. Ignoring.'
                ''.format(setting),
                DEBUG)
            continue
        m = re.search('os-(.+?)-hostname', setting)
        if m:
            networkspace = m.group(1)
        else:
            msg = ('Unexpected DNS hostname setting: {}. '
                   'Cannot determine network space name'
                   ''.format(setting))
            status_set('blocked', msg)
            raise DNSHAException(msg)

        hostname_key = 'res_{}_{}_hostname'.format(charm_name(), networkspace)
        if hostname_key in hostname_group:
            log('DNS HA: Resource {}: {} already exists in '
                'hostname group - skipping'.format(hostname_key, hostname),
                DEBUG)
            continue

        hostname_group.append(hostname_key)
        resources[hostname_key] = crm_ocf
        resource_params[hostname_key] = (
            'params fqdn="{}" ip_address="{}" '
            ''.format(hostname, resolve_address(endpoint_type=networkspace,
                                                override=False)))

    if len(hostname_group) >= 1:
        log('DNS HA: Hostname group is set with {} as members. '
            'Informing the ha relation'.format(' '.join(hostname_group)),
            DEBUG)
        relation_set(relation_id=relation_id, groups={
            'grp_{}_hostnames'.format(charm_name()): ' '.join(hostname_group)})
    else:
        msg = 'DNS HA: Hostname group has no members.'
        status_set('blocked', msg)
        raise DNSHAException(msg)


def assert_charm_supports_dns_ha():
    """Validate prerequisites for DNS HA
    The MAAS client is only available on Xenial or greater
    """
    if lsb_release().get('DISTRIB_RELEASE') < '16.04':
        msg = ('DNS HA is only supported on 16.04 and greater '
               'versions of Ubuntu.')
        status_set('blocked', msg)
        raise DNSHAException(msg)
    return True
