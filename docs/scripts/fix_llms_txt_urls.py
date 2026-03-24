#!/usr/bin/env python3
"""
Post-processing script to convert relative paths in llms.txt to absolute URLs.

This script is run after the Sphinx build to ensure LLM consumers can fetch
documentation pages without having to infer the base URL.

Usage:
    python3 scripts/fix_llms_txt_urls.py _build/llms.txt [base_url]

The script determines the version in the following order:
1. Explicit base_url argument (if provided)
2. READTHEDOCS_VERSION environment variable (on ReadTheDocs)
3. Auto-detected from ../version/version.go (for local builds)
4. Falls back to 'latest' if version cannot be determined
"""

import os
import re
import sys
from pathlib import Path


def get_juju_version():
    """
    Detect the Juju version from version/version.go.
    Returns major.minor version (e.g., '3.6') or None if not found.
    """
    try:
        # Look for version.go relative to the docs directory
        version_file = Path(__file__).parent.parent.parent / 'version' / 'version.go'
        if version_file.exists():
            content = version_file.read_text()
            # Look for: const version = "3.6.20"
            match = re.search(r'const version = "(\d+\.\d+)\.\d+"', content)
            if match:
                return match.group(1)
    except Exception:
        pass
    return None


def convert_llms_txt_to_absolute_urls(file_path, base_url):
    """
    Convert all relative markdown links in llms.txt to absolute URLs.

    Args:
        file_path: Path to the llms.txt file
        base_url: Base URL for the documentation (e.g., 'https://documentation.ubuntu.com/juju/latest/')
    """
    file_path = Path(file_path)

    if not file_path.exists():
        print(f"Error: File {file_path} does not exist", file=sys.stderr)
        return False

    # Read the file
    content = file_path.read_text(encoding='utf-8')

    # Function to convert relative path to absolute URL
    def convert_path(match):
        title = match.group(1)
        rel_path = match.group(2)

        # Skip if URL is already absolute
        if rel_path.startswith(('http://', 'https://', '/')):
            return match.group(0)  # Return unchanged

        # Build absolute URL by prepending base URL to relative path
        # Keep .md extension and anchors intact for AI-friendliness
        absolute_url = base_url + rel_path

        return f'[{title}]({absolute_url})'

    # Replace all markdown links with relative paths
    # Pattern matches: [Title](path.md) or [Title](path.md#anchor)
    # Must contain .md to be processed
    pattern = r'\[([^\]]+)\]\(([^)]*\.md[^)]*)\)'

    # Count conversions to be made
    original_count = len(re.findall(pattern, content))

    # Perform replacements
    new_content = re.sub(pattern, convert_path, content)

    # Verify conversions worked
    absolute_url_pattern = r'\]\(https?://.*\.md'
    converted_count = len(re.findall(absolute_url_pattern, new_content))

    if converted_count < original_count:
        print(f"Warning: Only {converted_count}/{original_count} conversions verified", file=sys.stderr)

    # Write back to file
    file_path.write_text(new_content, encoding='utf-8')

    print(f"Successfully converted {original_count} relative paths to absolute URLs in {file_path}")
    return True


if __name__ == '__main__':
    if len(sys.argv) < 2:
        print("Usage: python3 fix_llms_txt_urls.py <llms.txt_path> [base_url]", file=sys.stderr)
        sys.exit(1)

    llms_txt_path = sys.argv[1]

    # Determine base URL
    if len(sys.argv) >= 3:
        # Use explicitly provided base URL
        base_url = sys.argv[2]
    elif 'READTHEDOCS_VERSION' in os.environ:
        # Use ReadTheDocs version (e.g., '3.6', 'latest', etc.)
        version = os.environ['READTHEDOCS_VERSION']
        base_url = f'https://documentation.ubuntu.com/juju/{version}/'
    else:
        # Try to detect version from version.go
        detected_version = get_juju_version()
        if detected_version:
            base_url = f'https://documentation.ubuntu.com/juju/{detected_version}/'
        else:
            # Fall back to 'latest' if version cannot be detected
            base_url = 'https://documentation.ubuntu.com/juju/latest/'

    # Ensure base URL ends with /
    if not base_url.endswith('/'):
        base_url += '/'

    print(f"Converting relative paths to absolute URLs with base: {base_url}")
    success = convert_llms_txt_to_absolute_urls(llms_txt_path, base_url)
    sys.exit(0 if success else 1)
