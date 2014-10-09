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


def check_devel_not_stable(old_tools, new_tools, purpose):
    """Return an error message if the version can be included in the stream.

    Devel versions cannot be proposed or release because the letters in
    the version break older jujus.
    """
    if purpose in (TESTING, DEVEL):
        return None
    stable_pattern = re.compile(r'\d+\.\d+\.\d+-*')
    devel_versions = [
        v for v in new_tools.keys() if not stable_pattern.match(v)]
    if devel_versions:
        return 'Devel versions in {} stream: {}'.format(
            purpose, devel_versions)


def check_expected_tools(old_tools, new_tools, version, retracted=None):
    """Return a 4-tuple of new_expected, new_errors, old_expected, old_errors

    The new and old expected dicts are the tools common to old and new streams.
    The new and old errors are strings of missing ot extra versions.
    """
    # Remove the expected difference between the two collections of tools.
    old_expected = dict(old_tools)
    new_expected = dict(new_tools)
    missing_errors = None
    extra_errors = None
    expected = {}
    if retracted:
        # Retracted domiates version because streams.canonical.com always
        # needs a version to install to make streams, even when it
        # intends to remove something.
        for n, t in old_expected.items():
            if t['version'] == retracted:
                expected.update([(n, t)])
                del old_expected[n]
    else:
        for n, t in new_expected.items():
            if t['version'] == version:
                expected.update([(n, t)])
                del new_expected[n]
        if version != 'IGNORE' and not expected:
            missing_errors = 'Missing versions: {}'.format(version)
    # The old and new should be identical. but if there is a problem,
    # we want to explain what problems are in each set of versions.
    old_versions = set(old_expected.keys())
    new_versions = set(new_expected.keys())
    missing = list(old_versions - new_versions)
    if missing:
        missing_errors = 'Missing versions: {}'.format(missing)
    extras = list(new_versions - old_versions)
    if extras:
        extra_errors = 'Extra versions: {}'.format(extras)
    return new_expected, extra_errors, old_expected, missing_errors


def check_tools_content(old_tools, new_tools):
    """Return the error messages if tools content changes.

    Are the old versions identical to the new versions?
    We care about change values, not new keys in the new tool.
    """
    if not new_tools:
        return None
    for name, old_tool in old_tools.items():
        new_tool = new_tools[name]
        for old_key, old_val in old_tool.items():
            new_val = new_tool[old_key]
            if old_val != new_val:
                return 'Tool {} {} changed from {} to {}'.format(
                    name, old_key, old_val, new_val)


def compare_tools(old_tools, new_tools, purpose, version, retracted=None):
    """Return a tuple of an exit code and an explanation.

    An exit code of 1 will have a list of strings explaining the problems.
    An exit code of 0 is a pass and the exlanation is None.
    """
    errors = []
    devel_versions = check_devel_not_stable(old_tools, new_tools, purpose)
    if devel_versions:
        errors.append(devel_versions)
    tools = check_expected_tools(old_tools, new_tools, version, retracted)
    new_expected, extra_errors, old_expected, missing_errors = tools
    if missing_errors:
        errors.append(missing_errors)
    if extra_errors:
        errors.append(extra_errors)
    content_changes = check_tools_content(old_expected, new_expected)
    if content_changes:
        errors.append(content_changes)
    return errors or None


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
        messages = compare_tools(
            old_tools, new_tools, args.purpose, args.version,
            retracted=args.retracted)
        if messages:
            print('\n'.join(messages))
            return 1
    except Exception as e:
        print(e)
        if args.verbose:
            print(sys.exc_info()[0])
        return 2
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv))
