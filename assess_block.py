#!/usr/bin/env python
"""Assess juju blocks prevent users from making changes and models"""

from __future__ import print_function

import argparse
import logging
import sys

import yaml

from assess_min_version import (
    JujuAssertionError
)
from deploy_stack import (
    BootstrapManager,
    deploy_dummy_stack,
)
from jujupy import until_timeout
from utility import (
    add_basic_testing_arguments,
    configure_logging,
)


__metaclass__ = type


log = logging.getLogger("assess_block")


def get_block_list(client):
    """Return a list of blocks and their status."""
    return yaml.safe_load(client.get_juju_output(
        'block list', '--format', 'yaml'))


def make_block_list(client, des_env, rm_obj, all_changes):
    """Return a manually made list of blocks and their status"""
    block_list = [
        {'block': client.destroy_model_command, 'enabled': des_env},
        {'block': 'remove-object', 'enabled': rm_obj},
        {'block': 'all-changes', 'enabled': all_changes}]
    if des_env:
        block_list[0]['message'] = ''
    if rm_obj:
        block_list[1]['message'] = ''
    if all_changes:
        block_list[2]['message'] = ''
    return block_list


def test_blocked(client, command, args, include_e=True):
    """Test if a command is blocked as expected"""
    try:
        if command == 'deploy':
            client.deploy(args)
        else:
            client.juju(command, args, include_e=include_e)
        raise JujuAssertionError()
    except Exception:
        pass


def wait_for_removed_services(client, charm):
    """Timeout until the remove process ends"""
    for ignored in until_timeout(30):
        status = client.get_status()
        if charm not in status.status['services']:
            break
            # else:
            #     raise JujuAssertionError('dummy-source not destroyed')


def assess_block_destroy_model(client, charm_series):
    """Test Block Functionality: block destroy-model"""
    client.juju('block ' + client.destroy_model_command, ())
    block_list = get_block_list(client)
    if block_list != make_block_list(client, True, False, False):
        raise JujuAssertionError(block_list)
    test_blocked(client, client.destroy_model_command,
                 ('-y', client.env.environment), include_e=False)
    deploy_dummy_stack(client, charm_series)
    client.remove_service('dummy-source')
    wait_for_removed_services(client, 'dummy-source')
    client.remove_service('dummy-sink')
    wait_for_removed_services(client, 'dummy-sink')


def assess_block_remove_object(client, charm_series):
    """Test Block Functionality: block remove-object"""
    client.juju('block remove-object', ())
    block_list = get_block_list(client)
    if block_list != make_block_list(client, False, True, False):
        raise JujuAssertionError(block_list)
    test_blocked(client, client.destroy_model_command,
                 ('-y', client.env.environment), include_e=False)
    deploy_dummy_stack(client, charm_series)
    test_blocked(client, 'remove-service', ('dummy-source',))
    test_blocked(client, 'remove-unit', ('dummy-source/1',))
    test_blocked(client, 'remove-relation', ('dummy-source', 'dummy-sink'))


def assess_block_all_changes(client, charm_series):
    """Test Block Functionality: block all-changes"""
    client.juju('block all-changes', ())
    block_list = get_block_list(client)
    if block_list != make_block_list(client, False, False, True):
        raise JujuAssertionError(block_list)
    test_blocked(client, client.destroy_model_command,
                 ('-y', client.env.environment), include_e=False)
    try:
        deploy_dummy_stack(client, charm_series)
        raise JujuAssertionError()
    except Exception:
        pass


def assess_unblock(client, type):
    """
    Test Block Functionality:
    unblock destroy-model/remove-object/all-changes.
    """
    client.juju('unblock ' + type, ())
    block_list = get_block_list(client)
    if block_list != make_block_list(client, False, False, False):
        raise JujuAssertionError(block_list)
    if type == 'remove-object':
        client.remove_service('dummy-source')
        wait_for_removed_services(client, 'dummy-source')
        client.remove_service('dummy-sink')
        wait_for_removed_services(client, 'dummy-sink')


def assess_block(client, charm_series):
    """
    Test Block Functionality:
    block/unblock destroy-model/remove-object/all-changes.
    """
    block_list = get_block_list(client)
    client.wait_for_started()
    expected_none_blocked = make_block_list(client, False, False, False)
    if block_list != expected_none_blocked:
        raise JujuAssertionError(block_list)
    assess_block_destroy_model(client, charm_series)
    assess_unblock(client, client.destroy_model_command)
    assess_block_remove_object(client, charm_series)
    assess_unblock(client, 'remove-object')
    assess_block_all_changes(client, charm_series)
    assess_unblock(client, 'all-changes')


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="Test Block Functionality")
    add_basic_testing_arguments(parser)
    pargs = parser.parse_args(argv)
    if pargs.series is None:
        pargs.series = 'trusty'
    return pargs


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        assess_block(bs_manager.client, bs_manager.series)
    return 0


if __name__ == '__main__':
    sys.exit(main())
