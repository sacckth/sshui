package tui

import (
	"testing"

	scfg "github.com/sacckth/sshui/internal/config"
)

func TestSSHConnectPlanFromHost(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		want     sshConnectPlan
	}{
		{
			name:     "empty",
			patterns: nil,
			want:     sshConnectPlan{Invalid: true},
		},
		{
			name:     "single literal",
			patterns: []string{"myhost"},
			want: sshConnectPlan{
				FirstPattern: "myhost", PatternCount: 1,
				DirectTarget: "myhost",
			},
		},
		{
			name:     "multi uses first literal",
			patterns: []string{"first", "second"},
			want: sshConnectPlan{
				FirstPattern: "first", PatternCount: 2,
				DirectTarget: "first",
			},
		},
		{
			name:     "suffix star",
			patterns: []string{"win*"},
			want: sshConnectPlan{
				FirstPattern: "win*", PatternCount: 1,
				NeedWildcard: true, WildKind: sshWildSuffixStar, LitPrefix: "win",
			},
		},
		{
			name:     "prefix star",
			patterns: []string{"*prod"},
			want: sshConnectPlan{
				FirstPattern: "*prod", PatternCount: 1,
				NeedWildcard: true, WildKind: sshWildPrefixStar, LitSuffix: "prod",
			},
		},
		{
			name:     "lone star",
			patterns: []string{"*"},
			want: sshConnectPlan{
				FirstPattern: "*", PatternCount: 1,
				NeedWildcard: true, WildKind: sshWildPrefixStar, LitSuffix: "",
			},
		},
		{
			name:     "middle star manual",
			patterns: []string{"a*b*c"},
			want: sshConnectPlan{
				FirstPattern: "a*b*c", PatternCount: 1,
				NeedWildcard: true, WildKind: sshWildManual,
			},
		},
		{
			name:     "question manual",
			patterns: []string{"h?st"},
			want: sshConnectPlan{
				FirstPattern: "h?st", PatternCount: 1,
				NeedWildcard: true, WildKind: sshWildManual,
			},
		},
		{
			name:     "negation manual",
			patterns: []string{"!*.corp"},
			want: sshConnectPlan{
				FirstPattern: "!*.corp", PatternCount: 1,
				NeedWildcard: true, WildKind: sshWildManual,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sshConnectPlanFromHost(&scfg.HostBlock{Patterns: tt.patterns})
			if got.Invalid != tt.want.Invalid ||
				got.FirstPattern != tt.want.FirstPattern ||
				got.PatternCount != tt.want.PatternCount ||
				got.DirectTarget != tt.want.DirectTarget ||
				got.NeedWildcard != tt.want.NeedWildcard ||
				got.WildKind != tt.want.WildKind ||
				got.LitPrefix != tt.want.LitPrefix ||
				got.LitSuffix != tt.want.LitSuffix {
				t.Fatalf("got %+v want %+v", got, tt.want)
			}
		})
	}
}

func TestComposeSSHWildcardTarget(t *testing.T) {
	if g := composeSSHWildcardTarget(sshWildSuffixStar, "win", "", "dows"); g != "windows" {
		t.Fatalf("suffix: got %q", g)
	}
	if g := composeSSHWildcardTarget(sshWildSuffixStar, "win", "", ""); g != "win" {
		t.Fatalf("suffix empty user: got %q", g)
	}
	if g := composeSSHWildcardTarget(sshWildPrefixStar, "", "prod", "my"); g != "myprod" {
		t.Fatalf("prefix: got %q", g)
	}
	if g := composeSSHWildcardTarget(sshWildManual, "", "", "  host.example  "); g != "host.example" {
		t.Fatalf("manual trim: got %q", g)
	}
}
