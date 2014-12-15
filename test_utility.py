from datetime import (
    datetime,
    timedelta,
    )
from contextlib import contextmanager
import os
from unittest import TestCase
from time import time

from mock import patch
from StringIO import StringIO

from utility import (
    find_candidates,
    get_auth_token,
    get_candidates_path,
    temp_dir,
    until_timeout,
    )


class TestUntilTimeout(TestCase):

    def test_no_timeout(self):

        iterator = until_timeout(0)

        def now_iter():
            yield iterator.start
            yield iterator.start
            assert False

        with patch.object(iterator, 'now', now_iter().next):
            for x in iterator:
                self.assertIs(None, x)
                break

    @contextmanager
    def patched_until(self, timeout, deltas):
        iterator = until_timeout(timeout)

        def now_iter():
            for d in deltas:
                yield iterator.start + d
            assert False
        with patch.object(iterator, 'now', now_iter().next):
            yield iterator

    def test_timeout(self):
        with self.patched_until(
                5, [timedelta(), timedelta(0, 4), timedelta(0, 5)]) as until:
            results = list(until)
        self.assertEqual([5, 1], results)

    def test_long_timeout(self):
        deltas = [timedelta(), timedelta(4, 0), timedelta(5, 0)]
        with self.patched_until(86400 * 5, deltas) as until:
            self.assertEqual([86400 * 5, 86400], list(until))

    def test_start(self):
        now = datetime.now() + timedelta(days=1)
        now_iter = iter([now, now, now + timedelta(10)])
        with patch('utility.until_timeout.now', side_effect=now_iter.next):
            self.assertEqual(list(until_timeout(10, now - timedelta(10))), [])


def write_config(root, job_name, token):
    job_dir = os.path.join(root, 'jobs', job_name)
    os.makedirs(job_dir)
    job_config = os.path.join(job_dir, 'config.xml')
    with open(job_config, 'w') as config:
        config.write(
            '<config><authToken>{}</authToken></config>'.format(token))


@contextmanager
def parse_error(test_case):
    stderr = StringIO()
    with test_case.assertRaises(SystemExit):
        with patch('sys.stderr', stderr):
            yield stderr


class TestGetAuthToken(TestCase):

    def test_get_auth_token(self):
        with temp_dir() as root:
            write_config(root, 'job-name', 'foo')
            self.assertEqual(get_auth_token(root, 'job-name'), 'foo')


class TestFindCandidates(TestCase):

    def test_find_candidates(self):
        with temp_dir() as root:
            candidates_path = get_candidates_path(root)
            os.mkdir(candidates_path)
            self.assertEqual(list(find_candidates(root)), [])
            master_path = os.path.join(candidates_path, 'master')
            os.mkdir(master_path)
            self.assertEqual(list(find_candidates(root)), [])
            open(os.path.join(master_path, 'buildvars.json'), 'w')
            self.assertEqual(list(find_candidates(root)), [master_path])

    def test_find_candidates_old_buildvars(self):
        with temp_dir() as root:
            candidates_path = get_candidates_path(root)
            os.mkdir(candidates_path)
            master_path = os.path.join(candidates_path, 'master')
            os.mkdir(master_path)
            buildvars_path = os.path.join(master_path, 'buildvars.json')
            open(buildvars_path, 'w')
            a_week_ago = time() - timedelta(days=7, seconds=1).total_seconds()
            os.utime(buildvars_path, (time(), a_week_ago))
            self.assertEqual(list(find_candidates(root)), [])

    def test_find_candidates_artifacts(self):
        with temp_dir() as root:
            candidates_path = get_candidates_path(root)
            os.mkdir(candidates_path)
            master_path = os.path.join(candidates_path, 'master-artifacts')
            os.mkdir(master_path)
            open(os.path.join(master_path, 'buildvars.json'), 'w')
            self.assertEqual(list(find_candidates(root)), [])
