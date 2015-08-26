#!/usr/bin/python

from __future__ import print_function

from argparse import ArgumentParser
import os
import subprocess
import sys

from launchpadlib.launchpad import Launchpad


def get_changes(package_dir):
    file_name = [
        f for f in os.listdir(package_dir) if f.endswith('_source.changes')]
    source_name, version, ignore = file_name.split('_')
    return source_name, version, file_name


def upload_package(team, archive, package_dir, dry_run=False):
    source_name, version, file_name = get_changes(package_dir)
    package_histories = archive.getPublishedSources(
        source_name=source_name, version=version)
    if package_histories:
        print('{} {} is uploaded'.format(source_name, version))
        return False
    print('uploading {} {}'.format(source_name, version))
    uri = 'ppa:{}/{}'.format(team.name, archive.name)
    if not dry_run:
        subprocess.call(['dput', uri, file_name], cwd=package_dir)
    return True


def upload_packages(lp, team_name, archive_name, package_dirs, dry_run=False):
    """Upload new source packages to the archive."""
    team = lp.people[team_name]
    archive = team.getPPAByName(name=archive_name)
    for package_dir in package_dirs:
        upload_package(team, archive, package_dir, dry_run=dry_run)


def get_args(argv=None):
    """Return the option parser for this program."""
    parser = ArgumentParser('Upload new source packages to Launchpad.')
    parser.add_argument(
        '--dry-run', action="store_true", default=False,
        help='Explain what will happen without making changes')
    parser.add_argument(
        "-c", "--credentials", default=None, type=os.path.expanduser,
        help="Launchpad credentials file.")
    parser.add_argument(
        'team_name', help='The team that owns the archive.')
    parser.add_argument(
        'archive_name', help='The archive to upload the source packages to.')
    parser.add_argument(
        'package_dirs', nargs='+', type=os.path.expanduser,
        help='One or more source package directories.')
    return parser.parse_args(argv)


def main(argv=None):
    args = get_args(argv)
    lp = Launchpad.login_with(
        'upload-packages', service_root='https://api.launchpad.net',
        version='devel', credentials_file=args.credentials)
    ret_code = upload_packages(
        lp, args.team_name, args.archive_name, args.package_dirs, args.dry_run)
    return ret_code


if __name__ == '__main__':
    sys.exit(main(sys.argv[1:]))
