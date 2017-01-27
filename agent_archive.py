#!/usr/bin/python

from __future__ import print_function

from argparse import ArgumentParser
import hashlib
import os
import re
import subprocess
import sys
import traceback


# The S3 container and path to add to and get from.
S3_CONTAINER_DEFAULT = 's3://juju-qa-data/agent-archive'
# The set of agents to make.
AGENT_TEMPLATES = (
    'juju-{}-centos7-amd64.tgz',
    'juju-{}-win2012hvr2-amd64.tgz',
    'juju-{}-win2012hv-amd64.tgz',
    'juju-{}-win2012r2-amd64.tgz',
    'juju-{}-win2012-amd64.tgz',
    'juju-{}-win2016-amd64.tgz',
    'juju-{}-win2016nano-amd64.tgz',
    'juju-{}-win7-amd64.tgz',
    'juju-{}-win8-amd64.tgz',
    'juju-{}-win81-amd64.tgz',
    'juju-{}-win10-amd64.tgz',
    'juju-{}-ubuntu-amd64.tgz',
    'juju-{}-ubuntu-arm64.tgz',
    'juju-{}-ubuntu-ppc64el.tgz',
    'juju-{}-ubuntu-s390x.tgz',
    )


AGENT_PATTERN = re.compile(
    'juju-(.+)-(win|centos|ubuntu)[^-]*-(amd64|arm64|ppc64el|s390x)\.tgz')


def run(args, config=None, verbose=False, dry_run=False):
    """Run s3cmd with sensible options.

    s3cmd is guaranteed to be on every machine that juju-release-tools runs on.
    """
    command = ['s3cmd', '--no-progress']
    if dry_run:
        command.append('--dry-run')
    if config:
        command.extend(['-c', config])
    command.extend(args)
    if verbose:
        print(command)
    return subprocess.check_output(command, stderr=subprocess.STDOUT)


def get_source_agent_version(source_agent):
    """Parse the version from the source agent's file name."""
    match = AGENT_PATTERN.match(source_agent)
    if match:
        return match.group(1)
    return None


def get_source_agent_os(source_agent):
    match = AGENT_PATTERN.match(source_agent)
    if match:
        return match.group(2)
    raise ValueError('The unknown OS version: %s' % source_agent)


def get_source_agent_arch(source_agent):
    match = AGENT_PATTERN.match(source_agent)
    if match:
        return match.group(3)
    raise ValueError('Invalid arch in agent: ' + source_agent)


def get_input(prompt):
    """Return the user input from a prompted question.

    Wrap deprecated raw_input for testing.
    """
    return raw_input(prompt)  # NOQA


def listing_to_files(listing):
    """Convert an S3 ls output to a list of remote files."""
    agents = []
    for line in listing.splitlines():
        parts = line.split()
        agents.append(parts[-1])
    return agents


def is_new_version(source_path, config, s3_container, verbose=False):
    """Return True when the version is new, else False.

    :raises: ValueError if the version exists and is different.
    :return: True when the version is new, else False.
    """
    source_agent = os.path.basename(source_path)
    agent_path = '%s/%s' % (s3_container, source_agent)
    existing_version = run(
        ['ls', '--list-md5', agent_path], config=config, verbose=verbose)
    md5 = hashlib.md5()
    with open(source_path, mode='rb') as local_file:
        md5.update(local_file.read())
    local_hash = str(md5.hexdigest())
    if verbose:
        print('Checking that %s and hash %s does not already exist.' % (source_agent, local_hash))
    if not existing_version:
        if verbose:
            print('No existing version found compared to %s and hash %s' % (source_path, local_hash))
        return True
    remote_hash = existing_version.strip().split()[3]
    if remote_hash != local_hash:
        raise ValueError(
            '%s already exists. Cannot overwrite with %s and hash %s.' %
            (existing_version, source_path, local_hash))
    if verbose:
        print('This exact agent is archived. No need to upload.')
    return False


