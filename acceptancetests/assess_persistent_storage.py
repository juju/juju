#!/usr/bin/env python
"""Testing Juju's persistent storage feature."""

from __future__ import print_function

import os
import sys
import time
import yaml
import json
import ipdb
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

def assess_charm_deploy_single_block_and_filesystem_storage(client):
    """This function tests charm deployment with a single filesystem
       storage unit and a single persistent block device storage unit.

       Steps taken to the test:
       - Deploy dummy-storage charm with a single block storage unit (ebs)
         and a single filesystem storage unit (rootfs).
       - Check charm status once the deployment is done.
           > Application status should be active.
       - Check charm storage units once the deployment is done.
           > Total number of storage units should be 2.
           > Name of storage units should be in align with charm config.
           > Properties of storage units should be as defined.
               - Storage Type, Persistent Setting and Pool.

       :param client: ModelClient object to deploy the charm on.
    """

    charm_name = 'dummy-storage'
    charm_path = local_charm_path(
        charm=charm_name, juju_ver=client.version)
    log.info(
        '{} is going to be deployed with 1 ebs block storage unit and '\
        '1 rootfs filesystem storage unit.'.format(charm_name))

    # Run juju deploy dummy-storage --storage single-blk=ebs --storage single-fs=rootfs
    client.get_juju_output(
        'deploy', charm_path,
        '--storage', 'single-blk=ebs',
        '--storage', 'single-fs=rootfs',
        include_e=False)
    client.wait_for_started()
    client.wait_for_workloads()

    # Run juju status --format json
    status_output = json.loads(
        client.get_juju_output('status', '--format', 'json', include_e=False))
    app_status = status_output['applications'][charm_name]['application-status']['current']

    if app_status != 'active':
        raise JujuAssertionError(
            'App status is incorrect. '\
            'Found: {}\nExpected: {}\ntest terminated.'.format(
            app_status, 'active'))
    else:
        # Run juju storage --format json
        storage_output = json.loads(
            client.get_juju_output('storage', '--format', 'json', include_e=False))

        # check the total number of storage unit(s) and name(s)
        storage_list = storage_output['storage'].keys()
        if len(storage_list) != 2:
            log.error(
                '{} storage unit(s) found, should be 2, test terminated.'.format(
                str(len(storage_list))))
            sys.exit(1)
        else:
            log.info(
                'Following storage units have been found:\n{}'.format(
                '\n'.join(storage_list)))
            single_fs_id = ''
            single_blk_id = ''
            for elem in storage_list:
                if elem.startswith('single-fs'):
                    single_fs_id = elem
                    log.info(
                        'Single filesystem storage {} has been found.'.format(
                        single_fs_id))
                elif elem.startswith('single-blk'):
                    single_blk_id = elem
                    log.info(
                        'Single block device storage {} has been found.'.format(
                        single_blk_id))
            if single_fs_id == '':
                log.error(
                    'Name mismatch on Single filesystem storage, test terminated.')
                sys.exit(1)
            elif single_blk_id == '':
                log.error(
                    'Name mismatch on Single block device storage, test terminated.')
                sys.exit(1)
            else:
                log.info('Check name and total number of storage unit: PASSED.')

        # check type, persistent setting and pool of single block storage unit
        storage_type = storage_output['storage'][single_blk_id]['kind']
        persistent_setting = storage_output['storage'][single_blk_id]['persistent']
        pool_storage = storage_output['volumes']['0']['storage']
        pool_setting = storage_output['volumes']['0']['pool']

        if storage_type != 'block':
            log.error(
                'Incorrect type for single block device storage detected '\
                '- {}, test terminated.'.format(storage_type))
            sys.exit(1)
        # Be careful, the type of persistent_setting's value is boolean.
        # Using != to clearly highlight it here.
        elif persistent_setting != True:
            log.error(
                'Incorrect value for persistent setting on single block device '\
                'storage - {}, test terminated.'.format(persistent_setting))
            sys.exit(1)
        elif (pool_storage != single_blk_id) and (pool_setting != 'ebs'):
            log.error('Incorrect volumes detected '\
            '- {} with {}, test terminated.'.format(
            pool_storage, pool_setting))
            sys.exit(1)
        else:
            log.info('Check properties of single block device storage unit: PASSED.')

        # check type, persistent setting and pool of single filesystem storage unit
        storage_type = storage_output['storage'][single_fs_id]['kind']
        persistent_setting = storage_output['storage'][single_fs_id]['persistent']
        pool_storage = storage_output['filesystems']['0/0']['storage']
        pool_setting = storage_output['filesystems']['0/0']['pool']

        if storage_type != 'filesystem':
            log.error(
                'Incorrect type for single filesystem storage detected '\
                '- {}, test terminated.'.format(storage_type))
            sys.exit(1)
        elif persistent_setting != False:
            log.error(
                'Incorrect value for persistent setting on single filesystem '\
                'storage - {}, test terminated.'.format(persistent_setting))
            sys.exit(1)
        elif (pool_storage != single_fs_id) and (pool_setting != 'rootfs'):
            log.error('Incorrect filesystems detected '\
            '- {} with {}, test terminated.'.format(
            pool_storage, pool_setting))
            sys.exit(1)
        else:
            log.info('Check properties of single filesystem storage unit: PASSED.')
    return (single_fs_id, single_blk_id)


