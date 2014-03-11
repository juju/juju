#!/usr/bin/env python
# Backup and restore a stack.

from __future__ import print_function

__metaclass__ = type

from argparse import ArgumentParser
import os
import re
import subprocess
import sys

from jujupy import (
    Environment,
    until_timeout,
)


backup_file_pattern = re.compile('(juju-backup-[0-9-]+\.tgz)')
running_instance_pattern = re.compile('\["([^"]+)"\]')


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
    env.get_status()
    for ignored in until_timeout(30):
        agent_versions = env.get_status().get_agent_versions()
        if 'unknown' not in agent_versions and len(agent_versions) == 1:
            break
    if agent_versions.keys() != [agent_version]:
        print("Current versions: %s" % ', '.join(agent_versions.keys()))
        env.juju('upgrade-juju', '--version', agent_version)
    env.wait_for_version(env.get_matching_agent_version())
    env.juju('deploy', charm_prefix + 'ubuntu')
    env.wait_for_started().status
    print("%s is ready to testing" % env.environment)
    return env


def backup_state_server(env):
    """juju-backup provides a tarball."""
    environ = dict(os.environ)
    environ['JUJU_ENV'] = env.environment
    output = subprocess.check_output(["juju-backup"], env=environ)
    match = backup_file_pattern.search(output)
    if match is None:
        raise Exception("The backup file was not found in output: %s" % output)
    backup_file_name = match.group(1)
    backup_file_path = os.path.abspath(backup_file_name)
    print("State-Server backup at %s" % backup_file_path)
    return backup_file_path


def restore_present_state_server(env, backup_file):
    """juju-restore wont restore when the state-server is still present."""
    environ = dict(os.environ)
    environ['JUJU_ENV'] = env.environment
    proc = subprocess.Popen(
        ["juju-restore", backup_file], env=environ,
        stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    output, err = proc.communicate()
    if proc.returncode == 0:
        raise Exception(
            "juju-restore restored to an operational state-server: %s" % err)
    else:
        print(
            "juju-restore correctly refused to restore "
            "because the state-server was still up.")
        match = running_instance_pattern.search(err)
        if match is None:
            raise Exception("The instance was not found in output: %s" % err)
        instance_id = match.group(1)
    return instance_id


def delete_instance(env, instance_id):
    """Delete the instance using the providers tools."""
    print("Instrumenting a bootstrap node failure.")
    provider_type = env.config.get('type')
    if provider_type == 'ec2':
        environ = dict(os.environ)
        ec2_url = 'https://%s.ec2.amazonaws.com' % env.config['region']
        environ['EC2_URL'] = ec2_url
        environ['EC2_ACCESS_KEY'] = env.config['access-key']
        environ['EC2_SECRET_KEY'] = env.config['secret-key']
        print("Deleting %s." % instance_id)
        output = subprocess.check_output(
            ['euca-terminate-instances', instance_id], env=environ)
        print(output)
    else:
        raise ValueError(
            "This test does not support the %s provider" % provider_type)


def restore_missing_state_server(env, backup_file):
    """juju-restore creates a replacement state-server for the services."""
    environ = dict(os.environ)
    environ['JUJU_ENV'] = env.environment
    subprocess.check_call(
        ['juju-restore', '--constraints', 'mem=2G', backup_file], env=environ)
    env.wait_for_started().status
    print("%s restored" % env.environment)


def main():
    parser = ArgumentParser('Backup and restore a stack')
    parser.add_argument(
        '--charm-prefix', help='A prefix for charm urls.', default='')
    parser.add_argument('juju_path')
    parser.add_argument('env_name')
    args = parser.parse_args()
    try:
        setup_juju_path(args.juju_path)
        env = Environment.from_config(args.env_name)
        deploy_stack(env, args.charm_prefix)
        backup_file = backup_state_server(env)
        instance_id = restore_present_state_server(env, backup_file)
        delete_instance(env, instance_id)
        restore_missing_state_server(env, backup_file)
    except Exception as e:
        print(e)
        sys.exit(1)


if __name__ == '__main__':
    main()
