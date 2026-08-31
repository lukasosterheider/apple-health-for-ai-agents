package healthsync

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const relayBase = "https://snpiylxajnxpklpwdtdg.supabase.co/functions/v1/"
const qrURL = relayBase + "qr-code-generator"
const fetchURL = relayBase + "get-data-v2"
const unlinkURL = relayBase + "unlink-device"
const publishableKey = "sb_publishable_HW9XhDFQLrcPoGsbYIz7zg_FnFOePtQ"
const region = "eu-west-1"
const appStoreURL = "https://apps.apple.com/app/health-sync-for-openclaw/id6759522298"

type state struct {
	root, configDir, secretsDir, configPath string
	config                                  map[string]any
}

func absolutePath(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, strings.TrimLeft(path[1:], "/\\"))
	}
	return filepath.Abs(path)
}

func loadState(raw string, create bool) (*state, error) {
	if raw == "" {
		raw = "~/.apple-health-sync"
	}
	root, err := absolutePath(raw)
	if err != nil {
		return nil, err
	}
	s := &state{root: root, configDir: filepath.Join(root, "config"), secretsDir: filepath.Join(root, "config", "secrets"), configPath: filepath.Join(root, "config", "config.json"), config: map[string]any{}}
	for _, path := range []string{s.root, s.configDir, s.secretsDir} {
		if create {
			err = secureDir(path)
		} else {
			err = checkDir(path)
		}
		if err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *state) readConfig() error {
	for _, path := range []string{s.configPath, filepath.Join(s.configDir, "runtime.json")} {
		data, err := readPrivate(path, 4<<20)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if err = decodeObject(data, &s.config); err != nil {
			return fmt.Errorf("invalid user configuration: %w", err)
		}
		if s.config["protocol_version"] != json.Number("5") {
			return errors.New("only v5 identities are supported; v4 or unknown state was left unchanged. Use a separate state directory")
		}
		if textValue(s.config, "user_id") == "" {
			s.config["user_id"] = textValue(s.config, "record_id")
		}
		delete(s.config, "record_id")
		if s.config["storage"] == "custom" {
			s.config["storage"] = "sqlite"
		}
		for key := range s.config {
			if strings.HasPrefix(key, "supabase_") || strings.HasPrefix(key, "v5_") || oneOf(key, "onboarding_version", "ios_app_link", "custom_sink_command", "onboarding_deeplink") {
				delete(s.config, key)
			}
		}
		return nil
	}
	return nil
}

func (s *state) save() error { return writeJSON(s.configPath, s.config) }
func (s *state) storage(o options) (string, string, error) {
	kind := o.storage
	if kind == "auto" || kind == "" {
		kind = textValue(s.config, "storage")
		if kind == "" {
			kind = "sqlite"
		}
	}
	var path string
	switch kind {
	case "sqlite":
		path = first(o.sqlitePath, textValue(s.config, "sqlite_path"), filepath.Join(s.root, "health_data.db"))
	case "json":
		path = first(o.jsonPath, textValue(s.config, "json_path"), filepath.Join(s.configDir, "health_data.ndjson"))
	default:
		return "", "", errors.New("unsupported storage backend in configuration")
	}
	resolved, err := absolutePath(path)
	return kind, resolved, err
}

func (a *app) withState(command string, o options) error {
	s, err := loadState(o.state, command == "onboard")
	if err != nil {
		return err
	}
	lock, err := lockState(filepath.Join(s.configDir, ".operation.lock"))
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = s.readConfig(); err != nil {
		return err
	}
	if command != "onboard" && len(s.config) == 0 {
		return errors.New("no configured identity; run healthsync onboard first")
	}
	switch command {
	case "onboard":
		return a.onboard(s, o)
	case "fetch":
		return a.fetch(s, o)
	case "unlink":
		return a.unlink(s, o)
	case "summary":
		return a.summary(s, o)
	}
	return errors.New("unknown state operation")
}

func decodeObject(data []byte, target *map[string]any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if err := d.Decode(target); err != nil {
		return err
	}
	if *target == nil {
		return errors.New("expected a JSON object")
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("unexpected trailing JSON")
	}
	return nil
}
func first(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
