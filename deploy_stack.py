#!/usr/bin/env python
from __future__ import print_function
__metaclass__ = type


from argparse import ArgumentParser
import sys

from jujupy import (
    check_wordpress,
    Environment,
    until_timeout,
)


def deploy_stack(environment, charm_prefix, already_bootstrapped):
    """"Deploy a Wordpress stack in the specified environment.

    :param environment: The name of the desired environment.
    """
    if sys.platform == 'win32':
        # Ensure OpenSSH is never in the path for win tests.
        sys.path = [p for p in sys.path if 'OpenSSH' not in p]
    env = Environment.from_config(environment)
    if not already_bootstrapped:
        env.bootstrap()
    agent_version = env.get_matching_agent_version()
    status = env.get_status()
    for ignored in until_timeout(30):
        agent_versions = env.get_status().get_agent_versions()
        if 'unknown' not in agent_versions and len(agent_versions) == 1:
            break
        status = env.get_status()
    if agent_versions.keys() != [agent_version]:
        print("Current versions: %s" % ', '.join(agent_versions.keys()))
        env.juju('upgrade-juju', '--version', agent_version)
    if sys.platform == 'win32':
        # The win client tests only verify the client to the state-server.
        return
    env.wait_for_version(env.get_matching_agent_version())
    env.juju('deploy', charm_prefix + 'wordpress')
    env.juju('deploy', charm_prefix + 'mysql')
    env.juju('add-relation', 'mysql', 'wordpress')
    env.juju('expose', 'wordpress')
    status = env.wait_for_started().status
    wp_unit_0 = status['services']['wordpress']['units']['wordpress/0']
    check_wordpress(wp_unit_0['public-address'])


def main():
    parser = ArgumentParser('Deploy a WordPress stack')
    parser.add_argument('--charm-prefix', help='A prefix for charm urls.',
                        default='')
    parser.add_argument('--already-bootstrapped',
                        help='The environment is already bootstrapped.',
                        action='store_true')
    parser.add_argument('env', help='The environment to deploy on.')
    args = parser.parse_args()
    try:
        deploy_stack(args.env, args.charm_prefix, args.already_bootstrapped)
    except Exception as e:
        print(e)
        sys.exit(1)


if __name__ == '__main__':
    main()
