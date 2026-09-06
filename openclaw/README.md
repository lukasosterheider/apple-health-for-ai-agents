# Apple Health Sync for OpenClaw

## Install

With OpenClaw installed, run:

```bash
openclaw skills verify @lukasosterheider/apple-health-sync
openclaw skills install @lukasosterheider/apple-health-sync
```

Continue after both commands succeed. On first use, the skill downloads the matching
Health Sync executable and verifies its checksum. No Python installation is needed.

## Start onboarding and Connect your iPhone

Start a new OpenClaw chat and send:

```text
Use Apple Health Sync and start onboarding with a QR code.
```

Open [Health Sync on your iPhone](https://apps.apple.com/app/health-sync-for-openclaw/id6759522298),
choose **QR Code Setup**, scan the code, grant Apple Health permissions and complete
your first sync. Then ask OpenClaw: `Fetch my health data and summarize this week.`

[View on ClawHub](https://clawhub.ai/lukasosterheider/skills/apple-health-sync)
· [Illustrated guide](https://gethealthsync.app/#install-openclaw)
