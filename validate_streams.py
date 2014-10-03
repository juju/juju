#!/usr/bin/python

from __future__ import print_function

from argparse import ArgumentParser
import json
import sys

RELEASE = 'release'
PROPOSED = 'proposed'
DEVEL = 'devel'
TESTING = 'testing'
PURPOSES = (RELEASE, PROPOSED, DEVEL, TESTING)
IGNORE = 'IGNORE'


def find_tools(file_path):
    with open(file_path) as f:
        raw = f.read()
    stream = json.loads(raw)
    tools = {}
    for name, product in stream['products'].items():
        versions = product['versions']
        for version in versions.values():
            if isinstance(version, dict):
                items = version['items']
                tools.update(items)
    return tools


def compare_tools(old_tools, new_tools, purpose, version, retracted=None):
    # Remove the expected difference between the two collections of tools.
    expected = {}
    if retracted:
        for n, t in old_tools.items():
            if t['version'] == retracted:
                expected.update(n, t)
                del old_tools[t]
    else:
        for n, t in new_tools.items():
            if t['version'] == retracted:
                expected.update(n, t)
                del new_tools[t]
    unexpected = [(x, y) for x, y in zip(old_tools, new_tools) if x != y]
    if unexpected:
        return 1, unexpected
    else:
        return 0, None


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
        '-r', '--retracted', default=None,
        help='The release version removed')
    parser.add_argument('purpose', help="<{}>".format(' | '.join(PURPOSES)))
    parser.add_argument('version', help="The version added")
    parser.add_argument('old_json', help="The old simple streams data file")
    parser.add_argument('new_json', help="The new simple streams data file")
    return parser.parse_args(args)


def main(argv):
    args = parse_args(argv[1:])
    try:
        old_tools = find_tools(args.old_data)
        new_tools = find_tools(args.new_data)
        compare_tools(
            old_tools, new_tools, args.purpose, args.version,
            retracted=args.retracted)
    except Exception as e:
        print(e)
        if args.verbose:
            print(sys.exc_info()[0])
        return 2
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv))
