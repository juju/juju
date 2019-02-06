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

#
# The following acceptance test has been modifed to remove the HA testing
# strategy from the test. The CLI no longer supports the following:
#  - constraints when restoring a backup file
#  - HA backup and restore
#
# Once HA backup and restore are reinstanted in Juju, we should restore the
# the HA nature of this acceptance test.
#

__metaclass__ = type

running_instance_pattern = re.compile('\["([^"]+)"\]')

log = logging.getLogger("assess_recovery")

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
    bs_manager.client.set_config('dummy-source', {'token': 'Two'})
    bs_manager.client.wait_for_started()
    bs_manager.client.wait_for_workloads()
    check_token(bs_manager.client, 'Two')
    log.info("%s restored", bs_manager.client.env.environment)
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

    backup_file = controller_client.backup()
    create_tmp_model(client)
    restore_backup(bs_manager, controller_client, backup_file)

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
