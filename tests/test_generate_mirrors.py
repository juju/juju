import datetime
import json
from mock import patch
import os
from unittest import TestCase

from utils import temp_dir
from generate_mirrors import (
    generate_mirrors_file,
    generate_cpc_mirrors_file
)


class GenerateMirrors(TestCase):

    def test_generate_mirrors_file(self):
        updated = datetime.datetime.utcnow()
        with temp_dir() as stream_path:
            generate_mirrors_file(updated, stream_path)
            self.assertTrue(os.path.exists('%s/mirrors.json' % stream_path))
