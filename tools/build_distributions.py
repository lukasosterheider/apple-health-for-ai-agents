#!/usr/bin/env python3
"""Build all adapters from src; never install, publish or access the network."""

from __future__ import annotations

import argparse
import re
import hashlib
import json
import shutil
import tempfile
from pathlib import Path

PUBLIC_ROOT = Path(__file__).resolve().parents[1]
SOURCE_ROOT = PUBLIC_ROOT / "src"
SKILL_ROOT = SOURCE_ROOT / "skill"
GO_ROOT = PUBLIC_ROOT / "cli"
REPOSITORY = "lukasosterheider/apple-health-for-ai-agents"
PLATFORMS = ("codex", "claude", "openclaw", "generic-skill")


def source_version() -> str:
    match = re.search(r'const Version = "([^"]+)"', (GO_ROOT / "internal/healthsync/version.go").read_text())
    if not match:
        raise ValueError("Missing CLI version")
    return match.group(1)


VERSION = source_version()


def render_skill(platform: str, *, native_only: bool = True, source_text: str | None = None) -> str:
    text = (SKILL_ROOT / "SKILL.md").read_text() if source_text is None else source_text
    invocation = "sh {baseDir}/bin/healthsync.sh" if platform == "openclaw" else "bin/healthsync"
    instructions = (
        "- Resolve this skill's package root before running commands. "
        "In a plugin, it is two directories above this SKILL.md; in a standalone skill it contains this SKILL.md.\n"
        f"- Run `{invocation} <command>` on macOS/Linux or `bin\\healthsync.cmd <command>` on Windows. "
        "The examples below abbreviate that exact launcher as `healthsync`; do not substitute an unrelated executable on PATH.\n"
        "- The launcher uses the bundled native CLI when present. "
    )
    instructions += (
        "Otherwise it downloads the executable specified by this package's release manifest. "
        "Version and checksum verification are required. Python is not needed.\n"
    )
    text = text.replace(
        "- **Network:** Use only",
        f"- **Runtime installation:** The launcher may download its checksum-pinned executable from "
        f"`https://github.com/{REPOSITORY}/releases/download/plugin-v{VERSION}/` and follow GitHub HTTPS asset redirects. "
        "It sends no keys or health data.\n- **Network:** Use only",
    )
    if platform == "openclaw":
        instructions = instructions.replace("`bin\\healthsync.cmd <command>`", "`powershell -File bin/healthsync.ps1 <command>`")
    text = text.replace("<!-- runtime-instructions -->", instructions.rstrip())
    if platform == "openclaw":
        _, frontmatter, body = text.split("---", 2)
        description = next(line for line in frontmatter.splitlines() if line.startswith("description:"))
        metadata = {"openclaw": {
            "homepage": "https://gethealthsync.app/",
            "requires": {"bins": ["sh"]},
            "config": {"stateDirs": [".apple-health-sync"]},
        }}
        text = f"---\nname: apple-health-sync\n{description}\nmetadata: {json.dumps(metadata, separators=(',', ':'))}\n---{body}"
    return text


def write_json(path: Path, payload: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(payload, indent=2) + "\n")


