package workspace

import "testing"

func TestSplitURLQuery(t *testing.T) {
	cases := []struct {
		in                    string
		base, query, fragment string
	}{
		{"https://x/y", "https://x/y", "", ""},
		{"https://x/y?a=1&b=2", "https://x/y", "a=1&b=2", ""},
		{"https://x/y?a=1#frag", "https://x/y", "a=1", "#frag"},
		{"https://x/y#frag", "https://x/y", "", "#frag"},
	}
	for _, c := range cases {
		b, q, f := splitURLQuery(c.in)
		if b != c.base || q != c.query || f != c.fragment {
			t.Errorf("splitURLQuery(%q) = (%q,%q,%q), want (%q,%q,%q)", c.in, b, q, f, c.base, c.query, c.fragment)
		}
	}
}

func TestParamsSyncFromURL(t *testing.T) {
	tab := NewRequestTab("t")
	tab.URLInput.SetText("https://api.example.com/users?page=2&limit=50&flag")
	tab.syncParamsFromURL()
	if len(tab.Params) != 3 {
		t.Fatalf("expected 3 params, got %d", len(tab.Params))
	}
	want := [][2]string{{"page", "2"}, {"limit", "50"}, {"flag", ""}}
	for i, w := range want {
		if tab.Params[i].Key.Text() != w[0] || tab.Params[i].Value.Text() != w[1] {
			t.Errorf("param[%d] = (%q,%q), want (%q,%q)", i, tab.Params[i].Key.Text(), tab.Params[i].Value.Text(), w[0], w[1])
		}
	}
}

func TestParamsSyncToURL(t *testing.T) {
	tab := NewRequestTab("t")
	tab.URLInput.SetText("https://api.example.com/users#top")
	tab.addParam("q", "gophers")
	tab.addParam("n", "10")
	tab.addParam("", "")
	tab.syncURLFromParams()
	got := tab.URLInput.Text()
	want := "https://api.example.com/users?q=gophers&n=10#top"
	if got != want {
		t.Errorf("URL = %q, want %q", got, want)
	}
}

func TestAuthHeaderValue(t *testing.T) {
	tab := NewRequestTab("t")
	if v := tab.authHeaderValue(nil); v != "" {
		t.Errorf("none auth = %q, want empty", v)
	}

	tab.AuthType = authBearer
	tab.AuthToken.SetText("abc.def")
	if v := tab.authHeaderValue(nil); v != "Bearer abc.def" {
		t.Errorf("bearer = %q", v)
	}

	tab.AuthType = authBasic
	tab.AuthUser.SetText("user")
	tab.AuthPass.SetText("pass")
	if v := tab.authHeaderValue(nil); v != "Basic dXNlcjpwYXNz" {
		t.Errorf("basic = %q, want Basic dXNlcjpwYXNz", v)
	}
}

func TestCookieHeaderValue(t *testing.T) {
	tab := NewRequestTab("t")
	if v := tab.cookieHeaderValue(nil); v != "" {
		t.Errorf("empty cookies = %q", v)
	}
	tab.addCookie("session_id", "abc123")
	tab.addCookie("theme", "dark")
	tab.addCookie("", "ignored")
	if v := tab.cookieHeaderValue(nil); v != "session_id=abc123; theme=dark" {
		t.Errorf("cookies = %q", v)
	}
}
