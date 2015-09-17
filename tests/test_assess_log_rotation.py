from argparse import Namespace
from unittest import TestCase
from assess_log_rotation import (
    check_for_extra_backup,
    check_expected_backup,
    check_log0,
    LogRotateError,
    parse_args,
)
from jujupy import yaml_loads

good_yaml = \
    """
results:
  result-map:
    log0:
      name: /var/log/juju/unit-fill-logs-0.log
      size: "25"
    log1:
      name: /var/log/juju/unit-fill-logs-0-2015-05-21T09-57-03.123.log
      size: "299"
    log1:
      name: /var/log/juju/unit-fill-logs-0-2015-05-22T12-57-03.123.log
      size: "300"
status: completed
timing:
  completed: 2015-05-21 09:57:03 -0400 EDT
  enqueued: 2015-05-21 09:56:59 -0400 EDT
  started: 2015-05-21 09:57:02 -0400 EDT
"""

good_obj = yaml_loads(good_yaml)

big_yaml = \
    """
results:
  result-map:
    log0:
      name: /var/log/juju/unit-fill-logs-0.log
      size: "400"
    log1:
      name: /var/log/juju/unit-fill-logs-0-2015-05-21T09-57-03.123.log
      size: "400"
    log2:
      name: /var/log/juju/unit-fill-logs-0-not-a-valid-timestamp.log
      size: "299"
    log3:
      name: something-just-plain-bad.log
      size: "299"
status: completed
timing:
  completed: 2015-05-21 09:57:03 -0400 EDT
  enqueued: 2015-05-21 09:56:59 -0400 EDT
  started: 2015-05-21 09:57:02 -0400 EDT
"""

big_obj = yaml_loads(big_yaml)

no_files_yaml = \
    """
results:
  result-map:
status: completed
timing:
  completed: 2015-05-21 09:57:03 -0400 EDT
  enqueued: 2015-05-21 09:56:59 -0400 EDT
  started: 2015-05-21 09:57:02 -0400 EDT
"""

no_files_obj = yaml_loads(no_files_yaml)


class TestCheckForExtraBackup(TestCase):

    def test_not_found(self):
        try:
            # log2 should not be found, and thus no exception.
            check_for_extra_backup("log2", good_obj)
        except Exception as e:
            self.fail("unexpected exception: %s" % e.msg)

    def test_find_extra(self):
        with self.assertRaises(LogRotateError):
            # log1 should be found, and thus cause an exception.
            check_for_extra_backup("log1", good_obj)


class TestCheckBackup(TestCase):

    def test_exists(self):
        try:
            # log1 should be found, and thus no exception.
            check_expected_backup("log1", "unit-fill-logs-0", good_obj)
        except Exception as e:
            self.fail("unexpected exception: %s" % e.msg)

    def test_not_found(self):
        with self.assertRaises(LogRotateError):
            # log2 should not be found, and thus cause an exception.
            check_expected_backup("log2", "unit-fill-logs-0", good_obj)

    def test_too_big(self):
        with self.assertRaises(LogRotateError):
            # log1 is too big, and thus should cause an exception.
            check_expected_backup("log1", "unit-fill-logs-0", big_obj)

    def test_bad_timestamp(self):
        with self.assertRaises(LogRotateError):
            # log2 has an invalid timestamp, and thus should cause an
            # exception.
            check_expected_backup("log2", "unit-fill-logs-0", big_obj)

    def test_bad_name(self):
        with self.assertRaises(LogRotateError):
            # log3 has a completely invalid name, and thus should cause an
            # exception.
            check_expected_backup("log3", "unit-fill-logs-0", big_obj)


class TestCheckLog0(TestCase):

    def test_exists(self):
        try:
            # log0 should be found, and thus no exception.
            check_log0("/var/log/juju/unit-fill-logs-0.log", good_obj)
        except Exception as e:
            self.fail("unexpected exception: %s" % e.msg)

    def test_not_found(self):
        with self.assertRaises(AttributeError):
            # There's no value under result-map, which causes the yaml parser
            # to consider it None, and thus it'll cause an AttributeError
            check_log0("/var/log/juju/unit-fill-logs-0.log", no_files_obj)

    def test_too_big(self):
        with self.assertRaises(LogRotateError):
            # log0 is too big, and thus should cause an exception.
            check_log0(
                "/var/log/juju/unit-fill-logs-0.log", big_obj)


class TestParseArgs(TestCase):

    def test_parse_args(self):
        args = parse_args(['machine', 'b', 'c', 'd', 'e'])
        self.assertEqual(args, Namespace(
            agent='machine', juju_path='b', env_name='c', logs='d',
            temp_env_name='e', debug=False))

    def test_parse_args_debug(self):
        args = parse_args(['--debug', 'unit', 'b', 'c', 'd', 'e'])
        self.assertEqual(args, Namespace(
            agent='unit', juju_path='b', env_name='c', logs='d',
            temp_env_name='e', debug=True))
