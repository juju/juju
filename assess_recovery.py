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
    make_client,
    parse_new_state_server_from_error,
)
from substrate import (
    terminate_instances,
)
from utility import (
    LoggedException,
    print_now,
)


__metaclass__ = type


running_instance_pattern = re.compile('\["([^"]+)"\]')


def deploy_stack(client, charm_prefix):
    """"Deploy a simple stack, state-server and ubuntu."""
    if charm_prefix and not charm_prefix.endswith('/'):
        charm_prefix = charm_prefix + '/'
    client.juju('deploy', (charm_prefix + 'ubuntu',))
    client.wait_for_started().status
    print_now("%s is ready to testing" % client.env.environment)


def restore_present_state_server(admin_client, backup_file):
    """juju-restore won't restore when the state-server is still present."""
    try:
        output = admin_client.restore_backup(backup_file)
    except CalledProcessError as e:
        print_now(
            "juju-restore correctly refused to restore "
            "because the state-server was still up.")
        match = running_instance_pattern.search(e.stderr)
        if match is None:
            print_now("WARNING: Could not find the instance_id in output:")
            print_now(e.stderr)
            print_now("")
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
        print_now("Instrumenting node failure for member {}: {} at {}".format(
                  machine.machine_id, instance_id, host))
        terminate_instances(client.env, [instance_id])
        wait_for_state_server_to_shutdown(host, client, instance_id)
        deleted_machines.append(machine.machine_id)
    return deleted_machines


def restore_missing_state_server(client, admin_client, backup_file):
    """juju-restore creates a replacement state-server for the services."""
    print_now("Starting restore.")
    try:
        output = admin_client.restore_backup(backup_file)
    except CalledProcessError as e:
        print_now('Call of juju restore exited with an error\n')
        print_now('Call: {} \n'.format(e.cmd))
        message = 'Restore failed: \n%s' % e.stderr
        print_now(message)
        print_now('\n')
        logging.exception(e)
        raise LoggedException(e)
    print_now(output)
    admin_client.wait_for_started(600).status
    print_now("%s restored" % client.env.environment)
    print_now("PASS")


def parse_args(argv=None):
    parser = ArgumentParser('Test recovery strategies.')
    parser.add_argument(
        '--charm-prefix', help='A prefix for charm urls.', default='')
    parser.add_argument(
        '--debug', action='store_true', default=False,
        help='Use --debug juju logging.')
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
    parser.add_argument('juju_path')
    parser.add_argument('env_name')
    parser.add_argument('logs', help='Directory to store logs in.')
    parser.add_argument(
        'temp_env_name', nargs='?',
        help='Temporary environment name to use for this test.')
    parser.add_argument(
        '--agent-stream', help='Stream for retrieving agent binaries.')
    parser.add_argument(
        '--series', help='Name of the Ubuntu series to use.')
    return parser.parse_args(argv)


def make_client_from_args(args):
    return make_client(args.juju_path, args.debug, args.env_name,
                       args.temp_env_name)


@contextmanager
def detect_bootstrap_machine(bs_manager):
    try:
        yield
    except Exception as e:
        bs_manager.known_hosts['0'] = parse_new_state_server_from_error(e)
        raise


def assess_recovery(bs_manager, strategy, charm_prefix):
    client = bs_manager.client
    deploy_stack(client, charm_prefix)
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
    client = make_client_from_args(args)
    jes_enabled = client.is_jes_enabled()
    bs_manager = BootstrapManager(
        client.env.environment, client, client, None, [], args.series,
        agent_url=None, agent_stream=args.agent_stream, region=None,
        log_dir=args.logs, keep_env=False, permanent=jes_enabled,
        jes_enabled=jes_enabled)
    with bs_manager.booted_context(upload_tools=False):
        with detect_bootstrap_machine(bs_manager):
            assess_recovery(bs_manager, args.strategy, args.charm_prefix)


if __name__ == '__main__':
    main(sys.argv[1:])
