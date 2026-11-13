package semver

import "testing"

func TestParseAcceptsOnlyThreeNumbers(t *testing.T) {
	for _, valid := range []string{"0.1.0", "1.2.3", "10.20.30"} {
		if _, err := Parse(valid); err != nil {
			t.Errorf("Parse(%q) : %v", valid, err)
		}
	}

	for _, invalid := range []string{"1.0", "1", "1.2.3.4", "1.x.3", "", "-1.0.0", "1..2"} {
		if _, err := Parse(invalid); err == nil {
			t.Errorf("Parse(%q) aurait dû échouer", invalid)
		}
	}
}

func TestCompareUsesNumbersNotText(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.9.0", "0.10.0", -1},
		{"1.0.0", "1.0.0", 0},
		{"2.0.0", "1.99.99", 1},
		{"1.2.10", "1.2.9", 1},
	}

	for _, c := range cases {
		a, _ := Parse(c.a)
		b, _ := Parse(c.b)
		if got := a.Compare(b); got != c.want {
			t.Errorf("%s comparé à %s = %d ; attendu %d", c.a, c.b, got, c.want)
		}
	}
}
