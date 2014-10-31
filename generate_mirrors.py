#!/usr/bin/python

from __future__ import print_function

from argparse import ArgumentParser
import datetime
import json
import sys
import traceback


RELEASE = 'release'
PROPOSED = 'proposed'
DEVEL = 'devel'
TESTING = 'testing'
PURPOSES = (RELEASE, PROPOSED, DEVEL, TESTING)

MIRRORS_JSON = 'mirrors.json'


def generate_mirrors_file(updated, streams_path,
                          verbose=False, dry_run=False):
    if verbose:
        print('Creating %s' % MIRRORS_JSON)
    mirrors = {
        "mirrors": {}
    }
    if not dry_run:
        data = json.dumps(mirrors)
        file_path = '%s/%s' % (streams_path, MIRRORS_JSON)
        with open(file_path, 'w') as mirror_file:
            mirror_file.write(data)


def generate_cpc_mirrors_file(updated, streams_path,
                              verbose=False, dry_run=False):
    pass


def parse_args(args=None):
    """Return the argument parser for this program."""
    parser = ArgumentParser("Compare old and new stream data.")
    parser.add_argument(
        '-d', '--dry-run', action="store_true", default=False,
        help='Do not overwrite existing data.')
    parser.add_argument(
        '-v', '--verbose', action="store_true", default=False,
        help='Increse verbosity.')
    parser.add_argument(
        'streams_path', help="The dirextory to create the files in.")
    return parser.parse_args(args)


def main(argv):
    """Verify that the new json has all the expected changes.

    An exit code of 1 will have a list of strings explaining the problems.
    An exit code of 0 is a pass and the explanation is None.
    """
    args = parse_args(argv[1:])
    try:
        updated = datetime.datetime.utcnow()
        generate_cpc_mirrors_file(
            updated, args.streams_path, args.verbose, args.dry_run)
        generate_mirrors_file(
            updated, args.streams_path, args.verbose, args.dry_run)
    except Exception as e:
        print(e)
        if args.verbose:
            traceback.print_tb(sys.exc_info()[2])
        return 2
    if args.verbose:
        print("Created mirror json.")
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv))
