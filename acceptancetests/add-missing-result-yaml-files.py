#!/usr/bin/env python

"""Add missing result.yaml in S3; ensue that existing files contain
the final result.
"""

from __future__ import print_function
from argparse import ArgumentParser
from datetime import datetime
import json
import os
import re
from tempfile import NamedTemporaryFile
import yaml

from utility import (
    s3_cmd,
    temp_dir,
)

ARCHIVE_URL = 's3://juju-qa-data/juju-ci/products/'
ISO_8601_FORMAT = '%Y-%m-%dT%H:%M:%S.%fZ'
LONG_AGO = datetime(2000, 1, 1)


def get_ci_director_state():
    state_file_path = os.path.join(
        os.environ['HOME'], '.config/ci-director-state')
    with open(state_file_path) as state_file:
        return yaml.load(state_file)['versions']


def list_s3_files():
    text = s3_cmd(['ls', '-r', ARCHIVE_URL])
    for line in text.strip().split('\n'):
        file_date, file_time, size, url = re.split(r'\s+', line)
        file_date = [int(part) for part in file_date.split('-')]
        file_time = [int(part) for part in file_time.split(':')]
        file_time = datetime(*(file_date + file_time))
        revision_number, filename = re.search(
            r'^{}version-(\d+)/(.*)$'.format(ARCHIVE_URL), url).groups()
        yield int(revision_number), filename, file_time


def get_s3_revision_info():
    all_revisions = {}
    for revision_number, file_name, file_time in list_s3_files():
        revision = all_revisions.setdefault(revision_number, {
            'result': {},
            'artifact_time': LONG_AGO,
            })
        if file_name in ('result.yaml', 'result.json'):
            # Many result.json files were added on 2014-08-14 for older
            # builds, so we may have both a result.yaml file and a
            # result.json file.
            revision['result'][file_time] = file_name
        else:
            revision['artifact_time'] = max(
                revision['artifact_time'], file_time)
    # The most recent version may currently be building, hence a check
    # if the result file exists is useless.
    del all_revisions[max(all_revisions)]
    result_file_time = revision['artifact_time']
    for revision_number, revision_data in sorted(all_revisions.items()):
        if not revision_data['result']:
            result_file_name = None
        else:
            result_file_time = min(revision_data['result'])
            # If both a result.yaml and a result.json file exist, use
            # the newer one.
            newer = max(revision_data['result'])
            result_file_name = revision_data['result'][newer]
        yield revision_number, result_file_name, result_file_time


def main(args):
    ci_director_state = get_ci_director_state()
    for revision_number, result_file, artifact_time in get_s3_revision_info():
        state_file_result = ci_director_state.get(revision_number)
        if state_file_result is None:
            print(
                "Warning: No state file data available for revision",
                revision_number)
            continue
        if result_file is not None:
            with temp_dir() as workspace:
                copy_from = '{}version-{}/{}'.format(
                    ARCHIVE_URL, revision_number, result_file)
                copy_to = os.path.join(workspace, result_file)
                s3_cmd(['--no-progress', 'get', copy_from, copy_to])
                with open(copy_to) as f:
                    s3_result = yaml.load(f)
                # For paranoids: Check that the data from S3 is a subset
                # of the data from the state file
                s3_keys = set(s3_result)
                state_keys = set(ci_director_state[revision_number])
                if not s3_keys.issubset(state_keys):
                    print(
                        "Warning: S3 result file for {} contains keys that do "
                        "not exist in the main state file: {}".format(
                            revision_number, s3_keys.difference(state_keys)))
                    continue
                comparable_state_data = dict(
                    (k, v)
                    for k, v in ci_director_state[revision_number].items()
                    if k in s3_keys)
                if comparable_state_data != s3_result:
                    # This can happen when the result file was written
                    # when a -devel job is still running.
                    print(
                        "Warning: Diverging data for revision {} in S3 ({}) "
                        "and in state file ({}).".format(
                            revision_number, s3_result,
                            ci_director_state[revision_number]))
                if 'result' in s3_result:
                    continue

        if 'finished' not in state_file_result:
            state_file_result['finished'] = artifact_time.strftime(
                ISO_8601_FORMAT)
        with NamedTemporaryFile() as new_result_file:
            json.dump(state_file_result, new_result_file)
            new_result_file.flush()
            dest_url = '{}version-{}/result.json'.format(
                ARCHIVE_URL, revision_number)
            params = ['put', new_result_file.name, dest_url]
            if args.dry_run:
                print(*(['s3cmd'] + params))
            else:
                s3_cmd(params)


if __name__ == '__main__':
    parser = ArgumentParser()
    parser.add_argument('--dry-run', action='store_true')
    args = parser.parse_args()
    main(args)
