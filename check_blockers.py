#!/usr/bin/python

from __future__ import print_function

from argparse import ArgumentParser
import json
import sys
import urllib2

from launchpadlib.launchpad import Launchpad


BUG_STATUSES = [
    'Incomplete', 'Confirmed', 'Triaged', 'In Progress', 'Fix Committed']
BUG_IMPORTANCES = ['Critical']
BUG_TAGS = ['blocker']
LP_BUGS = (
    'https://api.launchpad.net/devel/{target}'
    '?ws.op=searchTasks&tags_combinator=All{tags}{importances}{statuses}'
)
GH_COMMENTS = 'https://api.github.com/repos/juju/juju/issues/{}/comments'
LP_SERIES = 'https://api.launchpad.net/devel/juju-core/series'


def get_json(uri):
    request = urllib2.Request(uri, headers={
        "Cache-Control": "max-age=0, must-revalidate",
    })
    data = urllib2.urlopen(request).read()
    if data:
        return json.loads(data)
    return None


def get_lp(script_name, credentials_file=None):
    """Return an LP API client."""
    lp_args = dict(service_root='https://api.launchpad.net', version='devel')
    if credentials_file:
        lp_args['credentials_file'] = credentials_file
    lp = Launchpad.login_with(script_name, **lp_args)
    return lp


def parse_args(args=None):
    parser = ArgumentParser('Check if a branch is blocked from landing')
    parser.add_argument(
        "-c", "--credentials-file", default=None,
        help="Launchpad credentials file.")
    subparsers = parser.add_subparsers(help='sub-command help', dest="command")
    check_parser = subparsers.add_parser(
        'check', help='Check if merges are blocked for a branch.')
    check_parser.add_argument('branch', help='The branch to merge into.')
    check_parser.add_argument(
        'pull_request', help='The pull request to be merged')
    update_parser = subparsers.add_parser(
        'update', help='Update blocking for a branch that passed CI.')
    update_parser.add_argument(
        '-d', '--dry-run', action='store_true', default=False,
        help='Do not make changes.')
    update_parser.add_argument('branch', help='The branch that passed.')
    update_parser.add_argument(
        'build', help='The build-revision build number.')
    return parser.parse_args(args)


def get_lp_bugs(lp, branch, with_ci=False):
    bugs = {}
    project = lp.projects['juju-core']
    if branch == 'master':
        target = project
    else:
        target = project.getSeries(name=branch)
    if not target:
        return bugs
    if with_ci:
        bug_tags = BUG_TAGS + ['ci']
    else:
        bug_tags = BUG_TAGS
    bug_tasks = target.searchTasks(
        status=BUG_STATUSES, importance=BUG_IMPORTANCES,
        tags=bug_tags, tags_combinator='All')
    for bug_task in bug_tasks:
        bug_id = bug_task.self_link.split('/')[-1]
        bugs[bug_id] = bug_task
    return bugs


def get_reason(bugs, args):
    if not bugs:
        return 0, 'No blocking bugs'
    fixes_ids = ['fixes-{}'.format(bug_id) for bug_id in bugs]
    uri = GH_COMMENTS.format(args.pull_request)
    comments = get_json(uri)
    if comments:
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


def update_bugs(bugs, dry_run=False):
    changes = []
    for bug_id, bug_task in bugs.items():
        changes.append('Updating bug %s [%s]' % (bug_id, bug_task.title))
        bug_task.status = 'Fix Released'
        if not dry_run:
            bug_task.lp_save()
    changes = '\n'.join(changes)
    return 0, changes


def main(argv):
    args = parse_args(argv)
    lp = get_lp('check_blockers', credentials_file=args.credentials_file)
    if args.command == 'check':
        bugs = get_lp_bugs(lp, args.branch, with_ci=False)
        code, reason = get_reason(bugs, args)
        print(reason)
    elif args.command == 'update':
        bugs = get_lp_bugs(lp, args.branch, with_ci=True)
        code, changes = update_bugs(bugs, dry_run=args.dry_run)
        print(changes)
    return code


if __name__ == '__main__':
    sys.exit(main(sys.argv[1:]))
