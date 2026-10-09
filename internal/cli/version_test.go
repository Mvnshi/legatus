package cli

import (
	"runtime/debug"
	"testing"
)

func TestVersionFromBuildInfo(t *testing.T) {
	original := Version
	t.Cleanup(func() { Version = original })
	revisionInfo := func(modified string) *debug.BuildInfo {
		return &debug.BuildInfo{
			Main: debug.Module{Version: "(devel)"},
			Settings: []debug.BuildSetting{
				{Key: "vcs.modified", Value: modified},
				{Key: "vcs.revision", Value: "abcdef1234567890"},
			},
		}
	}
	for _, tt := range []struct {
		name, version string
		info          *debug.BuildInfo
		want          string
	}{
		{"release override", "v2.0.0", &debug.BuildInfo{Main: debug.Module{Version: "v1.0.0"}}, "v2.0.0"},
		{"override without info", "v2.0.0", nil, "v2.0.0"},
		{"module version", "dev", &debug.BuildInfo{Main: debug.Module{Version: "v1.0.0"}, Settings: revisionInfo("true").Settings}, "v1.0.0"},
		{"clean revision", "dev", revisionInfo("false"), "dev abcdef1"},
		{"dirty revision", "dev", revisionInfo("true"), "dev abcdef1+dirty"},
		{"short revision", "dev", &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "abc"}}}, "dev abc"},
		{"no info", "dev", nil, "dev"},
		{"empty info", "dev", &debug.BuildInfo{}, "dev"},
		{"devel without revision", "dev", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}, Settings: []debug.BuildSetting{{Key: "vcs.modified", Value: "true"}}}, "dev"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			Version = tt.version
			if got := versionFromBuildInfo(tt.info); got != tt.want {
				t.Errorf("versionFromBuildInfo() = %q, want %q", got, tt.want)
			}
		})
	}
}
