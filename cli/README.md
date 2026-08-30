# Health Sync CLI

Pair your iPhone, fetch Apple Health data and create summaries from your terminal.
The CLI supports v5 and runs without Python, Go or other runtime dependencies.

## Install

Download and extract the archive for your system:

| System | Download |
| --- | --- |
| macOS 13+, Apple Silicon | [ARM64](https://github.com/lukasosterheider/apple-health-for-ai-agents/releases/download/plugin-v2.0.0/apple-health-sync-runtime-2.0.0-darwin-arm64.tar.gz) |
| macOS 13+, Intel | [x64](https://github.com/lukasosterheider/apple-health-for-ai-agents/releases/download/plugin-v2.0.0/apple-health-sync-runtime-2.0.0-darwin-x64.tar.gz) |
| Linux, Intel/AMD | [x64](https://github.com/lukasosterheider/apple-health-for-ai-agents/releases/download/plugin-v2.0.0/apple-health-sync-runtime-2.0.0-linux-x64.tar.gz) |
| Linux, ARM64 | [ARM64](https://github.com/lukasosterheider/apple-health-for-ai-agents/releases/download/plugin-v2.0.0/apple-health-sync-runtime-2.0.0-linux-arm64.tar.gz) |
| Windows 10+, Intel/AMD | [x64](https://github.com/lukasosterheider/apple-health-for-ai-agents/releases/download/plugin-v2.0.0/apple-health-sync-runtime-2.0.0-windows-x64.zip) |

The [release page](https://github.com/lukasosterheider/apple-health-for-ai-agents/releases/tag/plugin-v2.0.0)
includes `SHA256SUMS` to verify downloads. Place `healthsync` or `healthsync.exe`
in a directory on your `PATH`. No other files are needed beside the executable.
Without changing `PATH`, use `./healthsync` on macOS/Linux or `.\healthsync.exe`
in PowerShell. Respect your operating system's download security prompts.

## Start onboarding and Connect your iPhone

```bash
healthsync onboard
```

Open the QR image shown in the output. In the
[Health Sync iPhone app](https://apps.apple.com/app/health-sync-for-openclaw/id6759522298),
choose **QR Code Setup**, scan the code, grant Apple Health permissions and complete
your first sync. Then run:

```bash
healthsync fetch
healthsync summary --period weekly
```

## Commands

| Command | What it does |
| --- | --- |
| `onboard` | Create or reuse your identity and show pairing details |
| `fetch` | Download, decrypt and store health data |
| `summary --period daily\|weekly\|monthly` | Summarize stored data |
| `summary --output json` | Return a structured JSON report |
| `unlink` | Unlink the paired iPhone, keeping local keys and data |
| `self-test` | Check the executable without network access |
| `network-diagnostics` | Check HTTPS connectivity to the relay |
| `licenses` | Show third-party notices |

Run `healthsync <command> --help` for every option. To save a private report:

```bash
healthsync summary --output json --save report.json --confirm-sensitive-save
```

The destination directory must exist. Existing files are never overwritten.
Only the save confirmation is printed; the report stays in the file.

## State and upgrades

State is stored in `~/.apple-health-sync`. Use the same `--state-dir <path>` with
every command to choose another location. Keep its keys and data private and backed
up when replacing the executable. Existing v5 state works without conversion;
v4 requires a separate state directory and new iPhone pairing.

`onboard --rotate` archives the old identity and creates new keys and a new user ID.
Use it only when you intend to pair again. `onboard --offline` creates the Hex
pairing payload without requesting a QR image.

[Configuration details](../src/skill/references/config.md) · [Build from source](../CONTRIBUTING.md)
