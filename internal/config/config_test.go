package config

import (
	"testing"
	"time"
)

func TestParseLatencyRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw     string
		wantMin time.Duration
		wantMax time.Duration
		wantErr bool
	}{
		{raw: "", wantMin: 0, wantMax: 0},
		{raw: "0", wantMin: 0, wantMax: 0},
		{raw: "15ms", wantMin: 15 * time.Millisecond, wantMax: 15 * time.Millisecond},
		{raw: "10ms-25ms", wantMin: 10 * time.Millisecond, wantMax: 25 * time.Millisecond},
		{raw: "25ms-10ms", wantErr: true},
		{raw: "-1ms", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			t.Parallel()

			gotMin, gotMax, err := parseLatencyRange(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if gotMin != tt.wantMin || gotMax != tt.wantMax {
				t.Fatalf("got %s-%s, want %s-%s", gotMin, gotMax, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestEnvInt(t *testing.T) {
	t.Setenv("GODRIVE_TEST_INT", "12")
	if got := envInt("GODRIVE_TEST_INT", 3); got != 12 {
		t.Fatalf("got %d, want 12", got)
	}

	t.Setenv("GODRIVE_TEST_INT", "invalid")
	if got := envInt("GODRIVE_TEST_INT", 3); got != 3 {
		t.Fatalf("got %d, want fallback", got)
	}

	t.Setenv("GODRIVE_TEST_INT", "")
	if got := envInt("GODRIVE_TEST_INT", 3); got != 3 {
		t.Fatalf("got %d, want fallback", got)
	}
}
