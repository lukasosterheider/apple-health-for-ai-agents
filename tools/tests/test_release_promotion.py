#!/usr/bin/env python3

from __future__ import annotations

import sys
import unittest
from pathlib import Path


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


if __name__ == "__main__":
    unittest.main()
