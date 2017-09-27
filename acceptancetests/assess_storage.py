#!/usr/bin/env python
"""Assess juju charm storage."""

from __future__ import print_function

import argparse
import copy
import json
import logging
import os
import sys
import time

from deploy_stack import BootstrapManager
from distutils.version import (
    LooseVersion,
    StrictVersion,
)
from jujucharm import (
    Charm,
    local_charm_path,
)
from utility import (
    until_timeout,
    add_basic_testing_arguments,
    assert_dict_is_subset,
    configure_logging,
    temp_dir,
)
from jujupy.client import get_stripped_version_number


__metaclass__ = type


log = logging.getLogger("assess_storage")


DEFAULT_STORAGE_POOL_DETAILS = {}


AWS_DEFAULT_STORAGE_POOL_DETAILS = {
    'ebs': {
        'provider': 'ebs'},
    'ebs-ssd': {
        'attrs': {
            'volume-type': 'ssd'},
        'provider': 'ebs'},
    'tmpfs': {
        'provider': 'tmpfs'},
    'loop': {
        'provider': 'loop'},
    'rootfs': {
        'provider': 'rootfs'}
    }


storage_pool_details = {
    "loopy": {
        "provider": "loop",
        "attrs": {"size": "1G"}
        },
    "rooty": {
        "provider": "rootfs",
        "attrs": {"size": "1G"}
        },
    "tempy": {
        "provider": "tmpfs",
        "attrs": {"size": "1G"}
        },
    "ebsy": {
        "provider": "ebs",
        "attrs": {"size": "1G"}
        },
    }
storage_pool_1x = copy.deepcopy(storage_pool_details)
storage_pool_1x["ebs-ssd"] = {
    "provider": "ebs",
    "attrs": {"volume-type": "ssd"}
    }


def after_2dot3(client_version):
    # LooseVersion considers 2.3-somealpha to be newer than 2.3.0.
    # Attempt strict versioning first.
    client_version = get_stripped_version_number(client_version)
    try:
        return StrictVersion(client_version) >= StrictVersion('2.3.0')
    except ValueError:
        pass
    return LooseVersion(client_version) >= LooseVersion('2.3.0')


def wait_for_storage_detach(client, storage_id, interval, timeout):
    """
    Due to the asynchronous nature of Juju, detaching a persistent storage
    takes some time.
    This function will wait then check if the status of a specific persistent
    storage unit changed to detached. Once detached, waiting stops.
    """
    for ignored in until_timeout(timeout):
        time.sleep(interval)
        storage_output = json.loads(client.list_storage())
        try:
            index = [
                elem for elem in storage_output['volumes'].keys()
                if storage_output['volumes'][elem]['storage'] == storage_id][0]
        except IndexError:
            log.info('Volume index for {} cannot be found.'.format(storage_id))
            break
        storage_status = storage_output['volumes'][index]['status']['current']
        if storage_status == 'detached':
            break


def wait_for_storage_removal(client, storage_id, interval, timeout):
    """
    Due to the asynchronous nature of Juju, storage removal takes some time.
    This function will wait then check if a specific storage id can be found in
    juju storage output. If the storage id disappeared, waiting stops.
    """
    for ignored in until_timeout(timeout):
        time.sleep(interval)
        storage_raw_output = client.list_storage()
        if storage_id not in storage_raw_output:
            break


def make_expected_ls(client, storage_name, unit_name, kind='filesystem'):
    """Return the expected data from list-storage for filesystem or block."""
    if kind == 'block':
        location = ''
    else:
        location = '/srv/data'
    data = {
        "storage": {
            storage_name: {
                "kind": kind,
                "attachments": {
                    "units": {
                        unit_name: {
                            "location": location,
                            "life": "alive"
                            }
                        }
                    },
                "life": "alive"
                }
            }
        }
    return data