def assess_charm_removal_single_block_and_filesystem_storage(client):
    """This function tests charm removal while a single filesystem storage
       and a single persistent block device storage attached.

       Steps taken to the test:
       - Run assess_charm_deploy_single_block_and_filesystem_storage(client)
       - Remove dummy-storage charm while a single block storage unit (ebs)
         and a single filesystem storage unit (rootfs) attached.
           > The output should states that there is a persistent storage unit.
           > The application should be removed successfully.
       - Check charm storage units once the removal is done.
           > The filesystem storage unit (rootfs)should be removed successfully.
           > The block device storage unit (ebs) should remain and detached,

       :param client: ModelClient object to deploy the charm on.
    """

    charm_name = 'dummy-storage'
    single_fs_id, single_blk_id = \
        assess_charm_deploy_single_block_and_filesystem_storage(client)
    # Run juju remove-application dummy-storage
    app_removal_output = client.get_juju_output(
        'remove-application', charm_name, '--show-log', include_e=False, merge_stderr=True)
    # pre-set the result to FAILED
    remove_app_output_check = 'FAILED'
    remove_single_fs_output_check = 'FAILED'
    detach_single_blk_output_check = 'FAILED'

    for line in app_removal_output.split('\n'):
        if line.find('will remove unit {}'.format(charm_name)) != -1:
            log.info(line)
            remove_app_output_check = 'PASSED'
        elif line.find('will remove storage {}'.format(single_fs_id)) != -1:
            log.info(line)
            remove_single_fs_output_check = 'PASSED'
        elif line.find('will detach storage {}'.format(single_blk_id)) != -1:
            log.info(line)
            detach_single_blk_output_check = 'PASSED'

    if remove_app_output_check != 'PASSED':
        raise JujuAssertionError(
            'Missing application name in remove-application stdout.')
    else:
        log.info('Remove Application output check: {}'.format(
            remove_app_output_check))
    if remove_single_fs_output_check != 'PASSED':
        raise JujuAssertionError(
            'Missing single filesystem id in remove-application stdout.')
    else:
        log.info('Remove single filesystem storage output check: {}'.format(
            remove_single_fs_output_check))
    if detach_single_blk_output_check != 'PASSED':
        raise JujuAssertionError(
            'Missing single block device id in remove-application stdout.')
    else:
        log.info('Detach single block device storage output check: {}'.format(
            detach_single_blk_output_check))
    # storage status change after remove-application takes some time.
    # from experiments even 30 seconds is not enough.
    time.sleep(60)
    # check the real status of storage after remove-application
    storage_output = json.loads(
        client.get_juju_output('storage', '--format', 'json', include_e=False))
    storage_list = storage_output['storage'].keys()
    if len(storage_list) != 1:
        log.error('\n'.join(storage_list))
        raise JujuAssertionError(
            'Unexpected number of storage unit(s). '\
            'Found: {}\nExpected: 1\ntest terminated.'.format(
            str(len(storage_list))))
    elif single_fs_id in storage_list:
        raise JujuAssertionError(
            '{} should be removed along with remove-application.'.format(
            single_fs_id))
    elif single_blk_id not in storage_list:
        raise JujuAssertionError(
            '{} missing from storage list after remove-application.'.format(
            single_blk_id))
    else:
        log.info(
            'Check existence of persistent storage {} '\
            'after remove-application: PASSED'.format(single_blk_id))
    pool_storage = storage_output['volumes']['0']['storage']
    storage_status = storage_output['volumes']['0']['status']['current']
    if pool_storage != single_blk_id:
        raise JujuAssertionError(
            '{} missing from volumes.'.format(single_blk_id))
    elif storage_status != 'detached':
        raise JujuAssertionError(
            'Incorrect status for {}. '\
            'Found: {}\nExpected: detached'.format(
            single_blk_id, storage_status))
    else:
        log.info(
            'Check status of persistent storage {} '\
            'after remove-application: PASSED'.format(single_blk_id))
    return single_blk_id


def assess_deploy_charm_with_existing_storage(client):
    """This function tests charm deploy with an existing detached storage.

       Steps taken to the test:
       - Run assess_charm_removal_single_block_and_filesystem_storage(client)
       - Deploy charm dummy-storage with existing detached storage single_blk_id
       - Check charm status, should be active once the deploy is completed.
       - Check storage status, single_blk_id should show as attached

       :param client: ModelClient object to deploy the charm on.
    """
    # ipdb.set_trace()
    charm_name = 'dummy-storage'
    single_blk_id = \
        assess_charm_removal_single_block_and_filesystem_storage(client)


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
    # PR7635 landed later today, brings in persistent storage support on lxd
    # The storage type for persistent storage on lxd is 'lxd'
    # https://github.com/juju/juju/pull/7635

    environment = os.getenv('ENV', default='parallel-aws')
    if environment == 'parallel-lxd':
        log.error('TODO: Add persistent storage test on lxd.')
        sys.exit(1)
    else:
        assess_charm_removal_single_block_and_filesystem_storage(client)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        assess_persistent_storage(bs_manager.client)
    return 0


if __name__ == '__main__':
    sys.exit(main())
