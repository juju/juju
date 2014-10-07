#!/usr/bin/python

from __future__ import print_function

from argparse import ArgumentParser
import json
import re
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
    # devel versions cannot be proposed or release because the letters in
    # the version break older jujus
    old_tools = dict(old_tools)
    new_tools = dict(new_tools)
    errors = []
    devel_versions = []
    if purpose in (PROPOSED, RELEASE):
        stable_pattern = re.compile(r'\d+\.\d+\.\d+-*')
        devel_versions = [
            v for v in new_tools.keys() if not stable_pattern.match(v)]
        if devel_versions:
            errors.append(
                'Devel versions in {} stream: {}'.format(
                    purpose, devel_versions))
    # Remove the expected difference between the two collections of tools.
    expected = {}
    if retracted:
        # Retracted domiates version because streams.canonical.com always
        # needs a version to get and use to make streams, even when it
        # intends to remove something.
        for n, t in old_tools.items():
            if t['version'] == retracted:
                expected.update([(n, t)])
                del old_tools[n]
    else:
        for n, t in new_tools.items():
            if t['version'] == version:
                expected.update([(n, t)])
                del new_tools[n]
    # The old and new should be identical. but if there is a problem,
    # we want to explain what problems are in each set of versions.
    old_versions = set(old_tools.keys())
    new_versions = set(new_tools.keys())
    old_extras = list(old_versions - new_versions)
    if old_extras:
        errors.append('Missing versions: {}'.format(old_extras))
    new_extras = list(new_versions - old_versions)
    if new_extras:
        errors.append('Extra versions: {}'.format(new_extras))
    # The version are what we expect, but are they identical?
    # We care are change values, not new keys in the new tool.
    changed = []
    if new_tools:
        for name, old_tool in old_tools.items():
            new_tool = new_tools[name]
            for old_key, old_val in old_tool.items():
                new_val = new_tool[old_key]
                if old_val != new_val:
                    changed.append((name, old_key, old_val, new_val))
    errors = errors + changed
    if errors:
        return 1, errors
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
