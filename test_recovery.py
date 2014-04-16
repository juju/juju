#!/usr/bin/env python
# Backup and restore a stack.

from __future__ import print_function

__metaclass__ = type

from argparse import ArgumentParser
import errno
import os
import re
import socket
import subprocess
import sys
from time import sleep

from jujuconfig import translate_to_env
from jujupy import (
    Environment,
    until_timeout,
)


backup_file_pattern = re.compile('(juju-backup-[0-9-]+\.tgz)')
running_instance_pattern = re.compile('\["([^"]+)"\]')


def print_now(string):
    print(string)
    sys.stdout.flush()


def setup_juju_path(juju_path):
    """Ensure the binaries and scripts under test are found first."""
    full_path = os.path.abspath(juju_path)
    if not os.path.isdir(full_path):
        raise ValueError("The juju_path does not exist: %s" % full_path)
    sys.path.insert(0, full_path)


def deploy_stack(env, charm_prefix):
    """"Deploy a simple stack, state-server and ubuntu."""
    if charm_prefix and not charm_prefix.endswith('/'):
        charm_prefix = charm_prefix + '/'
    env.bootstrap()
    agent_version = env.get_matching_agent_version()
    instance_id = env.get_status().status['machines']['0']['instance-id']
    for ignored in until_timeout(30):
        agent_versions = env.get_status().get_agent_versions()
        if 'unknown' not in agent_versions and len(agent_versions) == 1:
            break
    if agent_versions.keys() != [agent_version]:
        print_now("Current versions: %s" % ', '.join(agent_versions.keys()))
        env.juju('upgrade-juju', '--version', agent_version)
    env.wait_for_version(env.get_matching_agent_version())
    env.juju('deploy', charm_prefix + 'ubuntu')
    env.wait_for_started().status
    print_now("%s is ready to testing" % env.environment)
    return instance_id


def backup_state_server(env):
    """juju-backup provides a tarball."""
    environ = dict(os.environ)
    # juju-backup does not support the -e flag.
    environ['JUJU_ENV'] = env.environment
    output = subprocess.check_output(["juju-backup"], env=environ)
    print_now(output)
    match = backup_file_pattern.search(output)
    if match is None:
        raise Exception("The backup file was not found in output: %s" % output)
    backup_file_name = match.group(1)
    backup_file_path = os.path.abspath(backup_file_name)
    print_now("State-Server backup at %s" % backup_file_path)
    return backup_file_path


def restore_present_state_server(env, backup_file):
    """juju-restore wont restore when the state-server is still present."""
    environ = dict(os.environ)
    proc = subprocess.Popen(
        ["juju-restore", '-e', env.environment, backup_file], env=environ,
        stdout=subprocess.PIPE, stderr=subprocess.PIPE)
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
            raise Exception("The instance was not found in output above.")
        instance_id = match.group(1)
    return instance_id


def delete_instance(env, instance_id):
    """Delete the instance using the providers tools."""
    print_now("Instrumenting a bootstrap node failure.")
    provider_type = env.config.get('type')
    if provider_type == 'ec2':
        environ = dict(os.environ)
        ec2_url = 'https://%s.ec2.amazonaws.com' % env.config['region']
        environ['EC2_URL'] = ec2_url
        environ['EC2_ACCESS_KEY'] = env.config['access-key']
        environ['EC2_SECRET_KEY'] = env.config['secret-key']
        command_args = ['euca-terminate-instances', instance_id]
    elif provider_type == 'openstack':
        environ = dict(os.environ)
        environ.update(translate_to_env(env.config))
        command_args = ['nova', 'delete', instance_id]
    else:
        raise ValueError(
            "This test does not support the %s provider" % provider_type)
    print_now("Deleting %s." % instance_id)
    output = subprocess.check_output(command_args, env=environ)
    print_now(output)


def restore_missing_state_server(env, backup_file):
    """juju-restore creates a replacement state-server for the services."""
    environ = dict(os.environ)
    proc = subprocess.Popen(
        ['juju-restore', '-e', env.environment,  '--constraints', 'mem=2G',
         backup_file],
        env=environ, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    output, err = proc.communicate()
    if proc.returncode != 0:
        raise Exception("Restore failed: \n%s" % err)
    print_now(output)
    env.wait_for_started(150).status
    print_now("%s restored" % env.environment)
    print_now("PASS")


def wait_for_port(host, port, closed=False):
    for remaining in until_timeout(30):
        conn = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        conn.settimeout(max(remaining, 5))
        try:
            conn.connect((host, port))
        except socket.timeout:
            if closed:
                return
        except socket.error as e:
            if e.errno != errno.ECONNREFUSED:
                raise
            if closed:
                return
        except Exception as e:
            print_now('Unexpected %r: %s' % (type(e), e))
            raise
        else:
            conn.close()
            if not closed:
                return
            sleep(1)
    raise Exception('Timed out waiting for port.')


def main():
    parser = ArgumentParser('Test recovery strategies.')
    parser.add_argument(
        '--charm-prefix', help='A prefix for charm urls.', default='')
    strategy = parser.add_argument_group('test strategy')
    strategy.add_argument(
        '--ha', action='store_const', dest='strategy', const='ha',
        default='backup', help="Test HA.")
    strategy.add_argument(
        '--backup', action='store_const', dest='strategy', const='backup',
        help="Test backup/restore.")
    parser.add_argument('juju_path')
    parser.add_argument('env_name')
    args = parser.parse_args()
    try:
        setup_juju_path(args.juju_path)
        env = Environment.from_config(args.env_name)
        instance_id = deploy_stack(env, args.charm_prefix)
        if args.strategy == 'ha':
            env.juju('ensure-availability', '-n', '3')
        else:
            backup_file = backup_state_server(env)
            restore_present_state_server(env, backup_file)
        host = env.get_status().status['machines']['0']['dns-name']
        delete_instance(env, instance_id)
        print_now("Waiting for port to close on %s" % host)
        wait_for_port(host, 17070, closed=True)
        print_now("Closed.")
        if args.strategy == 'ha':
            env.client.get_juju_output(env, 'status', timeout=10)
        else:
            restore_missing_state_server(env, backup_file)
    except Exception as e:
        print_now(e)
        if getattr(e, 'output', None):
            print_now(e.output)
        print_now("FAIL")
        sys.exit(1)


if __name__ == '__main__':
    main()
