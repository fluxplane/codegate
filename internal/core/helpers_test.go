package core

import "testing"

func TestCleanPathAndPositions(t *testing.T) {
	if got := CleanPath(`./a\..\b//c.go`); got != "b/c.go" {
		t.Fatalf("unexpected clean path %q", got)
	}
	src := []byte("one\ntwo\n")
	if got := LineColumnOffset(src, 2, 2); got != 5 {
		t.Fatalf("unexpected offset %d", got)
	}
	if got := PositionForOffset(src, 5); got.Line != 2 || got.Column != 2 || got.Offset != 5 {
		t.Fatalf("unexpected position %#v", got)
	}
}

func TestDebtMarkersIgnoreMarkdownCodeSpansAndFences(t *testing.T) {
	src := []byte("TODO: visible\n`TODO: ignored`\n```\nFIXME: ignored\n```\nHACK: visible\n")
	markers := FindMarkdownDebtMarkers("README.md", src)
	counts := CountDebtMarkers(markers)
	if counts["TODO"] != 1 || counts["HACK"] != 1 || counts["FIXME"] != 0 {
		t.Fatalf("unexpected debt marker counts %#v", counts)
	}
}
