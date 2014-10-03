from contextlib import contextmanager
#from mock import patch
import shutil
from tempfile import mkdtemp
from unittest import TestCase

from validate_streams import (
    parse_args,
)


@contextmanager
def temp_dir():
    dirname = mkdtemp()
    try:
        yield dirname
    finally:
        shutil.rmtree(dirname)
# with temp_dir() as base:


class ValidateStreams(TestCase):

    def test_parge_args(self):
        # The purpose, release, old json and new json are required
        required = ['proposed', '1.20.9', 'old/json', 'new/json']
        args = parse_args(required)
        self.assertEqual('proposed', args.purpose)
        self.assertEqual('1.20.9', args.release)
        self.assertEqual('old/json', args.old_json)
        self.assertEqual('new/json', args.new_json)
        # A bad release version can be retracted.
        args = parse_args(['--retracted', 'bad'] + required)
        self.assertEqual('bad', args.retracted)
