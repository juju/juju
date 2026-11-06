import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


CHECKER = Path(__file__).with_name("check_error_contract.py")


class FormattedWrapperTest(unittest.TestCase):
    def test_documented_chain_tracks_formatted_wrapper(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            source = root / "source"
            docs = root / "docs"
            source.mkdir()
            docs.mkdir()
            go = source / "errors.go"
            go.write_text('''package example
import "github.com/juju/errors"
import "fmt"
func example(err error, tag string) error {
    return errors.NewNotValid(err, fmt.Sprintf("cloud tag %q not valid", tag))
}
func reason() error { return fmt.Errorf("invalid cloud identifier") }
''')
            (docs / "defaults.md").write_text('''## Model default calls
### Setting model defaults
- *Errors:*
  - **`cloud tag "<tag>" not valid: invalid cloud identifier`**
''')
            command = [sys.executable, str(CHECKER), "--source", str(source),
                       "--docs", str(docs), "--modules", str(root)]
            result = subprocess.run(command, capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            go.write_text(go.read_text().replace('cloud tag %q not valid',
                                               'invalid cloud tag %q'))
            result = subprocess.run(command, capture_output=True, text=True)
            self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
            self.assertIn('UNMATCHED defaults.md:', result.stdout)


if __name__ == "__main__":
    unittest.main()
