"""Tests for perf_graphing support module."""

import perf_graphing as pg
from tests import TestCase


class TestMongoStatsData(TestCase):

    def test_converts_Mvalues_to_bytes(self):
        test_stats = pg.MongoStatsData(None, '0', '0', '0', '0', '1M', '1M')

        self.assertEqual(test_stats.vsize, 1048576)
        self.assertEqual(test_stats.res, 1048576)

    def test_removes_star_indicators(self):
        test_stats = pg.MongoStatsData(None, '*1', '*1', '*1', '*1', '0', '0')
        self.assertEqual(test_stats.insert, 1)
        self.assertEqual(test_stats.query, 1)
        self.assertEqual(test_stats.update, 1)
        self.assertEqual(test_stats.delete, 1)

    def test_stores_timestamp(self):
        test_stats = pg.MongoStatsData(1234, '0', '0', '0', '0', '0', '0')
        self.assertEqual(test_stats.timestamp, 1234)
