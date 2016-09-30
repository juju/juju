"""Tests for assess_perf_test_simple module."""

from datetime import datetime, timedelta

import logbreakdown as lb
from generate_perfscale_results import TimingData
from tests import (
    TestCase,
)


class TestExtractDateFromLine(TestCase):

    def test_returns_just_date(self):
        example_line = '2016-09-01 02:51:31 INFO juju.apiserver ...'
        self.assertEqual(
            '2016-09-01 02:51:31', lb.extract_date_from_line(example_line))


class TestLogLineWithinStartRange(TestCase):

    def test_returns_false_when_line_isnt_dated(self):
        line = 'Warning some undated line.'
        self.assertFalse(lb.log_line_within_start_range(line, None))

    def test_returns_true_when_line_is_after_start_date(self):
        line = '2016-09-01 02:51:31 INFO testing.'
        end_date = datetime.strptime('2016-09-01 02:51:20', lb.dt_format)
        self.assertTrue(
            lb.log_line_within_start_range(line, end_date))

    def test_returns_true_when_line_is_same_as_start_date(self):
        line = '2016-09-01 02:51:31 INFO testing.'
        end_date = datetime.strptime('2016-09-01 02:51:31', lb.dt_format)
        self.assertTrue(
            lb.log_line_within_start_range(line, end_date))

    def test_returns_false_when_line_is_newer_then_start_date(self):
        line = '2016-09-01 02:51:20 INFO testing.'
        end_date = datetime.strptime('2016-09-01 02:51:31', lb.dt_format)
        self.assertFalse(
            lb.log_line_within_start_range(line, end_date))


class TestLogLineWithinEndRange(TestCase):

    def test_returns_true_when_line_isnt_dated(self):
        line = 'Warning some undated line.'
        self.assertTrue(lb.log_line_within_end_range(line, None))

    def test_returns_true_when_line_is_before_end_date(self):
        line = '2016-09-01 02:51:31 INFO testing.'
        end_date = datetime.strptime('2016-09-01 02:51:35', lb.dt_format)
        self.assertTrue(
            lb.log_line_within_end_range(line, end_date))

    def test_returns_true_when_line_is_same_as_end_date(self):
        line = '2016-09-01 02:51:31 INFO testing.'
        end_date = datetime.strptime('2016-09-01 02:51:31', lb.dt_format)
        self.assertTrue(
            lb.log_line_within_end_range(line, end_date))

    def test_returns_false_when_line_is_older_then_end_date(self):
        line = '2016-09-01 02:51:31 INFO testing.'
        end_date = datetime.strptime('2016-09-01 02:51:20', lb.dt_format)
        self.assertFalse(
            lb.log_line_within_end_range(line, end_date))


class TestChunkEventRange(TestCase):
    def test_returns_start_and_end_when_period_less_than_time_step(self):
        """If the start and end period is less than the default step (20
        seconds) only the start and end should be considered.
        """
        start = datetime.utcnow()
        end = start + timedelta(seconds=5)
        event = TimingData(start, end)

        start_dt = datetime.strptime(event.start, lb.dt_format)
        end_dt = datetime.strptime(event.end, lb.dt_format)

        self.assertEqual(lb._chunk_event_range(event), [(start_dt, end_dt)])

    def test_chunks_event_spanning_multiple_periods(self):
        start = datetime.utcnow().replace(microsecond=0)
        end = start + timedelta(seconds=2*lb.LOG_BREAKDOWN_SECONDS)
        event = TimingData(start, end)

        first_end = start + timedelta(seconds=lb.LOG_BREAKDOWN_SECONDS)
        second_start = start + timedelta(seconds=lb.LOG_BREAKDOWN_SECONDS + 1)

        expected_chunks = [(start, first_end), (second_start, end)]
        self.assertEqual(
            lb._chunk_event_range(event),
            expected_chunks)
