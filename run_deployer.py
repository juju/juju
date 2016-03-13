#!/usr/bin/env python
from argparse import ArgumentParser
import logging
import subprocess
import sys

from deploy_stack import (
    assess_upgrade,
    boot_context,
)
from jujupy import (
    EnvJujuClient,
    SimpleEnvironment,
)
from remote import (
    remote_from_unit,
)
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    scoped_environ,
)


class ErrUnitCondition(Exception):
    """An exception for an unknown condition type."""


def parse_args(argv=None):
    parser = ArgumentParser()
    parser.add_argument('--upgrade', action="store_true", default=False,
                        help='Perform an upgrade test.')
    parser.add_argument('bundle_path',
                        help='URL or path to a bundle')
    add_basic_testing_arguments(parser)
    parser.add_argument('--allow-native-deploy', action='store_true',
                        help='Let juju 2 use native bundle deploying.')
    parser.add_argument('--bundle-name', default=None,
                        help='Name of the bundle to deploy.')
    parser.add_argument('--health-cmd', default=None,
                        help='A binary for checking the health of the'
                        ' deployed bundle.')
    parser.add_argument('--upgrade-condition', action='append', default=None,
                        help='unit_name:<conditions>'
                        ' One or more of the following conditions to apply'
                        ' to the given unit_name: clock_skew.')
    parser.add_argument('--agent-timeout', type=int, default=1200,
                        help='The time to wait for agents to start')
    parser.add_argument('--workload-timeout', type=int, default=1800,
                        help='The time to wait for workloads to active')
    args = parser.parse_args(argv)
    if args.allow_native_deploy and args.bundle_name:
        parser.error('cannot supply bundle name with native juju deploying')
    return args


def check_health(cmd_path, env_name='', environ=None):
    """Run the health checker command and raise on error."""
    try:
        cmd = (cmd_path, env_name)
        logging.info('Calling {}'.format(cmd))
        if environ:
            logging.info('PATH: {}'.format(environ.get('PATH')))
            logging.info('JUJU_HOME: {}'.format(environ.get('JUJU_HOME')))
        with scoped_environ(environ):
            sub_output = subprocess.check_output(cmd)
        logging.info('Health check output: {}'.format(sub_output))
    except OSError as e:
        logging.error(
            'Failed to execute {}: {}'.format(
                cmd, e))
        raise
    except subprocess.CalledProcessError as e:
        logging.error('Non-zero exit code returned from {}: {}'.format(
            cmd, e))
        logging.error(e.output)
        raise


CLOCK_SKEW_SCRIPT = """
        now=$(date +%s)
        let future=$now+600
        sudo date --set @$future
    """


def apply_condition(client, condition):
    """Apply an adverse condition to the given unit."""
    unit, action = condition.split(':', 1)
    logging.info('Applying {} to unit {}'.format(action, unit))
    remote = remote_from_unit(client, unit)
    if not remote.is_windows():
        if action == 'clock_skew':
            result = remote.run(CLOCK_SKEW_SCRIPT)
            logging.info('Clock on {} set to: {}'.format(unit, result))
        else:
            raise ErrUnitCondition("%s: Unknown condition type." % action)


def assess_deployer(args, client, agent_timeout, workload_timeout):
    """Run juju-deployer, based on command line configuration values."""
    if args.allow_native_deploy:
        client.deploy_bundle(args.bundle_path)
        client.wait_for_started(timeout=agent_timeout)
    else:
        client.deployer(args.bundle_path, args.bundle_name)
    client.wait_for_workloads(timeout=workload_timeout)
    if args.health_cmd:
        environ = client._shell_environ()
        check_health(args.health_cmd, args.temp_env_name, environ)
    if args.upgrade:
        client.show_status()
        if args.upgrade_condition:
            for condition in args.upgrade_condition:
                apply_condition(client, condition)
        assess_upgrade(client, args.juju_bin)
        client.wait_for_workloads()
        if args.health_cmd:
            environ = client._shell_environ()
            check_health(args.health_cmd, args.temp_env_name, environ)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    env = SimpleEnvironment.from_config(args.env)
    start_juju_path = None if args.upgrade else args.juju_bin
    client = EnvJujuClient.by_version(env, start_juju_path, debug=args.debug)
    with boot_context(args.temp_env_name, client, None, [], args.series,
                      args.agent_url, args.agent_stream, args.logs,
                      args.keep_env, upload_tools=args.upload_tools,
                      region=args.region):
        assess_deployer(
            args, client, args.agent_timeout, args.workload_timeout)
    return 0


if __name__ == '__main__':
    sys.exit(main())
