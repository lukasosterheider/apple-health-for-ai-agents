#!/usr/bin/env python3

from __future__ import annotations

import sys
import unittest
from pathlib import Path
from unittest import mock


TOOLS_ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(TOOLS_ROOT))

import check_release_promotion as promotion


class ReleasePromotionTests(unittest.TestCase):
    VERSION = "2.0.0"

    def release(self) -> dict:
        return {
            "tag_name": f"plugin-v{self.VERSION}",
            "draft": False,
            "prerelease": False,
            "assets": [
                {"name": name} for name in promotion.required_asset_names(self.VERSION)
            ],
        }

    def test_complete_published_release_is_accepted(self) -> None:
        promotion.validate_release(self.release(), self.VERSION)

    def test_draft_release_is_rejected(self) -> None:
        release = self.release()
        release["draft"] = True

        with self.assertRaisesRegex(RuntimeError, "published stable release"):
            promotion.validate_release(release, self.VERSION)

    def test_missing_asset_is_rejected(self) -> None:
        release = self.release()
        release["assets"].pop()

        with self.assertRaisesRegex(RuntimeError, "missing required assets"):
            promotion.validate_release(release, self.VERSION)

    @mock.patch("check_release_promotion.subprocess.run")
    def test_source_check_excludes_generated_cli_package(self, run: mock.Mock) -> None:
        run.return_value.returncode = 0

        promotion.validate_release_source("plugin-v2.0.0", Path("checkout"))

        command = run.call_args.args[0]
        self.assertIn(":(exclude)cli/apple-health-sync", command)
        self.assertIn("cli", command)
        self.assertIn("src", command)
        self.assertIn("tools", command)

    @mock.patch("check_release_promotion.subprocess.run")
    def test_source_drift_is_rejected(self, run: mock.Mock) -> None:
        run.return_value.returncode = 1

        with self.assertRaisesRegex(RuntimeError, "Build inputs differ"):
            promotion.validate_release_source("plugin-v2.0.0")


if __name__ == "__main__":
    unittest.main()
