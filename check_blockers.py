#!/usr/bin/python

from __future__ import print_function

from argparse import ArgumentParser
import json
import sys
import urllib2

from launchpadlib.launchpad import (
    Launchpad,
    uris,
)


BUG_STATUSES = [
    'Incomplete', 'Confirmed', 'Triaged', 'In Progress', 'Fix Committed']
BUG_IMPORTANCES = ['Critical']
BUG_TAGS = ['blocker']
GH_COMMENTS = 'https://api.github.com/repos/juju/juju/issues/{}/comments'


def get_json(uri):
    """Return the json dict response for the uri request."""
    request = urllib2.Request(uri, headers={
        "Cache-Control": "max-age=0, must-revalidate",
    })
    response = urllib2.urlopen(request)
    data = response.read()
    if response.getcode() == 200 and data:
        return json.loads(data)
    return None


def get_lp(script_name, credentials_file=None):
    """Return an LP API client."""
    lp_args = dict()
    if credentials_file:
        lp_args['credentials_file'] = credentials_file
    lp = Launchpad.login_with(
        script_name, service_root=uris.LPNET_SERVICE_ROOT,
        version='devel', **lp_args)
    return lp


def parse_args(args=None):
    parser = ArgumentParser('Check if a branch is blocked from landing')
    parser.add_argument(
        "-c", "--credentials-file", default=None,
        help="Launchpad credentials file.")
    subparsers = parser.add_subparsers(help='sub-command help', dest="command")
    check_parser = subparsers.add_parser(
        'check', help='Check if merges are blocked for a branch.')
    check_parser.add_argument(
        'branch', default='master', nargs='?', type=str.lower,
        help='The branch to merge into.')
    check_parser.add_argument('pull_request', default=None, nargs='?',
                              help='The pull request to be merged')
    block_ci_testing_parser = subparsers.add_parser(
        'block-ci-testing',
        help='Check if ci testing is blocked for the branch.')
    block_ci_testing_parser.add_argument(
        'branch', type=str.lower, help='The branch to merge into.')
    update_parser = subparsers.add_parser(
        'update', help='Update blocking for a branch that passed CI.')
    update_parser.add_argument(
        '-d', '--dry-run', action='store_true', default=False,
        help='Do not make changes.')
    update_parser.add_argument(
        'branch', type=str.lower, help='The branch that passed.')
    update_parser.add_argument(
        'build', help='The build-revision build number.')
    args = parser.parse_args(args)
    if not getattr(args, 'pull_request', None):
        args.pull_request = None
    return args


def get_lp_bugs(lp, branch, tags):
    """Return a dict of blocker critical bug tasks for the branch."""
    if not tags:
        raise ValueError('tags must be a list of bug tags')
    bug_tags = tags
    bugs = {}
    project = lp.projects['juju-core']
    if branch == 'master':
        # Lp implicitly assigns bugs to trunk, which is not a series query.
        target = project
    else:
        target = project.getSeries(name=branch)
    if not target:
        return bugs
    bug_tasks = target.searchTasks(
        status=BUG_STATUSES, importance=BUG_IMPORTANCES,
        tags=bug_tags, tags_combinator='All')
    for bug_task in bug_tasks:
        # Avoid an extra network call to get the bug report.
        bug_id = bug_task.self_link.split('/')[-1]
        bugs[bug_id] = bug_task
    return bugs


def get_reason(bugs, args):
    """Return the success code and reason why the branch can be merged."""
    if not bugs:
        return 0, 'No blocking bugs'
    fixes_ids = ['fixes-{}'.format(bug_id) for bug_id in bugs]
    if args.pull_request is None:
        return 1, 'Blocked waiting on {}'.format(fixes_ids)
    uri = GH_COMMENTS.format(args.pull_request)
    comments = get_json(uri)
    if comments is not None:
        for comment in comments:
            user = comment['user']
            if user['login'] == 'jujubot' or 'Juju bot' in comment['body']:
                continue
            if '__JFDI__' in comment['body']:
                return 0, 'Engineer says JFDI'
            for fid in fixes_ids:
                if fid in comment['body']:
                    return 0, 'Matches {}'.format(fid)
        else:
            return 1, 'Does not match {}'.format(fixes_ids)
    return 1, 'Could not get {} comments from github'.format(args.pull_request)


def update_bugs(bugs, branch, build, dry_run=False):
    """Update the critical blocker+ci bugs for the branch to Fix Released."""
    changes = []
    for bug_id, bug_task in bugs.items():
        changes.append('Updated %s' % bug_task.title)
        bug_task.status = 'Fix Released'
        if not dry_run:
            bug_task.lp_save()
            subject = 'Fix Released in juju-core %s' % branch
            content = (
                'Juju-CI verified that this issue is %s:\n'
                '    http://reports.vapour.ws/releases/%s' % (subject, build))
            bug_task.bug.newMessage(subject=subject, content=content)
    changes = '\n'.join(changes)
    return 0, changes


def main(argv):
    args = parse_args(argv)
    lp = get_lp('check_blockers', credentials_file=args.credentials_file)
    if args.command == 'check':
        bugs = get_lp_bugs(lp, args.branch, ['blocker'])
        code, reason = get_reason(bugs, args)
        print(reason)
    if args.command == 'block-ci-testing':
        bugs = get_lp_bugs(lp, args.branch, ['block-ci-testing'])
        code, reason = get_reason(bugs, args)
        print(reason)
    elif args.command == 'update':
        bugs = get_lp_bugs(lp, args.branch, ['blocker', 'ci'])
        code, changes = update_bugs(
            bugs, args.branch, args.build, dry_run=args.dry_run)
        print(changes)
    return code


if __name__ == '__main__':
    sys.exit(main(sys.argv[1:]))