def make_expected_disk(client, disk_num, unit_name):
    """Return the expected data from list storage for disks."""
    all_disk = {'storage': {}}
    for num in range(1, disk_num + 1):
        next_disk = make_expected_ls(
            client, 'disks/{}'.format(num), unit_name, kind='block')
        all_disk['storage'].update(next_disk['storage'])
    return all_disk


def storage_list(client):
    """Return the storage list."""
    list_storage = json.loads(client.list_storage())
    for instance in list_storage["storage"].keys():
        try:
            list_storage["storage"][instance].pop("status")
            list_storage["storage"][instance].pop("persistent")
            attachments = list_storage["storage"][instance]["attachments"]
            unit = attachments["units"].keys()[0]
            attachments["units"][unit].pop("machine")
            if instance.startswith("disks"):
                attachments["units"][unit]["location"] = ""
        except Exception:
            pass
    return list_storage


def storage_pool_list(client):
    """Return the list of storage pool."""
    return json.loads(client.list_storage_pool())


def create_storage_charm(charm_dir, name, summary, storage):
    """Manually create a temporary charm to test with storage."""
    storage_charm = Charm(name, summary, storage=storage, series=['trusty'])
    charm_root = storage_charm.to_repo_dir(charm_dir)
    return charm_root


def assess_create_pool(client):
    """Test creating storage pool."""
    for name, pool in storage_pool_details.iteritems():
        client.create_storage_pool(name, pool["provider"],
                                   pool["attrs"]["size"])


def assess_add_storage(client, unit, storage_type, amount="1"):
    """Test adding storage instances to service.

    Only type 'disk' is able to add instances.
    """
    client.add_storage(unit, storage_type, amount)


def deploy_storage(client, charm, series, pool, amount="1G", charm_repo=None):
    """Test deploying charm with storage."""
    if pool == "loop":
        client.deploy(charm, series=series, repository=charm_repo,
                      storage="disks=" + pool + "," + amount)
    elif pool is None:
        client.deploy(charm, series=series, repository=charm_repo,
                      storage="data=" + amount)
    else:
        client.deploy(charm, series=series, repository=charm_repo,
                      storage="data=" + pool + "," + amount)
    client.wait_for_started()


def assess_deploy_storage(client, charm_series,
                          charm_name, provider_type, pool=None):
    """Set up the test for deploying charm with storage."""
    if provider_type == "block":
        storage = {
            "disks": {
                "type": provider_type,
                "multiple": {
                    "range": "0-10"
                }
            }
        }
    else:
        storage = {
            "data": {
                "type": provider_type,
                "location": "/srv/data"
            }
        }
    with temp_dir() as charm_dir:
        charm_root = create_storage_charm(charm_dir, charm_name,
                                          'Test charm for storage', storage)
        platform = 'ubuntu'
        charm = local_charm_path(charm=charm_name, juju_ver=client.version,
                                 series=charm_series,
                                 repository=os.path.dirname(charm_root),
                                 platform=platform)
        deploy_storage(client, charm, charm_series, pool, "1G", charm_dir)


def assess_multiple_provider(client, charm_series, amount, charm_name,
                             provider_1, provider_2, pool_1, pool_2):
    storage = {}
    for provider in [provider_1, provider_2]:
        if provider == "block":
            storage.update({
                "disks": {
                    "type": provider,
                    "multiple": {
                        "range": "0-10"
                    }
                }
            })
        else:
            storage.update({
                "data": {
                    "type": provider,
                    "location": "/srv/data"
                }
            })
    with temp_dir() as charm_dir:
        charm_root = create_storage_charm(charm_dir, charm_name,
                                          'Test charm for storage', storage)
        platform = 'ubuntu'
        charm = local_charm_path(charm=charm_name, juju_ver=client.version,
                                 series=charm_series,
                                 repository=os.path.dirname(charm_root),
                                 platform=platform)
        if pool_1 == "loop":
            command = "disks=" + pool_1 + "," + amount
        else:
            command = "data=" + pool_1 + "," + amount
        if pool_2 == "loop":
            command = command + ",disks=" + pool_2
        else:
            command = command + ",data=" + pool_2
        client.deploy(charm, series=charm_series, repository=charm_dir,
                      storage=command)
        client.wait_for_started()


