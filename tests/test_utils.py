import os
from unittest import TestCase
from utils import (
    JujuSeries,
    Series,
    temp_dir,
)


SUPPORTED_RELEASES = """\
# This is the list of series that must be recognised for tools.
12.04 precise LTS
14.04 trusty LTS
14.10 utopic HISTORIC
15.04 vivid SUPPORTED
15.10 wily DEVEL
"""


def get_juju_series():
    with temp_dir() as base:
        data_path = os.path.join(base, 'sr.txt')
        with open(data_path, 'wb') as f:
            f.write(SUPPORTED_RELEASES)
        return JujuSeries(data_path=data_path)


class JujuSeriesTestCase(TestCase):

    def test_init(self):
        juju_series = JujuSeries()
        self.assertEqual(
            Series('14.10', 'utopic', 'HISTORIC'),
            juju_series.all['utopic'])

    def test_init_with_data_path(self):
        juju_series = get_juju_series()
        self.assertEqual(
            Series('14.10', 'utopic', 'HISTORIC'),
            juju_series.all['utopic'])
