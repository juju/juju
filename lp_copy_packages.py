#!/usr/bin/python

from __future__ import print_function

from argparse import ArgumentParser
import os
import sys

from launchpadlib.launchpad import Launchpad

# Juju devel versions
DEVEL = 'devel'
# Juju supported versions
PROPOSED = 'proposed'
STABLE = 'stable'
# Ubuntu supported versions
PROPOSED_1_22 = '1.22-proposed'
SUPPORTED_1_22 = '1.22'


def get_archives(lp, to_archive_name):
    """Return the archives used in the copy.

    The build archives are private and owned by a different team than the
    public archives. The policy for building and copying is:
    1. Always build in private PPAs because the client cannot be made public
       before the tools made from the client package.
    2. Devel packages are built in a separate PPA from stable packages
       because devel can change the archive deps.
    3. Built devel packages can be copied to the public devel PPA.
    4. Built stable packages can be copied to the public proposed PPA.
    5. After evaluation, the stable packages in public proposed can be copied
       to the public stable PPA.
    The Supported Ubuntu versions have a similar path for the version:
    juju-packaging/<version> -> juju/<version>-proposed -> juju/<version>
    """
    to_team_name = 'juju'
    if to_archive_name == DEVEL:
        from_archive_name = DEVEL
        from_team_name = 'juju-packaging'
    elif to_archive_name == PROPOSED:
        from_archive_name = STABLE
        from_team_name = 'juju-packaging'
    elif to_archive_name == STABLE:
        from_archive_name = PROPOSED
        from_team_name = 'juju'
    # Ubuntu supported builds.
    elif to_archive_name == PROPOSED_1_22:
        from_archive_name = SUPPORTED_1_22
        from_team_name = 'juju-packaging'
    elif to_archive_name == SUPPORTED_1_22:
        from_archive_name = PROPOSED_1_22
        from_team_name = 'juju'
    else:
        raise ValueError('{} is not a valid archive'.format(to_archive_name))
    from_team = lp.people[from_team_name]
    from_archive = from_team.getPPAByName(name=from_archive_name)
    to_team = lp.people[to_team_name]
    to_archive = to_team.getPPAByName(name=to_archive_name)
    return from_archive, to_archive


def copy_packages(lp, version, to_archive_name, dry_run=False):
    """Copy the juju-core/juju2 source and binary packages to and archive."""
    from_archive, to_archive = get_archives(lp, to_archive_name)
    # Look for juju2 first.
    package_histories = from_archive.getPublishedSources(
        source_name='juju2', status='Published')
    package_histories = [
        package for package in package_histories
        if package.source_package_version.startswith(version)]
    # Look for juju-core second.
    if len(package_histories) == 0:
        package_histories = from_archive.getPublishedSources(
            source_name='juju-core', status='Published')
        package_histories = [
            package for package in package_histories
            if package.source_package_version.startswith(version)]
    if len(package_histories) == 0:
        raise ValueError(
            'No packages matching {} were found in {} to copy to {}.'.format(
                version, from_archive.web_link, to_archive.web_link))
    for package in package_histories:
        print(
            'Copying {} and its binaries to {}.'.format(
                package.display_name, to_archive_name))
        if not dry_run:
            to_archive.copyPackage(
                from_archive=from_archive,
                source_name=package.source_package_name,
                version=package.source_package_version,
                to_pocket='Release', include_binaries=True, unembargo=True)
    return 0


def get_args(argv=None):
    """Return the option parser for this program."""
    parser = ArgumentParser('Copy juju-core/juju2 from one archive to another')
    parser.add_argument(
        '--dry-run', action="store_true", default=False,
        help='Explain what will happen without making changes')
    parser.add_argument(
        "-c", "--credentials", default=None, type=os.path.expanduser,
        help="Launchpad credentials file.")
    parser.add_argument('version', help='The package version like 1.20.8')
    parser.add_argument(
        'to_archive_name',
        help='The archive to copy the source and binary packages to.')
    return parser.parse_args(argv)


def main(argv=None):
    args = get_args(argv)
    lp = Launchpad.login_with(
        'lp-copy-packages', service_root='https://api.launchpad.net',
        version='devel', credentials_file=args.credentials)
    ret_code = copy_packages(
        lp, args.version, args.to_archive_name, args.dry_run)
    return ret_code


if __name__ == '__main__':
    sys.exit(main(sys.argv[1:]))
