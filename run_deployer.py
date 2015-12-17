#!/usr/bin/env python
from argparse import ArgumentParser
import logging
import subprocess

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
    parser.add_argument('--bundle-name', default=None,
                        help='Name of the bundle to deploy.')
    parser.add_argument('--health-cmd', default=None,
                        help='A binary for checking the health of the'
                        ' deployed bundle.')
    parser.add_argument('--upgrade-condition', action='append', default=None,
                        help='Unit_name:<conditions>'
                        ' One or more of the following conditions to apply'
                        ' to the given Unit_name: clock_skew, exausted_disk.')
    return parser.parse_args(argv)


def check_health(cmd_path, env_name=''):
    """Run the health checker command and raise on error."""
    try:
        cmd = (cmd_path, env_name)
        logging.debug('Calling {}'.format(cmd))
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
    unit_action = condition.split(':')
    unit = unit_action[0]
    action = unit_action[1]
    logging.info('Applying {} to unit {}'.format(action, unit))
    remote = remote_from_unit(client, unit)
    if not remote.is_windows():
        if action == 'clock_skew':
            result = remote.run(CLOCK_SKEW_SCRIPT)
            logging.info('Clock on {} set to: {}'.format(unit, result))
        else:
            raise ErrUnitCondition("%s: Unknown condition type." % action)


def run_deployer(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    env = SimpleEnvironment.from_config(args.env)
    start_juju_path = None if args.upgrade else args.juju_bin
    client = EnvJujuClient.by_version(env, start_juju_path, debug=args.debug)
    with boot_context(args.temp_env_name, client, None, [], args.series,
                      args.agent_url, args.agent_stream, args.logs,
                      args.keep_env, False, region=args.region):
        client.deployer(args.bundle_path, args.bundle_name)
        if args.health_cmd:
            check_health(args.health_cmd, args.temp_env_name)
        if args.upgrade:
            client.juju('status', ())
            if args.upgrade_condition:
                for condition in args.upgrade_condition:
                    apply_condition(client, condition)
            assess_upgrade(client, args.juju_bin)
            if args.health_cmd:
                check_health(args.health_cmd, args.temp_env_name)
if __name__ == '__main__':
    run_deployer()
