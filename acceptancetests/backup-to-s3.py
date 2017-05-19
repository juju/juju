#!/usr/bin/env python
"""Backup Jenkins data to S3 and remove old backups."""

from __future__ import print_function

from datetime import datetime
import re

from utility import s3_cmd


MAX_BACKUPS = 10
BACKUP_URL = 's3://juju-qa-data/juju-ci/backups/'
# Exclude hidden files in the home directory, workspace and build data
# of jobs, caches and Bazaar repositories.
BACKUP_PARAMS = [
    r'--rexclude=^\.',
    '--rexclude=^jobs/.*?/workspace/',
    '--rexclude=^jobs/.*?/builds/',
    '--rexclude=^jobs/disabled-repository',
    '--rexclude=^local-tools-cache/',
    '--rexclude=^ci-director/',
    '--rexclude=^cloud-city/',
    '--rexclude=^failure-emails/',
    '--rexclude=^juju-ci-tools/',
    '--rexclude=^juju-release-tools/',
    '--rexclude=^repository',
    ]


def current_backups():
    """Return a list of S3 URLs of existing backups."""
    # We expect lines like
    # "         DIR   s3://juju-qa-data/juju-ci/backups/2014-07-25/"
    result = []
    for line in s3_cmd(['ls', BACKUP_URL]).split('\n'):
        mo = re.search(r'^\s+DIR\s+(%s\d\d\d\d-\d\d-\d\d/)$' % BACKUP_URL,
                       line)
        if mo is None:
            continue
        url = mo.group(1)
        result.append(url)
    return sorted(result)


def run_backup(url):
    s3_cmd(['sync', '.', url] + BACKUP_PARAMS, drop_output=True)


def remove_backups(urls):
    if urls:
        s3_cmd(['del', '-r'] + urls, drop_output=True)


if __name__ == '__main__':
    all_backups = current_backups()
    today = datetime.now().strftime('%Y-%m-%d')
    todays_url = '%s%s/' % (BACKUP_URL, today)
    if todays_url in all_backups:
        print("backup for %s already exists." % today)
    else:
        run_backup(todays_url)
        all_backups.append(todays_url)
    remove_backups(all_backups[:-MAX_BACKUPS])
