#!/usr/bin/python

from __future__ import print_function

from argparse import ArgumentParser
import json
import urllib2
import sys

BUG_STATUSES = (
    'Incomplete', 'Confirmed', 'Triaged', 'In+Progress', 'Fix+Committed')
BUG_IMPORTANCES = ('Critical', )
BUG_TAGS = ('blocker', )
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


def parse_args(args=None):
    parser = ArgumentParser('Check if a branch is blocked from landing')
    subparsers = parser.add_subparsers(help='sub-command help', dest="command")
    check_parser = subparsers.add_parser(
        'check', help='Check if merges are blocked for a branch.')
    check_parser.add_argument('branch', help='The branch to merge into.')
    check_parser.add_argument(
        'pull_request', help='The pull request to be merged')
    return parser.parse_args(args)


def get_lp_bugs_url(target):
    """Return the target series url to query blocking bugs."""
    params = {'target': target}
    params['tags'] = ''.join('&tags%3Alist={}'.format(t) for t in BUG_TAGS)
    params['importances'] = ''.join(
        '&importance%3Alist={}'.format(i) for i in BUG_IMPORTANCES)
    params['statuses'] = ''.join(
        '&status%3Alist={}'.format(s) for s in BUG_STATUSES)
    return LP_BUGS.format(**params)


def get_lp_bugs(args):
    bugs = {}
    batch = get_json(LP_SERIES)
    series = [s['name'] for s in batch['entries']]
    if args.branch != 'master' and args.branch not in series:
        # This branch is not a registered series to target bugs too.
        return bugs
    if args.branch == 'master':
        # Lp implicitly assigns bugs to trunk, which is not a series query.
        target = 'juju-core'
    else:
        target = 'juju-core/%s' % args.branch
    uri = get_lp_bugs_url(target)
    batch = get_json(uri)
    if batch:
        for bug_data in batch['entries']:
            bug_id = bug_data['self_link'].split('/')[-1]
            bugs[bug_id] = bug_data
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


def main():
    args = parse_args()
    if args.command == 'checked':
        bugs = get_lp_bugs(args)
        code, reason = get_reason(bugs, args)
        print(reason)
    return code


if __name__ == '__main__':
    sys.exit(main())