def build_package(destination: Path, platform: str, *, native_only: bool = True,
                  runtime_root: Path | None = None) -> None:
    if platform not in (*PLATFORMS, "cli"):
        raise ValueError(f"Unknown platform: {platform}")
    destination.mkdir(parents=True, exist_ok=True)
    shutil.copytree(SOURCE_ROOT / "launchers", destination / "bin", dirs_exist_ok=True)
    (destination / "bin/healthsync").chmod(0o755)
    if platform == "openclaw":
        shutil.copy2(destination / "bin/healthsync", destination / "bin/healthsync.sh")
    (destination / "VERSION").write_text(VERSION + "\n")
    if runtime_root is not None:
        for platform_tag in ("darwin-arm64", "darwin-x64", "linux-arm64", "linux-x64", "windows-x64"):
            executable = "healthsync.exe" if platform_tag == "windows-x64" else "healthsync"
            source = runtime_root / platform_tag / executable
            if source.is_file():
                target = destination / "runtime" / platform_tag / executable
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(source, target)
                target.chmod(0o755)
    if platform == "cli":
        return
    plugin = platform in ("codex", "claude")
    skill_destination = destination / "skills/apple-health-sync" if plugin else destination
    skill_destination.mkdir(parents=True, exist_ok=True)
    (skill_destination / "SKILL.md").write_text(render_skill(platform, native_only=native_only))
    shutil.copytree(SKILL_ROOT / "references", skill_destination / "references", dirs_exist_ok=True)
    if plugin:
        for path in (SOURCE_ROOT / "plugin").rglob("*"):
            if not path.is_file():
                continue
            target = destination / path.relative_to(SOURCE_ROOT / "plugin")
            if path.suffix == ".json":
                payload = json.loads(path.read_text())
                payload["version"] = VERSION
                write_json(target, payload)
            else:
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(path, target)


def files(root: Path) -> dict[str, str]:
    return {
        path.relative_to(root).as_posix(): hashlib.sha256(path.read_bytes()).hexdigest()
        for path in root.rglob("*")
        if path.is_file() and not any(part in ("runtime", "runtime-downloads", "__pycache__")
                                      for part in path.relative_to(root).parts)
    }


def check_runtime_manifest(package: Path, *, require_release: bool = False) -> None:
    """Release-owned checksums cannot be rebuilt from source without native artifacts."""
    directory = package / "runtime-downloads"
    try:
        manifest = json.loads((directory / "manifest.json").read_text())
        if manifest["pluginVersion"] != VERSION or manifest["runtimeVersion"] != VERSION:
            raise ValueError("runtime and source versions differ")
        if manifest["schemaVersion"] != 1 or set(manifest["artifacts"]) != {"darwin-arm64", "darwin-x64", "linux-arm64", "linux-x64", "windows-x64"}:
            raise ValueError("all five runtime artifacts are required")
        lines = []
        for platform, artifact in sorted(manifest["artifacts"].items()):
            for field in ("sha256", "executableSha256"):
                checksum = artifact[field]
                if len(checksum) != 64 or any(char not in "0123456789abcdef" for char in checksum):
                    raise ValueError("invalid runtime checksum")
            if require_release:
                archive = "zip" if platform.startswith("windows-") else "tar.gz"
                executable = "healthsync.exe" if platform.startswith("windows-") else "healthsync"
                expected_url = f"https://github.com/{REPOSITORY}/releases/download/plugin-v{VERSION}/apple-health-sync-runtime-{VERSION}-{platform}.{archive}"
                if artifact["url"] != expected_url or artifact["archive"] != archive or artifact["executable"] != executable:
                    raise ValueError(f"unexpected runtime download target: {platform}")
            lines.append("|".join([platform, VERSION, artifact["url"], artifact["sha256"],
                                   artifact["executableSha256"], artifact["archive"], artifact["executable"]]))
        if (directory / "manifest.tsv").read_text() != "\n".join(lines) + "\n":
            raise ValueError("JSON and TSV runtime manifests differ")
    except (OSError, ValueError, KeyError, TypeError, AttributeError) as error:
        raise SystemExit(f"Invalid runtime manifest in {package}: {error}") from error


