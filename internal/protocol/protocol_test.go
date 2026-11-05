package protocol

import (
	"strings"
	"testing"

	"github.com/NandavardhanRadhakrishnan/HopBack/internal/model"
)

func TestHopbackMD_Content(t *testing.T) {
	s := string(HopbackMD())
	for _, want := range []string{Marker(), "## Objective", "## Done", "## Stopped at", "## Unfinished", "## Gotchas", "## Next step",
		"hopback session park", "hopback batch", "--origin user|agent", "hopback architecture change"} {
		if !strings.Contains(s, want) {
			t.Fatalf("HOPBACK.md missing %q", want)
		}
	}
}

func TestParseMarkerVersion(t *testing.T) {
	if v, ok := ParseMarkerVersion([]byte("x\n<!-- hopback-template v3 -->\n")); v != 3 || !ok {
		t.Fatal(v, ok)
	}
	if _, ok := ParseMarkerVersion([]byte("plain")); ok {
		t.Fatal("found marker in plain text")
	}
}

func TestParkSkill_FrontmatterFirst(t *testing.T) {
	s := string(ParkSkill())
	if !strings.HasPrefix(s, "---\nname: park\n") || !strings.Contains(s, Marker()) {
		t.Fatal(s)
	}
}

func TestPointerBlock(t *testing.T) {
	b := PointerBlock()
	if !strings.HasPrefix(b, "<!-- hopback:begin -->") || !strings.HasSuffix(b, "<!-- hopback:end -->\n") {
		t.Fatal(b)
	}
}

func TestPrompts(t *testing.T) {
	if p := BuildPrompt(&model.Task{ID: "T-12", Title: "Diff view"}); !strings.Contains(p, "T-12: Diff view") {
		t.Fatal(p)
	}
	if p := BuildPrompt(nil); !strings.Contains(p, "ask me what to work on") {
		t.Fatal(p)
	}
	if p := ConciergePrompt([]string{"a", "b"}, false); !strings.Contains(p, "- a\n- b") {
		t.Fatal(p)
	}
	if p := ConciergePrompt(nil, true); !strings.Contains(p, "first concierge session") {
		t.Fatal(p)
	}
	if len(BuildInstructions()) >= 2000 || len(ConciergeInstructions()) >= 2000 {
		t.Fatal("instructions too long")
	}
}