def add_agents(args):
    """Upload agents to the S3 agent-archive location.

    It is an error to overwrite an existing agent, or pass an agent that
    does not appear to be a win agent.

    As all win agents are functionally the same, only one agent is
    uploaded, and the other agents are created as copies with s3.
    """
    source_agent = os.path.basename(args.source_agent)
    version = get_source_agent_version(source_agent)
    if version is None:
        raise ValueError('%s does not look like a agent.' % source_agent)
    agent_versions = [t.format(version) for t in AGENT_TEMPLATES]
    if source_agent not in agent_versions:
        raise ValueError(
            '%s does not match an expected version.' % source_agent)
    source_path = os.path.abspath(os.path.expanduser(args.source_agent))
    if not is_new_version(source_path, args.config, args.s3_container,
                          verbose=args.verbose):
        if args.verbose:
            print("Nothing to do.")
        return
    # The fastest way to put the files in place is to upload the source_agent
    # then use the s3cmd cp to make remote versions.
    if args.verbose:
        print('Uploading %s to %s' % (source_agent, args.s3_container))
    remote_source = '%s/%s' % (args.s3_container, source_agent)
    run(['put', source_path, remote_source],
        config=args.config, dry_run=args.dry_run, verbose=args.verbose)


def get_agents(args):
    """Download agents matching a version to a destination path."""
    version = args.version
    agent_glob = '%s/juju-%s*' % (args.s3_container, version)
    destination = os.path.abspath(os.path.expanduser(args.destination))
    output = run(
        ['get', agent_glob, destination],
        config=args.config, dry_run=args.dry_run, verbose=args.verbose)
    if args.verbose:
        print(output)


def delete_agents(args):
    """Delete agents that match a version.

    Agents will only be deleted after a prompt to agree that the listing
    matches the expected operation.
    """
    version = args.version
    agent_glob = '%s/juju-%s*' % (args.s3_container, version)
    existing_versions = run(
        ['ls','--list-md5', agent_glob], config=args.config, verbose=args.verbose)
    if args.verbose:
        print('Checking for matching agents.')
    if version not in existing_versions:
        raise ValueError('No %s agents found.' % version)
    print(existing_versions)
    answer = get_input('Delete these versions? [y/N]')
    if answer not in ('Y', 'y', 'yes'):
        return
    agents = listing_to_files(existing_versions)
    for agent in agents:
        deleted = run(
            ['del', agent], config=args.config, dry_run=args.dry_run,
            verbose=args.verbose)
        if args.verbose:
            print(deleted)


def parse_args(args=None):
    """Return the argument parser for this program."""
    parser = ArgumentParser("Manage released agents in the agent-archive.")
    parser.add_argument(
        '-d', '--dry-run', action="store_true", default=False,
        help='Do not make changes.')
    parser.add_argument(
        '-v', '--verbose', action="store_true", default=False,
        help='Increase verbosity.')
    parser.add_argument(
        '-c', '--config', action='store', default=None,
        help='The S3 config file.')
    parser.add_argument(
        '-s', '--s3-container', action='store', default=S3_CONTAINER_DEFAULT,
        help='The S3 container to act on.')
    subparsers = parser.add_subparsers(help='sub-command help')
    # add juju-1.21.0-win2012-amd64.tgz
    parser_add = subparsers.add_parser(
        'add', help='Add an agent to the archive')
    parser_add.add_argument(
        'source_agent',
        help="The agent to add (and copy to other series).")
    parser_add.set_defaults(func=add_agents)
    # get 1.21.0 ./workspace
    parser_get = subparsers.add_parser(
        'get', help='get agents from the archive')
    parser_get.add_argument(
        'version', help="The version of agents to download")
    parser_get.add_argument(
        'destination', help="The path to download the files to.")
    parser_get.set_defaults(func=get_agents)
    # delete 1.21.0
    parser_delete = subparsers.add_parser(
        'delete', help='delete agent-archive')
    parser_delete.add_argument(
        'version', help="The version of agents to delete")
    parser_delete.set_defaults(func=delete_agents)
    return parser.parse_args(args)


def main(argv):
    """Manage agent-archive in the archive."""
    args = parse_args(argv)
    try:
        args.func(args)
    except Exception as e:
        print(e)
        if args.verbose:
            traceback.print_tb(sys.exc_info()[2])
        return 2
    if args.verbose:
        print("Done.")
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv[1:]))
