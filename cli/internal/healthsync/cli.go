package healthsync

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

type options struct {
	state, storage, protocol, userID, recordID, privateKey, publicKey, apiKey string
	sqlitePath, jsonPath, period, output, save                                string
	rotate, offline, confirmSave                                              bool
	timeout                                                                   int
}

type app struct {
	out, errOut io.Writer
	newRelay    func(int) (*relay, error)
	now         func() time.Time
}

// Run executes the same CLI used by all agent adapters. Parsing never touches state.
func Run(args []string, out, errOut io.Writer) int {
	return (&app{out: out, errOut: errOut, newRelay: newRelay, now: time.Now}).run(args)
}

func (a *app) run(args []string) int {
	if expected := os.Getenv("HEALTHSYNC_EXPECTED_VERSION"); expected != "" && expected != Version {
		fmt.Fprintln(a.errOut, "CLI version does not match this package; rebuild or reinstall it.")
		return 78
	}
	if len(args) == 1 && args[0] == "--version" {
		fmt.Fprintln(a.out, "healthsync "+Version)
		return 0
	}
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		fmt.Fprintln(a.out, "Health Sync CLI (v5 only)\n\nUsage: healthsync COMMAND [OPTIONS]\n\nCommands:\n  onboard (onboarding)  Initialize or rotate a local v5 identity\n  fetch                 Fetch, decrypt, validate and store health data\n  unlink                Unlink the paired iOS device\n  summary               Summarize local health data\n  self-test             Check crypto, resources, SQLite and TLS offline\n  network-diagnostics   Explicitly probe verified relay HTTPS\n  licenses              Show embedded third-party notices\n\nUse healthsync COMMAND --help for every option.\nUse --version to print the CLI version.")
		if len(args) == 0 {
			return 2
		}
		return 0
	}
	command := args[0]
	if command == "onboarding" {
		command = "onboard"
	}
	if !oneOf(command, "onboard", "fetch", "unlink", "summary", "self-test", "network-diagnostics", "licenses") {
		fmt.Fprintf(a.errOut, "Unknown command %q. Run healthsync --help.\n", command)
		return 2
	}
	o := options{}
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(a.errOut)
	fs.Usage = func() {
		fmt.Fprintf(a.out, "Usage: healthsync %s [OPTIONS]\n", command)
		fs.VisitAll(func(f *flag.Flag) { fmt.Fprintf(a.out, "  --%s\n      %s (default %s)\n", f.Name, f.Usage, f.DefValue) })
	}
	if oneOf(command, "onboard", "fetch", "unlink", "summary") {
		fs.StringVar(&o.state, "state-dir", "", "Private state directory (default ~/.apple-health-sync)")
	}
	if oneOf(command, "onboard", "fetch", "summary") {
		fs.StringVar(&o.storage, "storage", "auto", "Storage backend: auto, sqlite, json")
	}
	if command == "onboard" {
		fs.StringVar(&o.protocol, "protocol", "v5", "Protocol: v5 (v4 is not supported)")
		fs.BoolVar(&o.rotate, "rotate", false, "Archive existing identity, then create new keys and a new user ID")
		fs.BoolVar(&o.offline, "offline", false, "Generate local identity and Hex payload without a relay request")
	}
	if oneOf(command, "fetch", "unlink") {
		fs.StringVar(&o.userID, "user-id", "", "Override the user ID")
		fs.StringVar(&o.recordID, "record-id", "", "Legacy option name for --user-id; still uses v5")
		fs.StringVar(&o.privateKey, "private-key-path", "", "Override the Ed25519 signing private key PEM")
		fs.StringVar(&o.publicKey, "public-key", "", "Override the base64 Ed25519 public key; must match the private key")
		fs.IntVar(&o.timeout, "timeout-seconds", 20, "Timeout per relay request in seconds")
	}
	if command == "fetch" {
		fs.StringVar(&o.apiKey, "apikey", "", "Override the relay publishable API key")
	}
	if oneOf(command, "fetch", "summary") {
		fs.StringVar(&o.sqlitePath, "sqlite-path", "", "Override the SQLite database path")
		fs.StringVar(&o.jsonPath, "json-path", "", "Override the NDJSON storage path")
	}
	if command == "summary" {
		fs.StringVar(&o.period, "period", "weekly", "Summary period: daily, weekly, monthly")
		fs.StringVar(&o.output, "output", "text", "Output format: text, json")
		fs.StringVar(&o.save, "save", "", "Save sensitive report to a new file; requires --confirm-sensitive-save")
		fs.BoolVar(&o.confirmSave, "confirm-sensitive-save", false, "Confirm the report destination is intended for private health data")
	}
	if command == "network-diagnostics" {
		fs.IntVar(&o.timeout, "timeout-seconds", 10, "HTTPS probe timeout: 1..60 seconds")
	}
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	var argumentError string
	switch {
	case fs.NArg() != 0:
		argumentError = "unexpected positional arguments"
	case o.storage != "" && !oneOf(o.storage, "auto", "sqlite", "json"):
		argumentError = "--storage must be auto, sqlite or json"
	case command == "onboard" && o.protocol != "v5":
		argumentError = "only protocol v5 is supported; existing v4 state will not be changed"
	case oneOf(command, "fetch", "unlink", "network-diagnostics") && (o.timeout <= 0 || int64(o.timeout) > int64((1<<63-1)/time.Second)):
		argumentError = "--timeout-seconds must be a positive duration"
	case command == "network-diagnostics" && o.timeout > 60:
		argumentError = "--timeout-seconds must be between 1 and 60"
	case command == "summary" && !oneOf(o.period, "daily", "weekly", "monthly"):
		argumentError = "--period must be daily, weekly or monthly"
	case command == "summary" && !oneOf(o.output, "text", "json"):
		argumentError = "--output must be text or json"
	case command == "summary" && ((o.save != "") != o.confirmSave):
		argumentError = "use --save <new-file> and --confirm-sensitive-save together"
	case o.userID != "" && o.recordID != "" && o.userID != o.recordID:
		argumentError = "--user-id and --record-id conflict"
	}
	if argumentError != "" {
		fmt.Fprintln(a.errOut, "Error: "+argumentError)
		return 2
	}
	var err error
	switch command {
	case "licenses":
		_, err = fmt.Fprint(a.out, thirdPartyNotices)
	case "self-test":
		err = a.selfTest()
	case "network-diagnostics":
		err = a.networkDiagnostics(o)
	default:
		err = a.withState(command, o)
	}
	if err != nil {
		fmt.Fprintln(a.errOut, "Error: "+err.Error())
		return 1
	}
	return 0
}

func oneOf(value string, choices ...string) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}
func (a *app) timestamp() string {
	return a.now().UTC().Truncate(time.Second).Format("2006-01-02T15:04:05+00:00")
}
func textValue(m map[string]any, key string) string {
	value, _ := m[key].(string)
	return strings.TrimSpace(value)
}

func rawString(m map[string]any, key string) string { value, _ := m[key].(string); return value }
