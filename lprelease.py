#!/usr/bin/python
"""Register a release and update the milestone bugs.

These operations are idempotent. Milestones and releases are placed into
a finished state. When run multiple times, the actions will either have
no affect, or something will be corrected.
"""

from __future__ import print_function

from argparse import ArgumentParser
import datetime
import os
import subprocess
import sys
import traceback

from launchpadlib.launchpad import Launchpad


FIX_COMMITTED = u'Fix Committed'
UNFINISHED = ['New', 'Confirmed', 'Triaged', 'In Progress', 'Incomplete']


def run(command, verbose=False, dry_run=False):
    """Run command list and ensure stdout and error are available."""
    if verbose:
        print(command)
    try:
        subprocess.check_output(command, stderr=subprocess.STDOUT)
    except subprocess.CalledProcessError as e:
        print('FAIL: {} - Returncode: {}'.format(e.output, e.returncode))
        raise


def sign_file(file_path, gpgcmd, dry_run, verbose):
    sig_file_path = '%s.asc' % file_path
    if os.path.isfile(sig_file_path):
        if verbose:
            print('Found %s' % sig_file_path)
    else:
        if verbose:
            print('Creating %s' % sig_file_path)
        if not dry_run:
            run([gpgcmd, '--armor', '--sign', '--detach-sig', file_path],
                verbose, dry_run)

    return sig_file_path


def get_content_type(file_name, verbose):
    name = file_name.lower()
    if 'changelog' in name:
        content_type = 'ChangeLog File'
    elif 'readme' in name:
        content_type = 'README File'
    elif 'notes' in name:
        content_type = 'Release Notes'
    elif '_' in name and name.endswith('tar.gz'):
        content_type = 'Code Release Tarball'
    else:
        content_type = 'Installer file'
    return content_type


def get_lp(script_name, credentials_file=None):
    """Return an LP API client."""
    lp_args = dict(service_root='https://api.launchpad.net', version='devel')
    if credentials_file:
        credentials_file = os.path.expanduser(credentials_file)
        lp_args['credentials_file'] = credentials_file
    lp = Launchpad.login_with(script_name, **lp_args)
    return lp


def get_project(lp, project_name, verbose):
    if verbose:
        print('Getting project %s' % project_name)
    return lp.projects[project_name]


def get_milestone(project, milestone_name, verbose):
    if verbose:
        print('Getting milestone %s' % milestone_name)
    return project.getMilestone(name=milestone_name)


def close_milestone(milestone, dry_run, verbose):
    is_active = milestone.is_active
    if verbose:
        print('%s is_active: %s' % (milestone.name, is_active))
    milestone.is_active = False
    if not dry_run and is_active is True:
        milestone.lp_save()


def defer_bugs(milestone, deferred_milestone, dry_run, verbose):
    unfinished_bug_tasks = milestone.searchTasks(status=UNFINISHED)
    for bug_task in unfinished_bug_tasks:
        # This needs fixed; bug #1629115
        # if verbose:
        #     print('Retargeting bug %s [%s] to %s' % (
        #         bug_task.bug.id, bug_task.bug.title,
        #         deferred_milestone.name))
        bug_task.milestone = deferred_milestone
        if not dry_run:
            bug_task.lp_save()


def close_bugs(milestone, dry_run, verbose):
    fixed_bug_tasks = milestone.searchTasks(status=FIX_COMMITTED)
    for bug_task in fixed_bug_tasks:
        if verbose:
            print('Updating bug %s [%s]' % (
                bug_task.bug.id, bug_task.bug.title))
        bug_task.status = u'Fix Released'
        if not dry_run:
            bug_task.lp_save()


def register_release(milestone, dry_run, verbose):
    if verbose:
        print('Getting release %s' % milestone.name)
    release = milestone.release
    if release:
        return release
    now = datetime.datetime.utcnow()
    if not dry_run:
        if verbose:
            print('Registering release %s' % milestone.name)
        release = milestone.createProductRelease(date_released=now)
        return release


def add_release_files(release, release_notes_path, files, gpgcmd,
                      dry_run, verbose):
    if not release and dry_run:
        print("Returning early because there is no registered release.")
        return
    if verbose:
        print('Updating release %s' % release.version)
    if release_notes_path:
        with open(release_notes_path) as rn_file:
            release_notes = rn_file.read()
        if verbose:
            print('Updating release notes')
        release.release_notes = release_notes
        if not dry_run:
            release.lp_save()
    released_files = [os.path.basename(prf.self_link) for prf in release.files]
    for file_path in files:
        if ':' in file_path:
            file_path, description = file_path.split(':')
        else:
            description = ''
        file_name = os.path.basename(file_path)
        if file_name in released_files:
            if verbose:
                print('skipping %s because it is published' % file_name)
            continue
        content_type = get_content_type(file_name, verbose)
        sig_file_path = sign_file(file_path, gpgcmd, dry_run, verbose)
        sig_file_name = os.path.basename(sig_file_path)
        with open(file_path) as content_file:
            content = content_file.read()
        with open(sig_file_path) as sig_file:
            sig_content = sig_file.read()
        if not dry_run:
            release.add_file(
                filename=file_name, file_content=content,
                content_type=content_type, signature_filename=sig_file_name,
                signature_content=sig_content, description=description)


def main():
    """Execute the commands from the command line."""
    args = get_args()
    dry_run = args.dry_run
    verbose = args.verbose
    try:
        lp = get_lp('release-milestone', args.credentials)
        project = get_project(lp, args.project, verbose)
        milestone = get_milestone(project, args.milestone, verbose)
        close_milestone(milestone, dry_run, verbose)
        if args.deferred_milestone:
            deferred_milestone = get_milestone(
                project, args.deferred_milestone, verbose)
            defer_bugs(milestone, deferred_milestone, dry_run, verbose)
        close_bugs(milestone, dry_run, verbose)
        release = register_release(milestone, dry_run, verbose)
        add_release_files(
            release, args.release_notes, args.files, args.gpgcmd,
            dry_run, verbose)
    except Exception as e:
        print(e)
        if verbose:
            traceback.print_tb(sys.exc_info()[2])
        return 2
    if verbose:
        print("Done.")
    return 0


def get_args(argv=None):
    """Return the option parser for this program."""
    parser = ArgumentParser("Register a release and update the milestone.")
    parser.add_argument(
        "-d", "--dry-run", action="store_true", default=False,
        help="Do not make changes.")
    parser.add_argument(
        '-v', '--verbose', action="store_true", default=False,
        help='Increase verbosity.')
    parser.add_argument(
        "-c", "--credentials", default=None,
        help="Launchpad credentials file.")
    parser.add_argument(
        "-n", "--release-notes", default=None,
        help="The release nots file to add.")
    parser.add_argument(
        "-m", "--deferred-milestone", default=None,
        help="The defer unfinished bugs to the next milestone.")
    parser.add_argument(
        "-f", "--files", action="append", default=[],
        help="Launchpad downloadable file.")
    parser.add_argument(
        "-g", "--gpgcmd", default='gpg',
        help="Path to an alternate gpg cmd.")
    parser.add_argument(
        'project', help='The project to register the release in')
    parser.add_argument(
        'milestone', help='The released milestone')
    return parser.parse_args(argv)


if __name__ == '__main__':
    sys.exit(main())
