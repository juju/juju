from unittest import TestCase
from assess_log_rotation import (
    check_for_extra_backup,
    check_expected_backup,
    check_log0,
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
        try:
            check_for_extra_backup("log1", good_obj)
            # log1 should be found, and thus cause an exception.
            self.fail("Expected to get exception, but didn't.")
        except Exception:
            # this is the correct path, it should throw when it finds log1.
            pass


class TestCheckBackup(TestCase):
    def test_exists(self):
        try:
            # log1 should be found, and thus no exception.
            check_expected_backup("log1", "unit-fill-logs-0", good_obj)
        except Exception as e:
            self.fail("unexpected exception: %s" % e.msg)

    def test_not_found(self):
        try:
            check_expected_backup("log2", "unit-fill-logs-0", good_obj)
            # log2 should not be found, and thus cause an exception.
            self.fail("Expected to get exception, but didn't.")
        except Exception:
            # this is the correct path, it should throw when it doesn't find log2.
            pass

    def test_too_big(self):
        try:
            check_expected_backup("log1", "unit-fill-logs-0", big_obj)
            # log1 is too big, and thus should cause an exception.
            self.fail("Expected to get exception, but didn't.")
        except Exception:
            # this is the correct path, it should throw when it doesn't find log2.
            pass

    def test_bad_timestamp(self):
        try:
            check_expected_backup("log2", "unit-fill-logs-0", big_obj)
            # log2 has an invalid timestamp, and thus should cause an exception.
            self.fail("Expected to get exception, but didn't.")
        except Exception:
            # this is the correct path, it should throw when it doesn't find log2.
            pass

    def test_bad_name(self):
        try:
            check_expected_backup("log3", "unit-fill-logs-0", big_obj)
            # log3 has a completely invalid name, and thus should cause an exception.
            self.fail("Expected to get exception, but didn't.")
        except Exception:
            # this is the correct path, it should throw when it doesn't find log2.
            pass


class TestCheckLog0(TestCase):
    def test_exists(self):
        try:
            # log0 should be found, and thus no exception.
            check_log0("/var/log/juju/unit-fill-logs-0.log", good_obj)
        except Exception as e:
            self.fail("unexpected exception: %s" % e.msg)

    def test_not_found(self):
        try:
            check_log0("/var/log/juju/unit-fill-logs-0.log", no_files_obj)
            # log0 should not be found, and thus cause an exception.
            self.fail("Expected to get exception, but didn't.")
        except Exception:
            # this is the correct path, it should throw when it doesn't find log2.
            pass

    def test_too_big(self):
        try:
            check_expected_backup("/var/log/juju/unit-fill-logs-0.log", big_obj)
            # log0 is too big, and thus should cause an exception.
            self.fail("Expected to get exception, but didn't.")
        except Exception:
            # this is the correct path, it should throw when it doesn't find log2.
            pass
