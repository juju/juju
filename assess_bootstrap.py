#!/usr/bin/env python
from __future__ import print_function

from argparse import ArgumentParser
import os.path

from jujupy import (
    Environment,
    until_timeout,
    )
from utility import scoped_environ


def assess_bootstrap(juju, env, debug):
    with scoped_environ():
        juju_bin = os.path.dirname(os.path.abspath(juju))
        os.environ['PATH'] = '{}:{}'.format(juju_bin, os.environ['PATH'])
        environment = Environment.from_config(env)
    environment.client.debug = debug
    environment.destroy_environment()
    try:
        environment.bootstrap()
        for ignored in until_timeout(30):
            environment.get_status(1)
        print('Environment successfully bootstrapped.')
    finally:
        environment.destroy_environment()


def parse_args(argv=None):
    parser = ArgumentParser()
    parser.add_argument('juju', help="The Juju client to use.")
    parser.add_argument('env', help="The environment to test with.")
    parser.add_argument('--debug', action="store_true", default=False,
                        help='Use --debug juju logging.')
    return parser.parse_args(argv)


def main():
    args = parse_args()
    assess_bootstrap(**args.__dict__)


if __name__ == '__main__':
    main()
