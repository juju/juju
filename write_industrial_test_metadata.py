#!/usr/bin/env python
from argparse import ArgumentParser
import json

from jujupy.client import (
    describe_substrate,
    EnvJujuClient,
    SimpleEnvironment,
    )


def parse_args(args=None):
    """Parse commandline arguments into a Namespace."""
    parser = ArgumentParser()
    parser.add_argument('buildvars',
                        help="Path to the new client's buildvars.json.")
    parser.add_argument('env', help='The name of the environment.')
    parser.add_argument('output', help='Path of the file to write.')
    return parser.parse_args(args)


def make_metadata(buildvars_path, env_name):
    """Return metadata about the clients as json-compatible objects.

    :param buildbars_path: Path to the buildvars.json file for the new client.
    :param env_name: Name of the environment being used.
    """
    old_version = EnvJujuClient.get_version()
    env = SimpleEnvironment.from_config(env_name)
    with open(buildvars_path) as buildvars_file:
        buildvars = json.load(buildvars_file)
    metadata = {
        'old_client': {
            'type': 'release',
            'version': old_version
            },
        'new_client': {
            'type': 'build',
            'buildvars': buildvars,
            },
        'environment': {
            'name': env_name,
            'substrate': describe_substrate(env)
            },
        }
    return metadata


def main(argv=None):
    """Generate the date and write it to disk."""
    args = parse_args(argv)
    metadata = make_metadata(args.buildvars, args.env)
    with open(args.output, 'w') as output_file:
        json.dump(metadata, output_file, indent=2, sort_keys=True)
        output_file.write('\n')


if __name__ == '__main__':
    main()
