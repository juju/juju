#!/usr/bin/python

from __future__ import print_function

from argparse import ArgumentParser
import json
import urllib2
import sys


DEVEL = 'juju-core'
STABLE = 'juju-core/1.20'
LP_BUGS = (
    'https://api.launchpad.net/devel/{}'
    '?ws.op=searchTasks'
    '&status%3Alist=Triaged&status%3Alist=In+Progress'
    '&status%3Alist=Fix+Committed'
    '&importance%3Alist=Critical'
    '&tags%3Alist=regression&tags%3Alist=ci&tags_combinator=All'
    )
GH_COMMENTS = 'https://api.github.com/repos/juju/juju/issues/{}/comments'


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
    parser.add_argument('branch', help='The branch to merge into.')
    parser.add_argument('pull_request', help='The pull request to be merged')
    return parser.parse_args(args)


def get_lp_bugs(args):
    bugs = {}
    if args.branch == 'master':
        target = DEVEL
    else:
        target = STABLE
    uri = LP_BUGS.format(target)
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
    bugs = get_lp_bugs(args)
    code, reason = get_reason(bugs, args)
    print(reason)
    return code


if __name__ == '__main__':
    sys.exit(main())


