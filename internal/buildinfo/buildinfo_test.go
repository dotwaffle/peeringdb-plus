package buildinfo

import (
	"runtime/debug"
	"testing"
	"time"
)

func TestSourceTimeFromBuildInfo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		settings []debug.BuildSetting
		want     time.Time
	}{
		{
			name: "vcs time",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abcdef0"},
				{Key: "vcs.time", Value: "2026-07-24T12:34:56+02:00"},
			},
			want: time.Date(2026, 7, 24, 10, 34, 56, 0, time.UTC),
		},
		{
			name: "missing",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abcdef0"},
			},
		},
		{
			name: "malformed",
			settings: []debug.BuildSetting{
				{Key: "vcs.time", Value: "yesterday"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := sourceTime(&debug.BuildInfo{Settings: test.settings})
			if !got.Equal(test.want) {
				t.Errorf("sourceTime() = %v, want %v", got, test.want)
			}
		})
	}
}
