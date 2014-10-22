#!/usr/bin/python

from __future__ import print_function

from argparse import ArgumentParser
import json
import re
import sys
import traceback


RELEASE = 'release'
PROPOSED = 'proposed'
DEVEL = 'devel'
TESTING = 'testing'
PURPOSES = (RELEASE, PROPOSED, DEVEL, TESTING)


def find_tools(file_path):
    with open(file_path) as f:
        stream = json.load(f)
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
        return []
    stable_pattern = re.compile(r'\d+\.\d+\.\d+-*')
    devel_versions = [
        v for v in new_tools.keys() if not stable_pattern.match(v)]
    errors = []
    if devel_versions:
        errors.append(
            'Devel versions in {} stream: {}'.format(purpose, devel_versions))
    return errors


def check_expected_changes(new_tools, added=None, removed=None):
    """Return an list of errors if the expected changes are not present.

    :param new_tools: the dict of all the products/versions/*/items
                      in the new json.
    :param added: the version added to the new json, eg '1.20.9'
    :param removed: the version removed from the new json, eg '1.20.8'
    :return: a list of errors, which will be empty when there are none.
    """
    found = []
    seen = False
    for n, t in new_tools.items():
        if removed and t['version'] == removed:
            found.append(n)
        elif added and t['version'] == added:
            seen = True
    errors = []
    if added and not seen:
        errors.append('{} agents were not added'.format(added))
    if found:
        errors.append('{} agents were not removed: {}'.format(removed, found))
    return errors


def check_expected_unchanged(old_tools, new_tools, added=None, removed=None):
    """Return an error tuple if the expected unchanged versions do not match.

    :param old_tools: the dict of all the products/versions/*/items
                      in the old json.
    :param new_tools: the dict of all the products/versions/*/items
                      in the new json.
    :param added: the version added to the new json, eg '1.20.9'
    :param removed: the version removed from the new json, eg '1.20.8'
    :return: a tuple of missing_errors and extra_errors, which might be None
    """
    old_versions = set(k for (k, v) in old_tools.items()
                       if v['version'] != removed)
    new_versions = set(k for (k, v) in new_tools.items()
                       if v['version'] != added)
    missing_errors = old_versions - new_versions
    errors = []
    if missing_errors:
        missing_errors = list(missing_errors)
        errors.append('These agents are missing: {}'.format(missing_errors))
    found_errors = new_versions - old_versions
    if found_errors:
        found_errors = list(found_errors)
        errors.append('These unknown agents were found: {}'.format(
            found_errors))
    return errors


def check_tools_content(old_tools, new_tools):
    """Return the error messages if tools content changes.

    Are the old versions identical to the new versions?
    We care about change values, not new keys in the new tool.
    """
    if not new_tools:
        return None
    errors = []
    for name, old_tool in old_tools.items():
        try:
            new_tool = new_tools[name]
        except KeyError:
            # This is a missing version case reported by check_expected_tools.
            continue
        for old_key, old_val in old_tool.items():
            new_val = new_tool[old_key]
            if old_val != new_val:
                errors.append(
                    'Tool {} {} changed from {} to {}'.format(
                        name, old_key, old_val, new_val))
    return errors


def compare_tools(old_tools, new_tools, purpose, added=None, removed=None):
    """Return a tuple of an exit code and an explanation.

    An exit code of 1 will have a list of strings explaining the problems.
    An exit code of 0 is a pass and the exlanation is None.
    """
    errors = []
    errors.extend(
        check_devel_not_stable(old_tools, new_tools, purpose))
    errors.extend(
        check_expected_changes(new_tools, added, removed))
    errors.extend(
        check_expected_unchanged(old_tools, new_tools, added, removed))
    errors.extend(
        check_tools_content(old_tools, new_tools))
    return errors or None


def parse_args(args=None):
    """Return the argument parser for this program."""
    parser = ArgumentParser("Compare old and new stream data.")
    parser.add_argument(
        '-v', '--verbose', action="store_true", default=False,
        help='Increse verbosity.')
    parser.add_argument(
        '-r', '--removed', default=None, help='The release version removed')
    parser.add_argument(
        '-a', '--added', default=None, help="The release version added")
    parser.add_argument('purpose', help="<{}>".format(' | '.join(PURPOSES)))
    parser.add_argument('old_json', help="The old simple streams data file")
    parser.add_argument('new_json', help="The new simple streams data file")
    return parser.parse_args(args)


def main(argv):
    args = parse_args(argv[1:])
    try:
        old_tools = find_tools(args.old_json)
        new_tools = find_tools(args.new_json)
        messages = compare_tools(
            old_tools, new_tools, args.purpose, args.version,
            retracted=args.retracted)
        if messages:
            print('\n'.join(messages))
            return 1
    except Exception as e:
        print(e)
        if args.verbose:
            traceback.print_tb(sys.exc_info()[2])
        return 2
    if args.verbose:
        print("All changes are correct.")
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv))
