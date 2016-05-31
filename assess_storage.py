#!/usr/bin/env python
"""TODO: add rough description of what is assessed in this module."""

from __future__ import print_function
import os

import argparse
import logging
import sys
import yaml

from assess_block import (
    wait_for_removed_services,
)
from deploy_stack import (
    BootstrapManager,
)
from jujucharm import Charm
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    local_charm_path,
    temp_dir,
    until_timeout,
)


__metaclass__ = type


log = logging.getLogger("assess_storage")


def storage_list(client):
    return yaml.safe_load(client.get_juju_output(
        'list-storage', '--format', 'yaml'))


def storage_pool_list(client):
    return yaml.safe_load(client.get_juju_output(
        'list-storage-pools', '--format', 'yaml'))


def make_storage_pool_list(name, provider, size):
    pool_list = {}
    for i in xrange(len(name)):
        pool_list[name[i]] = {"provider": provider[i],
                              "attrs": {
                                  "size": size[i]
                              }}
    return pool_list



def create_storage_charm(charm_dir, name, summary, storage):
    path = os.path.join(charm_dir, name)
    if not os.path.exists(path):
        os.makedirs(path)
    storage_charm = Charm(name, summary, storage=storage)
    storage_charm.to_dir(path)


def assess_create_pool(client):
    client.juju('create-storage-pool', ('loopy', 'loop', 'size=1G'))
    client.juju('create-storage-pool', ('rooty', 'rootfs', 'size=1G'))
    client.juju('create-storage-pool', ('tempy', 'tmpfs', 'size=1G'))
    client.juju('create-storage-pool', ('ebsy', 'ebs', 'size=1G'))
    name_list = ["loopy", "rooty", "tempy", "ebsy"]
    provider_list = ["loop", "rootfs", "tmpfs", "ebs"]
    size_list = ["1G", "1G", "1G", "1G"]
    pool_list_expected = make_storage_pool_list(name_list, provider_list, size_list)
    if storage_pool_list(client) != pool_list_expected:
        raise JujuAssertionError()


def assess_add_storage(client, unit, amount=1):
    client.juju('add-storage', (unit, "data=" + amount))


def deploy_storage(client, charm, series, pool, amount="1G"):
    client.deploy(charm, series=series, storage="data=" + pool + "," + amount)
    client.wait_for_started()
    # if storage_list(client) != "TODO":
    #     raise JujuAssertionError()


def assess_deploy_storage(client, charm_name, storage_type, pool):
    storage = {
        "data": {
            "type": storage_type,
            "location": "/srv/data"
        }
    }
    charm_series = 'trusty'
    with temp_dir() as charm_dir:
        create_storage_charm(charm_dir, charm_name, 'Test charm for storage', storage)
        platform = 'ubuntu'
        charm = local_charm_path(charm=charm_name, juju_ver=client.version,
                                 series=charm_series, repository=charm_dir, platform=platform)
        deploy_storage(client, charm, charm_series, pool, "1G")
        assess_add_storage(client, charm_name + '/0', 1)
        client.wait_for_started()
        wait_for_removed_services(client, charm_name)


def assess_storage(client):
    assess_create_pool(client)
    assess_deploy_storage(client, 'dummy-storage-fs', 'filesystem', 'rootfs')
    assess_deploy_storage(client, 'dummy-storage-lp', 'loop', 'loop')
    assess_deploy_storage(client, 'dummy-storage-fs', 'filesystem', 'tmpfs')
    assess_deploy_storage(client, 'dummy-storage-fs', 'filesystem', 'ebs')
    # charm = local_charm_path(charm='dummy-storage', juju_ver=client.version,
    #                          series='trusty', platform='ubuntu')
    # deploy_storage(client,charm,'trusty','rootfs')


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="TODO: script info")
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        assess_storage(bs_manager.client)
    return 0


if __name__ == '__main__':
    sys.exit(main())
