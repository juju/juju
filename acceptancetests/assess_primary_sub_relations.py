#!/usr/bin/env python3
"""Test relations between primary and subordinates are cleanedup correctly"""

from __future__ import print_function

import argparse
import logging
import subprocess
import sys
import time

from deploy_stack import BootstrapManager
from jujucharm import local_charm_path
from utility import (
    JujuAssertionError,
    add_basic_testing_arguments,
    configure_logging,
    until_timeout,
    )

__metaclass__ = type

log = logging.getLogger('assess_primary_sub_relations')


def assess_primary_sub_relations(client):
    ensure_removing_relation_removes_sub_unit(client)
    ensure_removing_primary_succeeds(client)


def ensure_removing_primary_succeeds(client):
    """Ensure removing primary app with subordinate relation is successful.

    Must be able to remove a primary app (and it's subordinate) if the
    subordinate also has relations to other primary apps.

    Outline:
      - Deploy 2 different primary charms A and B
      - Deploy a subordinate charm
      - Relate subordinate to A, relate subordinate to B
      - Ensure subordinate unit is on machines and operational
      - Remove application A
        - Ensure app A is removed
        - Ensure that subordinate for app B is NOT removed
        - Ensure app B continues to exist and operate.
    """
    test_model = create_model_with_primary_and_sub_apps(
        client, 'remove-primary')
    assert_subordinate_unit_operational(
        test_model, primary_app_name='dummy-source')
    assert_subordinate_unit_operational(
        test_model, primary_app_name='dummy-sink')

    test_model.remove_application('dummy-source')

    # dummy-source must be removed
    assert_service_is_removed(test_model, 'dummy-source', timeout=180)
    # But dummy-sink must remain operational.
    assert_subordinate_unit_operational(
        test_model, primary_app_name='dummy-sink')


def ensure_removing_relation_removes_sub_unit(client):
    """Ensure when removing a relation of primary->sub the sub unit is removed.

    No other instances of the subordinate unit must be affected.
    If removing the relation of app A to subordinate, the existing relation of
    app B and it's subordinate must not be affected.

    Outline:
      - Deploy 2 different primary charms A and B
      - Deploy a sub-ordinate charm
      - Relate suboridinate to A, relate subordinate to B
      - Ensure sub-ordinate unit is on machines and operational
      - Remove relation from single primary app A
      - Ensure unit is removed from A
      - Ensure unit is not removed from B and remains operational
    """
    test_model = create_model_with_primary_and_sub_apps(
        client, 'remove-subordinate')

    assert_subordinate_unit_operational(
        test_model, primary_app_name='dummy-source')
    assert_subordinate_unit_operational(
        test_model, primary_app_name='dummy-sink')

    test_model.juju('remove-relation', ('ntp', 'dummy-source'))

    assert_subordinate_is_removed(
        test_model, 'dummy-source', 'ntp', timeout=180)

    # This subordinate must still remain operational, we didn't remove the
    # relation from dummy-sink.
    assert_subordinate_unit_operational(
        test_model, primary_app_name='dummy-sink')

    # We expect this check to fail now as the subordinate app shouldn't be
    # running.
    # If it doesn't fail then the unit is still operational and that is a fail
    # state.
    try:
        assert_subordinate_unit_operational(
            test_model, primary_app_name='dummy-source')
    except JujuAssertionError:
        log.info('Subordinate application relation successfully removed.')
    else:
        raise JujuAssertionError(
            'Subordinate unit still operational after relation removal')

    test_model.destroy_model()


def assert_service_is_removed(client, application_name, timeout=60):
    """Wait for the named application to be removed from status.

    Check status of `client` until the named application `application_name` no
    longer appears.
    Will raise if the application still appears after the timeout.abs

    :param client: ModelClient with which to check for application status.
    :param application_name: String name of the application to assert for in
      status.
    :param timeout: int seconds to wait for raising exception if application
      still exists.
    :raises JujuAssertionError: When application_name still appears in status
      after the timeout is encountered.
    """

    for _ in until_timeout(timeout):
        current_status = client.get_status()
        current_status.get_applications().keys()
        if application_name not in current_status.get_applications().keys():
            return
        else:
            time.sleep(1)
    raise JujuAssertionError(
        'Application "{}" failed to be removed in {} seconds'.format(
            application_name, timeout))


def assert_subordinate_is_removed(
        client, primary_name, subordinate_name, timeout=60):
    for _ in until_timeout(timeout):
        status = client.get_status()
        subordinate_names = [
            unit[0] for unit in status.service_subordinate_units(
                primary_name)
            ]
        if not any(subordinate_name in unit for unit in subordinate_names):
            return
        else:
            time.sleep(5)
    raise JujuAssertionError(
        'Subordinate "{}" failed to be removed from "{}" in {} seconds'.format(
            subordinate_name, primary_name, timeout))


def create_model_with_primary_and_sub_apps(client, model_name):
    test_model = client.add_model(model_name)
    deploy_primary_charm(test_model, 'dummy-source')
    deploy_primary_charm(test_model, 'dummy-sink')

    test_model.deploy('cs:ntp')

    test_model.set_config('dummy-source', {'token': 'subordinate-test'})
    test_model.juju('add-relation', ('dummy-source', 'dummy-sink'))
    test_model.juju('add-relation', ('ntp', 'dummy-source'))
    test_model.juju('add-relation', ('ntp', 'dummy-sink'))
    test_model.wait_for_workloads()
    return test_model


def assert_subordinate_unit_operational(client, primary_app_name):
    """Check status of subordinate charm on host.

    Determine subordinate unit name and run the command 'service status' for it

    :raises JujuAssertionError: When service status exits non-zero.
    """
    sub_unit_name = get_subordinate_unit_name(client, primary_app_name)
    primary_unit = '{}/0'.format(primary_app_name)
    try:
        client.juju(
            'run',
            (
                '--unit', primary_unit,
                'sudo', 'service', sub_unit_name, 'status'))
    except subprocess.CalledProcessError:
        raise JujuAssertionError('Subordinate charm unit not running.')


def get_subordinate_unit_name(client, primary_app_name):
    """get unit service name (jujud-unit-<name>-<number>)"""
    all_subordinates = client.get_status().service_subordinate_units(
        primary_app_name)
    try:
        # Get the first subordinate found.
        sub_unit_name = [unit[0] for unit in all_subordinates][0]
    except IndexError as e:
        raise JujuAssertionError(
            'Unable to find subordinates, key not found: {}'.format(e))
    return 'jujud-unit-{}'.format(sub_unit_name.replace('/', '-'))


def deploy_primary_charm(client, charm_name, series='bionic'):
    charm_path = local_charm_path(
        charm=charm_name, juju_ver=client.version, series=series)
    _, deploy_complete = client.deploy(charm_path, series)
    client.wait_for(deploy_complete)


def parse_args(argv):
    parser = argparse.ArgumentParser(
        description="Test relations between primary and subordinate charms.")
    add_basic_testing_arguments(parser, existing=True)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(upload_tools=args.upload_tools):
        assess_primary_sub_relations(bs_manager.client)


if __name__ == '__main__':
    sys.exit(main())
