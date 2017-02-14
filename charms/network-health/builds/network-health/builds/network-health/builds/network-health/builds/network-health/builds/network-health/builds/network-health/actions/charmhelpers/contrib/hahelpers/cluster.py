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

#
# Copyright 2012 Canonical Ltd.
#
# Authors:
#  James Page <james.page@ubuntu.com>
#  Adam Gandelman <adamg@ubuntu.com>
#

"""
Helpers for clustering and determining "cluster leadership" and other
clustering-related helpers.
"""

import subprocess
import os

from socket import gethostname as get_unit_hostname

import six

from charmhelpers.core.hookenv import (
    log,
    relation_ids,
    related_units as relation_list,
    relation_get,
    config as config_get,
    INFO,
    DEBUG,
    WARNING,
    unit_get,
    is_leader as juju_is_leader,
    status_set,
)
from charmhelpers.core.decorators import (
    retry_on_exception,
)
from charmhelpers.core.strutils import (
    bool_from_string,
)

DC_RESOURCE_NAME = 'DC'


class HAIncompleteConfig(Exception):
    pass


class HAIncorrectConfig(Exception):
    pass


class CRMResourceNotFound(Exception):
    pass


class CRMDCNotFound(Exception):
    pass


def is_elected_leader(resource):
    """
    Returns True if the charm executing this is the elected cluster leader.

    It relies on two mechanisms to determine leadership:
        1. If juju is sufficiently new and leadership election is supported,
        the is_leader command will be used.
        2. If the charm is part of a corosync cluster, call corosync to
        determine leadership.
        3. If the charm is not part of a corosync cluster, the leader is
        determined as being "the alive unit with the lowest unit numer". In
        other words, the oldest surviving unit.
    """
    try:
        return juju_is_leader()
    except NotImplementedError:
        log('Juju leadership election feature not enabled'
            ', using fallback support',
            level=WARNING)

    if is_clustered():
        if not is_crm_leader(resource):
            log('Deferring action to CRM leader.', level=INFO)
            return False
    else:
        peers = peer_units()
        if peers and not oldest_peer(peers):
            log('Deferring action to oldest service unit.', level=INFO)
            return False
    return True


def is_clustered():
    for r_id in (relation_ids('ha') or []):
        for unit in (relation_list(r_id) or []):
            clustered = relation_get('clustered',
                                     rid=r_id,
                                     unit=unit)
            if clustered:
                return True
    return False


def is_crm_dc():
    """
    Determine leadership by querying the pacemaker Designated Controller
    """
    cmd = ['crm', 'status']
    try:
        status = subprocess.check_output(cmd, stderr=subprocess.STDOUT)
        if not isinstance(status, six.text_type):
            status = six.text_type(status, "utf-8")
    except subprocess.CalledProcessError as ex:
        raise CRMDCNotFound(str(ex))

    current_dc = ''
    for line in status.split('\n'):
        if line.startswith('Current DC'):
            # Current DC: juju-lytrusty-machine-2 (168108163) - partition with quorum
            current_dc = line.split(':')[1].split()[0]
    if current_dc == get_unit_hostname():
        return True
    elif current_dc == 'NONE':
        raise CRMDCNotFound('Current DC: NONE')

    return False


@retry_on_exception(5, base_delay=2,
                    exc_type=(CRMResourceNotFound, CRMDCNotFound))
def is_crm_leader(resource, retry=False):
    """
    Returns True if the charm calling this is the elected corosync leader,
    as returned by calling the external "crm" command.

    We allow this operation to be retried to avoid the possibility of getting a
    false negative. See LP #1396246 for more info.
    """
    if resource == DC_RESOURCE_NAME:
        return is_crm_dc()
    cmd = ['crm', 'resource', 'show', resource]
    try:
        status = subprocess.check_output(cmd, stderr=subprocess.STDOUT)
        if not isinstance(status, six.text_type):
            status = six.text_type(status, "utf-8")
    except subprocess.CalledProcessError:
        status = None

    if status and get_unit_hostname() in status:
        return True

    if status and "resource %s is NOT running" % (resource) in status:
        raise CRMResourceNotFound("CRM resource %s not found" % (resource))

    return False


def is_leader(resource):
    log("is_leader is deprecated. Please consider using is_crm_leader "
        "instead.", level=WARNING)
    return is_crm_leader(resource)


def peer_units(peer_relation="cluster"):
    peers = []
    for r_id in (relation_ids(peer_relation) or []):
        for unit in (relation_list(r_id) or []):
            peers.append(unit)
    return peers


def peer_ips(peer_relation='cluster', addr_key='private-address'):
    '''Return a dict of peers and their private-address'''
    peers = {}
    for r_id in relation_ids(peer_relation):
        for unit in relation_list(r_id):
            peers[unit] = relation_get(addr_key, rid=r_id, unit=unit)
    return peers


def oldest_peer(peers):
    """Determines who the oldest peer is by comparing unit numbers."""
    local_unit_no = int(os.getenv('JUJU_UNIT_NAME').split('/')[1])
    for peer in peers:
        remote_unit_no = int(peer.split('/')[1])
        if remote_unit_no < local_unit_no:
            return False
    return True


