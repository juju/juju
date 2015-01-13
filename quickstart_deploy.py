#!/usr/bin/env python
from __future__ import print_function

__metaclass__ = type

from argparse import ArgumentParser
import subprocess
import sys

from jujupy import (
    CannotConnectEnv,
    Environment,
)


def run_quickstart(environment, bundle_path, debug):
    """"Deploy a bundle in the specified environment.

    :param environment: The name of the desired environment.
    :param bundle_path: The Path or URL of the bundle to installed.
    :param debug: Boolean for enabling client debug output.
    """
    env = Environment.from_config(environment)
    env.client.debug = debug
    # Clean up any leftover junk
    env.destroy_environment()
    env.quickstart(bundle_path)
    try:
        # wait for status info....
        try:
            try:
                env.get_status()
            except CannotConnectEnv:
                print("Status got Unable to connect to env.  Retrying...")
                env.get_status()
            env.wait_for_deploy_started(2)
            env.wait_for_started(3600)
        except subprocess.CalledProcessError as e:
            if getattr(e, 'stderr', None) is not None:
                sys.stderr.write(e.stderr)
            raise
    finally:
        env.juju('status')
        env.destroy_environment()


def main():
    parser = ArgumentParser('Test with quickstart')
    parser.add_argument('env', help='The juju environment to test')
    parser.add_argument('bundle_path', help='URL or path to a bundle')
    parser.add_argument('--debug', type=bool, default=False,
                        help='debug output')
    args = parser.parse_args()
    try:
        run_quickstart(args.env, args.bundle_path, args.debug)
    except Exception as e:
        print('%s: %s' % (type(e), e))
        sys.exit(1)


if __name__ == '__main__':
    main()
