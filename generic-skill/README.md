# Apple Health Sync for other agents

Use this skill with Hermes Agent or another agent that supports Agent Skills,
command execution and outbound HTTPS.

## Install

With Node.js and npm installed, run:

```bash
npx skills add https://github.com/lukasosterheider/apple-health-for-ai-agents/tree/main/generic-skill/apple-health-sync
```

Select your agent and installation scope when prompted. On first use, the skill
downloads the matching Health Sync executable and verifies its checksum. Python
is not required. See the [skills CLI guide](https://github.com/vercel-labs/skills#usage).

For manual installation, download
[apple-health-sync-agent-skill.zip](https://github.com/lukasosterheider/apple-health-for-ai-agents/releases/latest/download/apple-health-sync-agent-skill.zip)
and use your agent's ZIP import. This bundle includes the executables. Its checksum
is in the same release. [Claude Web uses this ZIP too](../claude/README.md#claude-web).

## Start onboarding and Connect your iPhone

Start a fresh agent conversation and send:

```text
Use Apple Health Sync and start onboarding with a QR code.
```

Open [Health Sync on your iPhone](https://apps.apple.com/app/health-sync-for-openclaw/id6759522298),
choose **QR Code Setup**, scan the code, grant Apple Health permissions and complete
your first sync. Then ask your agent: `Fetch my health data and summarize this week.`

Keep the state directory private and persistent, especially in hosted agents.
[CLI commands](../cli/README.md#commands) · [Configuration](apple-health-sync/references/config.md)
