#!/usr/bin/env python3
"""Assess juju blocks prevent users from making changes and models"""

from __future__ import print_function

import argparse
import logging
import sys

from assess_min_version import (
    JujuAssertionError
    )
from deploy_stack import (
    BootstrapManager,
    deploy_dummy_stack,
    )
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    )


__metaclass__ = type


log = logging.getLogger("assess_block")


def make_block_list(client, disabled_commands):
    """Return a manually made list of blocks and their status

    :param client: The client whose rules will be used.
    :param disabled_commands: list of client.command_set_* elements to
        include in simulated output.
    """
    if not disabled_commands:
        return []

    return [{'command-set': ','.join(disabled_commands)}]


def test_disabled(client, command, args, include_e=True):
    """Test if a command is disabled as expected"""
    try:
        if command == 'deploy':
            client.deploy(args)
        elif command == 'remove-application':
            client.remove_application(args)
        else:
            client.juju(command, args, include_e=include_e)
        raise JujuAssertionError()
    except Exception:
        pass


def assess_block_destroy_model(client, charm_series):
    """Test disabling the destroy-model command.

    When "disable-command destroy-model" is set,
    the model cannot be destroyed, but objects
    can be added, related, and removed.
    """
    client.disable_command(client.destroy_model_command)
    block_list = client.list_disabled_commands()
    if block_list != make_block_list(
            client, [client.command_set_destroy_model]):
        raise JujuAssertionError(block_list)
    test_disabled(
        client, client.destroy_model_command,
        ('-y', client.env.environment), include_e=False)
    # Adding, relating, and removing are not blocked.
    deploy_dummy_stack(client, charm_series)


def assess_block_remove_object(client, charm_series):
    """Test block remove-object

    When "disable-command remove-object" is set,
    objects can be added and related, but they
    cannot be removed or the model/environment deleted.
    """
    client.disable_command(client.command_set_remove_object)
    block_list = client.list_disabled_commands()
    if block_list != make_block_list(
            client, [client.command_set_remove_object]):
        raise JujuAssertionError(block_list)
    test_disabled(
        client, client.destroy_model_command,
        ('-y', client.env.environment), include_e=False)
    # Adding and relating are not blocked.
    deploy_dummy_stack(client, charm_series)
    test_disabled(client, 'remove-application', 'dummy-source')
    test_disabled(client, 'remove-unit', ('dummy-source/1',))
    test_disabled(client, 'remove-relation', ('dummy-source', 'dummy-sink'))


def assess_block_all_changes(client, charm_series):
    """Test Block Functionality: block all-changes"""
    client.juju('remove-relation', ('dummy-source', 'dummy-sink'))
    client.disable_command(client.command_set_all)
    block_list = client.list_disabled_commands()
    if block_list != make_block_list(client, [client.command_set_all]):
        raise JujuAssertionError(block_list)
    test_disabled(client, 'add-relation', ('dummy-source', 'dummy-sink'))
    test_disabled(client, 'unexpose', ('dummy-sink',))
    test_disabled(client, 'remove-application', 'dummy-sink')
    client.enable_command(client.command_set_all)
    client.juju('unexpose', ('dummy-sink',))
    client.disable_command(client.command_set_all)
    test_disabled(client, 'expose', ('dummy-sink',))
    client.enable_command(client.command_set_all)
    client.remove_application('dummy-sink')
    client.wait_for_started()
    client.disable_command(client.command_set_all)
    test_disabled(client, 'deploy', ('dummy-sink',))
    test_disabled(
        client, client.destroy_model_command,
        ('-y', client.env.environment), include_e=False)


def assess_unblock(client, type):
    """Test Block Functionality
    unblock destroy-model/remove-object/all-changes."""
    client.enable_command(type)
    block_list = client.list_disabled_commands()
    if block_list != make_block_list(client, []):
        raise JujuAssertionError(block_list)
    if type == client.destroy_model_command:
        client.remove_application('dummy-source')
        client.wait_for_started()
        client.remove_application('dummy-sink')
        client.wait_for_started()


def assess_block(client, charm_series):
    """Test Block Functionality:
    block/unblock destroy-model/remove-object/all-changes.
    """
    log.info('Test started')
    block_list = client.list_disabled_commands()
    client.wait_for_started()
    expected_none_blocked = make_block_list(client, [])
    if block_list != expected_none_blocked:
        log.error('Controller is not in the expected starting state')
        raise JujuAssertionError(
            'Expected {}\nFound {}'.format(expected_none_blocked, block_list))
    assess_block_destroy_model(client, charm_series)
    assess_unblock(client, client.destroy_model_command)
    assess_block_remove_object(client, charm_series)
    assess_unblock(client, client.command_set_remove_object)
    assess_block_all_changes(client, charm_series)
    assess_unblock(client, client.command_set_all)
    log.info('Test PASS')


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="Test Block Functionality")
    add_basic_testing_arguments(parser)
    parser.set_defaults(series='trusty')
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        assess_block(bs_manager.client, bs_manager.series)
    return 0


if __name__ == '__main__':
    sys.exit(main())
