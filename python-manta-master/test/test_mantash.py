#!/usr/bin/env python
# Copyright (c) 2012 Joyent, Inc.  All rights reserved.

"""Test mantash."""

import os
import sys
import re
from posixpath import join as ujoin
from pprint import pprint
import unittest

from testlib import TestError, TestSkipped, tag

from common import MantaTestCase, stor
import manta



#---- globals

TDIR = "tmp/test_mantash"



#---- test cases

class OptionsTestCase(MantaTestCase):
    def test_version(self):
        code, stdout, stderr = self.mantash(['--version'])
        self.assertTrue(re.compile("^mantash \d+\.\d+\.\d+$").search(stdout))
        self.assertEqual(stderr, "")
        self.assertEqual(code, 0)

    def test_help(self):
        code, stdout, stderr = self.mantash(['help'])
        self.assertTrue("mantash help" in stdout)
        self.assertEqual(stderr, "")
        self.assertEqual(code, 0)
        code, stdout, stderr = self.mantash(['--help'])
        self.assertTrue("mantash help" in stdout)
        self.assertEqual(stderr, "")
        self.assertEqual(code, 0)
        code, stdout, stderr = self.mantash(['-h'])
        self.assertTrue("mantash help" in stdout)
        self.assertEqual(stderr, "")
        self.assertEqual(code, 0)

class FindTestCase(MantaTestCase):
    def setUp(self):
        self.client = self.get_client()
        self.base = b = ujoin(TDIR, 'find')
        self.client.mkdirp(stor(b, "dir1/dir2"))
        self.client.put(stor(b, "dir1/obj1.txt"), "this is obj1")

    def test_empty(self):
        code, stdout, stderr = self.mantash(['-C', self.base, 'find'])
        self.assertTrue("obj1.txt" in stdout)
        self.assertTrue("dir2" in stdout)
        self.assertEqual(code, 0)

    def test_type(self):
        code, stdout, stderr = self.mantash(['-C', self.base, 'find', '-type', 'f'])
        self.assertTrue("obj1.txt" in stdout)
        self.assertTrue("dir2" not in stdout)
        self.assertEqual(code, 0)

        code, stdout, stderr = self.mantash(['-C', self.base, 'find', '-type', 'o'])
        self.assertTrue("obj1.txt" in stdout)
        self.assertTrue("dir2" not in stdout)
        self.assertEqual(code, 0)

        code, stdout, stderr = self.mantash(['-C', self.base, 'find', '-type', 'd'])
        self.assertTrue("obj1.txt" not in stdout)
        self.assertTrue("dir2" in stdout)
        self.assertEqual(code, 0)

class LsTestCase(MantaTestCase):
    def setUp(self):
        self.client = self.get_client()
        self.base = b = ujoin(TDIR, 'ls')
        self.client.mkdirp(stor(b, "a1"))
        self.client.put(stor(b, "a1/a2.txt"), "this is a1/a2.txt")
        self.client.mkdirp(stor(b, "a1/b2"))
        self.client.put(stor(b, "a1/b2/a3.txt"), "this is a1/b2/a3.txt")
        self.client.mkdirp(stor(b, "b1"))
        self.client.put(stor(b, "c1.txt"), "this is c1.txt")

    def test_bare(self):
        code, stdout, stderr = self.mantash(['-C', self.base, 'ls'])
        self.assertEqual(stdout, 'a1\nb1\nc1.txt\n')
        self.assertEqual(code, 0)

        code, stdout, stderr = self.mantash(['-C', self.base, 'ls', '-l'])
        lines = stdout.splitlines()
        self.assertEqual(len(lines), 3)
        for line in lines:
            self.assertTrue(self.account in line)
        self.assertEqual(code, 0)

        code, stdout, stderr = self.mantash(['-C', self.base, 'ls', '-F'])
        self.assertEqual(stdout, 'a1/\nb1/\nc1.txt\n')
        self.assertEqual(code, 0)

    def test_dir(self):
        code, stdout, stderr = self.mantash(['-C', self.base, 'ls', 'a1'])
        self.assertEqual(stdout, 'a2.txt\nb2\n')
        self.assertEqual(code, 0)

    def test_dirstar(self):
        code, stdout, stderr = self.mantash(['-C', self.base, 'ls', 'a1/*'])
        self.assertEqual(stdout, 'a1/a2.txt\n\na1/b2:\na3.txt\n')
        self.assertEqual(code, 0)
