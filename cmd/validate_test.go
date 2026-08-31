package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "worker-compose.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestValidateCleanConfig(t *testing.T) {
	dir := writeConfig(t, `
version: "1.0"
processes:
  api:
    command: sh
    args: [-c, "true"]
    type: service
`)
	var out strings.Builder
	if code := runValidate(dir, "", &out); code != 0 {
		t.Fatalf("exit code = %d, want 0; output:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "config valid") {
		t.Fatalf("output missing 'config valid':\n%s", out.String())
	}
}

func TestValidateInvalidConfigExitsNonzero(t *testing.T) {
	dir := writeConfig(t, `
version: "1.0"
processes:
  api:
    command: sh
    dev_mode: bogus
`)
	var out strings.Builder
	if code := runValidate(dir, "", &out); code != 1 {
		t.Fatalf("exit code = %d, want 1; output:\n%s", code, out.String())
	}
}

func TestValidatePreflightErrorExitsNonzero(t *testing.T) {
	dir := writeConfig(t, `
version: "1.0"
processes:
  api:
    command: definitely-not-a-real-binary-xyz
    type: service
`)
	var out strings.Builder
	if code := runValidate(dir, "", &out); code != 1 {
		t.Fatalf("exit code = %d, want 1; output:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "not found on PATH") {
		t.Fatalf("output missing preflight finding:\n%s", out.String())
	}
}

func TestValidateWarningsStillExitZero(t *testing.T) {
	dir := writeConfig(t, `
version: "1.0"
processes:
  api:
    command: sh
    args: [-c, "true"]
    type: service
defaults:
  env_file: missing-shared.env
`)
	var out strings.Builder
	if code := runValidate(dir, "", &out); code != 0 {
		t.Fatalf("exit code = %d, want 0 (warnings don't fail); output:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "missing-shared.env") {
		t.Fatalf("warning not printed:\n%s", out.String())
	}
}
