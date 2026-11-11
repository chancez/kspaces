package termtext

import (
	"reflect"
	"strings"
	"testing"
)

func TestLine(t *testing.T) {
	for in, want := range map[string]string{
		"plain text":               "plain text",
		"caf\u00e9":                "caf\u00e9",
		"\x1b[31mred\x1b[0m":       "^[[31mred^[[0m",
		"\x1b]0;title\x07":         "^[]0;title^G",
		"a\tb\nc\r":                "a^Ib^Jc^M",
		"del\x7f":                  "del^?",
		"csi\u009b31m":             `csi\u009b31m`,
		"bad\xff\xfe":              `bad\xff\xfe`,
		"replacement \ufffd stays": "replacement \ufffd stays",
	} {
		if got := Line(in); got != want {
			t.Errorf("Line(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBlock(t *testing.T) {
	in := "line one\n\tindented \x1b[2Jcleared\n"
	want := "line one\n\tindented ^[[2Jcleared\n"
	if got := Block(in); got != want {
		t.Errorf("Block(%q) = %q, want %q", in, got, want)
	}
}

func TestWriter(t *testing.T) {
	var b strings.Builder
	w := NewWriter(&b)
	in := "name\x1b]0;title\x07\n"
	n, err := w.Write([]byte(in))
	if err != nil || n != len(in) {
		t.Fatalf("Write = %d, %v", n, err)
	}
	if want := "name^[]0;title^G\n"; b.String() != want {
		t.Errorf("wrote %q, want %q", b.String(), want)
	}
}

// StyledLines keeps SGR sequences, carries the styles a line leaves set into the next, and drops every
// other sequence and control: prompt marks, cursor moves, a hyperlink, a title, a mode set with a private
// CSI, a C1 control, whose parameters are left as text, and bad bytes. Tabs stop every eight columns.
func TestStyledLines(t *testing.T) {
	in := "\x1b]133;A\x1b\\$ ls\r\n" +
		"\x1b[31mred \x1b[1mbold\x1b[22m\n" +
		"moved\x1b[5;5H\x1b[2J\x1b[>4;1m here\n" +
		"\x1b]8;;https://x\x1b\\link\x1b]8;;\x1b\\\x1b]2;title\x07\u009b1A\xff\n" +
		"\x1b[0;32mgreen\x1b[m\n" +
		"a\tb\x1b[1;38:2::1:2:3mc\td\n" +
		"after"
	want := []string{
		"$ ls",
		"\x1b[31mred \x1b[1mbold\x1b[22m",
		"\x1b[31m\x1b[1m\x1b[22mmoved here",
		"\x1b[31m\x1b[1m\x1b[22mlink1A",
		"\x1b[31m\x1b[1m\x1b[22m\x1b[0;32mgreen\x1b[m",
		"a       b\x1b[1;38:2::1:2:3mc      d",
		"\x1b[1;38:2::1:2:3mafter",
	}
	if got := StyledLines(in); !reflect.DeepEqual(got, want) {
		t.Errorf("StyledLines =\n%q\nwant\n%q", got, want)
	}
}
