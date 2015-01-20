#!/usr/bin/env python
from __future__ import print_function

__metaclass__ = type

from argparse import ArgumentParser
import os
import subprocess
import sys

from jujuconfig import (
    get_jenv_path,
    get_juju_home
)
from jujupy import (
    CannotConnectEnv,
    Environment,
    quickstart_from_env,
)
from utility import ensure_deleted


def update_env(env, series=None, agent_url=None, juju_home=None):
    if series is not None:
        env.config['default-series'] = series
    if agent_url is not None:
        env.config['tools-metadata-url'] = agent_url
    if juju_home is not None:
        # https://bugs.launchpad.net/juju-quickstart/+bug/1411846 workaround
        if env.config['type'] == 'azure':
            azure_pem_path = '%s/%s' % (juju_home, 'azure.pem')
            env.config['management-certificate-path'] = azure_pem_path


def run_quickstart(environment, bundle_path, count, series, agent_url, debug):
    """"Deploy a bundle in the specified environment.

    :param environment: The name of the desired environment.
    :param bundle_path: The Path or URL of the bundle to installed.
    :param count: The number of services to wait for before checking status.
    :param agent_url: Path the agent stream.
    :param debug: Boolean for enabling client debug output.
    """
    env = Environment.from_config(environment)
    env.client.debug = debug
    juju_home = get_juju_home()
    update_env(env, series=series, agent_url=agent_url, juju_home=juju_home)
    ensure_deleted(get_jenv_path(juju_home, env.environment))
    env.destroy_environment()
    quickstart_from_env(juju_home, env.client.get_env_client(env), bundle_path)
    try:
        # wait for status info....
        try:
            try:
                env.get_status()
            except CannotConnectEnv:
                print("Status got Unable to connect to env.  Retrying...")
                env.get_status()
            env.wait_for_deploy_started(count)
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
    parser.add_argument('env',
                        help='The juju environment to test')
    parser.add_argument('bundle_path',
                        help='URL or path to a bundle')
    parser.add_argument('--agent-url', default=None,
                        help='URL to use for retrieving agent binaries.')
    parser.add_argument('--service-count', type=int, default=2,
                        help='Minimum number of expected services.')
    parser.add_argument('--debug', action="store_true", default=False,
                        help='debug output')
    parser.add_argument('--new-juju-bin', default=False,
                        help='Dirctory containing the new Juju binary.')
    parser.add_argument('--series',
                        help='Name of the Ubuntu series to use.')
    args = parser.parse_args()
    if args.new_juju_bin:
        juju_path = os.path.abspath(args.new_juju_bin)
        new_path = '%s:%s' % (juju_path, os.environ['PATH'])
        os.environ['PATH'] = new_path
    try:
        run_quickstart(args.env, args.bundle_path, args.service_count,
                       args.series, args.agent_url, args.debug)
    except Exception as e:
        print('%s: %s' % (type(e), e))
        sys.exit(1)


if __name__ == '__main__':
    main()
