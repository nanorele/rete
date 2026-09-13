package widgets

import "testing"

func TestFindVarNestedBraces(t *testing.T) {
	s := `'[[[['''''[[[[[[[[{{{{{{{{{{{{{sdgsgds}}}}}}}}}}}}}]]]]]]]]''''']]]]'`
	start, end, ok := FindVar(s, 0)
	if !ok {
		t.Fatal("expected a variable")
	}
	if got := s[start:end]; got != "{{sdgsgds}}" {
		t.Fatalf("variable boundary = %q, want {{sdgsgds}}", got)
	}
	if _, _, ok := FindVar(s, end); ok {
		t.Fatal("no second variable expected")
	}
	if bs, be, ok := FindVar([]byte(s), 0); !ok || bs != start || be != end {
		t.Fatalf("byte variant = (%d,%d,%v), want (%d,%d,true)", bs, be, ok, start, end)
	}
}

func TestFindVarCases(t *testing.T) {
	cases := []struct {
		in   string
		from int
		want string
		ok   bool
	}{
		{"{{a}}", 0, "{{a}}", true},
		{"x{{ a }}y{{b}}", 0, "{{ a }}", true},
		{"x{{ a }}y{{b}}", 3, "{{b}}", true},
		{"{{a}b}}", 0, "", false},
		{"{{a{b}}", 0, "", false},
		{"{{{a}}}", 0, "{{a}}", true},
		{"{{}}", 0, "{{}}", true},
		{"{{a", 0, "", false},
		{"a}}", 0, "", false},
		{"", 0, "", false},
	}
	for _, c := range cases {
		s, e, ok := FindVar(c.in, c.from)
		if ok != c.ok || (ok && c.in[s:e] != c.want) {
			got := ""
			if ok {
				got = c.in[s:e]
			}
			t.Errorf("FindVar(%q, %d) = %q,%v want %q,%v", c.in, c.from, got, ok, c.want, c.ok)
		}
	}
}
