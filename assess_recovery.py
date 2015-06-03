#!/usr/bin/env python
# Backup and restore a stack.

from __future__ import print_function

__metaclass__ = type

from argparse import ArgumentParser
import logging
import os
import re
import subprocess
import sys

from deploy_stack import (
    dump_env_logs,
    get_machine_dns_name,
    wait_for_state_server_to_shutdown,
)
from jujuconfig import (
    get_jenv_path,
    get_juju_home,
)
from jujupy import (
    temp_bootstrap_env,
    until_timeout,
    make_client,
    parse_new_state_server_from_error,
)
from substrate import (
    terminate_instances,
)
from utility import (
    ensure_deleted,
    print_now,
)


running_instance_pattern = re.compile('\["([^"]+)"\]')


def setup_juju_path(juju_path):
    """Ensure the binaries and scripts under test are found first."""
    full_path = os.path.abspath(juju_path)
    if not os.path.isdir(full_path):
        raise ValueError("The juju_path does not exist: %s" % full_path)
    os.environ['PATH'] = '%s:%s' % (full_path, os.environ['PATH'])
    sys.path.insert(0, full_path)


def deploy_stack(client, charm_prefix):
    """"Deploy a simple stack, state-server and ubuntu."""
    if charm_prefix and not charm_prefix.endswith('/'):
        charm_prefix = charm_prefix + '/'
    agent_version = client.get_matching_agent_version()
    instance_id = client.get_status().status['machines']['0']['instance-id']
    for ignored in until_timeout(30):
        agent_versions = client.get_status().get_agent_versions()
        if 'unknown' not in agent_versions and len(agent_versions) == 1:
            break
    if agent_versions.keys() != [agent_version]:
        print_now("Current versions: %s" % ', '.join(agent_versions.keys()))
        client.juju('upgrade-juju', ('--version', agent_version))
    client.wait_for_version(client.get_matching_agent_version())
    client.juju('deploy', (charm_prefix + 'ubuntu',))
    client.wait_for_started().status
    print_now("%s is ready to testing" % client.env.environment)
    return instance_id


def restore_present_state_server(client, backup_file):
    """juju-restore won't restore when the state-server is still present."""
    environ = dict(os.environ)
    proc = subprocess.Popen(
        ['juju', '--show-log', 'restore', '-e', client.env.environment,
         backup_file],
        env=environ, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    output, err = proc.communicate()
    if proc.returncode == 0:
        raise Exception(
            "juju-restore restored to an operational state-server: %s" % err)
    else:
        print_now(
            "juju-restore correctly refused to restore "
            "because the state-server was still up.")
        match = running_instance_pattern.search(err)
        if match is None:
            print_now("WARNING: Could not find the instance_id in output:")
            print_now(err)
            print_now("")
            return None
        instance_id = match.group(1)
    return instance_id


def delete_instance(client, instance_id):
    """Delete the instance using the providers tools."""
    print_now("Instrumenting a bootstrap node failure.")
    return terminate_instances(client.env, [instance_id])


def delete_extra_state_servers(client, instance_id):
    """Delete the extra state-server instances."""
    status = client.get_status()
    for machine, info in status.iter_machines():
        extra_instance_id = info.get('instance-id')
        status = info.get('state-server-member-status')
        if extra_instance_id != instance_id and status is not None:
            print_now("Deleting state-server-member {}".format(machine))
            host = get_machine_dns_name(client, machine)
            delete_instance(client, extra_instance_id)
            wait_for_state_server_to_shutdown(host, client, extra_instance_id)


def restore_missing_state_server(client, backup_file):
    """juju-restore creates a replacement state-server for the services."""
    environ = dict(os.environ)
    print_now("Starting restore.")
    proc = subprocess.Popen(
        ['juju', '--show-log', 'restore', '-e', client.env.environment,
         '--constraints', 'mem=2G', backup_file],
        env=environ, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    output, err = proc.communicate()
    if proc.returncode != 0:
        print_now('Call of juju restore exited with an error\n')
        message = 'Restore failed: \n%s' % err
        print_now(message)
        print_now('\n')
        raise Exception(message)
    print_now(output)
    client.wait_for_started(600).status
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
    return parser.parse_args(argv)


def main():
    args = parse_args()
    log_dir = args.logs
    try:
        setup_juju_path(args.juju_path)
        client = make_client(args.juju_path, args.debug, args.env_name,
                             args.temp_env_name)
        juju_home = get_juju_home()
        ensure_deleted(get_jenv_path(juju_home, client.env.environment))
        with temp_bootstrap_env(juju_home, client):
            client.bootstrap()
        bootstrap_host = get_machine_dns_name(client, 0)
        try:
            instance_id = deploy_stack(client, args.charm_prefix)
            if args.strategy in ('ha', 'ha-backup'):
                client.juju('ensure-availability', ('-n', '3'))
                client.wait_for_ha()
            if args.strategy in ('ha-backup', 'backup'):
                backup_file = client.backup()
                restore_present_state_server(client, backup_file)
            if args.strategy == 'ha-backup':
                delete_extra_state_servers(client, instance_id)
            delete_instance(client, instance_id)
            wait_for_state_server_to_shutdown(bootstrap_host, client,
                                              instance_id)
            bootstrap_host = None
            if args.strategy == 'ha':
                client.get_status(600)
            else:
                restore_missing_state_server(client, backup_file)
        except Exception as e:
            if bootstrap_host is None:
                bootstrap_host = parse_new_state_server_from_error(e)
            dump_env_logs(client, bootstrap_host, log_dir)
            raise
        finally:
            client.destroy_environment()
    except Exception as e:
        print_now("\nEXCEPTION CAUGHT:\n")
        logging.exception(e)
        if getattr(e, 'output', None):
            print_now('\n')
            print_now(e.output)
        print_now("\nFAIL")
        sys.exit(1)


if __name__ == '__main__':
    main()
