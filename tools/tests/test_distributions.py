from __future__ import annotations
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "tools"))
import build_distributions as build
import build_runtime


class AdapterTests(unittest.TestCase):
    def test_every_source_adapter_is_native_only_and_versioned(self):
        with tempfile.TemporaryDirectory() as temporary:
            for adapter in (*build.PLATFORMS, "cli"):
                with self.subTest(adapter=adapter):
                    target = Path(temporary) / adapter
                    build.build_package(target, adapter)
                    self.assertFalse((target / "lib").exists())
                    self.assertFalse((target / "requirements.txt").exists())
                    self.assertFalse(list(target.rglob("*.py")))
                    self.assertEqual((target / "VERSION").read_text().strip(), build.VERSION)
                    self.assertNotIn("HEALTHSYNC_PYTHON", (target / "bin/healthsync").read_text())
                    self.assertNotIn("HEALTHSYNC_PYTHON", (target / "bin/healthsync.ps1").read_text())

    def test_all_adapters_execute_identical_go_binary(self):
        tag = build_runtime.native_platform_tag()
        name = "healthsync.exe" if tag.startswith("windows") else "healthsync"
        runtime_root = Path(os.environ.get("HEALTHSYNC_TEST_RUNTIME_ROOT", ROOT / "build/runtime-go"))
        binary = runtime_root / tag / name
        if not binary.is_file():
            self.skipTest("Run python tools/build_runtime.py before native adapter tests")
        digest = hashlib.sha256(binary.read_bytes()).hexdigest()
        with tempfile.TemporaryDirectory(prefix="healthsync go adapters ") as temporary:
            for adapter in (*build.PLATFORMS, "cli"):
                with self.subTest(adapter=adapter):
                    target = Path(temporary) / adapter / "package with spaces"
                    build.build_package(target, adapter, runtime_root=runtime_root)
                    self.assertEqual(hashlib.sha256((target / "runtime" / tag / name).read_bytes()).hexdigest(), digest)
                    launcher = target / "bin" / ("healthsync.cmd" if os.name == "nt" else "healthsync")
                    def run(*args: str, expected: int = 0):
                        result = subprocess.run([str(launcher), *args], capture_output=True, text=True)
                        self.assertEqual(result.returncode, expected, result.stdout + result.stderr)
                        return result
                    self.assertEqual(run("--version").stdout.strip(), f"healthsync {build.VERSION}")
                    self.assertEqual(json.loads(run("self-test").stdout)["protocol"], 5)
                    for option in ("--rotate", "--protocol", "--storage", "--state-dir", "--offline"):
                        self.assertIn(option, run("onboard", "--help").stdout)
                    state = Path(temporary) / adapter / "private state"
                    run("onboard", "--offline", "--state-dir", str(state))
                    self.assertEqual(json.loads((state / "config/config.json").read_text())["protocol_version"], 5)
                    run("runtime", "verify")
                    run("onboard", "--protocol", "v4", expected=2)
                    (target / "VERSION").write_text("wrong-version\n")
                    run("--version", expected=78)

    def test_unbuilt_source_package_does_not_download_a_different_release(self):
        if os.name == "nt":
            self.skipTest("Covered by equivalent Windows launcher smoke tests")
        with tempfile.TemporaryDirectory() as temporary:
            package = Path(temporary)
            build.build_package(package, "cli")
            result = subprocess.run([str(package / "bin/healthsync"), "--version"], capture_output=True, text=True)
            self.assertEqual(result.returncode, 78)
            self.assertIn("no release manifest", result.stderr)


if __name__ == "__main__":
    unittest.main()
