package workspace

import (
	"math/rand"
	"strings"
	"testing"
)

func serialFormatJSON(data []byte, st *JSONFormatterState) string {
	return string(appendFormatJSON(nil, data, st))
}

func randomJSONDoc(r *rand.Rand, min int) []byte {
	var b strings.Builder
	b.Grow(min + 4096)
	var write func(depth int)
	write = func(depth int) {
		switch {
		case depth > 6 || r.Intn(4) == 0:
			switch r.Intn(6) {
			case 0:
				b.WriteString(`"plain string"`)
			case 1:
				b.WriteString(`"esc \" \\ \n é tail"`)
			case 2:
				b.WriteString(`"backslashes \\\\\\\" done"`)
			case 3:
				b.WriteString("-12.5e+7")
			case 4:
				b.WriteString("null")
			default:
				b.WriteString("true")
			}
		case r.Intn(2) == 0:
			if r.Intn(20) == 0 {
				b.WriteString("[ ]")
				return
			}
			b.WriteByte('[')
			n := 1 + r.Intn(4)
			for i := 0; i < n; i++ {
				if i > 0 {
					b.WriteByte(',')
				}
				if r.Intn(5) == 0 {
					b.WriteString("\n  ")
				}
				write(depth + 1)
			}
			b.WriteByte(']')
		default:
			if r.Intn(20) == 0 {
				b.WriteString("{}")
				return
			}
			b.WriteByte('{')
			n := 1 + r.Intn(4)
			for i := 0; i < n; i++ {
				if i > 0 {
					b.WriteByte(',')
				}
				b.WriteString(`"k`)
				b.WriteString(string(rune('a' + r.Intn(26))))
				b.WriteString(`":`)
				if r.Intn(3) == 0 {
					b.WriteByte(' ')
				}
				write(depth + 1)
			}
			b.WriteByte('}')
		}
	}
	b.WriteByte('[')
	for i := 0; b.Len() < min; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		write(0)
	}
	b.WriteByte(']')
	return []byte(b.String())
}

func TestFormatJSONParallelMatchesSerial(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	for iter := 0; iter < 6; iter++ {
		doc := randomJSONDoc(r, jsonParallelMin+r.Intn(1<<20))

		wantState := JSONFormatterState{}
		want := serialFormatJSON(doc, &wantState)

		gotState := JSONFormatterState{}
		got, ok := formatJSONParallel(doc, &gotState)
		if !ok {
			t.Fatalf("iter %d: parallel path declined a %d B document", iter, len(doc))
		}
		if got != want {
			t.Fatalf("iter %d: parallel output differs from serial (len %d vs %d)", iter, len(got), len(want))
		}
		if gotState != wantState {
			t.Fatalf("iter %d: end state %+v, want %+v", iter, gotState, wantState)
		}
	}
}

func TestFormatJSONParallelResumesStreamState(t *testing.T) {
	r := rand.New(rand.NewSource(11))
	doc := randomJSONDoc(r, 3*jsonParallelMin)

	for _, cut := range []int{1, 7777, 1 << 20, jsonParallelMin + 12345} {
		if cut >= len(doc) {
			continue
		}
		wantState := JSONFormatterState{}
		want := serialFormatJSON(doc, &wantState)

		gotState := JSONFormatterState{}
		got := formatJSON(doc[:cut], &gotState) + formatJSON(doc[cut:], &gotState)
		if got != want {
			t.Fatalf("cut %d: streamed output differs from single-shot", cut)
		}
		if gotState != wantState {
			t.Fatalf("cut %d: end state %+v, want %+v", cut, gotState, wantState)
		}
	}
}

func TestFormatJSONParallelEdgeInputs(t *testing.T) {
	pad := strings.Repeat(`{"a":1},`, jsonParallelMin/8)
	cases := []struct {
		name     string
		src      string
		parallel bool
	}{
		{"empty containers", "[" + pad + strings.Repeat("[ ],{ },", 4096) + "0]", true},
		{"one huge string", `["` + strings.Repeat("x", jsonParallelMin) + `\\","tail"]`, true},
		{"escape heavy", `[` + strings.Repeat(`"a\\\"b\n",`, jsonParallelMin/10) + `0]`, true},
		{"whitespace heavy", "[" + strings.Repeat("1 ,\n\t", jsonParallelMin/5) + "2]", true},
		{"not json", strings.Repeat("plain text without structure ", jsonParallelMin/29), true},
		{"truncated", "[" + pad + `{"a":"unterminated`, true},
		{"trailing spaces", "[" + pad + "0]" + strings.Repeat(" ", jsonParallelMin), true},
		{"leading spaces", strings.Repeat(" ", jsonParallelMin) + "[" + pad + "0]", true},
		{"all whitespace", strings.Repeat(" \n\t", jsonParallelMin), false},
		{"deep nesting", strings.Repeat("[", 80) + pad + "1" + strings.Repeat("]", 80), false},
		{"unbalanced close", "[" + pad + "0]]]]", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data := []byte(c.src)
			wantState := JSONFormatterState{}
			want := serialFormatJSON(data, &wantState)

			gotState := JSONFormatterState{}
			got := formatJSON(data, &gotState)
			if got != want {
				t.Fatalf("output differs from serial (len %d vs %d)", len(got), len(want))
			}
			if gotState != wantState {
				t.Fatalf("end state %+v, want %+v", gotState, wantState)
			}

			checkState := JSONFormatterState{}
			if _, ok := formatJSONParallel(data, &checkState); ok != c.parallel {
				t.Fatalf("parallel path used = %v, want %v", ok, c.parallel)
			}
		})
	}
}
