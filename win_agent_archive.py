#!/usr/bin/python

from __future__ import print_function

from argparse import ArgumentParser
import os
import re
import subprocess
import sys
import traceback


# The set of agents to make.
WIN_AGENT_TEMPLATES = (
    'juju-{}-win2012hvr2-amd64.tgz',
    'juju-{}-win2012hv-amd64.tgz',
    'juju-{}-win2012r2-amd64.tgz',
    'juju-{}-win2012-amd64.tgz',
    'juju-{}-win7-amd64.tgz',
    'juju-{}-win8-amd64.tgz',
    'juju-{}-win81-amd64.tgz',
)
# The versions of agent that may or will exist. The agents will
# always start with juju, the series will start with "win" and the
# arch is always amd64.
AGENT_VERSION_PATTERN = re.compile('juju-(.+)-win[^-]+-amd64.tgz')


def run(*command, **kwargs):
    kwargs['stderr'] = subprocess.STDOUT
    return subprocess.check_output(command, **kwargs)


def get_source_agent_version(source_agent):
    match = AGENT_VERSION_PATTERN.match(source_agent)
    if match:
        return match.group(1)
    return None


def add_agents(args):
    source_path = os.path.abspath(os.path.expanduser(args.source_agent))
    source_agent = os.path.basename(args.source_agent)
    version = get_source_agent_version(source_agent)
    if version is None:
        raise ValueError('%s does not look like a agent.' % source_agent)
    version = get_source_agent_version(source_agent)
    for template in WIN_AGENT_TEMPLATES:
        agent_name = template.format(version)


def get_agents(args):
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
    subparsers = parser.add_subparsers(help='sub-command help')
    add_parser = subparsers.add_parser('add', help='Add win-agents')
    add_parser.add_argument(
        'source_agent',
        help="The win-agent to create all the agents from.")
    add_parser.set_defaults(func=add_agents)
    get_parser = subparsers.add_parser('get', help='get win-agents')
    get_parser.add_argument(
        'version', help="The version of win-agent to download")
    get_parser.set_defaults(func=get_agents)
    return parser.parse_args(args)


def main(argv):
    """Add to get win-agents."""
    args = parse_args(argv)
    try:
        args.func(args)
    except Exception as e:
        print(e)
        if args.verbose:
            traceback.print_tb(sys.exc_info()[2])
        return 2
    if args.verbose:
        print("Created mirror json.")
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv[1:]))
