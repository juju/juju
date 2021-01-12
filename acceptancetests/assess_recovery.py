#!/usr/bin/env python3
# Backup and restore a stack.

from __future__ import print_function

import logging
import re
import sys
from argparse import ArgumentParser
from contextlib import contextmanager
from subprocess import CalledProcessError

import yaml

from deploy_stack import (BootstrapManager, deploy_dummy_stack,
                          get_remote_machines, get_token_from_status,
                          wait_for_state_server_to_shutdown)
from jujupy import parse_new_state_server_from_error
from substrate import convert_to_azure_ids, terminate_instances
from utility import (JujuAssertionError, LoggedException,
                     add_basic_testing_arguments, configure_logging,
                     until_timeout)

__metaclass__ = type

running_instance_pattern = re.compile('\["([^"]+)"\]')

log = logging.getLogger("assess_recovery")

def set_token(client, token):
    # unfortunately deploying a stack currently requires a token setting, but
    # in reality the way the charm exposes the token via status means that we
    # sometimes can't read the token i.e. if the workload-status changes after
    # or before the token is set multiple times.
    client.set_config('dummy-source', {'token': token})
    return client.action_do('dummy-sink/0', 'echo', "value={}".format(token))

def check_token(client, id, token):
    result = client.action_fetch(id, 'echo', "300s")
    parsed = yaml.safe_load(result)
    value = parsed["results"]["echo"]["value"]
    if value and token in value:
        return value
    raise JujuAssertionError('Token is not {}: {}'.format(
                             token, value))

def deploy_stack(client, charm_series):
    """"Deploy a simple stack, state-server and ubuntu."""
    deploy_dummy_stack(client, charm_series)
    id = set_token(client, 'One')
    client.wait_for_workloads()
    check_token(client, id, 'One')
    log.info("%s is ready to testing", client.env.environment)

def enable_ha(bs_manager, controller_client):
    """Enable HA and wait for the controllers to be ready."""
    log.info("Enabling HA.")
    controller_client.enable_ha()
    controller_client.wait_for_ha()
    show_controller(controller_client)
    remote_machines = get_remote_machines(
        controller_client, bs_manager.known_hosts)
    return remote_machines

def disable_ha(bs_manager, controller_client):
    """Disable HA and wait for the controllers to be ready."""
    log.info("Disabling HA.")
    condition = controller_client.remove_machine(["1", "2"], force=True, controller=True)
    controller_client.wait_for(condition)
    show_controller(controller_client)
    remote_machines = get_remote_machines(
        controller_client, None)
    return remote_machines

def restore_ha(bs_manager, controller_client):
    """Restore HA after a backup, as there is a possiblility that the controllers
    will be in a down state"""
    log.info("Restoring HA")
    machines_to_remove = []
    show_controller(controller_client)
    controller_client.show_status()
    # pause here, as all the machines can report back as being down, but after
    # the pause, one will report back.
    controller_client._backend.pause(300)
    status = controller_client.get_status(controller=True)
    # the order of the machines are normally wrong after a restore, so iterating
    # through the machines until you find the correct ones to remove works.
    for name, machine in status.iter_machines():
        machine_status = machine['juju-status']
        if machine_status["current"] == "down":
            machines_to_remove.append(name)
    if len(machines_to_remove) > 0:
        controller_client.show_status()
        condition = controller_client.remove_machine(machines_to_remove, force=True, controller=True)
        controller_client.wait_for(condition)
    return enable_ha(bs_manager, controller_client)

def show_controller(client):
    controller_info = client.show_controller(format='yaml')
    log.info('Controller is:\n{}'.format(controller_info))

def restore_backup(bs_manager, client, backup_file,
                                 check_controller=True):
    """juju-restore restores the backup file."""
    log.info("Starting restore.")
    try:
        client.restore_backup(backup_file)
    except CalledProcessError as e:
        log.info('Call of juju restore exited with an error\n')
        log.info('Call:  %r\n', e.cmd)
        log.exception(e)
        raise LoggedException(e)
    if check_controller:
        client.wait_for_started(600)
    show_controller(bs_manager.client)
    bs_manager.has_controller = True

def assess_backup(client):
    id = set_token(client, 'Two')
    client.wait_for_started()
    client.wait_for_workloads()
    check_token(client, id, 'Two')
    log.info("%s restored", client.env.environment)
    log.info("PASS")

def create_tmp_model(client):
    model_name = 'temp-model'
    new_env = client.env.clone(model_name)
    return client.add_model(new_env)

def parse_args(argv=None):
    parser = ArgumentParser(description='Test recovery strategies.')
    add_basic_testing_arguments(parser, existing=False)
    parser.add_argument(
        '--charm-series', help='Charm series.', default='')
    strategy = parser.add_argument_group('test strategy')
    strategy.add_argument(
        '--backup', action='store_const', dest='strategy', const='backup',
        help="Test backup/restore.")
    strategy.add_argument(
        '--ha-backup', action='store_const', dest='strategy',
        const='ha-backup', help="Test backup of HA.")
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
    set_token(client, 'empty')
    log.info("Setup complete.")
    log.info("Test started.")
    controller_client = client.get_controller_client()

    # ha-backup allows us to still backup in HA mode, so allow us to still
    # create a cluster of controllers.
    haBackup = strategy == 'ha-backup'
    if haBackup:
        bs_manager.known_hosts = enable_ha(bs_manager, controller_client)
        backup_file = controller_client.backup()
        bs_manager.known_hosts = disable_ha(bs_manager, controller_client)
    else:
        backup_file = controller_client.backup()

    create_tmp_model(client)
    # check_controller if NOT in haBackup, as we know that some of the
    # controllers are currently down.
    restore_backup(bs_manager, controller_client, backup_file,
        check_controller=not haBackup)
    if haBackup:
        bs_manager.known_hosts = restore_ha(bs_manager, controller_client)
    assess_backup(bs_manager.client)
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
