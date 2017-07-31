#!/usr/bin/env python
"""Testing spaces and subnets settings for app deployment to lxd container.
   This is an validation of Bug 1685782:
   https://bugs.launchpad.net/juju/+bug/1685782
   The test procedures are based on comment #29.
"""

from __future__ import print_function

import os
import sys
import json
import logging
import argparse

from jujucharm import local_charm_path
from deploy_stack import BootstrapManager

from utility import (
    JujuAssertionError,
    add_basic_testing_arguments,
    configure_logging,
    )

__metaclass__ = type
log = logging.getLogger("assess_lxd_space")


def assert_initial_spaces(client):
    """Initial spaces status after bootstrap should show as:
       no spaces to display"""
    # Run juju spaces --format json
    # Bug 1704105: https://bugs.launchpad.net/juju/+bug/1704105
    # merge_stderr need to be set to True for the initial assert
    # because spaces list is empty at this point.
    raw_output = client.get_juju_output(
        'spaces', '--format', 'json', include_e=False, merge_stderr=True)
    expected_spaces = 'no spaces to display'
    if expected_spaces not in raw_output:
        raise JujuAssertionError(
            'Incorrect initial spaces status. Found: {}\nExpected: {}'.format(
            raw_output, expected_spaces))
    else:
        log.info('Initial spaces status is: {}.'.format(expected_spaces))


def get_subnets(client):
    # Run juju subnets --format json
    raw_output = client.get_juju_output(
        'subnets', '--format', 'json', include_e=False)
    try:
        subnets_output = json.loads(raw_output)
    except ValueError as e:
        log.error('Invalid output from juju subnets:\n{}'.format(raw_output))
        raise e
    subnets_dict = subnets_output['subnets']
    subnets_cidr_list = subnets_output['subnets'].keys()
    return (subnets_dict, subnets_cidr_list)


def assert_initial_subnets(client):
    """There should be at least 1 subnet available once bootstrap is completed.
       By default 4 subnets should be shown under an AWS controller."""
    subnets_dict = get_subnets(client)[0]
    if not subnets_dict:
        raise JujuAssertionError('No subnet can be Found.')
    else:
        log.info('{} subnet(s) have been found.'.format(str(len(subnets_dict))))


def add_space_with_existing_subnet(client, space_name):
    # Run juju add-space <space_name> <subnet_cidr>
    subnets_cidr_list = get_subnets(client)[1]
    subnet_cidr = subnets_cidr_list[0]
    # Bug 1704105, merge_stderr=True is required.
    client.get_juju_output(
        'add-space', space_name, subnet_cidr, include_e=False, merge_stderr=True)
    return (space_name, subnet_cidr)


def assert_added_space(client):
    """Validate if configurations in new added space are as expected.
       Five checkpoints:
       1. After new space is added, juju spaces --format json should give output
          in json format, in comparison to no space exists.
       2. Total number of space should be 1.
       3. The name of new added space should be the same as specified.
       4. Total number of subnet CIDR from this new added space should be 1.
       5. The CIDR data from new added space should be the same as specified.
    """
    space_name, subnet_cidr = add_space_with_existing_subnet(
        client, space_name='testspace')
    # New added space should exist at this point,
    # set merge_stderr to False to eliminate noise in output.
    raw_output = client.get_juju_output(
        'spaces', '--format', 'json', include_e=False, merge_stderr=False)
    try:
        space_output = json.loads(raw_output)
    except ValueError as e:
        log.error('New added space cannot be found.')
        raise e
    # These 4 validations are in sequence.
    if len(space_output['spaces'].keys()) != 1:
        raise JujuAssertionError(
            'Incorrect number of space(s). Found: {}; Expected: 1.'.format(
            str(len(space_output['spaces'].keys()))))
    elif space_output['spaces'].keys()[0] != space_name:
        raise JujuAssertionError(
            'Incorrect space name. Found: {}; Expected: {}.'.format(
            space_output['spaces'].keys()[0], space_name))
    elif len(space_output['spaces'][space_name].keys()) != 1:
        raise JujuAssertionError(
            'Incorrect number of CIDR(s). Found: {}; Expected: 1.'.format(
            str(len(space_output['spaces'][space_name].keys()))))
    elif space_output['spaces'][space_name].keys()[0] != subnet_cidr:
        raise JujuAssertionError(
            'Incorrect CIDR name. Found: {}; Expected: {}.'.format(
            space_output['spaces'][space_name].keys()[0], subnet_cidr))
    else:
        log.info('New added space {} has been validated.'.format(space_name))


def assert_app_status(client, charm_name, expected):
    # Run juju status --format json
    log.info('Checking current status of app {}.'.format(charm_name))
    status_output = json.loads(
        client.get_juju_output('status', '--format', 'json', include_e=False))
    app_status = status_output['applications'][charm_name]['application-status']['current']

    if app_status != expected:
        raise JujuAssertionError(
            'App status is incorrect. '\
            'Found: {}\nExpected: {}\n.'.format(
            app_status, expected))
    else:
        log.info('The current status of app {} is: {}; Expected: {}'.format(
        charm_name, app_status, expected))


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description="Test for lxd spaces and subnets constraints.")
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def assess_lxd_space(client):
    environment = os.getenv('ENV', default='parallel-aws')
    if environment != 'parallel-aws':
        log.error('Incorrect substrate, should be parallel-aws.')
        sys.exit(1)
    else:
        assert_initial_spaces(client)
        assert_initial_subnets(client)
        assert_added_space(client)
        # Run juju deploy ubuntu --constraints "spaces=testspace"
        client.deploy(charm='ubuntu', constraints='spaces=testspace')
        client.wait_for_started()
        client.wait_for_workloads()
        assert_app_status(client, charm_name='ubuntu', expected='active')
        # Run juju deploy ubuntu ubuntu-lxd --to lxd:0
        client.deploy(charm='ubuntu', service='ubuntu-lxd', to='lxd:0')
        client.wait_for_started()
        client.wait_for_workloads()
        assert_app_status(client, charm_name='ubuntu-lxd', expected='active')


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        assess_lxd_space(bs_manager.client)
    return 0


if __name__ == '__main__':
    sys.exit(main())
