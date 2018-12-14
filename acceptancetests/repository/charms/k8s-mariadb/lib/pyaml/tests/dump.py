# -*- coding: utf-8 -*-
from __future__ import unicode_literals, print_function

import itertools as it, operator as op, functools as ft
from collections import Mapping, OrderedDict, namedtuple
import os, sys, io, yaml, unittest

if sys.version_info.major > 2: unicode = str

try: import pyaml
except ImportError:
	sys.path.insert(1, os.path.join(__file__, *['..']*3))
	import pyaml


large_yaml = b'''
### Default (baseline) configuration parameters.
### DO NOT ever change this config, use -c commandline option instead!

# Note that this file is YAML, so YAML types can be used here, see http://yaml.org/type/
# For instance, large number can be specified as "10_000_000" or "!!float 10e6".

source:
  # Path or glob pattern (to match path) to backup, required
  path: # example: /srv/backups/weekly.*

  queue:
    # Path to intermediate backup queue-file (list of paths to upload), required
    path: # example: /srv/backups/queue.txt
    # Don't rebuild queue-file if it's newer than source.path
    check_mtime: true

  entry_cache:
    # Path to persistent db (sqlite) of remote directory nodes, required
    path: # example: /srv/backups/dentries.sqlite

  # How to pick a path among those matched by "path" glob
  pick_policy: alphasort_last # only one supported


destination:
  # URL of Tahoe-LAFS node webapi
  url: http://localhost:3456/uri

  result: # what to do with a cap (URI) of a resulting tree (with full backup)
    print_to_stdout: true
    # Append the entry to the specified file (creating it, if doesn't exists)
    # Example entry: "2012-10-10T23:12:43.904543 /srv/backups/weekly.2012-10-10 URI:DIR2-CHK:..."
    append_to_file: # example: /srv/backups/lafs_caps
    # Append the entry to specified tahoe-lafs directory (i.e. put it into that dir)
    append_to_lafs_dir: # example: URI:DIR2:...

  encoding:
    xz:
      enabled: true
      options: # see lzma.LZMAOptions, empty = module defaults
      min_size: 5120 # don't compress files smaller than 5 KiB (unless overidden in "path_filter")
      path_filter:
        # List of include/exclude regexp path-rules, similar to "filter" section below.
        # Same as with "filter", rules can be tuples with '+' or '-' (implied for strings) as first element.
        #  '+' will indicate that file is compressible, if it's size >= "min_size" option.
        #  Unlike "filter", first element of rule-tuple can also be a number,
        #   overriding "min_size" parameter for matched (by that rule) paths.
        # If none of the patterns match path, file is handled as if it was matched by '+' rule.

        - '\.(gz|bz2|t[gb]z2?|xz|lzma|7z|zip|rar)$'
        - '\.(rpm|deb|iso)$'
        - '\.(jpe?g|gif|png|mov|avi|ogg|mkv|webm|mp[34g]|flv|flac|ape|pdf|djvu)$'
        - '\.(sqlite3?|fossil|fsl)$'
        - '\.git/objects/[0-9a-f]+/[0-9a-f]+$'
        # - [500, '\.(txt|csv|log|md|rst|cat|(ba|z|k|c|fi)?sh|env)$']
        # - [500, '\.(cgi|py|p[lm]|php|c|h|[ce]l|lisp|hs|patch|diff|xml|xsl|css|x?html[45]?|js)$']
        # - [500, '\.(co?nf|cfg?|li?st|ini|ya?ml|jso?n|vg|tab)(\.(sample|default|\w+-new))?$']
        # - [500, '\.(unit|service|taget|mount|desktop|rules|rc|menu)$']
        # - [2000, '^/etc/']


http:
  request_pool_options:
    maxPersistentPerHost: 10
    cachedConnectionTimeout: 600
    retryAutomatically: true
  ca_certs_files: /etc/ssl/certs/ca-certificates.crt # can be a list
  debug_requests: false # insecure! logs will contain tahoe caps


filter:
  # Either tuples like "[action ('+' or '-'), regexp]" or just exclude-patterns (python
  #  regexps) to match relative (to source.path, starting with "/") paths to backup.
  # Patterns are matched against each path in order they're listed here.
  # Leaf directories are matched with the trailing slash
  #  (as with rsync) to be distinguishable from files with the same name.
  # If path doesn't match any regexp on the list, it will be included.
  #
  # Examples:
  #  - ['+', '/\.git/config$']   # backup git repository config files
  #  - '/\.git/'   # *don't* backup any repository objects
  #  - ['-', '/\.git/']   # exactly same thing as above (redundant)
  #  - '/(?i)\.?svn(/.*|ignore)$' # exclude (case-insensitive) svn (or .svn) paths and ignore-lists

  - '/(CVS|RCS|SCCS|_darcs|\{arch\})/$'
  - '/\.(git|hg|bzr|svn|cvs)(/|ignore|attributes|tags)?$'
  - '/=(RELEASE-ID|meta-update|update)$'


operation:
  queue_only: false # only generate upload queue file, don't upload anything
  reuse_queue: false # don't generate upload queue file, use existing one as-is
  disable_deduplication: false # make no effort to de-duplicate data (should still work on tahoe-level for files)

  # Rate limiting might be useful to avoid excessive cpu/net usage on nodes,
  #  and especially when uploading to rate-limited api's (like free cloud storages).
  # Only used when uploading objects to the grid, not when building queue file.
  # Format of each value is "interval[:burst]", where "interval" can be specified as rate (e.g. "1/3e5").
  # Simple token bucket algorithm is used. Empty values mean "no limit".
  # Examples:
  #   "objects: 1/10:50" - 10 objects per second, up to 50 at once (if rate was lower before).
  #   "objects: 0.1:50" - same as above.
  #   "objects: 10:20" - 1 object in 10 seconds, up to 20 at once.
  #   "objects: 5" - make interval between object uploads equal 5 seconds.
  #   "bytes: 1/3e6:50e6" - 3 MB/s max, up to 50 MB/s if connection was underutilized before.
  rate_limit:
    bytes: # limit on rate of *file* bytes upload, example: 1/3e5:20e6
    objects: # limit on rate of uploaded objects, example: 10:50


logging: # see http://docs.python.org/library/logging.config.html
  # "custom" level means WARNING/DEBUG/NOISE, depending on CLI options
  warnings: true # capture python warnings
  sql_queries: false # log executed sqlite queries (very noisy, caps will be there)

  version: 1
  formatters:
    basic:
      format: '%(asctime)s :: %(name)s :: %(levelname)s: %(message)s'
      datefmt: '%Y-%m-%d %H:%M:%S'
  handlers:
    console:
      class: logging.StreamHandler
      stream: ext://sys.stderr
      formatter: basic
      level: custom
    debug_logfile:
      class: logging.handlers.RotatingFileHandler
      filename: /srv/backups/debug.log
      formatter: basic
      encoding: utf-8
      maxBytes: 5242880 # 5 MiB
      backupCount: 2
      level: NOISE
  loggers:
    twisted:
      handlers: [console]
      level: 0
  root:
    level: custom
    handlers: [console]
'''

