#!/usr/bin/python

from __future__ import print_function

from argparse import ArgumentParser
import sys


def get_lp_blockers(args):
    return None


def get_reason(blockers, args):
    return 0, ''


def main():
    parser = ArgumentParser('Check if a branch is blocked from landing')
    parser.add_argument('branch', help='The branch to merge into.')
    parser.add_argument('pull_request', help='The pull request to be merged')
    args = parser.parse_args()
    blockers = get_lp_blockers()
    code, reason = get_reason(blockers, args)
    if reason:
        print(reason)
    return code


if __name__ == '__main__':
    sys.exit(main())


