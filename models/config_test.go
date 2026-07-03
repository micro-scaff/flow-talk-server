package models

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfigReadsRedisSection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.ini")
	content := `
[mysql]
username = root
database = flow_talk

[http]
addr = :8081

[jwt]
secret = test-secret
ttl = 2h

[redis]
enabled = true
addr = 127.0.0.1:6380
password = secret
db = 2
key_prefix = flow-talk-test
presence_ttl = 75s
channel = test:deliver
instance_id = instance-a
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if !cfg.Redis.Enabled {
		t.Fatalf("Redis.Enabled = false, want true")
	}
	if cfg.Redis.Addr != "127.0.0.1:6380" {
		t.Fatalf("Redis.Addr = %q", cfg.Redis.Addr)
	}
	if cfg.Redis.Password != "secret" {
		t.Fatalf("Redis.Password = %q", cfg.Redis.Password)
	}
	if cfg.Redis.DB != 2 {
		t.Fatalf("Redis.DB = %d", cfg.Redis.DB)
	}
	if cfg.Redis.KeyPrefix != "flow-talk-test" {
		t.Fatalf("Redis.KeyPrefix = %q", cfg.Redis.KeyPrefix)
	}
	if cfg.Redis.PresenceTTL != 75*time.Second {
		t.Fatalf("Redis.PresenceTTL = %s", cfg.Redis.PresenceTTL)
	}
	if cfg.Redis.Channel != "test:deliver" {
		t.Fatalf("Redis.Channel = %q", cfg.Redis.Channel)
	}
	if cfg.Redis.InstanceID != "instance-a" {
		t.Fatalf("Redis.InstanceID = %q", cfg.Redis.InstanceID)
	}
}
