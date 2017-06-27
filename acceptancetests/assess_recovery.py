#!/usr/bin/env python
# Backup and restore a stack.

from __future__ import print_function

from argparse import ArgumentParser
from contextlib import contextmanager
import logging
import re
from subprocess import CalledProcessError
import sys

from deploy_stack import (
    BootstrapManager,
    deploy_dummy_stack,
    get_remote_machines,
    get_token_from_status,
    wait_for_state_server_to_shutdown,
)
from jujupy import (
    parse_new_state_server_from_error,
)
from substrate import (
    convert_to_azure_ids,
    terminate_instances,
)
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    JujuAssertionError,
    LoggedException,
    until_timeout,
)


__metaclass__ = type


running_instance_pattern = re.compile('\["([^"]+)"\]')


log = logging.getLogger("assess_recovery")


class HARecoveryError(Exception):
    """The controllers failed to respond."""


def check_token(client, token):
    for ignored in until_timeout(300):
        found = get_token_from_status(client)
        if found and token in found:
            return found
    raise JujuAssertionError('Token is not {}: {}'.format(
                             token, found))


def deploy_stack(client, charm_series):
    """"Deploy a simple stack, state-server and ubuntu."""
    deploy_dummy_stack(client, charm_series)
    client.set_config('dummy-source', {'token': 'One'})
    client.wait_for_workloads()
    check_token(client, 'One')
    log.info("%s is ready to testing", client.env.environment)


def show_controller(client):
    controller_info = client.show_controller(format='yaml')
    log.info('Controller is:\n{}'.format(controller_info))


def enable_ha(bs_manager, controller_client):
    """Enable HA and wait for the controllers to be ready."""
    controller_client.enable_ha()
    controller_client.wait_for_ha()
    show_controller(controller_client)
    remote_machines = get_remote_machines(
        controller_client, bs_manager.known_hosts)
    bs_manager.known_hosts = remote_machines


def assess_ha_recovery(bs_manager, client):
    """Verify that the client can talk to a controller.


    The controller is given 5 minutes to respond to the client's request.
    Another possibly 5 minutes is given to return a sensible status.
    """
    # Juju commands will hang when the controller is down, so ensure the
    # call is interrupted and raise HARecoveryError. The controller
    # might return an error, but it still has
    try:
        client.juju('status', (), check=True, timeout=300)
        client.get_status(300)
    except CalledProcessError:
        raise HARecoveryError()
    bs_manager.has_controller = True
    log.info("HA recovered from leader failure.")
    log.info("PASS")


def restore_present_state_server(controller_client, backup_file):
    """juju-restore won't restore when the state-server is still present."""
    try:
        controller_client.restore_backup(backup_file)
    except CalledProcessError:
        log.info(
            "juju-restore correctly refused to restore "
            "because the state-server was still up.")
        return
    else:
        raise Exception(
            "juju-restore restored to an operational state-serve")


def delete_controller_members(bs_manager, client, leader_only=False):
    """Delete controller members.

    The all members are delete by default. The followers are deleted before the
    leader to simulates a total controller failure. When leader_only is true,
    the leader is deleted to trigger a new leader election.
    """
    if leader_only:
        leader = client.get_controller_leader()
        members = [leader]
    else:
        members = client.get_controller_members()
        members.reverse()
    deleted_machines = []
    for machine in members:
        instance_id = machine.info.get('instance-id')
        if client.env.provider == 'azure':
            instance_id = convert_to_azure_ids(client, [instance_id])[0]
        host = machine.info.get('dns-name')
        log.info("Instrumenting node failure for member {}: {} at {}".format(
                 machine.machine_id, instance_id, host))
        terminate_instances(client.env, [instance_id])
        wait_for_state_server_to_shutdown(
            host, client, instance_id, timeout=120)
        deleted_machines.append(machine.machine_id)
    log.info("Deleted {}".format(deleted_machines))
    # Do not gather data about the deleted controller.
    if not leader_only:
        bs_manager.has_controller = False
    for m_id in deleted_machines:
        if bs_manager.known_hosts.get(m_id):
            del bs_manager.known_hosts[m_id]
    return deleted_machines


def restore_missing_state_server(bs_manager, controller_client, backup_file,
                                 check_controller=True):
    """juju-restore creates a replacement state-server for the services."""
    log.info("Starting restore.")
    try:
        controller_client.restore_backup(backup_file)
    except CalledProcessError as e:
        log.info('Call of juju restore exited with an error\n')
        log.info('Call:  %r\n', e.cmd)
        log.exception(e)
        raise LoggedException(e)
    if check_controller:
        controller_client.wait_for_started(600)
        # status can return even if controller isn't ready
        logging.info('Waiting for application to be ready')
        self.wait_for_application('dummy-source', 600)
    show_controller(bs_manager.client)
    bs_manager.has_controller = True
    bs_manager.client.set_config('dummy-source', {'token': 'Two'})
    bs_manager.client.wait_for_started()
    bs_manager.client.wait_for_workloads()
    check_token(bs_manager.client, 'Two')
    log.info("%s restored", bs_manager.client.env.environment)
    log.info("PASS")


def parse_args(argv=None):
    parser = ArgumentParser(description='Test recovery strategies.')
    add_basic_testing_arguments(parser)
    parser.add_argument(
        '--charm-series', help='Charm series.', default='')
    strategy = parser.add_argument_group('test strategy')
    strategy.add_argument(
        '--ha', action='store_const', dest='strategy', const='ha',
        default='backup', help="Test HA.")
    strategy.add_argument(
        '--backup', action='store_const', dest='strategy', const='backup',
        help="Test backup/restore.")
    strategy.add_argument(
        '--ha-backup', action='store_const', dest='strategy',
        const='ha-backup', help="Test backup/restore of HA.")
    return parser.parse_args(argv)


@contextmanager
def detect_bootstrap_machine(bs_manager):
    try:
        yield
    except Exception as e:
        address = parse_new_state_server_from_error(e)
        if address is not None:
            bs_manager.known_hosts['0'] = address
        raise


def assess_recovery(bs_manager, strategy, charm_series):
    log.info("Setting up test.")
    client = bs_manager.client
    deploy_stack(client, charm_series)
    client.set_config('dummy-source', {'token': ''})
    log.info("Setup complete.")
    log.info("Test started.")
    controller_client = client.get_controller_client()
    if strategy in ('ha', 'ha-backup'):
        enable_ha(bs_manager, controller_client)
    if strategy in ('ha-backup', 'backup'):
        backup_file = controller_client.backup()
        restore_present_state_server(controller_client, backup_file)
    if strategy == 'ha':
        leader_only = True
    else:
        leader_only = False
    delete_controller_members(
        bs_manager, controller_client, leader_only=leader_only)
    if strategy == 'ha':
        assess_ha_recovery(bs_manager, client)
    else:
        check_controller = strategy != 'ha-backup'
        restore_missing_state_server(
            bs_manager, controller_client, backup_file,
            check_controller=check_controller)
    log.info("Test complete.")


def main(argv):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(upload_tools=args.upload_tools):
        with detect_bootstrap_machine(bs_manager):
            assess_recovery(bs_manager, args.strategy, args.charm_series)


if __name__ == '__main__':
    main(sys.argv[1:])
