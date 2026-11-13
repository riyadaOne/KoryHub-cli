package diff

import "testing"

func render(h []Hunk) string {
	out := ""
	for _, hunk := range h {
		for _, l := range hunk.Lines {
			out += map[Kind]string{Same: " ", Removed: "-", Added: "+"}[l.Kind] + l.Text + "\n"
		}
		out += "@@\n"
	}

	return out
}

func TestComputeShowsChangedLinesWithThreeLinesOfContext(t *testing.T) {
	before := Lines("a\nb\nc\nd\ne\nf\ng\nh\ni\nj\nk\nl\nm\n")
	after := Lines("a\nb\nc\nd\nE\nf\ng\nh\ni\nj\nk\nl\nm\nn\n")

	hunks, err := Compute(before, after, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(hunks) != 2 {
		t.Fatalf("blocs : %d\n%s", len(hunks), render(hunks))
	}
	want := " b\n c\n d\n-e\n+E\n f\n g\n h\n@@\n k\n l\n m\n+n\n@@\n"
	if got := render(hunks); got != want {
		t.Errorf("différence :\n%s\nattendu :\n%s", got, want)
	}
	if hunks[0].FromLine != 2 || hunks[0].ToLine != 2 {
		t.Errorf("lignes : %d, %d", hunks[0].FromLine, hunks[0].ToLine)
	}
}

func TestIdenticalTextsAndLineEndingsGiveNoHunk(t *testing.T) {
	hunks, _ := Compute(Lines("<?php\r\nfinal class A {}\r\n"), Lines("<?php\nfinal class A {}\n"), 3)
	if len(hunks) != 0 {
		t.Errorf("blocs : %v", hunks)
	}

	hunks, _ = Compute(nil, Lines("x\n"), 3)
	if len(hunks) != 1 || hunks[0].Lines[0].Kind != Added {
		t.Errorf("fichier vide : %v", hunks)
	}
}
