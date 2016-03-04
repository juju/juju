#!/usr/bin/python

from __future__ import print_function

from argparse import ArgumentParser
from datetime import (
    datetime,
    timedelta,
)
import re
import sys
from textwrap import wrap

from launchpadlib.launchpad import Launchpad


DEVEL = 'development'
PROPOSED = 'proposed'

DEVEL_TEMPLATE = """\
# juju-core {version}

A new development release of Juju, juju-core {version}, is now available.
This release replaces version {previous}.


## Getting Juju

Juju {version} is available for Xenial and backported to earlier
series in the following PPA:

    https://launchpad.net/~juju/+archive/devel

Windows, Centos, and OS X users will find installers at:

    https://launchpad.net/juju-core/+milestone/{version}

Upgrading from stable releases to development releases is not
supported. You can upgrade test environments to development releases
to test new features and fixes, but it is not advised to upgrade
production environments to {version}.


## Notable Changes

{notable}


## Resolved issues

{resolved_text}


Finally

We encourage everyone to subscribe the mailing list at
juju-dev@lists.canonical.com, or join us on #juju-dev on freenode.
"""

PROPOSED_TEMPLATE = """\
# juju-core {version}

A new proposed stable release of Juju, juju-core {version}, is now available.
This release may replace version {previous} on {release_date}.


## Getting Juju

juju-core {version} is available for Xenial and backported to earlier
series in the following PPA:

    https://launchpad.net/~juju/+archive/proposed

Windows, Centos, and OS X users will find installers at:

    https://launchpad.net/juju-core/+milestone/{version}

Proposed releases use the "proposed" simple-streams. You must configure
the `agent-stream` option in your environments.yaml to use the matching
juju agents.


## Notable Changes

{notable}


## Resolved issues

{resolved_text}


Finally

We encourage everyone to subscribe the mailing list at
juju-dev@lists.canonical.com, or join us on #juju-dev on freenode.
"""


def get_lp_bug_tasks(script, milestone_name):
    """Return an iterators of Lp BugTasks,"""
    lp = Launchpad.login_with(
        script, service_root='https://api.launchpad.net', version='devel')
    project = lp.projects['juju-core']
    milestone = project.getMilestone(name=milestone_name)
    return milestone.searchTasks(status=['Fix Committed'])


def get_purpose(milestone):
    """Return PROPOSED or DEVEL as implied by the milestone version."""
    parts = milestone.split('.')
    major = minor = micro = None
    if len(parts) == 2:
        major, minor = parts
    elif len(parts) == 3:
        major, minor, micro = parts
    else:
        raise ValueError(
            'Milestone version is not understood to be major.minor.micro.')
    if re.search('[a-z]+', minor):
        return DEVEL
    else:
        return PROPOSED


def get_bugs(script, milestone):
    """Return a list of bug tuples (id, title)."""
    bug_tasks = get_lp_bug_tasks(script, milestone)
    bugs = []
    for bugtask in bug_tasks:
        bug = bugtask.bug
        if 'tech-debt' not in bug.tags:
            bugs.append((bug.id, bug.title.capitalize()))
    return bugs


def make_resolved_text(bugs):
    """Return the list of bug tuples as formatted text."""
    resolved = []
    for bug in bugs:
        lines = wrap(
            '* {0}'.format(bug[1]), width=70, initial_indent='  ',
            subsequent_indent='    ')
        lines.append('    Lp {0}'.format(bug[0]))
        text = '\n'.join(lines)
        resolved.append(text)
    resolved_text = '\n\n'.join(resolved)
    return resolved_text


def make_release_date(now=None):
    if now is None:
        now = datetime.utcnow()
    week = timedelta(days=7)
    future = now + week
    release_date = future.strftime('%A %B %d')
    return release_date


def make_notes(version, purpose, resolved_text, previous=None, notable=None):
    """Return to formatted release notes."""
    if purpose == DEVEL:
        template = DEVEL_TEMPLATE
    else:
        template = PROPOSED_TEMPLATE
    if notable is None:
        notable = 'This releases addresses stability and performance issues.'
    elif notable == '':
        notable = '[[Add the notable changes here.]]'
    release_date = make_release_date()
    text = template.format(
        version=version, purpose=purpose, resolved_text=resolved_text,
        notable=notable, previous=previous, release_date=release_date)
    # Normalise the whitespace between sections. The text can have
    # extra whitespae when blank sections are interpolated.
    text = text.replace('\n\n\n\n', '\n\n\n')
    return text


def save_notes(text, file_name):
    """Save the notes to the named file or print to stdout."""
    if file_name is None:
        print(text)
    else:
        with open(file_name, 'w') as rn:
            rn.write(text)


def parse_args(args=None):
    parser = ArgumentParser('Create release notes from a milestone')
    parser.add_argument(
        '--previous', help='the previous release.', default=None)
    parser.add_argument(
        '--file-name', help='the name of file to write.', default=None)
    parser.add_argument('milestone', help='the milestone to examine.')
    return parser.parse_args(args)


def main(argv):
    args = parse_args(argv[1:])
    purpose = get_purpose(args.milestone)
    bugs = get_bugs(argv[0], args.milestone)
    resolved_text = make_resolved_text(bugs)
    text = make_notes(args.milestone, purpose, resolved_text, args.previous)
    save_notes(text, args.file_name)
    print('These are the bugs the package fixes:')
    print(' '.join([str(i) for i, t in bugs]))
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv))
