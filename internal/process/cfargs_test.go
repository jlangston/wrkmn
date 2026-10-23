package process

import (
	"reflect"
	"testing"

	"wrkmn/internal/config"
)

func TestBuildCfDevArgs(t *testing.T) {
	cases := []struct {
		name string
		dev  config.DevMode
		mode string
		want []string
	}{
		{"defaults yield nothing", config.DevModeRemoteBindings, "", nil},
		{"empty dev mode treated as remote-bindings", "", "", nil},
		{"local", config.DevModeLocal, "", []string{"--local"}},
		{"mode only", config.DevModeRemoteBindings, "staging", []string{"--mode", "staging"}},
		{"local with mode", config.DevModeLocal, "staging", []string{"--local", "--mode", "staging"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := buildCfDevArgs(c.dev, c.mode); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}
