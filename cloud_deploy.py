#!/usr/bin/env python
from __future__ import print_function

__metaclass__ = type

from argparse import ArgumentParser
import os
import subprocess
import sys

from jujupy import (
    CannotConnectEnv,
    Environment,
)


def deploy_stack(environment, debug):
    """"Deploy a test stack in the specified environment.

    :param environment: The name of the desired environment.
    """
    env = Environment.from_config(environment)
    env.client.debug = debug
    # Clean up any leftover junk
    env.destroy_environment()
    env.bootstrap()
    try:
        # wait for status info....
        try:
            try:
                env.get_status()
            except CannotConnectEnv:
                print("Status got Unable to connect to env.  Retrying...")
                env.get_status()
            env.wait_for_started()
        except subprocess.CalledProcessError as e:
            if getattr(e, 'stderr', None) is not None:
                sys.stderr.write(e.stderr)
            raise
    finally:
        env.destroy_environment()


def main():
    parser = ArgumentParser('Test a cloud')
    parser.add_argument('env', help='The juju environment to test')
    args = parser.parse_args()
    debug = bool(os.environ.get('DEBUG') == 'true')
    try:
        deploy_stack(args.env, debug)
    except Exception as e:
        print('%s: %s' % (type(e), e))
        sys.exit(1)


if __name__ == '__main__':
    main()
