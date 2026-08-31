# Build and contribute

## Source layout

| Directory | Contents |
| --- | --- |
| `cli/` | Go CLI and tests; version in `internal/healthsync/version.go` |
| `src/skill/` | Shared agent instructions and configuration reference |
| `src/launchers/` | Shell, PowerShell and Windows launchers |
| `src/plugin/` | Plugin metadata and README templates |
| `tools/` | Build and validation scripts |
| `*/apple-health-sync/` | Generated packages; edit the sources above |

## Build and test

Install the Go version from `cli/go.mod` and Python 3.10+ for build scripts.
Run from the repository root:

```bash
go -C cli test -race ./...
go -C cli vet ./...
python3 tools/build_runtime.py --all
python3 tools/build_distributions.py
python3 tools/build_distributions.py --check
python3 tools/build_distributions.py --output-root build/local --runtime-root build/runtime-go --local-marketplaces
python3 -m unittest discover -s tools/tests -v
```

Binaries are written to `build/runtime-go/<platform>/`; runnable packages are in
`build/local/<integration>/apple-health-sync/`. Cross-compilation does not replace
native tests. CI runs the CLI and launcher checks on macOS, Windows and Linux.

Use `onboard --offline --state-dir <temporary-private-directory>` for an isolated
smoke test. Do not use real health data or existing identities in automated tests.
`onboard` without `--offline`, `fetch`, `unlink` and `network-diagnostics` contact
the production relay. Unit tests use synthetic data and mock or localhost servers.

## Prepare a release

1. Update `cli/internal/healthsync/version.go` and versioned download links in the CLI guide.
2. Build all five binaries, run native tests and regenerate the packages.
3. Generate download manifests from the exact release archives. Each manifest must
   pin the version, archive checksum and executable checksum for every platform.
4. Run `python3 tools/build_distributions.py --check --require-release` against
   the assembled repository. Keep the manifest files with the generated packages.
5. Publish matching GitHub release assets before directing users to the updated
   marketplace. Never replace the contents of an existing release version.

The generator preserves valid runtime manifests when refreshing package templates.
After a version change, replace them with manifests for the new release.
Release archives belong in GitHub Releases, not in Git.

Keep private keys, QR payloads, databases and user configuration out of commits.
Third-party notices are embedded in the executable and available through
`healthsync licenses`.
