import datetime
import json
from mock import patch
import os
from unittest import TestCase

from utils import temp_dir
from generate_mirrors import (
    generate_mirrors_file,
    generate_cpc_mirrors_file,
    PURPOSES,
)


class GenerateMirrors(TestCase):

    def test_generate_mirrors_file(self):
        updated = datetime.datetime.utcnow()
        with temp_dir() as stream_path:
            generate_mirrors_file(updated, stream_path)
            mirror_path = '%s/mirrors.json' % stream_path
            self.assertTrue(os.path.exists(mirror_path))
            with open(mirror_path) as mirror_file:
                data = json.load(mirror_file)
        self.assertEqual(['mirrors'], data.keys())
        expected_produts = sorted(
            'com.ubuntu.juju:%s:tools' % p for p in PURPOSES)
        self.assertEqual(expected_produts, sorted(data['mirrors'].keys()))
        expected_updated = updated.strftime('%Y%m%d')
        self.assertEqual(
            expected_updated,
            data['mirrors']['com.ubuntu.juju:released:tools'][0]['updated'])
