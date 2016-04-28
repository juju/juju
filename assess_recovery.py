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
    wait_for_state_server_to_shutdown,
)
from jujupy import (
    parse_new_state_server_from_error,
)
from substrate import (
    terminate_instances,
)
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    local_charm_path,
    LoggedException,
)


__metaclass__ = type


running_instance_pattern = re.compile('\["([^"]+)"\]')


log = logging.getLogger("assess_recovery")


def deploy_stack(client, charm_series):
    """"Deploy a simple stack, state-server and ubuntu."""
    charm = local_charm_path(
        charm='ubuntu', juju_ver=client.version, series=charm_series)
    client.deploy(charm, series=charm_series)
    client.wait_for_started().status
    log.info("%s is ready to testing", client.env.environment)


def restore_present_state_server(admin_client, backup_file):
    """juju-restore won't restore when the state-server is still present."""
    try:
        output = admin_client.restore_backup(backup_file)
    except CalledProcessError as e:
        log.info(
            "juju-restore correctly refused to restore "
            "because the state-server was still up.")
        match = running_instance_pattern.search(e.stderr)
        if match is None:
            log.warning("Could not find the instance_id in output:\n%s\n",
                        e.stderr)
            return None
        return match.group(1)
    else:
        raise Exception(
            "juju-restore restored to an operational state-server: %s" %
            output)


def delete_controller_members(client, leader_only=False):
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
        host = machine.info.get('dns-name')
        log.info("Instrumenting node failure for member {}: {} at {}".format(
                  machine.machine_id, instance_id, host))
        terminate_instances(client.env, [instance_id])
        wait_for_state_server_to_shutdown(host, client, instance_id)
        deleted_machines.append(machine.machine_id)
    return deleted_machines


def restore_missing_state_server(client, admin_client, backup_file):
    """juju-restore creates a replacement state-server for the services."""
    log.info("Starting restore.")
    try:
        output = admin_client.restore_backup(backup_file)
    except CalledProcessError as e:
        log.info('Call of juju restore exited with an error\n')
        log.info('Call:  %r\n', e.cmd)
        log.info('Restore failed: \n%s\n', e.stderr)
        log.exception(e)
        raise LoggedException(e)
    log.info(output)
    admin_client.wait_for_started(600).status
    log.info("%s restored", client.env.environment)
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
        bs_manager.known_hosts['0'] = parse_new_state_server_from_error(e)
        raise


def assess_recovery(bs_manager, strategy, charm_series):
    client = bs_manager.client
    deploy_stack(client, charm_series)
    admin_client = client.get_admin_client()
    if strategy in ('ha', 'ha-backup'):
        admin_client.enable_ha()
        admin_client.wait_for_ha()
    if strategy in ('ha-backup', 'backup'):
        backup_file = admin_client.backup()
        restore_present_state_server(admin_client, backup_file)
    if strategy == 'ha':
        leader_only = True
    else:
        leader_only = False
    deleted_machine_ids = delete_controller_members(
        admin_client, leader_only=leader_only)
    for m_id in deleted_machine_ids:
        if bs_manager.known_hosts.get(m_id):
            del bs_manager.known_hosts[m_id]
    if strategy == 'ha':
        client.get_status(600)
    else:
        restore_missing_state_server(client, admin_client, backup_file)


def main(argv):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(upload_tools=args.upload_tools):
        with detect_bootstrap_machine(bs_manager):
            assess_recovery(bs_manager, args.strategy, args.charm_series)


if __name__ == '__main__':
    main(sys.argv[1:])