data = dict(
	path='/some/path',
	query_dump=OrderedDict([
		('key1', 'тест1'),
		('key2', 'тест2'),
		('key3', 'тест3'),
		('последний', None) ]),
	ids=OrderedDict(),
	a=[1,None,'asd', 'не-ascii'], b=3.5, c=None,
	asd=OrderedDict([('b', 1), ('a', 2)]) )
data['query_dump_clone'] = data['query_dump']
data['ids']['id в уникоде'] = [4, 5, 6]
data['ids']['id2 в уникоде'] = data['ids']['id в уникоде']
# data["'asd'\n!\0\1"] =OrderedDict([('b', 1), ('a', 2)]) <-- fails in many ways

data_str_multiline = dict(cert=(
	'-----BEGIN CERTIFICATE-----\n'
	'MIIDUjCCAjoCCQD0/aLLkLY/QDANBgkqhkiG9w0BAQUFADBqMRAwDgYDVQQKFAdm\n'
	'Z19jb3JlMRYwFAYDVQQHEw1ZZWthdGVyaW5idXJnMR0wGwYDVQQIExRTdmVyZGxv\n'
	'dnNrYXlhIG9ibGFzdDELMAkGA1UEBhMCUlUxEjAQBgNVBAMTCWxvY2FsaG9zdDAg\n'
	'Fw0xMzA0MjQwODUxMTRaGA8yMDUzMDQxNDA4NTExNFowajEQMA4GA1UEChQHZmdf\n'
	'Y29yZTEWMBQGA1UEBxMNWWVrYXRlcmluYnVyZzEdMBsGA1UECBMUU3ZlcmRsb3Zz\n'
	'a2F5YSBvYmxhc3QxCzAJBgNVBAYTAlJVMRIwEAYDVQQDEwlsb2NhbGhvc3QwggEi\n'
	'MA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQCnZr3jbhfb5bUhORhmXOXOml8N\n'
	'fAli/ak6Yv+LRBtmOjke2gFybPZFuXYr0lYGQ4KgarN904vEg7WUbSlwwJuszJxQ\n'
	'Lz3xSDqQDqF74m1XeBYywZQIywKIbA/rfop3qiMeDWo3WavYp2kaxW28Xd/ZcsTd\n'
	'bN/eRo+Ft1bor1VPiQbkQKaOOi6K8M9a/2TK1ei2MceNbw6YrlCZe09l61RajCiz\n'
	'y5eZc96/1j436wynmqJn46hzc1gC3APjrkuYrvUNKORp8y//ye+6TX1mVbYW+M5n\n'
	'CZsIjjm9URUXf4wsacNlCHln1nwBxUe6D4e2Hxh2Oc0cocrAipxuNAa8Afn5AgMB\n'
	'AAEwDQYJKoZIhvcNAQEFBQADggEBADUHf1UXsiKCOYam9u3c0GRjg4V0TKkIeZWc\n'
	'uN59JWnpa/6RBJbykiZh8AMwdTonu02g95+13g44kjlUnK3WG5vGeUTrGv+6cnAf\n'
	'4B4XwnWTHADQxbdRLja/YXqTkZrXkd7W3Ipxdi0bDCOSi/BXSmiblyWdbNU4cHF/\n'
	'Ex4dTWeGFiTWY2upX8sa+1PuZjk/Ry+RPMLzuamvzP20mVXmKtEIfQTzz4b8+Pom\n'
	'T1gqPkNEbe2j1DciRNUOH1iuY+cL/b7JqZvvdQK34w3t9Cz7GtMWKo+g+ZRdh3+q\n'
	'2sn5m3EkrUb1hSKQbMWTbnaG4C/F3i4KVkH+8AZmR9OvOmZ+7Lo=\n'
	'-----END CERTIFICATE-----' ))

