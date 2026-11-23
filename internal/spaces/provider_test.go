package spaces

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fakeProvider puts a session provider on PATH, called prov, speaking the protocol from files: for each
// verb it saves its stdin as VERB.in in dir and records the verb, then prints out[VERB], or fails with
// fails[VERB] on its stderr. It returns dir and the verbs in the order called.
func fakeProvider(t *testing.T, out, fails map[string]string) (string, func() []string) {
	t.Helper()
	dir := t.TempDir()
	for verb, text := range out {
		if err := os.WriteFile(filepath.Join(dir, verb+".out"), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for verb, text := range fails {
		if err := os.WriteFile(filepath.Join(dir, verb+".err"), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	script := "#!/bin/sh\nd=" + dir + "\n/bin/cat > \"$d/$1.in\"\necho \"$1\" >> \"$d/calls\"\n" +
		"if [ -f \"$d/$1.err\" ]; then /bin/cat \"$d/$1.err\" >&2; exit 1; fi\n" +
		"if [ -f \"$d/$1.out\" ]; then /bin/cat \"$d/$1.out\"; fi\n"
	if err := os.WriteFile(filepath.Join(dir, "prov"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/bin:/usr/bin")
	return dir, func() []string {
		data, err := os.ReadFile(filepath.Join(dir, "calls"))
		if err != nil {
			return nil
		}
		return strings.Fields(string(data))
	}
}

// sent is the request the fake provider in dir was last sent for verb, decoded.
func sent(t *testing.T, dir, verb string) any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, verb+".in"))
	if err != nil {
		t.Fatal(err)
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("%s sent %q: %v", verb, data, err)
	}
	return v
}

// A provider says what it does; one that fails to, or prints nothing, keeps alone, as a keeper does.
func TestProviderCapabilities(t *testing.T) {
	fakeProvider(t, map[string]string{"capabilities": `{"capabilities": ["list", "open", "kill"]}`}, nil)
	got := []Capabilities{ProviderCapabilities([]string{"prov"})}
	fakeProvider(t, nil, map[string]string{"capabilities": "unknown verb capabilities"})
	got = append(got, ProviderCapabilities([]string{"prov"}))
	fakeProvider(t, nil, nil)
	got = append(got, ProviderCapabilities([]string{"prov"}), ProviderCapabilities(nil))
	want := []Capabilities{{CapList: true, CapOpen: true, CapKill: true}, {CapKeep: true}, {CapKeep: true}, {}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %v\nwant %v", got, want)
	}
}
