#!/usr/bin/env python
from __future__ import print_function

from argparse import ArgumentParser
import os
import sys
from textwrap import dedent

import yaml

from jujupy import (
    fake_juju_client,
    JujuData,
    )
from jujupy.configuration import get_environments


def write_new_config(env, out):
    client = fake_juju_client(env=env)
    out.write('# cloud/region: {}\n'.format(client.get_cloud_region(
        client.env.get_cloud(), client.env.get_region())))
    config = client.make_model_config()
    agent_version = env.get_option('agent-version')
    if agent_version is not None:
        config['agent-version'] = agent_version
    else:
        config.pop('agent-version', None)
    yaml.dump(config, out, default_flow_style=False)


def main():
    parser = ArgumentParser(
        description=dedent('''\
            Convert environments.yaml to 2.0 format.

            environments.yaml from JUJU_HOME will be used.
            Existing configs in the output directory will be overwritten.

            Does not support configs of type 'local'.
            '''))
    parser.add_argument('config_dir', metavar='OUTPUT_DIR',
                        help='Directory to write updated configs to.')
    args = parser.parse_args()
    clouds_credentials = JujuData('', {})
    clouds_credentials.load_yaml()
    for environment, config in get_environments().items():
        if config['type'] == 'local':
            continue
        env = JujuData(environment, config)
        env.clouds = clouds_credentials.clouds
        env.credentials = clouds_credentials.credentials
        print(environment)
        sys.stdout.flush()
        out_path = os.path.join(args.config_dir,
                                '{}.yaml'.format(environment))
        with open(out_path, 'w') as out_file:
            write_new_config(env, out_file)


if __name__ == '__main__':
    main()
