"""Tests for log_check script."""

from mock import patch
import subprocess

import log_check as lc
from tests import TestCase


class TestCheckFile(TestCase):

    regex = 'test123'
    file_path = '/tmp/file.log'

    def test_calls_check_call(self):
        with patch.object(lc.subprocess, 'check_call') as m_checkcall:
            lc.check_file(self.regex, self.file_path)

            m_checkcall.assert_called_once_with(
                ['sudo', 'egrep', self.regex, self.file_path])

    def test_fails_after_attempting_multiple_times(self):
        with patch.object(lc.subprocess, 'check_call') as m_checkcall:
            m_checkcall.side_effect = subprocess.CalledProcessError(
                1, ['sudo', 'egrep', self.regex, self.file_path])
            with patch.object(lc.time, 'sleep') as m_sleep:
                self.assertEqual(
                    lc.check_file(self.regex, self.file_path),
                    lc.check_result.failure)
                self.assertEqual(m_sleep.call_count, 10)

    def test_fails_when_meeting_unexpected_outcome(self):
        with patch.object(lc.subprocess, 'check_call') as m_checkcall:
            m_checkcall.side_effect = subprocess.CalledProcessError(
                -1, ['sudo', 'egrep', self.regex, self.file_path])
            self.assertEqual(
                lc.check_file(self.regex, self.file_path),
                lc.check_result.exception)

    def test_succeeds_when_regex_found(self):
        with patch.object(lc.subprocess, 'check_call'):
            self.assertEqual(
                lc.check_file(self.regex, self.file_path),
                lc.check_result.success)


class TestRaiseIfFileNotFound(TestCase):

    def test_raises_when_file_not_found(self):
        with self.assertRaises(ValueError):
            lc.raise_if_file_not_found('/thisfilewontexists')

    def test_does_not_raise_when_file_not_found(self):
        lc.raise_if_file_not_found('/')


class TestParseArgs(TestCase):

    def test_basic_args(self):
        args = ['test .*', '/tmp/log.file']
        parsed = lc.parse_args(args)
        self.assertEqual(parsed.regex, 'test .*')
        self.assertEqual(parsed.file_path, '/tmp/log.file')
