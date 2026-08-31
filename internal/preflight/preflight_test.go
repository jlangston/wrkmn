package preflight

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wrkmn/internal/config"
)

func issueMessages(issues []Issue) string {
	var b strings.Builder
	for _, i := range issues {
		fmt.Fprintf(&b, "[%s] %s: %s\n", i.Severity, i.Process, i.Message)
	}
	return b.String()
}

func findIssue(issues []Issue, process string, sev Severity, substr string) bool {
	for _, i := range issues {
		if i.Process == process && i.Severity == sev && strings.Contains(i.Message, substr) {
			return true
		}
	}
	return false
}

func TestMissingCommandIsError(t *testing.T) {
	cfg := &config.Config{BaseDir: t.TempDir(), Processes: map[string]config.Process{
		"api": {Command: "definitely-not-a-real-binary-xyz", Type: config.ProcessTypeService},
	}}
	issues := Check(cfg)
	if !findIssue(issues, "api", SeverityError, "not found on PATH") {
		t.Fatalf("missing command not reported:\n%s", issueMessages(issues))
	}
}

func TestBadWorkingDirIsError(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{BaseDir: dir, Processes: map[string]config.Process{
		"api": {Command: "sh", WorkingDir: filepath.Join(dir, "nope"), Type: config.ProcessTypeService},
	}}
	issues := Check(cfg)
	if !findIssue(issues, "api", SeverityError, "not a directory") {
		t.Fatalf("bad working_dir not reported:\n%s", issueMessages(issues))
	}
}

func TestEnvFileSeverityByProvenance(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		BaseDir:  dir,
		Defaults: config.ProcessDefaults{EnvFile: "shared-missing.env"},
		Processes: map[string]config.Process{
			"api": {Command: "sh", WorkingDir: dir, EnvFile: "explicit-missing.env", Type: config.ProcessTypeService},
		},
	}
	issues := Check(cfg)
	if !findIssue(issues, "api", SeverityError, "explicit-missing.env") {
		t.Fatalf("explicit missing env file should be an error:\n%s", issueMessages(issues))
	}
	if !findIssue(issues, "api", SeverityWarning, "shared-missing.env") {
		t.Fatalf("defaulted missing env file should be a warning:\n%s", issueMessages(issues))
	}
}

func TestDuplicateInspectorPortsIsError(t *testing.T) {
	cfg := &config.Config{BaseDir: t.TempDir(), Processes: map[string]config.Process{
		"a": {Command: "sh", InspectorPort: 9329, Type: config.ProcessTypeWorker},
		"b": {Command: "sh", InspectorPort: 9329, Type: config.ProcessTypeWorker},
	}}
	issues := Check(cfg)
	if !findIssue(issues, "", SeverityError, "9329") {
		t.Fatalf("duplicate inspector port not reported:\n%s", issueMessages(issues))
	}
}

func TestOccupiedInspectorPortIsWarning(t *testing.T) {
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	cfg := &config.Config{BaseDir: t.TempDir(), Processes: map[string]config.Process{
		"api": {Command: "sh", InspectorPort: port, Type: config.ProcessTypeWorker},
	}}
	issues := Check(cfg)
	if !findIssue(issues, "api", SeverityWarning, "already in use") {
		t.Fatalf("occupied inspector port not reported:\n%s", issueMessages(issues))
	}
}

func TestCleanConfigHasNoIssues(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".dev.vars"), []byte("A=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{BaseDir: dir, Processes: map[string]config.Process{
		"api": {Command: "sh", WorkingDir: dir, EnvFile: ".dev.vars", Type: config.ProcessTypeService},
	}}
	if issues := Check(cfg); len(issues) != 0 {
		t.Fatalf("clean config produced issues:\n%s", issueMessages(issues))
	}
}
