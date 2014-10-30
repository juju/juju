#!/usr/bin/env python
from argparse import ArgumentParser
import json

from jujupy import EnvJujuClient


def parse_args(args=None):
    """Parse commandline arguments into a Namespace."""
    parser = ArgumentParser()
    parser.add_argument('buildvars')
    parser.add_argument('output')
    return parser.parse_args(args)


def make_metadata(buildvars_path):
    """Return metadata about the clients as json-compatible objects.

    :param buildbars_path: Path to the buildvars.json file for the new client.
    """
    old_version = EnvJujuClient.get_version()
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
        }
    return metadata


def main(argv=None):
    """Generate the date and write it to disk."""
    args = parse_args(argv)
    metadata = make_metadata(args.buildvars)
    with open(args.output, 'w') as output_file:
        json.dump(metadata, output_file, indent=2, sort_keys=True)
        output_file.write('\n')


if __name__ == '__main__':
    main()
