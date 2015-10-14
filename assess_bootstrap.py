#!/usr/bin/env python
from __future__ import print_function

from argparse import ArgumentParser
import logging
import os.path

from deploy_stack import update_env
from jujupy import (
    EnvJujuClient,
    SimpleEnvironment,
    temp_bootstrap_env,
    )
from utility import (
    configure_logging,
    scoped_environ,
)


log = logging.getLogger("assess_bootstrap")


def assess_bootstrap(juju, env, debug, region, temp_env_name):
    with scoped_environ():
        juju_bin = os.path.dirname(os.path.abspath(juju))
        os.environ['PATH'] = '{}:{}'.format(juju_bin, os.environ['PATH'])
        client = EnvJujuClient.by_version(SimpleEnvironment.from_config(env),
                                          juju, debug)
    if temp_env_name is None:
        temp_env_name = client.env.environment
    update_env(client.env, temp_env_name, region=region)
    with temp_bootstrap_env(client.juju_home, client):
        client.destroy_environment()
        try:
            client.bootstrap()
        except:
            client.destroy_environment()
            raise
    try:
        client.get_status(1)
        log.info('Environment successfully bootstrapped.')
    finally:
        client.destroy_environment()


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
    assess_bootstrap(**args.__dict__)


if __name__ == '__main__':
    main()
