#!/usr/bin/env python
from argparse import ArgumentParser
import logging
import subprocess

from deploy_stack import (
    boot_context,
    get_log_level,
    get_juju_path,
)
from jujupy import (
    EnvJujuClient,
    SimpleEnvironment,
)
from utility import (
    configure_logging,
)


def parse_args(argv=None):
    parser = ArgumentParser()
    parser.add_argument('bundle_path',
                        help='URL or path to a bundle')
    parser.add_argument('env',
                        help='The juju environment to test')
    parser.add_argument('logs', help='log directory.')
    parser.add_argument('job_name', help='Name of the Jenkins job.')
    parser.add_argument('--bundle-name', default=None,
                        help='Name of the bundle to deploy.')
    parser.add_argument('--health-cmd', default=None,
                        help='A binary for checking the health of the'
                        ' deployed bundle.')
    parser.add_argument('--keep-env', action='store_true', default=False,
                        help='Keep the Juju environment after the test'
                        ' completes.')
    parser.add_argument('--agent-url', default=None,
                        help='URL to use for retrieving agent binaries.')
    parser.add_argument('--agent-stream', default=None,
                        help='stream name for retrieving agent binaries.')
    parser.add_argument('--series',
                        help='Name of the Ubuntu series to use.')
    parser.add_argument('--debug', action="store_true", default=False,
                        help='Use --debug juju logging.')
    parser.add_argument('--verbose', '-v', action="store_true", default=False,
                        help='Increase logging verbosity.')

    parser.add_argument('--new-juju-bin', default=None,
                        help='Dirctory containing the new Juju binary.')
    return parser.parse_args(argv)


def is_healthy(cmd_path):
    """Returns a boolean after running the health_checker."""
    try:
        sub_output = subprocess.check_output(cmd_path)
        logging.info('Health check output: {}'.format(sub_output))
    except OSError as e:
        logging.error(
            'The health check script failed to execute with: {}'.format(
                e))
        raise
    except subprocess.CalledProcessError as e:
        logging.error('Non-zero exit code returned from {}: {}'.format(
            cmd_path, e))
        logging.error(e.output)
        return False
    return True


def run_deployer():
    args = parse_args()
    juju_path = get_juju_path(args)
    configure_logging(get_log_level(args))
    env = SimpleEnvironment.from_config(args.env)
    client = EnvJujuClient.by_version(env, juju_path, debug=args.debug)
    with boot_context(args.job_name, client, None, [], args.series,
                      args.agent_url, args.agent_stream, args.logs,
                      args.keep_env, False):
        client.deployer(args.bundle_path, args.bundle_name)
        if args.health_cmd:
            is_healthy(args.health_cmd)
if __name__ == '__main__':
    run_deployer()
