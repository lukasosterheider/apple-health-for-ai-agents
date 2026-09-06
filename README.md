# Apple Health for AI Agents

Access Apple Health data from your terminal, Codex, Claude, OpenClaw or another
AI agent. Health Sync encrypts data on your iPhone and decrypts it in your CLI or
agent environment.

## Get started

You need an iPhone with [Health Sync](https://apps.apple.com/app/health-sync-for-openclaw/id6759522298)
and a computer or agent environment with internet access. Choose your installation:

| Use with | Guide |
| --- | --- |
| Terminal or custom integration | [Health Sync CLI](cli/README.md) |
| OpenClaw | [Install the skill](openclaw/README.md) |
| Codex | [Install the plugin](codex/README.md) |
| Claude Code or Claude Web | [Install the plugin or skill](claude/README.md) |
| Hermes Agent or another compatible agent | [Install the generic skill](generic-skill/README.md) |

All integrations use the same Go executable. No Python, Go or system SQLite
installation is needed to run it. Version 2 supports v5 identities only.

## Your data

Keys, configuration and synced data are stored in `~/.apple-health-sync` by default.
Keep this directory private and backed up. Decrypted data and keys are protected by
file permissions, not encrypted on disk; use disk encryption. Installation alone
does not pair a device, fetch data or create scheduled tasks.

[Website](https://gethealthsync.app/) · [Downloads](https://github.com/lukasosterheider/apple-health-for-ai-agents/releases)
· [Build and contribute](CONTRIBUTING.md) · [Support](mailto:contact@gethealthsync.app)
