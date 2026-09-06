#!/usr/bin/env python3
"""Build standalone Go executables. No installs, relay calls or publishing by default."""
from __future__ import annotations

import argparse
import hashlib
import json
import os
import platform
from pathlib import Path
import shutil
import subprocess

from build_distributions import VERSION

PUBLIC_ROOT = Path(__file__).resolve().parents[1]
DEFAULT_OUTPUT_ROOT = PUBLIC_ROOT / "build/runtime-go"
SUPPORTED_PLATFORM_TAGS = {"darwin-arm64", "darwin-x64", "linux-arm64", "linux-x64", "windows-x64"}
RUNTIME_VERSION = VERSION


def native_platform_tag(system: str | None = None, machine: str | None = None) -> str:
    system = (system or platform.system()).lower()
    machine = (machine or platform.machine()).lower()
    aliases = {"aarch64": "arm64", "arm64": "arm64", "amd64": "x64", "x86_64": "x64"}
    tag = f"{system}-{aliases.get(machine, machine)}"
    if tag not in SUPPORTED_PLATFORM_TAGS:
        raise RuntimeError(f"Unsupported build platform: {system}/{machine}")
    return tag


def go_binary() -> str:
    selected = os.environ.get("GO_BINARY") or shutil.which("go")
    if selected:
        return selected
    local = PUBLIC_ROOT / "build/toolchains/go/bin/go"
    if local.is_file():
        return str(local)
    raise RuntimeError("Go is required to build, but not to run the CLI. Install the Go version in cli/go.mod or set GO_BINARY.")


def sha256_file(path: Path) -> str:
    with path.open("rb") as source:
        digest = hashlib.sha256()
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
        return digest.hexdigest()


def build_runtime(output_root: Path = DEFAULT_OUTPUT_ROOT, verify: bool = True,
                  network_test: bool = False, platform_tag: str | None = None) -> dict[str, Path | str]:
    platform_tag = platform_tag or native_platform_tag()
    if platform_tag not in SUPPORTED_PLATFORM_TAGS:
        raise ValueError(f"Unsupported target: {platform_tag}")
    native = platform_tag == native_platform_tag()
    if network_test and (not native or not verify):
        raise ValueError("Network diagnostics require a verified native build")
    goos, arch = platform_tag.split("-")
    executable_name = "healthsync.exe" if goos == "windows" else "healthsync"
    artifact_directory = output_root.resolve() / platform_tag
    artifact_directory.mkdir(parents=True, exist_ok=True)
    executable = artifact_directory / executable_name
    environment = dict(os.environ, CGO_ENABLED="0", GOOS=goos,
                       GOARCH="amd64" if arch == "x64" else arch, GOTOOLCHAIN="local")
    environment.setdefault("GOPATH", str(PUBLIC_ROOT / "build/go"))
    environment.setdefault("GOCACHE", str(PUBLIC_ROOT / "build/go-cache"))
    subprocess.run([go_binary(), "build", "-mod=readonly", "-trimpath", "-buildvcs=false",
                    "-ldflags=-s -w", "-o", str(executable), "./cmd/healthsync"],
                   cwd=PUBLIC_ROOT / "cli", env=environment, check=True)
    if os.name != "nt":
        executable.chmod(0o755)
    if verify and native:
        verification_environment = dict(os.environ)
        for key in ("SSL_CERT_FILE", "SSL_CERT_DIR", "HEALTHSYNC_EXPECTED_VERSION"):
            verification_environment.pop(key, None)
        def run(*arguments: str) -> str:
            return subprocess.run([str(executable), *arguments], capture_output=True, text=True,
                                  check=True, env=verification_environment).stdout
        if run("--version").strip() != f"healthsync {VERSION}":
            raise RuntimeError("Unexpected CLI version")
        result = json.loads(run("self-test"))
        if result.get("ok") is not True or result.get("protocol") != 5 or result["tls"].get("bundled_ca_loaded") is not True:
            raise RuntimeError("Go CLI self-test failed")
        if network_test and json.loads(run("network-diagnostics")).get("ok") is not True:
            raise RuntimeError("Verified relay HTTPS test failed")
    checksum = artifact_directory / f"{executable_name}.sha256"
    checksum.write_text(f"{sha256_file(executable)}  {executable_name}\n")
    return {"platform": platform_tag, "executable": executable, "checksum": checksum,
            "verification": "native offline self-test" if verify and native else "cross-compiled; execute on target before release"}


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output-root", type=Path, default=DEFAULT_OUTPUT_ROOT)
    targets = parser.add_mutually_exclusive_group()
    targets.add_argument("--platform", choices=sorted(SUPPORTED_PLATFORM_TAGS))
    targets.add_argument("--all", action="store_true", help="Cross-compile all five targets; execute tests only on this host")
    parser.add_argument("--no-verify", action="store_true")
    parser.add_argument("--network-test", action="store_true", help="Explicitly probe production relay HTTPS on this host")
    args = parser.parse_args()
    if args.network_test and args.all:
        parser.error("--network-test cannot be combined with --all")
    for target in sorted(SUPPORTED_PLATFORM_TAGS) if args.all else [args.platform]:
        for key, value in build_runtime(args.output_root, not args.no_verify, args.network_test, target).items():
            print(f"{key}: {value}", flush=True)


if __name__ == "__main__":
    main()
