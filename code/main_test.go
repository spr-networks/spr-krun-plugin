package main

import (
	"strings"
	"testing"
	"time"
)

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config
		wantErr string
	}{
		{
			name: "valid",
			cfg: config{
				mode:        "listen",
				port:        4040,
				socketPath:  "/run/spr-krun-plugin/plugin.sock",
				dialTimeout: time.Second,
			},
		},
		{
			name: "valid connect",
			cfg: config{
				mode:        "connect",
				port:        4041,
				socketPath:  "/run/spr-krun-plugin/eventbus.sock",
				dialTimeout: time.Second,
			},
		},
		{
			name: "zero port",
			cfg: config{
				mode:        "listen",
				socketPath:  "/run/plugin.sock",
				dialTimeout: time.Second,
			},
			wantErr: "vsock port",
		},
		{
			name: "relative socket",
			cfg: config{
				mode:        "listen",
				port:        4040,
				socketPath:  "plugin.sock",
				dialTimeout: time.Second,
			},
			wantErr: "absolute",
		},
		{
			name: "invalid mode",
			cfg: config{
				mode:        "dial",
				port:        4040,
				socketPath:  "/run/plugin.sock",
				dialTimeout: time.Second,
			},
			wantErr: "mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.validate()
			if tt.wantErr == "" && err != nil {
				t.Fatalf("validate returned %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("validate error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}
