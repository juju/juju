#!/usr/bin/python

from __future__ import print_function

from argparse import ArgumentParser
import os
import subprocess
import sys

from launchpadlib.launchpad import Launchpad


def get_changes(package_dir):
    """Return source_name, version, file_name of the changes in the dir."""
    file_names = [
        f for f in os.listdir(package_dir) if f.endswith('_source.changes')]
    file_name = file_names[0]
    for file_name in os.listdir(package_dir):
        if file_name.endswith('_source.changes'):
            break
    else:
        raise ValueError('*.changes file not found in {}'.format(package_dir))
    with open(os.path.join(package_dir, file_name)) as changes_file:
        changes = changes_file.read()
    version = source_name = None
    for line in changes.splitlines():
        if line.startswith('Version:'):
            ignore, version = line.split(' ')
        if line.startswith('Source:'):
            ignore, source_name = line.split(' ')
        if version and source_name:
            break
    else:
        raise AssertionError(
            'Version: and Source: not found in {}'.format(file_name))
    return source_name, version, file_name


def upload_package(ppa, archive, package_dir, dry_run=False):
    """Upload a source package found in the directory to the PPA.

    Return True when the upload is performed, or False when the archive already
    has the source package.
    """
    source_name, version, file_name = get_changes(package_dir)
    package_histories = archive.getPublishedSources(
        source_name=source_name, version=version)
    if package_histories:
        print('{} {} is uploaded'.format(source_name, version))
        return False
    print('uploading {} {}'.format(source_name, version))
    if not dry_run:
        subprocess.check_call(['dput', ppa, file_name], cwd=package_dir)
    return True


def upload_packages(lp, ppa, package_dirs, dry_run=False):
    """Upload new source packages to the PPA."""
    ignore, team_archive = ppa.split(':')
    team_name, archive_name = team_archive.split('/')
    team = lp.people[team_name]
    archive = team.getPPAByName(name=archive_name)
    for package_dir in package_dirs:
        upload_package(ppa, archive, package_dir, dry_run=dry_run)


def get_args(argv=None):
    """Return the option parser for this program."""
    parser = ArgumentParser('Upload new source packages to Launchpad.')
    parser.add_argument(
        '-d', '--dry-run', action="store_true", default=False,
        help='Explain what will happen without making changes')
    parser.add_argument(
        "-c", "--credentials", default=None, type=os.path.expanduser,
        help="Launchpad credentials file.")
    parser.add_argument(
        'ppa', help='The ppa to upload to: ppa:<person>/<archive>.')
    parser.add_argument(
        'package_dirs', nargs='+', type=os.path.expanduser,
        help='One or more source package directories.')
    return parser.parse_args(argv)


def main(argv=None):
    """Upload new source packages to a PPA."""
    args = get_args(argv)
    lp = Launchpad.login_with(
        'upload-packages', service_root='https://api.launchpad.net',
        version='devel', credentials_file=args.credentials)
    upload_packages(
        lp, args.ppa, args.package_dirs, dry_run=args.dry_run)
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv[1:]))
