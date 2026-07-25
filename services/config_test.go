package services

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConf(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "cogged.conf.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return p
}

func TestLoadConfigFromExplicitPath(t *testing.T) {
	p := writeConf(t, `{"db.host":"1.2.3.4","db.port":"9080","listen.port":"8090"}`)
	conf := LoadConfig(p)
	if conf == nil {
		t.Fatal("LoadConfig returned nil")
	}
	if got := conf.Get("db.host"); got != "1.2.3.4" {
		t.Errorf("db.host = %q, want 1.2.3.4", got)
	}
	if got := conf.Get("db.port"); got != "9080" {
		t.Errorf("db.port = %q, want 9080", got)
	}
	if got := conf.Get("missing.key"); got != "" {
		t.Errorf("missing key should return empty, got %q", got)
	}
}

func TestLoadConfigFromEnvVar(t *testing.T) {
	p := writeConf(t, `{"db.host":"env-host","db.port":"7777"}`)
	t.Setenv("COGGED_CONFIG_FILE", p)
	// empty CLI value forces the env-var branch
	conf := LoadConfig("")
	if conf.Get("db.host") != "env-host" {
		t.Errorf("expected config loaded from COGGED_CONFIG_FILE, got db.host=%q", conf.Get("db.host"))
	}
}

func TestConfigGet(t *testing.T) {
	c := Config{"a.b": "c"}
	if c.Get("a.b") != "c" {
		t.Error("Config.Get did not return the stored value")
	}
	if c.Get("nope") != "" {
		t.Error("Config.Get of missing key should be empty string")
	}
}
