# Configuration and private state

## State directory

The default is `~/.apple-health-sync`. To use another directory, pass the same
`--state-dir <path>` to every command. Keep the entire directory private and backed up.

| Path under the state directory | Contents |
| --- | --- |
| `config/config.json` | v5 identity, key paths, storage settings and operation status |
| `config/secrets/identities/<user-id>/` | Ed25519 signing and X25519 encryption key pairs |
| `config/registration-qr.json` | Public pairing payload |
| `config/registration-qr.png` | QR image, when available |
| `health_data.db` | SQLite health snapshots, the default storage |
| `config/health_data.ndjson` | Snapshots when using `--storage json` |
| `config/key-backups/` | Previous identities archived by rotation |
| `config/.operation.lock` | Lock preventing concurrent commands on the same state |

POSIX permissions are `0700` for directories and `0600` for files. On Windows,
access is limited to the current user and SYSTEM. Keys and decrypted health data
are plaintext on disk; use disk encryption and private backups.

## Options and storage

`--storage auto` uses the configured backend, defaulting to SQLite. Use
`--storage sqlite` or `--storage json` to choose explicitly. `--sqlite-path` and
`--json-path` override data locations. Existing parent-directory permissions are
not changed. Run `healthsync <command> --help` for all supported options.

SQLite stores one row per user and date in `health_data`. NDJSON appends each fetched
snapshot, so repeated fetches can contribute repeated samples to summaries.
Summaries include dates from the UTC start date onward. Daily, weekly and monthly
windows use 1, 7 and 30 days. JSON includes counts, minimum, maximum, average and
latest values; it never includes user IDs.

`--save <path> --confirm-sensitive-save` creates a new private report. Its parent
must exist; symbolic links and existing files are rejected. Only a confirmation
is printed, not the report contents.

## Existing identities

Existing v5 PEM keys, SQLite databases and NDJSON files are supported. The CLI
honors key paths in `config.json`, including earlier v5 layouts. It reads
`config/runtime.json` as a fallback. Missing or mismatched keys cause an error.

v4 identities and RSA relay records are unsupported. Keep their installation and
state separate; there is no automatic upgrade to v5.

## Rotation

`healthsync onboard --rotate` archives and verifies the old configuration, keys
and available pairing artifacts before creating a new user ID and key pair.
The database is retained. Old server data still requires the archived identity.
Reset pairing in the iPhone app, scan the new QR code and complete a first sync.
Backups contain unencrypted private keys and are not deleted automatically.

`onboard --offline` skips QR rendering. Read `onboarding_payload_hex` in the selected
state's `config/config.json` for the public Hex pairing payload.

## Security and troubleshooting

Relay URLs and protocol limits are compiled into the executable; configuration
cannot redirect health data to another service. v5 uses Ed25519, X25519, HKDF-SHA256
and ChaCha20-Poly1305. All downloaded rows must authenticate and validate before
storage; unsupported text fields are removed and valid recent data overrides history.

HTTPS verifies certificates and hostnames and requires TLS 1.2+. Native trust and
embedded CA roots are used by default. Explicit `SSL_CERT_FILE` or `SSL_CERT_DIR`
overrides must contain valid certificates; never disable verification to fix an error.

Use `self-test` for an offline check or `network-diagnostics` for an explicit HTTPS
check. Configuration fields `last_fetch_status`, `last_fetch_error` and
`last_fetch_success_at`, with corresponding `last_unlink_*` fields, distinguish
failed attempts from the last successful operation. Share only the error message
with [support](mailto:contact@gethealthsync.app), never config, keys or health data.
