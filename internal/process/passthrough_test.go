package process

import (
	"reflect"
	"testing"

	"wrkmn/internal/config"
)

func TestBuildWranglerPassthrough(t *testing.T) {
	cases := []struct {
		name string
		mode config.DevMode
		port int
		env  string
		want []string
	}{
		{"remote-bindings with inspector", config.DevModeRemoteBindings, 9229, "",
			[]string{"--", "--inspector-port", "9229"}},
		{"empty mode treated as remote-bindings", "", 9229, "",
			[]string{"--", "--inspector-port", "9229"}},
		{"local adds --local", config.DevModeLocal, 9229, "",
			[]string{"--", "--inspector-port", "9229", "--local"}},
		{"remote omits inspector, adds --remote", config.DevModeRemote, 9229, "",
			[]string{"--", "--remote"}},
		{"env appended for all modes", config.DevModeRemoteBindings, 9230, "staging",
			[]string{"--", "--inspector-port", "9230", "--env", "staging"}},
		{"remote with non-production env", config.DevModeRemote, 9229, "staging",
			[]string{"--", "--remote", "--env", "staging"}},
		{"production env is the root sentinel, no --env", config.DevModeRemoteBindings, 9229, "production",
			[]string{"--", "--inspector-port", "9229"}},
		{"no port no env remote-bindings yields nil", config.DevModeRemoteBindings, 0, "", nil},
		{"local with no inspector port", config.DevModeLocal, 0, "",
			[]string{"--", "--local"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := buildWranglerPassthrough(c.mode, c.port, c.env)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}
