#!/usr/bin/env python3
"""Regression tests for security-sensitive skill dependency constraints."""

from pathlib import Path
import unittest


SKILL_DIR = Path(__file__).resolve().parent.parent
REQUIREMENTS_PATH = SKILL_DIR / "requirements.txt"
SKILL_PATH = SKILL_DIR / "SKILL.md"
SAFE_CRYPTOGRAPHY_REQUIREMENT = "cryptography>=50.0.0,<51"
TRUSTSTORE_REQUIREMENT = "truststore==0.10.4"
CERTIFI_REQUIREMENT = "certifi==2026.7.22"


class RequirementsTests(unittest.TestCase):
    def test_cryptography_range_starts_at_fully_patched_release(self) -> None:
        cryptography_requirements = [
            line.strip()
            for line in REQUIREMENTS_PATH.read_text(encoding="utf-8").splitlines()
            if line.strip().startswith("cryptography")
        ]

        self.assertEqual(cryptography_requirements, [SAFE_CRYPTOGRAPHY_REQUIREMENT])

    def test_skill_uses_the_pinned_requirements_file_without_duplicating_the_range(self) -> None:
        skill_text = SKILL_PATH.read_text(encoding="utf-8")

        self.assertIn("python3 -m pip install -r requirements.txt", skill_text)
        self.assertNotIn(SAFE_CRYPTOGRAPHY_REQUIREMENT, skill_text)
        self.assertNotIn('metadata: {"openclaw"', skill_text)

    def test_tls_trust_dependencies_are_pinned(self) -> None:
        requirements = REQUIREMENTS_PATH.read_text(encoding="utf-8").splitlines()

        self.assertIn(TRUSTSTORE_REQUIREMENT, requirements)
        self.assertIn(CERTIFI_REQUIREMENT, requirements)


if __name__ == "__main__":
    unittest.main()
