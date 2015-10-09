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


def find_agents(file_path):
    with open(file_path) as f:
        stream = json.load(f)
    agents = {}
    for name, product in stream['products'].items():
        versions = product['versions']
        for version in versions.values():
            if isinstance(version, dict):
                items = version['items']
                agents.update(items)
    return agents


def check_devel_not_stable(old_agents, new_agents, purpose):
    """Return a list of errors if the version can be included in the stream.

    Devel versions cannot be proposed or release because the letters in
    the version break older jujus.

    :param old_agents: the dict of all the products/versions/*/items
                       in the old json.
    :param new_agents: the dict of all the products/versions/*/items
                       in the new json.
    :param purpose: either release, proposed, devel, or testing.
    :return: a list of errors, which will be empty when there are none.
    """
    if purpose in (TESTING, DEVEL):
        return []
    stable_pattern = re.compile(r'\d+\.\d+\.\d+-*')
    devel_versions = [
        v for v in new_agents.keys() if not stable_pattern.match(v)]
    errors = []
    if devel_versions:
        errors.append(
            'Devel versions in {} stream: {}'.format(purpose, devel_versions))
    return errors


def check_expected_changes(new_agents, added=None, removed=None):
    """Return an list of errors if the expected changes are not present.

    :param new_agents: the dict of all the products/versions/*/items
                       in the new json.
    :param added: the version added to the new json, eg '1.20.9'.
    :param removed: the version removed from the new json, eg '1.20.8'.
    :return: a list of errors, which will be empty when there are none.
    """
    found = []
    seen = False
    for n, t in new_agents.items():
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


def reconcile_aliases(found_errors, new_agents):
    """Remove aliases from the found_errors.

    Juju agents can be reused for equivalent archs and series. Juju will
    add aliases for a series-arch combination when it finds a compatible
    agent already registered. An alias's path and sha256sum will match
    the compatible agent.

    The ppc64 arch is a synonym for ppc64el. Juju will register a ppc64el
    agent when it doesn't find a specific ppc64 agent.

    Juju may add other alias conditions in the future:
    * There is really one win agent that is copied to each win series,
      the copy isn't needed if the single agent is aliased.
    * All amd64, 386, and armhf agents work for all Ubuntu series. Only
      one is actually needed.
    """
    for found_name in list(found_errors):
        if found_name.endswith('ppc64'):
            # ppc64 can be an alias of a ppc64el agent.
            real_name = '{}el'.format(found_name)
            if real_name in new_agents:
                real_agent = new_agents[real_name]
                found_agent = new_agents[found_name]
                if (found_agent['path'] == real_agent['path'] and
                        found_agent['sha256'] == real_agent['sha256']):
                    found_errors.remove(found_name)


def check_expected_unchanged(old_agents, new_agents,
                             added=None, removed=None, ignored=None):
    """Return a list of errors if the expected unchanged versions do not match.

    :param old_agents: the dict of all the products/versions/*/items
                       in the old json.
    :param new_agents: the dict of all the products/versions/*/items
                       in the new json.
    :param added: the version added to the new json, eg '1.20.9'.
    :param removed: the version removed from the new json, eg '1.20.8'.
    :param ignored: The version that may be added, but can be ignored.
        There are cases where extra stable versions are included when
        adding to a devel stream.
    :return: a list of errors, which will be empty when there are none.
    """
    old_versions = set(k for (k, v) in old_agents.items()
                       if v['version'] != removed)
    new_versions = set(k for (k, v) in new_agents.items()
                       if v['version'] != added)
    missing_errors = old_versions - new_versions
    errors = []
    if missing_errors:
        missing_errors = list(missing_errors)
        errors.append('These agents are missing: {}'.format(missing_errors))
    found_errors = new_versions - old_versions
    reconcile_aliases(found_errors, new_agents)
    if ignored:
        for found_name in list(found_errors):
            found_agent = new_agents[found_name]
            if found_agent['version'].startswith(ignored):
                found_errors.remove(found_name)
    if found_errors:
        found_errors = list(found_errors)
        errors.append('These unknown agents were found: {}'.format(
            found_errors))
    return errors


def check_agents_content(old_agents, new_agents):
    """Return a list of error messages if agents content changes.

    Are the old versions identical to the new versions?
    We care about change values, not new keys in the new tool.

    :param old_agents: the dict of all the products/versions/*/items
                       in the old json.
    :param new_agents: the dict of all the products/versions/*/items
                       in the new json.
    :return: a list of errors, which will be empty when there are none.
    """
    if not new_agents:
        return None
    errors = []
    for name, old_tool in old_agents.items():
        try:
            new_tool = new_agents[name]
        except KeyError:
            # This is a missing version case reported by check_expected_agents.
            continue
        for old_key, old_val in old_tool.items():
            new_val = new_tool[old_key]
            if old_val != new_val:
                errors.append(
                    'Tool {} {} changed from {} to {}'.format(
                        name, old_key, old_val, new_val))
    return errors


def compare_agents(old_agents, new_agents, purpose,
                   added=None, removed=None, ignored=None):
    """Return a list of error messages from all the validation checks.

    :param old_agents: the dict of all the products/versions/*/items
                       in the old json.
    :param new_agents: the dict of all the products/versions/*/items
                       in the new json.
    :return: a list of errors, which will be empty when there are none.
    """
    errors = []
    errors.extend(
        check_devel_not_stable(old_agents, new_agents, purpose))
    errors.extend(
        check_expected_changes(new_agents, added, removed))
    errors.extend(
        check_expected_unchanged(
            old_agents, new_agents, added, removed, ignored))
    errors.extend(
        check_agents_content(old_agents, new_agents))
    return errors or None


def parse_args(args=None):
    """Return the argument parser for this program."""
    parser = ArgumentParser("Compare old and new stream data.")
    parser.add_argument(
        '-v', '--verbose', action="store_true", default=False,
        help='Increase verbosity.')
    parser.add_argument(
        '-r', '--removed', default=None, help='The release version removed')
    parser.add_argument(
        '-a', '--added', default=None, help="The release version added")
    parser.add_argument(
        '-i', '--ignored', default=None,
        help="Ignore a version that might be added")
    parser.add_argument('purpose', help="<{}>".format(' | '.join(PURPOSES)))
    parser.add_argument('old_json', help="The old simple streams data file")
    parser.add_argument('new_json', help="The new simple streams data file")
    return parser.parse_args(args)


def main(argv):
    """Verify that the new json has all the expected changes.

    An exit code of 1 will have a list of strings explaining the problems.
    An exit code of 0 is a pass and the explanation is None.
    """
    args = parse_args(argv[1:])
    try:
        old_agents = find_agents(args.old_json)
        new_agents = find_agents(args.new_json)
        errors = compare_agents(
            old_agents, new_agents, args.purpose, args.added,
            args.removed, args.ignored)
        if errors:
            print('\n'.join(errors))
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
