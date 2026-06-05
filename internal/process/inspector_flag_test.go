package process

import (
	"reflect"
	"testing"
)

func TestAppendInspectorFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
		port int
		want []string
	}{
		{
			name: "bun run dev",
			args: []string{"run", "dev"},
			port: 9229,
			want: []string{"run", "dev", "--", "--inspector-port", "9229"},
		},
		{
			name: "empty args",
			args: nil,
			port: 9230,
			want: []string{"--", "--inspector-port", "9230"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := appendInspectorFlag(tt.args, tt.port)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("appendInspectorFlag(%v, %d) = %v, want %v", tt.args, tt.port, got, tt.want)
			}
		})
	}
}

func TestAppendInspectorFlagDoesNotMutateInput(t *testing.T) {
	in := []string{"run", "dev"}
	_ = appendInspectorFlag(in, 9229)
	if !reflect.DeepEqual(in, []string{"run", "dev"}) {
		t.Fatalf("input slice was mutated: %v", in)
	}
}
