#!/usr/bin/env python
__metaclass__ = type


from argparse import ArgumentParser
import sys

from jujupy import (
    Environment,
    until_timeout,
)


def deploy_stack(environment):
    """"Deploy a test stack in the specified environment.

    :param environment: The name of the desired environment.
    """
    env = Environment.from_config(environment)
    # Clean up any leftover junk
    env.destroy_environment()
    env.bootstrap()
    try:
        # wait for status info....
        try:
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
    try:
        deploy_stack(args.env)
    except Exception as e:
        print e
        sys.exit(1)


if __name__ == '__main__':
    main()
