#!/usr/bin/env python
"""Test for relations between primary and subordinate charms."""

from __future__ import print_function

import argparse
import logging
import subprocess
import sys
import time

from deploy_stack import (
    BootstrapManager,
    get_random_string,
    )
from jujucharm import local_charm_path
from utility import (
    JujuAssertionError,
    add_basic_testing_arguments,
    configure_logging
    )

__metaclass__ = type

log = logging.getLogger('assess_primary_sub_relations')


def assess_primary_sub_relations(client):
    ensure_removing_relation_removes_sub_unit(client)


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
    test_model = client.add_model('remove-relation')
    token = get_random_string()
    deploy_primary_charm(test_model, 'dummy-source')
    deploy_primary_charm(test_model, 'dummy-sink')

    test_model.deploy('cs:ntp')

    test_model.set_config('dummy-source', {'token': token})
    test_model.juju('add-relation', ('dummy-source', 'dummy-sink'))
    test_model.juju('add-relation', ('ntp', 'dummy-source'))
    test_model.juju('add-relation', ('ntp', 'dummy-sink'))
    test_model.wait_for_workloads()

    ensure_subordinate_unit_operational(
        test_model, primary_app_name='dummy-source')
    ensure_subordinate_unit_operational(
        test_model, primary_app_name='dummy-sink')

    test_model.juju('remove-relation', ('ntp', 'dummy-source'))
    # Wait for application to be removed.
    time.sleep(10)

    # This subordinate must still remain operational, we didn't remove the
    # relation from this app.
    ensure_subordinate_unit_operational(
        test_model, primary_app_name='dummy-sink')

    # We expect this check to fail now as the subordinate app shouldn't be
    # running.
    # If it doesn't fail then the unit is still operational and that is a fail
    # state.
    try:
        ensure_subordinate_unit_operational(
            test_model, primary_app_name='dummy-source')
    except JujuAssertionError:
        log.info('Subordinate application relation successfully removed.')
    else:
        raise JujuAssertionError(
            'Subordinate unit still operational after relation removal')


def ensure_subordinate_unit_operational(client, primary_app_name):
    """Check status of subordinate charm on host.

    Determine subordinate unit name (jujud-unit-<name>-<number>) and
    'service status' it.

    :raises JujuAssertionError: When service status exits non-zero.
    """
    sub_unit_name = get_subordinate_unit_name(client, primary_app_name)
    primary_unit = '{}/0'.format(primary_app_name)
    try:
        client.juju(
            'ssh',
            (primary_unit, 'sudo', 'service', sub_unit_name, 'status'))
    except subprocess.CalledProcessError:
        raise JujuAssertionError('Subordinate charm unit not running.')


def get_subordinate_unit_name(client, primary_app_name):
    apps = client.get_status().get_applications()
    primary_unit = '{}/0'.format(primary_app_name)
    try:
        sub_unit_name = apps[
            primary_app_name]['units'][primary_unit]['subordinates'].keys()[0]
    except KeyError as e:
        raise JujuAssertionError(
            'Unable to find subordinates, key not found: {}'.format(e))
    return 'jujud-unit-{}'.format(sub_unit_name.replace('/', '-'))


def deploy_primary_charm(client, charm_name):
    charm_path = local_charm_path(
        charm=charm_name, juju_ver=client.version, series='xenial')
    _, deploy_complete = client.deploy(charm_path, series='xenial')
    client.wait_for(deploy_complete)


def parse_args(argv):
    parser = argparse.ArgumentParser(
        description="Test relations between primary and subordinate charms.")
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(upload_tools=args.upload_tools):
        assess_primary_sub_relations(bs_manager.client)

if __name__ == '__main__':
    sys.exit(main())
