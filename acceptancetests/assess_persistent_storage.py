#!/usr/bin/env python
"""Testing Juju's persistent storage feature."""

from __future__ import print_function

import argparse
import json
import logging
import sys
import time

from deploy_stack import (
    BootstrapManager,
    test_on_controller,
    )
from jujucharm import local_charm_path
from utility import (
    until_timeout,
    JujuAssertionError,
    add_basic_testing_arguments,
    configure_logging,
    )

__metaclass__ = type
log = logging.getLogger("assess_persistent_storage")


def assert_equal(error_msg, found, expected):
    if found != expected:
        raise JujuAssertionError('{} Found: {}\nExpected: {}'.format(
            error_msg, found, expected))


def wait_for_storage_status_update(client, storage_id, interval, timeout):
    """The sole purpose of this function is to wait until an update of storage
       status occurred. Due to the asynchronous nature of Juju, the status of
       storage may change time to time after a Juju CLI issued, in this test
       only the existence of storage id is the point of interest.
       This function does not judge the changed status, it will be done by the
       assert function after it, as the conditions of assert may vary."""
    for ignored in until_timeout(timeout):
        time.sleep(interval)
        storage_list = get_storage_list(client)[0]
        if storage_id not in storage_list:
            break


def get_storage_list(client):
    raw_output = client.get_juju_output(
        'storage', '--format', 'json', include_e=False)
    # Bug 1708340 https://bugs.launchpad.net/juju/+bug/1708340
    # juju storage --format json output will be empty if no storage exists.
    if raw_output == '':
        storage_list = storage_output = ''
    else:
        storage_output = json.loads(raw_output)
        storage_list = storage_output['storage'].keys()
    return (storage_list, storage_output)


def get_storage_property(client, storage_id):
    storage_list, storage_output = get_storage_list(client)
    if storage_list == '':
        storage_type = persistent_setting = ''
        pool_storage = pool_setting = storage_status = ''
    else:
        storage_type = storage_output['storage'][storage_id]['kind']
        # Be careful, the type of persistent_setting's value is boolean.
        persistent_setting = storage_output[
            'storage'][storage_id]['persistent']
        if storage_type == 'block':
            pool_storage = storage_output['volumes']['0']['storage']
            pool_setting = storage_output['volumes']['0']['pool']
            storage_status = storage_output[
                'volumes']['0']['status']['current']
        elif storage_type == 'filesystem':
            pool_storage = storage_output['filesystems']['0/0']['storage']
            pool_setting = storage_output['filesystems']['0/0']['pool']
            storage_status = ''
        else:
            raise JujuAssertionError(
                'Incorrect storage type. '
                'Found: {}\nExpected: block or filesystem'.format(
                    storage_type))
    return (
        storage_list, storage_type,
        persistent_setting, pool_storage, pool_setting, storage_status)


def assert_app_status(client, charm_name, expected):
    # Run juju status --format json
    log.info('Checking current status of app {}.'.format(charm_name))
    status_output = json.loads(
        client.get_juju_output('status', '--format', 'json', include_e=False))
    app_status = status_output[
        'applications'][charm_name]['application-status']['current']
    assert_equal(
        error_msg='App status is incorrect.',
        found=app_status,
        expected=expected)
    log.info('The current status of app {} is: {}; Expected: {}'.format(
        charm_name, app_status, expected))


def assert_storage_count(storage_list, expected):
    log.info('Checking total number of storage unit(s).')
    found = len(storage_list)
    assert_equal(
        error_msg='Unexpected number of storage unit(s).',
        found=found, expected=expected)
    log.info(
        'Found {} storage unit(s). Expected: {}.'.format(found, expected))


def assert_single_blk_existence(storage_list, storage_id):
    log.info(
        'Checking existence of single block device storage.'.format(
            storage_id))
    if storage_id not in storage_list:
        raise JujuAssertionError(
            '{} missing from storage list.'.format(storage_id))
    else:
        log.info('{} found in storage list'.format(storage_id))


def assert_single_blk_removal(storage_list, storage_id):
    log.info(
        'Checking removal of single block device storage.'.format(storage_id))
    if storage_id in storage_list:
        raise JujuAssertionError(
            '{} still exists in storage list.'.format(storage_id))
    else:
        log.info('{} has been removed from storage list'.format(storage_id))


def assert_persistent_setting(storage_id, found, expected):
    log.info(
        'Checking persistent setting of storage unit {}.'.format(storage_id))
    err_msg = 'Incorrect persistent setting value on storage unit {}.'.format(
        storage_id)
    assert_equal(error_msg=err_msg, found=found, expected=expected)
    log.info(
        'Persistent setting of storage unit {}, '
        'Found: {}; Expected: {}.'.format(storage_id, found, expected))


def assert_storage_status(
        found_id, expected_id, found_status, expected_status):
    log.info(
        'Checking the status of storage {} in volumes.'.format(expected_id))
    volume_err_msg = '{} missing from volumes.'.format(expected_id)
    assert_equal(
        error_msg=volume_err_msg, found=found_id, expected=expected_id)
    status_err_msg = 'Incorrect status for {}.'.format(found_id)
    assert_equal(
        error_msg=status_err_msg, found=found_status, expected=expected_status)
    log.info(
        'The current status of storage {} in volumes is: {}; '
        'Expected: {}.'.format(found_id, found_status, expected_status))


