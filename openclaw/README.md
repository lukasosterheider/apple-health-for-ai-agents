# Apple Health Sync for OpenClaw

Install the published skill through OpenClaw's native ClawHub integration, as
shown in the [website installation guide](https://gethealthsync.app/#install-openclaw).

## Install

With OpenClaw installed, run both commands in your terminal:

```bash
openclaw skills verify @lukasosterheider/apple-health-sync
openclaw skills install @lukasosterheider/apple-health-sync
```

Continue only after both commands succeed. You can inspect the published package
on [ClawHub](https://clawhub.ai/lukasosterheider/skills/apple-health-sync).
The skill uses Python 3 and the dependencies listed in its `requirements.txt`.

## Connect your iPhone

Start a new OpenClaw chat and send:

```text
Use Apple Health Sync and start onboarding with a QR code.
```

Approve initialization when prompted. Open
[Health Sync on your iPhone](https://apps.apple.com/app/health-sync-for-openclaw/id6759522298),
choose QR Code Setup, scan the code, select Apple Health permissions, and complete
the first sync. Then ask OpenClaw: `What Apple Health data can I access?`

## Source and packages

The shared implementation lives in
[`generic-skill/apple-health-sync`](../generic-skill/apple-health-sync).
ClawHub receives a generated OpenClaw variant with its platform metadata and
`{baseDir}` paths. This directory documents that installation route; it does not
maintain a second copy of the Python implementation.