data_str_long = dict(cert=(
	'MIIDUjCCAjoCCQD0/aLLkLY/QDANBgkqhkiG9w0BAQUFADBqMRAwDgYDVQQKFAdm'
	'Z19jb3JlMRYwFAYDVQQHEw1ZZWthdGVyaW5idXJnMR0wGwYDVQQIExRTdmVyZGxv'
	'dnNrYXlhIG9ibGFzdDELMAkGA1UEBhMCUlUxEjAQBgNVBAMTCWxvY2FsaG9zdDAg'
	'Fw0xMzA0MjQwODUxMTRaGA8yMDUzMDQxNDA4NTExNFowajEQMA4GA1UEChQHZmdf'
	'Y29yZTEWMBQGA1UEBxMNWWVrYXRlcmluYnVyZzEdMBsGA1UECBMUU3ZlcmRsb3Zz'
	'a2F5YSBvYmxhc3QxCzAJBgNVBAYTAlJVMRIwEAYDVQQDEwlsb2NhbGhvc3QwggEi'
	'MA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQCnZr3jbhfb5bUhORhmXOXOml8N'
	'fAli/ak6Yv+LRBtmOjke2gFybPZFuXYr0lYGQ4KgarN904vEg7WUbSlwwJuszJxQ'
	'Lz3xSDqQDqF74m1XeBYywZQIywKIbA/rfop3qiMeDWo3WavYp2kaxW28Xd/ZcsTd'
	'bN/eRo+Ft1bor1VPiQbkQKaOOi6K8M9a/2TK1ei2MceNbw6YrlCZe09l61RajCiz'
	'y5eZc96/1j436wynmqJn46hzc1gC3APjrkuYrvUNKORp8y//ye+6TX1mVbYW+M5n'
	'CZsIjjm9URUXf4wsacNlCHln1nwBxUe6D4e2Hxh2Oc0cocrAipxuNAa8Afn5AgMB'
	'AAEwDQYJKoZIhvcNAQEFBQADggEBADUHf1UXsiKCOYam9u3c0GRjg4V0TKkIeZWc'
	'uN59JWnpa/6RBJbykiZh8AMwdTonu02g95+13g44kjlUnK3WG5vGeUTrGv+6cnAf'
	'4B4XwnWTHADQxbdRLja/YXqTkZrXkd7W3Ipxdi0bDCOSi/BXSmiblyWdbNU4cHF/'
	'Ex4dTWeGFiTWY2upX8sa+1PuZjk/Ry+RPMLzuamvzP20mVXmKtEIfQTzz4b8+Pom'
	'T1gqPkNEbe2j1DciRNUOH1iuY+cL/b7JqZvvdQK34w3t9Cz7GtMWKo+g+ZRdh3+q'
	'2sn5m3EkrUb1hSKQbMWTbnaG4C/F3i4KVkH+8AZmR9OvOmZ+7Lo=' ))


