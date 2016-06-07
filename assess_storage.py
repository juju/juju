#!/usr/bin/env python
"""Assess juju charm storage."""

from __future__ import print_function
import os

import argparse
import json
import logging
import sys

from deploy_stack import (
    BootstrapManager,
)
from jujucharm import Charm
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    JujuAssertionError,
    local_charm_path,
    temp_dir,
)


__metaclass__ = type


log = logging.getLogger("assess_storage")


storage_pool_details = {
    "loopy":
        {
            "provider": "loop",
            "attrs":
                {
                    "size": "1G"
                }},
    "rooty":
        {
            "provider": "rootfs",
            "attrs":
                {
                    "size": "1G"
                }},
    "tempy":
        {
            "provider": "tmpfs",
            "attrs":
                {
                    "size": "1G"
                }},
    "ebsy":
        {
            "provider": "ebs",
            "attrs":
                {
                    "size": "1G"
                }}
}


def storage_list(client):
    """Return the storage list."""
    return json.loads(client.get_juju_output(
        'list-storage', '--format', 'json'))


def assert_storage_lists_equal(storage_list_derived, storage_list_expected):
    """Check if two storage lists are equal as requested."""
    equal = True
    derived_keys = storage_list_derived["storage"].keys().sort()
    expected_keys = storage_list_expected["storage"].keys().sort()
    if derived_keys != expected_keys:
        equal = False
    while equal:
        for key, value in storage_list_derived["storage"].iteritems():
            if value["kind"] != storage_list_expected["storage"][key]["kind"]:
                equal = False
            expected_unit = storage_list_expected["storage"][key]["attachments"]["units"]
            if value["attachments"]["units"].keys() != expected_unit.keys():
                equal = False
            expected_charm = expected_unit.keys()[0]
            if value["kind"] == "filesystem":
                location_expected = expected_unit[expected_charm]["location"]
                if value["attachments"]["units"][expected_charm]["location"] != location_expected:
                    equal = False
    return equal


def storage_pool_list(client):
    """Return the list of storage pool."""
    return json.loads(client.get_juju_output(
        'list-storage-pools', '--format', 'json'))


def create_storage_charm(charm_dir, name, summary, storage):
    """Manually create a temporary charm to test with storage."""
    path = os.path.join(charm_dir, name)
    if not os.path.exists(path):
        os.makedirs(path)
    storage_charm = Charm(name, summary, storage=storage)
    storage_charm.to_dir(path)


def assess_create_pool(client):
    """Test creating storage pool."""
    for name, pool in storage_pool_details.iteritems():
        client.juju('create-storage-pool',
                    (name, pool["provider"],
                     'size={}'.format(pool["attrs"]["size"])))
    pool_list = storage_pool_list(client)
    if pool_list != storage_pool_details:
        raise JujuAssertionError(pool_list)


def assess_add_storage(client, unit, storage_type, amount="1"):
    """Test adding storage instances to service.
    Only type 'disk' is able to add instances"""
    client.juju('add-storage', (unit, storage_type + "=" + amount))


def deploy_storage(client, charm, series, pool, amount="1G"):
    """Test deploying charm with storage."""
    if pool == "loop":
        client.deploy(charm, series=series,
                      storage="disks=" + pool + "," + amount)
    else:
        client.deploy(charm, series=series,
                      storage="data=" + pool + "," + amount)
    client.wait_for_started()
    # if storage_list(client) != "TODO":
    #     raise JujuAssertionError()


def assess_deploy_storage(client, charm_series,
                          charm_name, provider_type, pool):
    """Set up the test for deploying charm with storage."""
    if provider_type == 'filesystem':
        storage = {
            "data": {
                "type": provider_type,
                "location": "/srv/data"
            }
        }
    elif provider_type == "block":
        storage = {
            "disks": {
                "type": provider_type,
                "multiple": {
                    "range": "0-10"
                }
            }
        }
    with temp_dir() as charm_dir:
        create_storage_charm(charm_dir, charm_name,
                             'Test charm for storage', storage)
        platform = 'ubuntu'
        charm = local_charm_path(charm=charm_name, juju_ver=client.version,
                                 series=charm_series,
                                 repository=charm_dir, platform=platform)
        deploy_storage(client, charm, charm_series, pool, "1G")


def assess_storage(client, charm_series):
    """Test the storage feature."""
    assess_create_pool(client)
    assess_deploy_storage(client, charm_series,
                          'dummy-storage-fs', 'filesystem', 'rootfs')
    storage_list_expected = {"storage":
                                 {"data/0": {
                                     "kind": "filesystem",
                                     "attachments":
                                         {"units":
                                              {"dummy-storage-fs/0":
                                                   {"location": "/srv/data"}}}}}}
    storage_list_derived = storage_list(client)
    if not assert_storage_lists_equal(storage_list_derived, storage_list_expected):
        raise JujuAssertionError(storage_list_derived)
    assess_deploy_storage(client, charm_series,
                          'dummy-storage-lp', 'block', 'loop')
    storage_list_expected["storage"]["disk/1"] = {"kind": "block",
                                                  "attachments":
                                                      {"units":
                                                           {"dummy-storage-lp/0":
                                                                {"location": ""}}}}
    storage_list_derived = storage_list(client)
    if not assert_storage_lists_equal(storage_list_derived, storage_list_expected):
        raise JujuAssertionError(storage_list_derived)
    assess_add_storage(client, 'dummy-storage-lp/0', 'disk', "1")
    storage_list_expected["storage"]["disk/2"] = {"kind": "block",
                                                  "attachments":
                                                      {"units":
                                                           {"dummy-storage-lp/0":
                                                                {"location": ""}}}}
    storage_list_derived = storage_list(client)
    if not assert_storage_lists_equal(storage_list_derived, storage_list_expected):
        raise JujuAssertionError(storage_list_derived)
    assess_deploy_storage(client, charm_series,
                          'dummy-storage-tp', 'filesystem', 'tmpfs')
    storage_list_expected["storage"]["data/3"] = {"kind": "filesystem",
                                                  "attachments":
                                                      {"units":
                                                           {"dummy-storage-tp/0":
                                                                {"location": "/srv/data"}}}}
    storage_list_derived = storage_list(client)
    if not assert_storage_lists_equal(storage_list_derived, storage_list_expected):
        raise JujuAssertionError(storage_list_derived)
    # storage with provider 'ebs' is still under observation
    # assess_deploy_storage(client, charm_series,
    #                       'dummy-storage-eb', 'filesystem', 'ebs')


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="Test Storage Feature")
    add_basic_testing_arguments(parser)
    parser.set_defaults(series='trusty')
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        assess_storage(bs_manager.client, bs_manager.series)
    return 0


if __name__ == '__main__':
    sys.exit(main())

