#!/usr/bin/python

from __future__ import print_function

from argparse import ArgumentParser
import sys

RELEASE = 'release'
PROPOSED = 'proposed'
DEVEL = 'devel'
TESTING = 'testing'
PURPOSES = (RELEASE, PROPOSED, DEVEL, TESTING)


def find_tools(file_path):
    return None


def parse_args(args=None):
    """Return the argument parser for this program."""
    parser = ArgumentParser("Compare old and new stream data.")
    parser.add_argument(
        "-d", "--dry-run", action="store_true", default=False,
        help="Do not publish or delete")
    parser.add_argument(
        '-v', '--verbose', action="store_true", default=False,
        help='Increse verbosity.')
    parser.add_argument(
        '-r', '--retracted', help='The release version removed')
    parser.add_argument('purpose', help="<{}>".format(' | '.join(PURPOSES)))
    parser.add_argument('release', help="The release version added")
    parser.add_argument('old_json', help="The old simple streams data file")
    parser.add_argument('new_json', help="The new simple streams data file")
    return parser.parse_args(args)


def main(argv):
    args = parse_args(argv[1:])
    old_tools = find_tools(args.old_data)


if __name__ == '__main__':
    sys.exit(main(sys.argv))
