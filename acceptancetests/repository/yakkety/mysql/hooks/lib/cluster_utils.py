#
# Copyright 2012 Canonical Ltd.
#
# This file is sourced from lp:openstack-charm-helpers
#
# Authors:
#  James Page <james.page@ubuntu.com>
#  Adam Gandelman <adamg@ubuntu.com>
#

from lib.utils import (
    juju_log,
    relation_ids,
    relation_list,
    relation_get,
    get_unit_hostname,
    config_get
    )
import subprocess
import os


def is_clustered():
    for r_id in (relation_ids('ha') or []):
        for unit in (relation_list(r_id) or []):
            clustered = relation_get('clustered',
                                     rid=r_id,
                                     unit=unit)
            if clustered:
                return True
    return False


def is_leader(resource):
    cmd = [
        "crm", "resource",
        "show", resource
        ]
    try:
        status = subprocess.check_output(cmd)
    except subprocess.CalledProcessError:
        return False
    else:
        if get_unit_hostname() in status:
            return True
        else:
            return False


def peer_units():
    peers = []
    for r_id in (relation_ids('cluster') or []):
        for unit in (relation_list(r_id) or []):
            peers.append(unit)
    return peers


def oldest_peer(peers):
    local_unit_no = os.getenv('JUJU_UNIT_NAME').split('/')[1]
    for peer in peers:
        remote_unit_no = peer.split('/')[1]
        if remote_unit_no < local_unit_no:
            return False
    return True


def eligible_leader(resource):
    if is_clustered():
        if not is_leader(resource):
            juju_log('INFO', 'Deferring action to CRM leader.')
            return False
    else:
        peers = peer_units()
        if peers and not oldest_peer(peers):
            juju_log('INFO', 'Deferring action to oldest service unit.')
            return False
    return True


def https():
    '''
    Determines whether enough data has been provided in configuration
    or relation data to configure HTTPS
    .
    returns: boolean
    '''
    if config_get('use-https') == "yes":
        return True
    if config_get('ssl_cert') and config_get('ssl_key'):
        return True
    for r_id in relation_ids('identity-service'):
        for unit in relation_list(r_id):
            if (relation_get('https_keystone', rid=r_id, unit=unit) and
                relation_get('ssl_cert', rid=r_id, unit=unit) and
                relation_get('ssl_key', rid=r_id, unit=unit) and
                relation_get('ca_cert', rid=r_id, unit=unit)):
                return True
    return False


def determine_api_port(public_port):
    '''
    Determine correct API server listening port based on
    existence of HTTPS reverse proxy and/or haproxy.

    public_port: int: standard public port for given service

    returns: int: the correct listening port for the API service
    '''
    i = 0
    if len(peer_units()) > 0 or is_clustered():
        i += 1
    if https():
        i += 1
    return public_port - (i * 10)


def determine_haproxy_port(public_port):
    '''
    Description: Determine correct proxy listening port based on public IP +
    existence of HTTPS reverse proxy.

    public_port: int: standard public port for given service

    returns: int: the correct listening port for the HAProxy service
    '''
    i = 0
    if https():
        i += 1
    return public_port - (i * 10)