def check_storage_list(client, expected):
    storage_list_derived = storage_list(client)
    assert_dict_is_subset(expected, storage_list_derived)


def assess_storage(client, charm_series):
    """Test the storage feature.

    Each storage test is deployed as a charm. The application is removed
    when the test succeeds. Logs will be retrieved from failing machines.
    """

    log.info('Assessing create-pool')
    assess_create_pool(client)
    log.info('create-pool PASSED')

    log.info('Assessing storage pool')
    if client.is_juju1x():
        expected_pool = storage_pool_1x
    else:
        if client.env.provider == 'ec2':
            expected_pool = dict(AWS_DEFAULT_STORAGE_POOL_DETAILS)
        else:
            expected_pool = dict(DEFAULT_STORAGE_POOL_DETAILS)
        expected_pool.update(storage_pool_details)
    pool_list = storage_pool_list(client)
    assert_dict_is_subset(expected_pool, pool_list)
    log.info('Storage pool PASSED')

    log.info('Assessing filesystem rootfs')
    assess_deploy_storage(client, charm_series,
                          'dummy-storage-fs', 'filesystem', 'rootfs')
    expected = make_expected_ls(client, 'data/0', 'dummy-storage-fs/0')
    check_storage_list(client, expected)
    log.info('Filesystem rootfs PASSED')
    client.remove_service('dummy-storage-fs')

    log.info('Assessing block loop disk 1')
    assess_deploy_storage(client, charm_series,
                          'dummy-storage-lp', 'block', 'loop')
    expected_block1 = make_expected_disk(client, 1, 'dummy-storage-lp/0')
    check_storage_list(client, expected_block1)
    log.info('Block loop disk 1 PASSED')

    log.info('Assessing add storage block loop disk 2')
    assess_add_storage(client, 'dummy-storage-lp/0', 'disks', "1")
    expected_block2 = make_expected_disk(client, 2, 'dummy-storage-lp/0')
    check_storage_list(client, expected_block2)
    log.info('Block loop disk 2 PASSED')
    client.remove_service('dummy-storage-lp')

    log.info('Assessing filesystem tmpfs')
    assess_deploy_storage(client, charm_series,
                          'dummy-storage-tp', 'filesystem', 'tmpfs')
    expected = make_expected_ls(client, 'data/3', 'dummy-storage-tp/0')
    check_storage_list(client, expected)
    log.info('Filesystem tmpfs PASSED')
    client.remove_service('dummy-storage-tp')

    log.info('Assessing filesystem')
    assess_deploy_storage(client, charm_series,
                          'dummy-storage-np', 'filesystem')
    expected = make_expected_ls(client, 'data/4', 'dummy-storage-np/0')
    check_storage_list(client, expected)
    log.info('Filesystem tmpfs PASSED')

    client.remove_service('dummy-storage-np')
    # persistent storage feature requires Juju 2.3+
    # data/4 is persistent in Juju 2.3+, detach is required before removal,
    # it needs to be removed otherwise will interfere the next test result.
    if after_2dot3(client.version):
        wait_for_storage_detach(
            client, storage_id='data/4', interval=15, timeout=60)
        client.get_juju_output(
            'remove-storage', 'data/4', include_e=False, merge_stderr=True)
        wait_for_storage_removal(
            client, storage_id='data/4', interval=15, timeout=90)

    log.info('Assessing multiple filesystem, block, rootfs, loop')
    assess_multiple_provider(client, charm_series, "1G", 'dummy-storage-mp',
                             'filesystem', 'block', 'rootfs', 'loop')
    expected = make_expected_ls(client, 'data/5', 'dummy-storage-mp/0')
    check_storage_list(client, expected)
    log.info('Multiple filesystem, block, rootfs, loop PASSED')
    client.remove_service('dummy-storage-mp')
    log.info('All storage tests PASSED')


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