def assert_app_removal_msg(client, charm_name):
    # Run juju remove-application dummy-storage
    # Bug 1704105: https://bugs.launchpad.net/juju/+bug/1704105
    # merge_stderr=True is required
    app_removal_output = client.get_juju_output(
        'remove-application', charm_name, '--show-log',
        include_e=False, merge_stderr=True)
    remove_app_output_check = False
    for line in app_removal_output.split('\n'):
        if 'will remove unit {}'.format(charm_name) in line:
            log.info(line)
            remove_app_output_check = True
            break
    if not remove_app_output_check:
        raise JujuAssertionError(
            'Missing application name in remove-application output message.')
    else:
        log.info('Remove Application output message check: {}'.format(
            remove_app_output_check))
    return app_removal_output


def assert_single_fs_removal_msg(single_fs_id, app_removal_output):
    remove_single_fs_output_check = False
    for line in app_removal_output.split('\n'):
        if 'will remove storage {}'.format(single_fs_id) in line:
            log.info(line)
            remove_single_fs_output_check = True
            break
    if not remove_single_fs_output_check:
        raise JujuAssertionError(
            'Missing single filesystem id {} in '
            'remove-application output message.'.format(single_fs_id))
    else:
        log.info(
            'Remove single filesystem storage {} '
            'output message check: {}'.format(
                single_fs_id, remove_single_fs_output_check))