def eligible_leader(resource):
    log("eligible_leader is deprecated. Please consider using "
        "is_elected_leader instead.", level=WARNING)
    return is_elected_leader(resource)


def https():
    '''
    Determines whether enough data has been provided in configuration
    or relation data to configure HTTPS
    .
    returns: boolean
    '''
    use_https = config_get('use-https')
    if use_https and bool_from_string(use_https):
        return True
    if config_get('ssl_cert') and config_get('ssl_key'):
        return True
    for r_id in relation_ids('identity-service'):
        for unit in relation_list(r_id):
            # TODO - needs fixing for new helper as ssl_cert/key suffixes with CN
            rel_state = [
                relation_get('https_keystone', rid=r_id, unit=unit),
                relation_get('ca_cert', rid=r_id, unit=unit),
            ]
            # NOTE: works around (LP: #1203241)
            if (None not in rel_state) and ('' not in rel_state):
                return True
    return False


def determine_api_port(public_port, singlenode_mode=False):
    '''
    Determine correct API server listening port based on
    existence of HTTPS reverse proxy and/or haproxy.

    public_port: int: standard public port for given service

    singlenode_mode: boolean: Shuffle ports when only a single unit is present

    returns: int: the correct listening port for the API service
    '''
    i = 0
    if singlenode_mode:
        i += 1
    elif len(peer_units()) > 0 or is_clustered():
        i += 1
    if https():
        i += 1
    return public_port - (i * 10)


def determine_apache_port(public_port, singlenode_mode=False):
    '''
    Description: Determine correct apache listening port based on public IP +
    state of the cluster.

    public_port: int: standard public port for given service

    singlenode_mode: boolean: Shuffle ports when only a single unit is present

    returns: int: the correct listening port for the HAProxy service
    '''
    i = 0
    if singlenode_mode:
        i += 1
    elif len(peer_units()) > 0 or is_clustered():
        i += 1
    return public_port - (i * 10)


def get_hacluster_config(exclude_keys=None):
    '''
    Obtains all relevant configuration from charm configuration required
    for initiating a relation to hacluster:

        ha-bindiface, ha-mcastport, vip, os-internal-hostname,
        os-admin-hostname, os-public-hostname, os-access-hostname

    param: exclude_keys: list of setting key(s) to be excluded.
    returns: dict: A dict containing settings keyed by setting name.
    raises: HAIncompleteConfig if settings are missing or incorrect.
    '''
    settings = ['ha-bindiface', 'ha-mcastport', 'vip', 'os-internal-hostname',
                'os-admin-hostname', 'os-public-hostname', 'os-access-hostname']
    conf = {}
    for setting in settings:
        if exclude_keys and setting in exclude_keys:
            continue

        conf[setting] = config_get(setting)

    if not valid_hacluster_config():
        raise HAIncorrectConfig('Insufficient or incorrect config data to '
                                'configure hacluster.')
    return conf


def valid_hacluster_config():
    '''
    Check that either vip or dns-ha is set. If dns-ha then one of os-*-hostname
    must be set.

    Note: ha-bindiface and ha-macastport both have defaults and will always
    be set. We only care that either vip or dns-ha is set.

    :returns: boolean: valid config returns true.
    raises: HAIncompatibileConfig if settings conflict.
    raises: HAIncompleteConfig if settings are missing.
    '''
    vip = config_get('vip')
    dns = config_get('dns-ha')
    if not(bool(vip) ^ bool(dns)):
        msg = ('HA: Either vip or dns-ha must be set but not both in order to '
               'use high availability')
        status_set('blocked', msg)
        raise HAIncorrectConfig(msg)

    # If dns-ha then one of os-*-hostname must be set
    if dns:
        dns_settings = ['os-internal-hostname', 'os-admin-hostname',
                        'os-public-hostname', 'os-access-hostname']
        # At this point it is unknown if one or all of the possible
        # network spaces are in HA. Validate at least one is set which is
        # the minimum required.
        for setting in dns_settings:
            if config_get(setting):
                log('DNS HA: At least one hostname is set {}: {}'
                    ''.format(setting, config_get(setting)),
                    level=DEBUG)
                return True

        msg = ('DNS HA: At least one os-*-hostname(s) must be set to use '
               'DNS HA')
        status_set('blocked', msg)
        raise HAIncompleteConfig(msg)

    log('VIP HA: VIP is set {}'.format(vip), level=DEBUG)
    return True


def canonical_url(configs, vip_setting='vip'):
    '''
    Returns the correct HTTP URL to this host given the state of HTTPS
    configuration and hacluster.

    :configs    : OSTemplateRenderer: A config tempating object to inspect for
                                      a complete https context.

    :vip_setting:                str: Setting in charm config that specifies
                                      VIP address.
    '''
    scheme = 'http'
    if 'https' in configs.complete_contexts():
        scheme = 'https'
    if is_clustered():
        addr = config_get(vip_setting)
    else:
        addr = unit_get('private-address')
    return '%s://%s' % (scheme, addr)
