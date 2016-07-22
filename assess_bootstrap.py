#!/usr/bin/env python
from __future__ import print_function

from argparse import ArgumentParser
import logging
import os.path
import sys

from deploy_stack import (
    BootstrapManager,
    tear_down,
    )
from jujupy import (
    client_from_config,
    )
from utility import (
    configure_logging,
    LoggedException,
    scoped_environ,
    temp_dir,
)


log = logging.getLogger("assess_bootstrap")


def assess_bootstrap(juju, env, debug, region, temp_env_name):
    with scoped_environ():
        juju_bin = os.path.dirname(os.path.abspath(juju))
        os.environ['PATH'] = '{}:{}'.format(juju_bin, os.environ['PATH'])
        client = client_from_config(env, juju, debug)
    jes_enabled = client.is_jes_enabled()
    if temp_env_name is None:
        temp_env_name = client.env.environment
    with temp_dir() as log_dir:
        bs_manager = BootstrapManager(
            temp_env_name, client, client, region=region,
            permanent=jes_enabled, jes_enabled=jes_enabled, log_dir=log_dir,
            bootstrap_host=None, machines=[], series=None, agent_url=None,
            agent_stream=None, keep_env=False)
        with bs_manager.top_context() as machines:
            with bs_manager.bootstrap_context(machines):
                tear_down(client, jes_enabled)
                client.bootstrap()
            with bs_manager.runtime_context(machines):
                client.get_status(1)
                log.info('Environment successfully bootstrapped.')


def parse_args(argv=None):
    parser = ArgumentParser()
    parser.add_argument('juju', help="The Juju client to use.")
    parser.add_argument('env', help="The environment to test with.")
    parser.add_argument('temp_env_name', nargs='?',
                        help="Temporary environment name to use.")
    parser.add_argument('--debug', action="store_true", default=False,
                        help='Use --debug juju logging.')
    parser.add_argument('--region', help='Override environment region.')
    return parser.parse_args(argv)


def main():
    args = parse_args()
    configure_logging(logging.INFO)
    try:
        assess_bootstrap(**args.__dict__)
    except LoggedException:
        sys.exit(1)


if __name__ == '__main__':
    main()