def assert_single_blk_detach_msg(single_blk_id, app_removal_output):
    detach_single_blk_output_check = False
    for line in app_removal_output.split('\n'):
        if 'will detach storage {}'.format(single_blk_id) in line:
            log.info(line)
            detach_single_blk_output_check = True
            break
    if not detach_single_blk_output_check:
        raise JujuAssertionError(
            'Missing single block device id {} in '
            'remove-application output message.'.format(single_blk_id))
    else:
        log.info(
            'Detach single block device storage {} '
            'output message check: {}'.format(
                single_blk_id, detach_single_blk_output_check))


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
        '{} is going to be deployed with 1 ebs block storage unit and '
        '1 rootfs filesystem storage unit.'.format(charm_name))
    # Run juju deploy dummy-storage --storage single-blk=ebs
    #    --storage single-fs=rootfs
    client.deploy(charm_path, storage=['single-blk=ebs', 'single-fs=rootfs'])
    client.wait_for_started()
    client.wait_for_workloads()
    # check current status of app dummy-storage
    assert_app_status(client, charm_name=charm_name, expected='active')
    storage_list = get_storage_list(client)[0]
    # check the total number of storage unit(s) and name(s)
    assert_storage_count(storage_list=storage_list, expected=2)
    log.info(
        'Following storage units have been found:\n{}'.format(
            '\n'.join(storage_list)))
    try:
        single_fs_id = [elem for elem in storage_list
                        if elem.startswith('single-fs')][0]
    except IndexError:
        raise JujuAssertionError('Name mismatch on Single filesystem storage.')
    try:
        single_blk_id = [elem for elem in storage_list
                         if elem.startswith('single-blk')][0]
    except IndexError:
        raise JujuAssertionError(
            'Name mismatch on Single block device storage.')
    log.info('Check name and total number of storage unit: PASSED.')
    # check type, persistent setting and pool of single block storage unit
    storage_list, \
        storage_type, \
        persistent_setting,\
        pool_storage, \
        pool_setting,\
        storage_status = get_storage_property(client, storage_id=single_blk_id)
    if storage_type != 'block':
        raise JujuAssertionError(
            'Incorrect type for single block device storage detected '
            '- {}.'.format(storage_type))

    assert_persistent_setting(
        storage_id=single_blk_id, found=persistent_setting, expected=True)

    if (pool_storage != single_blk_id) and (pool_setting != 'ebs'):
        raise JujuAssertionError(
            'Incorrect volumes detected - {} with {}.'.format(
                pool_storage, pool_setting))
    log.info('Check properties of single block device storage unit: PASSED.')
    # check type, persistent setting and pool of single filesystem storage unit
    storage_list, \
        storage_type, \
        persistent_setting, \
        pool_storage, \
        pool_setting, \
        storage_status = get_storage_property(client, storage_id=single_fs_id)
    if storage_type != 'filesystem':
        raise JujuAssertionError(
            'Incorrect type for single filesystem storage detected '
            '- {}.'.format(storage_type))

    assert_persistent_setting(
        storage_id=single_fs_id, found=persistent_setting, expected=False)

    if (pool_storage != single_fs_id) and (pool_setting != 'rootfs'):
        raise JujuAssertionError(
            'Incorrect filesystems detected - {} with {}.'.format(
                pool_storage, pool_setting))
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
           > The filesystem storage unit (rootfs) should be
               removed successfully.
           > The block device storage unit (ebs) should remain and detached,

       :param client: ModelClient object to deploy the charm on.
    """
    charm_name = 'dummy-storage'
    single_fs_id, single_blk_id = \
        assess_charm_deploy_single_block_and_filesystem_storage(client)
    app_removal_output = assert_app_removal_msg(client, charm_name=charm_name)
    assert_single_fs_removal_msg(
        single_fs_id=single_fs_id, app_removal_output=app_removal_output)
    assert_single_blk_detach_msg(
        single_blk_id=single_blk_id, app_removal_output=app_removal_output)
    # storage status change after remove-application takes some time.
    # from experiments even 30 seconds is not enough.
    wait_for_storage_status_update(
        client, storage_id=single_fs_id, interval=15, timeout=90)
    storage_list = get_storage_list(client)[0]
    assert_storage_count(storage_list=storage_list, expected=1)
    if single_fs_id in storage_list:
        raise JujuAssertionError(
            '{} should be removed along with remove-application.'.format(
                single_fs_id))
    assert_single_blk_existence(
        storage_list=storage_list, storage_id=single_blk_id)
    log.info(
        'Check existence of persistent storage {} '
        'after remove-application: PASSED'.format(single_blk_id))
    storage_list, \
        storage_type, \
        persistent_setting, \
        pool_storage, \
        pool_setting, \
        storage_status = get_storage_property(client, storage_id=single_blk_id)
    assert_storage_status(
        found_id=pool_storage, expected_id=single_blk_id,
        found_status=storage_status, expected_status='detached')
    log.info(
        'Check status of persistent storage {} '
        'after remove-application: PASSED'.format(single_blk_id))
    return single_blk_id


def assess_deploy_charm_with_existing_storage_and_removal(client):
    """This function tests charm deploy with an existing detached storage.

       Steps taken to the test:
       - Run assess_charm_removal_single_block_and_filesystem_storage(client)
       - Deploy charm dummy-storage with existing detached storage
           single_blk_id
       - Check charm status, should be active once the deploy is completed.
       - Check storage status, single_blk_id should show as attached

       :param client: ModelClient object to deploy the charm on.
    """

    single_blk_id = \
        assess_charm_removal_single_block_and_filesystem_storage(client)
    charm_name = 'dummy-storage'
    charm_path = local_charm_path(
        charm=charm_name, juju_ver=client.version)
    # Run juju deploy dummy-storage --attach-storage <single_blk_id>
    # Note: --attach-storage has the issue of Bug 1704105 too:
    # https://bugs.launchpad.net/juju/+bug/1704105
    client.get_juju_output(
        'deploy', charm_path,
        '--attach-storage', single_blk_id,
        include_e=False, merge_stderr=True)
    client.wait_for_started()
    client.wait_for_workloads()
    assert_app_status(client, charm_name=charm_name, expected='active')
    storage_list, \
        storage_type, \
        persistent_setting, \
        pool_storage, \
        pool_setting, \
        storage_status = get_storage_property(client, storage_id=single_blk_id)
    assert_storage_count(storage_list=storage_list, expected=1)
    assert_single_blk_existence(
        storage_list=storage_list, storage_id=single_blk_id)
    assert_persistent_setting(
        storage_id=single_blk_id, found=persistent_setting, expected=True)
    assert_storage_status(
        found_id=pool_storage, expected_id=single_blk_id,
        found_status=storage_status, expected_status='attached')
    # Run juju remove-application dummy-storage
    client.get_juju_output('remove-application', charm_name, include_e=False)
    # persistent storage single_blk_id should remain after remove-application
    wait_for_storage_status_update(
        client, storage_id=single_blk_id, interval=15, timeout=90)
    storage_list = get_storage_list(client)[0]
    assert_single_blk_existence(
        storage_list=storage_list, storage_id=single_blk_id)
    # Run juju remove-storage <single_blk_id>
    # Note: remove-storage has the issue of Bug 1704105:
    # https://bugs.launchpad.net/juju/+bug/1704105
    client.get_juju_output(
        'remove-storage', single_blk_id, include_e=False, merge_stderr=True)
    wait_for_storage_status_update(
        client, storage_id=single_blk_id, interval=15, timeout=90)
    storage_list = get_storage_list(client)[0]
    assert_single_blk_removal(
        storage_list=storage_list, storage_id=single_blk_id)


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description="Test for Persistent Storage feature.")
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def assess_persistent_storage(client):
    """Functional test for persistent storage features
    Because the whole test is one user scenario,
    the assess functions are connected. e.g.:
      assess_deploy_charm_with_existing_storage_and_removal(client) depends on
      assess_charm_removal_single_block_and_filesystem_storage(client) which
      depends on
      assess_charm_deploy_single_block_and_filesystem_storage(client).
    Therefore only the last assess function need to be called.
    """
    assess_deploy_charm_with_existing_storage_and_removal(client)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    if bs_manager.client.env.get_cloud() != 'aws':
        # Due to the differences in volume type, at this stage
        # the test is designed to run on AWS only.
        # PR7635 is for persistent storage feature on lxd.
        log.error('Incorrect substrate, must be AWS.')
        sys.exit(1)
    test_on_controller(assess_persistent_storage, args)
    return 0


if __name__ == '__main__':
    sys.exit(main())
