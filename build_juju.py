#!/usr/bin/python
"""Build and package juju for an non-debian OS."""

from __future__ import print_function

from argparse import ArgumentParser
import os
import sys
import traceback

from candidate import run_command
from jujuci import (
    add_artifacts,
    add_credential_args,
    get_artifacts,
    get_credentials,
    BUILD_REVISION,
    setup_workspace,
)


DEFAULT_JUJU_RELEASE_TOOLS = os.path.realpath(
    os.path.join(__file__, '..', '..', 'juju-release-tools'))


ARTIFACT_GLOBS = [
    'juju-setup-*.exe', 'juju-*-win2012-amd64.tgz', 'juju-*-osx.tar.gz',
    'juju-*-centos7-amd64.tgz', 'juju-*-centos7.tar.gz',
    'juju-*-ubuntu-*.tgz',
    ]


def get_crossbuild_script(juju_release_tools=None):
    """Return the full path to the crossbuild script.

    The juju-release-tools dir is assumed to be a sibling of the juju-ci-tools
    directory.
    """
    if not juju_release_tools:
        juju_release_tools = DEFAULT_JUJU_RELEASE_TOOLS
    script = os.path.join(juju_release_tools, 'crossbuild.py')
    return script


def build_juju(credentials, product, workspace_dir, build,
               juju_release_tools=None, dry_run=False, verbose=False):
    """Build the juju product from a Juju CI build-revision in a workspace.

    The product is passed to juju-release-tools/crossbuild.py. The options
    include osx-client, win-client, win-agent.

    The tarfile from the build-revision build number is downloaded and passed
    to crossbuild.py.
    """
    setup_workspace(workspace_dir, dry_run=dry_run, verbose=verbose)
    artifacts = get_artifacts(
        credentials, BUILD_REVISION, build, 'juju-core_*.tar.gz',
        workspace_dir, archive=False, dry_run=dry_run, verbose=verbose)
    tar_artifact = artifacts[0]
    crossbuild = get_crossbuild_script(juju_release_tools)
    command = [
        crossbuild, product, '-b', '~/crossbuild', tar_artifact.file_name]
    run_command(command, dry_run=dry_run, verbose=verbose)
    add_artifacts(workspace_dir, ARTIFACT_GLOBS, dry_run=dry_run,
                  verbose=verbose)


def parse_args(args=None):
    """Return the argument parser for this program."""
    parser = ArgumentParser("Build and package juju for an non-debian OS.")
    parser.add_argument(
        '-d', '--dry-run', action='store_true', default=False,
        help='Do not make changes.')
    parser.add_argument(
        '-v', '--verbose', action='store_true', default=False,
        help='Increase verbosity.')
    parser.add_argument(
        '-b', '--build', default='lastSuccessfulBuild',
        help="The specific revision-build number to get the tarball from")
    parser.add_argument(
        '-t', '--juju-release-tools',
        help='The path to the juju-release-tools dir, default: %s' %
              DEFAULT_JUJU_RELEASE_TOOLS)
    parser.add_argument(
        'product', choices=['win-client', 'win-agent', 'osx-client', 'centos',
                            'ubuntu-agent'],
        help='the kind of juju to make and package.')
    parser.add_argument(
        'workspace',  help='The path to the workspace to build in.')
    add_credential_args(parser)
    parsed = parser.parse_args(args)
    return parsed, get_credentials(parsed)


def main(argv):
    """Build and package juju for an non-debian OS."""
    args, credentials = parse_args(argv)
    try:
        build_juju(
            credentials, args.product, args.workspace, args.build,
            juju_release_tools=args.juju_release_tools,
            dry_run=args.dry_run, verbose=args.verbose)
    except Exception as e:
        print(e)
        print(getattr(e, 'output', ''))
        if args.verbose:
            traceback.print_tb(sys.exc_info()[2])
        return 2
    if args.verbose:
        print("Done.")
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv[1:]))
