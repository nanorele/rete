package faker

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestEveryNameGenerates(t *testing.T) {
	if n := len(Names()); n != 118 {
		t.Fatalf("expected 118 dynamic variables, got %d", n)
	}
	for _, name := range Names() {
		if !strings.HasPrefix(name, "$") {
			t.Errorf("%s: dynamic names must start with $", name)
		}
		if !IsDynamic(name) {
			t.Errorf("%s: IsDynamic false", name)
		}
		if Describe(name) == "" {
			t.Errorf("%s: missing description", name)
		}
		for i := 0; i < 20; i++ {
			v, ok := Lookup(name)
			if !ok {
				t.Fatalf("%s: Lookup failed", name)
			}
			if v == "" {
				t.Errorf("%s: produced empty value", name)
			}
		}
	}
	if _, ok := Lookup("$nope"); ok {
		t.Error("unknown name resolved")
	}
	if IsDynamic("base_url") {
		t.Error("plain env names are not dynamic")
	}
}

func TestFormats(t *testing.T) {
	uuidRe := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	for _, n := range []string{"$guid", "$randomUUID"} {
		v, _ := Lookup(n)
		if !uuidRe.MatchString(v) {
			t.Errorf("%s: bad uuid %q", n, v)
		}
	}
	ts, _ := Lookup("$timestamp")
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || time.Now().Unix()-sec > 5 {
		t.Errorf("$timestamp: %q", ts)
	}
	iso, _ := Lookup("$isoTimestamp")
	if _, err := time.Parse("2006-01-02T15:04:05.000Z", iso); err != nil {
		t.Errorf("$isoTimestamp: %q: %v", iso, err)
	}
	for i := 0; i < 100; i++ {
		v, _ := Lookup("$randomInt")
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 1000 {
			t.Fatalf("$randomInt: %q", v)
		}
		p, _ := Lookup("$randomPrice")
		f, err := strconv.ParseFloat(p, 64)
		if err != nil || f < 0 || f > 1000 {
			t.Fatalf("$randomPrice: %q", p)
		}
	}
	ip, _ := Lookup("$randomIP")
	if !regexp.MustCompile(`^\d{1,3}(\.\d{1,3}){3}$`).MatchString(ip) {
		t.Errorf("$randomIP: %q", ip)
	}
	hexc, _ := Lookup("$randomHexColor")
	if !regexp.MustCompile(`^#[0-9a-f]{6}$`).MatchString(hexc) {
		t.Errorf("$randomHexColor: %q", hexc)
	}
	phone, _ := Lookup("$randomPhoneNumber")
	if !regexp.MustCompile(`^\d{3}-\d{3}-\d{4}$`).MatchString(phone) {
		t.Errorf("$randomPhoneNumber: %q", phone)
	}
	pw, _ := Lookup("$randomPassword")
	if len(pw) != 15 {
		t.Errorf("$randomPassword: %q", pw)
	}
	email, _ := Lookup("$randomExampleEmail")
	if !strings.Contains(email, "@example.") {
		t.Errorf("$randomExampleEmail: %q", email)
	}
	cc, _ := Lookup("$randomCountryCode")
	if len(cc) != 2 {
		t.Errorf("$randomCountryCode: %q", cc)
	}
	ba, _ := Lookup("$randomBankAccount")
	if len(ba) != 8 {
		t.Errorf("$randomBankAccount: %q", ba)
	}
}
