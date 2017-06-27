#!/usr/bin/env python
"""Testing persistent storage function of Juju."""

from __future__ import print_function

import os
import sys
import time
import yaml
import logging
import argparse
import subprocess

from jujucharm import local_charm_path

from deploy_stack import (
    BootstrapManager,
    # get_random_string,
    )

from utility import (
    JujuAssertionError,
    add_basic_testing_arguments,
    configure_logging,
    )

__metaclass__ = type
log = logging.getLogger("assess_persistent_storage")


def assess_storage_remove_single_storage(client, storage_type):
    """remove-storage should fail if deployed charm has one storage only.

       Steps taken to test:
       - Deploy dummy-storage charm configured with single filesystem.
       - Get storage list once the charm deployment is done.
       - Run remove-storage command to remove the only storage.
       - Test Passed if the remove-storage failed, otherwise Test Failed.

       :param client: ModelClient object to deploy the charm on.
    """

    charm_name = 'dummy-storage'
    charm_path = local_charm_path(
        charm=charm_name, juju_ver=client.version)
    log.info('{} is going to be deployed with storage type: {}.'.format(
        charm_name, storage_type))
    client.deploy(charm_path, storage=storage_type)
    client.wait_for_started()
    client.wait_for_workloads()

    try:
        single_storage_name = get_storage_filesystems(
            client, 'single-fs')[0]
        log.info(
            'Single storage ID is {}'.format(single_storage_name))
    except IndexError:
        raise JujuAssertionError('Storage was not found, test aborted')

    try:
        client.juju('remove-storage', single_storage_name)
        log.info(
            'Test Failed - remove-storage is expected to fail in single-fs!')
    except subprocess.CalledProcessError:
        log.info(
            'Test Passed - storage cannot be removed if it is the only one.')

    log.info('Test is done, time to clean up.')
    remove_deployed_charm(client, charm_id=charm_name)
    log.info('{} has been removed'.format(charm_name))


def assess_storage_remove_multi_fs(client):
    """remove-storage should succeed if deployed charm has multiple storages.

       Steps taken to test:
       - Deploy dummy-storage charm configured with multiple filesystem.
       - Get storage list once the charm deployment is done.
       - Check if the number of storages deployed matches to the spec.
       - Run remove-storage command to remove one of the storages.
       - Get storage list again to check if the removed storage still exists.
       - Test Passed if the removed storage is gone, otherwise Test Failed.

       :param client: ModelClient object to deploy the charm on.
    """
    charm_name = 'dummy-storage'
    charm_path = local_charm_path(
        charm=charm_name, juju_ver=client.version)
    storage_number = 2
    log.info(
        '{} is going to be deployed with {} storages.'.format(
        charm_name, str(storage_number)))
    client.deploy(
        charm_path,
        storage='multi-fs=rootfs,{}'.format(str(storage_number)))
    client.wait_for_started()
    client.wait_for_workloads()

    try:
        multi_filesystem_name = get_storage_filesystems(client, 'multi-fs')
        log.info(
            'Following filesystems have been found:\n{}'.format(
            ', '.join(multi_filesystem_name)))
    except IndexError:
        raise JujuAssertionError('Storage was not found, test aborted')

    if len(multi_filesystem_name) == storage_number:
        try:
            client.juju('remove-storage', multi_filesystem_name[-1])
            log.info('Command remove-storage has been excuted.')
        except subprocess.CalledProcessError:
            log.info('Test Failed - Run command remove-storage failed!')
    else:
        log.info('{} deployment failed - '
                 'the number of storages deployed does not match the spec!'
                 .format(charm_name))

    # Get file systems after waited 30 seconds
    time.sleep(30)
    try:
        filesystem_remain = get_storage_filesystems(client, 'multi-fs')
    except subprocess.CalledProcessError:
        log.info('Get remaining storage list failed!')
    if multi_filesystem_name[-1] in filesystem_remain:
        log.info(
            'Test Failed - {} still exists after remove-storage!'.format(
            multi_filesystem_name[-1]))
    else:
        log.info(
            'Test Passed - {} has been successfully removed.'.format(
            multi_filesystem_name[-1]))

    log.info('Test is done, time to clean up.')
    remove_deployed_charm(client, charm_id=charm_name)
    log.info('{} has been removed'.format(charm_name))


def remove_deployed_charm(client, charm_id):
    client.remove_service(charm_id)
    time.sleep(30)


def get_storage_filesystems(client, storage_name):
    """Return storage unit names for a named storage.

    :param client: ModelClient object to query.
    :param storage_name: Name of storage unit to get filesystem names for.
    :return: List of filesystem names
    """
    all_storage = yaml.safe_load(client.list_storage())['filesystems']
    return [
        details['storage'] for unit, details in all_storage.items()
        if details['storage'].startswith(storage_name)]


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description="Test for Persistent Storage feature.")
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def assess_persistent_storage(client):
    # Based on the test spec, persistent storage need to be tested on both
    # LXD and AWS and ebs volume is unavailable on LXD, a switcher is
    # required here to decide which test should be run.
    environment = os.getenv('ENV', default='parallel-lxd')
    if environment == 'parallel-lxd':
        assess_storage_remove_single_storage(
            client, storage_type='single-fs=rootfs')
    elif environment == 'parallel-aws':
        assess_storage_remove_single_storage(
            client, storage_type='single-fs=ebs,10G')
    #assess_storage_remove_multi_fs(client)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        assess_persistent_storage(bs_manager.client)
    return 0


if __name__ == '__main__':
    sys.exit(main())
