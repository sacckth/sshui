package tui

import (
	"strings"
	"testing"

	"github.com/sacckth/sshui/internal/appcfg"
	scfg "github.com/sacckth/sshui/internal/config"
	"github.com/sacckth/sshui/internal/overlay"
)

func TestBuildHostItemsFilteredPasswordGroupWithoutHosts(t *testing.T) {
	cfg := &scfg.Config{}
	ov := &overlay.File{Version: 1, Groups: []string{"myteam"}}
	items := buildHostItemsFiltered(cfg, ov, appcfg.BrowseModeMerged, 80, false)
	var sawPwGroup bool
	for _, it := range items {
		if gh, ok := it.(groupHeaderEntry); ok && gh.pwGroup == "myteam" {
			sawPwGroup = true
			break
		}
	}
	if !sawPwGroup {
		t.Fatalf("expected password group header for declared group with no hosts; got %d items", len(items))
	}
}

func TestBuildHostItemsFilteredPasswordNoOverlayContent(t *testing.T) {
	cfg := &scfg.Config{}
	ov := &overlay.File{Version: 1}
	items := buildHostItemsFiltered(cfg, ov, appcfg.BrowseModeMerged, 80, false)
	for _, it := range items {
		if _, ok := it.(passwordHostRowEntry); ok {
			t.Fatal("unexpected password row with empty overlay")
		}
	}
}

func TestBuildHostItemsFilteredMergedSameGroupName(t *testing.T) {
	cfg := &scfg.Config{
		Groups: []scfg.Group{{
			Name: "prod",
			Hosts: []scfg.HostBlock{
				{Patterns: []string{"ssh-a"}},
			},
		}},
	}
	ov := &overlay.File{
		Version: 1,
		PasswordHosts: []overlay.PasswordHost{
			{Group: "prod", Patterns: []string{"pw-b"}, Hostname: "x.example"},
		},
	}
	items := buildHostItemsFiltered(cfg, ov, appcfg.BrowseModeMerged, 80, false)
	var mergedHeaders int
	var sshRows, pwRows int
	for _, it := range items {
		switch v := it.(type) {
		case groupHeaderEntry:
			if v.groupIdx == 0 && v.pwGroup == "prod" && strings.Contains(v.label, "🔀") {
				mergedHeaders++
			}
		case hostRowEntry:
			sshRows++
		case passwordHostRowEntry:
			pwRows++
		}
	}
	if mergedHeaders != 1 {
		t.Fatalf("want 1 merged group header, got %d", mergedHeaders)
	}
	if sshRows != 1 || pwRows != 1 {
		t.Fatalf("want 1 openssh + 1 password row under merged group, got ssh=%d pw=%d", sshRows, pwRows)
	}
}
