# Apple Health Sync

Pair your iPhone, fetch encrypted Apple Health data and generate summaries in your
agent. Start with:

```text
Use Apple Health Sync and start onboarding with a QR code.
```

The plugin uses the Health Sync executable, bundled or downloaded with a pinned
checksum. No Python installation is needed. State is stored in `~/.apple-health-sync`;
keep its keys and health data private and backed up.

For terminal use, run `bin/healthsync <command>` from this package on macOS/Linux
or `bin\healthsync.cmd <command>` on Windows. Run `--help` for available commands.

[Installation guides](https://github.com/lukasosterheider/apple-health-for-ai-agents)
· [Support](mailto:contact@gethealthsync.app)