def check_release_metadata(root: Path) -> None:
    try:
        marketplace = json.loads((root / "claude/marketplace.json").read_text())
        source = marketplace["plugins"][0]["source"]
        if marketplace["name"] != "healthsync" or marketplace["version"] != VERSION or source != {
            "source": "npm", "package": "apple-health-sync-agent-plugin", "version": VERSION,
            "registry": f"https://raw.githubusercontent.com/{REPOSITORY}/main/claude/npm/",
        }:
            raise ValueError("Claude marketplace does not match the release version")
        npm = json.loads((root / "claude/npm/apple-health-sync-agent-plugin").read_text())
        package = npm["versions"][VERSION]
        expected = f"https://github.com/{REPOSITORY}/releases/download/plugin-v{VERSION}/apple-health-sync-agent-plugin-{VERSION}.tgz"
        if npm["dist-tags"]["latest"] != VERSION or package["dist"]["tarball"] != expected or not package["dist"]["integrity"].startswith("sha512-"):
            raise ValueError("Claude package metadata does not match the release archive")
        codex = json.loads((root / ".agents/plugins/marketplace.json").read_text())
        if codex["name"] != "healthsync" or codex["plugins"][0]["source"] != {"source": "local", "path": "./codex/apple-health-sync"}:
            raise ValueError("Codex marketplace must point to the generated Codex package")
    except (OSError, ValueError, KeyError, TypeError, AttributeError) as error:
        raise SystemExit(f"Invalid release metadata: {error}") from error


def generate(output_root: Path = PUBLIC_ROOT, *, check: bool = False,
             runtime_root: Path | None = None, require_release: bool = False) -> None:
    if require_release and not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", VERSION):
        raise SystemExit("A release requires a stable major.minor.patch version")
    with tempfile.TemporaryDirectory(prefix="healthsync-adapters-") as temporary:
        for platform in (*PLATFORMS, "cli"):
            relative = Path(platform) / "apple-health-sync"
            staged = Path(temporary) / relative
            target = output_root / relative
            # Release-owned hashes are checked separately from source templates.
            native_only = True
            manifest_directory = target / "runtime-downloads"
            if check and require_release and not manifest_directory.is_dir():
                raise SystemExit(f"Missing release runtime manifest: {relative}")
            if manifest_directory.exists():
                check_runtime_manifest(target, require_release=require_release)
            build_package(staged, platform, native_only=native_only, runtime_root=runtime_root)
            if manifest_directory.exists():
                shutil.copytree(manifest_directory, staged / "runtime-downloads")
            if check:
                if files(staged) != files(target):
                    raise SystemExit(f"Generated package is stale: {relative}. Run python3 tools/build_distributions.py")
            else:
                # Each destination is owned entirely by this generator; platform READMEs remain untouched.
                if target.exists():
                    shutil.rmtree(target)
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copytree(staged, target)
    if require_release:
        check_release_metadata(output_root)
    print("Generated adapters are current." if check else f"Built CLI and four adapters in {output_root}")


def local_marketplaces(output_root: Path) -> None:
    if output_root.resolve() == PUBLIC_ROOT:
        raise ValueError("Use a separate --output-root for local test marketplaces")
    write_json(output_root / ".agents/plugins/marketplace.json", {
        "name": "healthsync-local",
        "interface": {"displayName": "Health Sync development"},
        "plugins": [{"name": "apple-health-sync", "source": {"source": "local", "path": "./codex/apple-health-sync"},
                     "policy": {"installation": "AVAILABLE", "authentication": "ON_USE"}, "category": "Productivity"}],
    })
    write_json(output_root / ".claude-plugin/marketplace.json", {
        "name": "healthsync-local", "owner": {"name": "Lukas Osterheider"},
        "plugins": [{"name": "apple-health-sync", "source": "./claude/apple-health-sync", "version": VERSION}],
    })


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true")
    parser.add_argument("--require-release", action="store_true", help="Require a stable version and complete download manifests when checking packages")
    parser.add_argument("--output-root", type=Path, default=PUBLIC_ROOT)
    parser.add_argument("--runtime-root", type=Path)
    parser.add_argument("--local-marketplaces", action="store_true")
    args = parser.parse_args()
    if args.require_release and not args.check:
        parser.error("--require-release must be combined with --check")
    if args.local_marketplaces and args.check:
        parser.error("--local-marketplaces cannot be combined with --check")
    if args.local_marketplaces and args.output_root.resolve() == PUBLIC_ROOT:
        parser.error("Use a separate --output-root for local test marketplaces")
    generate(args.output_root.resolve(), check=args.check, runtime_root=args.runtime_root, require_release=args.require_release)
    if args.local_marketplaces:
        local_marketplaces(args.output_root.resolve())