# Restore Python2-like heterogeneous list sorting functionality in Python3
# Based on https://gist.github.com/pR0Ps/1e1a1e892aad5b691448
def compare(x, y):
	if x == y: return 0
	try:
		if x < y: return -1
		else: return 1

	except TypeError as e:
		# The case where both are None is taken care of by the equality test
		if x is None: return -1
		elif y is None: return 1

		if type(x) != type(y):
			return compare(*map(lambda t: type(t).__name__, [x, y]))

		# Types are the same but a native compare didn't work.
		# x and y might be indexable, recursively compare elements
		for a, b in zip(x, y):
			c = compare(a, b)
			if c != 0: return c

		return compare(len(x), len(y))


class DumpTests(unittest.TestCase):

	def flatten(self, data, path=tuple()):
		dst = list()
		if isinstance(data, (tuple, list)):
			for v in data:
				dst.extend(self.flatten(v, path + (list,)))
		elif isinstance(data, Mapping):
			for k,v in data.items():
				dst.extend(self.flatten(v, path + (k,)))
		else: dst.append((path, data))
		return tuple(sorted(dst, key=ft.cmp_to_key(compare)))

	def test_dst(self):
		buff = io.BytesIO()
		self.assertIs(pyaml.dump(data, buff), None)
		self.assertIsInstance(pyaml.dump(data, str), str)
		self.assertIsInstance(pyaml.dump(data, unicode), unicode)

	def test_simple(self):
		a = self.flatten(data)
		b = pyaml.dump(data, unicode)
		self.assertEqual(a, self.flatten(yaml.safe_load(b)))

	def test_vspacing(self):
		data = yaml.safe_load(large_yaml)
		a = self.flatten(data)
		b = pyaml.dump(data, unicode, vspacing=[2, 1])
		self.assertEqual(a, self.flatten(yaml.safe_load(b)))
		pos, pos_list = 0, list()
		while True:
			pos = b.find(u'\n', pos+1)
			if pos < 0: break
			pos_list.append(pos)
		self.assertEqual( pos_list,
			[ 12, 13, 25, 33, 53, 74, 89, 108, 158, 185, 265, 300, 345, 346, 356, 376, 400, 426, 427,
				460, 461, 462, 470, 508, 564, 603, 604, 605, 611, 612, 665, 666, 690, 691, 715, 748,
				777, 806, 807, 808, 817, 818, 832, 843, 878, 948, 949, 961, 974, 1009, 1032, 1052,
				1083, 1102, 1123, 1173, 1195, 1234, 1257, 1276, 1300, 1301, 1312, 1325, 1341, 1359,
				1374, 1375, 1383, 1397, 1413, 1431, 1432, 1453, 1454, 1467, 1468, 1485, 1486, 1487,
				1498, 1499, 1530, 1531, 1551, 1552, 1566, 1577, 1590, 1591, 1612, 1613, 1614, 1622,
				1623, 1638, 1648, 1649, 1657, 1658, 1688, 1689, 1698, 1720, 1730 ] )
		b = pyaml.dump(data, unicode)
		self.assertNotIn('\n\n', b)

	def test_ids(self):
		b = pyaml.dump(data, unicode)
		self.assertNotIn('&id00', b)
		self.assertIn('query_dump_clone: *query_dump_clone', b)
		self.assertIn("'id в уникоде': &ids_-_id2_v_unikode", b) # kinda bug - should be just "id"

	def test_force_embed(self):
		b = pyaml.dump(data, unicode, force_embed=True)
		c = pyaml.dump(data, unicode, safe=True, force_embed=True)
		for char, dump in it.product('*&', [b, c]):
			self.assertNotIn(char, dump)

	def test_encoding(self):
		b = pyaml.dump(data, unicode, force_embed=True)
		b_lines = list(map(unicode.strip, b.splitlines()))
		chk = ['query_dump:', 'key1: тест1', 'key2: тест2', 'key3: тест3', 'последний:']
		pos = b_lines.index('query_dump:')
		self.assertEqual(b_lines[pos:pos + len(chk)], chk)

	def test_str_long(self):
		b = pyaml.dump(data_str_long, unicode)
		self.assertNotIn('"', b)
		self.assertNotIn("'", b)
		self.assertEqual(len(b.splitlines()), 1)

	def test_str_multiline(self):
		b = pyaml.dump(data_str_multiline, unicode)
		b_lines = b.splitlines()
		self.assertGreater(len(b_lines), len(data_str_multiline['cert'].splitlines()))
		for line in b_lines: self.assertLess(len(line), 100)

	def test_dumps(self):
		b = pyaml.dumps(data_str_multiline)
		self.assertIsInstance(b, bytes)

	def test_print(self):
		self.assertIs(pyaml.print, pyaml.pprint)
		self.assertIs(pyaml.print, pyaml.p)
		buff = io.BytesIO()
		b = pyaml.dump(data_str_multiline, dst=bytes)
		pyaml.print(data_str_multiline, file=buff)
		self.assertEqual(b, buff.getvalue())

	def test_print_args(self):
		buff = io.BytesIO()
		args = 1, 2, 3
		b = pyaml.dump(args, dst=bytes)
		pyaml.print(*args, file=buff)
		self.assertEqual(b, buff.getvalue())

	def test_str_style_pick(self):
		a = pyaml.dump(data_str_multiline)
		b = pyaml.dump(data_str_multiline, string_val_style='|')
		self.assertEqual(a, b)
		b = pyaml.dump(data_str_multiline, string_val_style='plain')
		self.assertNotEqual(a, b)
		self.assertTrue(pyaml.dump('waka waka', string_val_style='|').startswith('|-\n'))
		self.assertEqual(pyaml.dump(dict(a=1), string_val_style='|'), 'a: 1\n')

	def test_colons_in_strings(self):
		val1 = {'foo': ['bar:', 'baz', 'bar:bazzo', 'a: b'], 'foo:': 'yak:'}
		val1_str = pyaml.dump(val1)
		val2 = yaml.safe_load(val1_str)
		val2_str = pyaml.dump(val2)
		val3 = yaml.safe_load(val2_str)
		self.assertEqual(val1, val2)
		self.assertEqual(val1_str, val2_str)
		self.assertEqual(val2, val3)

	def test_empty_strings(self):
		val1 = {'key': ['', 'stuff', '', 'more'], '': 'value', 'k3': ''}
		val1_str = pyaml.dump(val1)
		val2 = yaml.safe_load(val1_str)
		val2_str = pyaml.dump(val2)
		val3 = yaml.safe_load(val2_str)
		self.assertEqual(val1, val2)
		self.assertEqual(val1_str, val2_str)
		self.assertEqual(val2, val3)

	def test_single_dash_strings(self):
		strip_seq_dash = lambda line: line.lstrip().lstrip('-').lstrip()
		val1 = {'key': ['-', '-stuff', '- -', '- more-', 'more-', '--']}
		val1_str = pyaml.dump(val1)
		val2 = yaml.safe_load(val1_str)
		val2_str = pyaml.dump(val2)
		val3 = yaml.safe_load(val2_str)
		self.assertEqual(val1, val2)
		self.assertEqual(val1_str, val2_str)
		self.assertEqual(val2, val3)
		val1_str_lines = val1_str.splitlines()
		self.assertEqual(strip_seq_dash(val1_str_lines[2]), '-stuff')
		self.assertEqual(strip_seq_dash(val1_str_lines[5]), 'more-')
		self.assertEqual(strip_seq_dash(val1_str_lines[6]), '--')
		val1 = {'key': '-'}
		val1_str = pyaml.dump(val1)
		val2 = yaml.safe_load(val1_str)
		val2_str = pyaml.dump(val2)
		val3 = yaml.safe_load(val2_str)

	def test_namedtuple(self):
		TestTuple = namedtuple('TestTuple', 'y x z')
		val = TestTuple(1, 2, 3)
		val_str = pyaml.dump(val)
		self.assertEqual(val_str, u'y: 1\nx: 2\nz: 3\n') # namedtuple order was preserved

	def test_ordereddict(self):
		d = OrderedDict((i, '') for i in reversed(range(10)))
		lines = pyaml.dump(d).splitlines()
		self.assertEqual(lines, list(reversed(sorted(lines))))

	def test_pyyaml_params(self):
		d = {'foo': 'lorem ipsum ' * 30} # 300+ chars
		for w in 40, 80, 200:
			lines = pyaml.dump(d, width=w, indent=10).splitlines()
			for n, line in enumerate(lines, 1):
				self.assertLess(len(line), w*1.2)
				if n != len(lines): self.assertGreater(len(line), w*0.8)


if __name__ == '__main__':
	unittest.main()
	# print('-'*80)
	# pyaml.dump(yaml.safe_load(large_yaml), sys.stdout)
	# print('-'*80)
	# pyaml.dump(data, sys.stdout)
