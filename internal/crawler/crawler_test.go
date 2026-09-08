package crawler

import (
	"testing"
)

func TestSplitURLs(t *testing.T) {
	tests := []struct {
		name string
		urls []string
		n    int
		want [][]string
	}{
		{
			name: "even distribution",
			urls: []string{"a", "b", "c", "d"},
			n:    2,
			want: [][]string{{"a", "c"}, {"b", "d"}},
		},
		{
			name: "more urls than chunks",
			urls: []string{"a", "b", "c", "d", "e"},
			n:    3,
			want: [][]string{{"a", "d"}, {"b", "e"}, {"c"}},
		},
		{
			name: "fewer urls than chunks",
			urls: []string{"a", "b"},
			n:    5,
			want: [][]string{{"a"}, {"b"}, nil, nil, nil},
		},
		{
			name: "single chunk",
			urls: []string{"a", "b", "c"},
			n:    1,
			want: [][]string{{"a", "b", "c"}},
		},
		{
			name: "empty urls",
			urls: []string{},
			n:    3,
			want: [][]string{nil, nil, nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitURLs(tt.urls, tt.n)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if len(got[i]) != len(tt.want[i]) {
					t.Errorf("chunk[%d] len = %d, want %d", i, len(got[i]), len(tt.want[i]))
					continue
				}
				for j := range got[i] {
					if got[i][j] != tt.want[i][j] {
						t.Errorf("chunk[%d][%d] = %q, want %q", i, j, got[i][j], tt.want[i][j])
					}
				}
			}
		})
	}
}

func TestContainsAny(t *testing.T) {
	tests := []struct {
		name string
		s    string
		subs []string
		want bool
	}{
		{"match found", "hello world", []string{"world", "foo"}, true},
		{"no match", "hello world", []string{"foo", "bar"}, false},
		{"empty string", "", []string{"a"}, false},
		{"empty subs", "hello", []string{}, false},
		{"exact match", "abc", []string{"abc"}, true},
		{"substring at start", "prefix_suffix", []string{"prefix"}, true},
		{"substring at end", "prefix_suffix", []string{"suffix"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsAny(tt.s, tt.subs)
			if got != tt.want {
				t.Errorf("containsAny(%q, %v) = %v, want %v", tt.s, tt.subs, got, tt.want)
			}
		})
	}
}

func TestToString(t *testing.T) {
	tests := []struct {
		name string
		v    any
		want string
	}{
		{"string value", "hello", "hello"},
		{"nil value", nil, ""},
		{"int value", 42, "42"},
		{"float value", 3.14, "3.14"},
		{"bool value", true, "true"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toString(tt.v)
			if got != tt.want {
				t.Errorf("toString(%v) = %q, want %q", tt.v, got, tt.want)
			}
		})
	}
}

func TestToFloat(t *testing.T) {
	tests := []struct {
		name string
		v    any
		want float64
	}{
		{"float64 value", 3.14, 3.14},
		{"nil value", nil, 0},
		{"int value", 42, 0}, // int is not float64, returns 0
		{"string value", "3.14", 0},
		{"zero float", 0.0, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toFloat(tt.v)
			if got != tt.want {
				t.Errorf("toFloat(%v) = %v, want %v", tt.v, got, tt.want)
			}
		})
	}
}

func TestMapGet(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]any
		key  string
		want any
	}{
		{"key exists", map[string]any{"a": 1, "b": 2}, "a", 1},
		{"key missing", map[string]any{"a": 1}, "b", nil},
		{"nil map", nil, "a", nil},
		{"empty map", map[string]any{}, "a", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapGet(tt.m, tt.key)
			if got != tt.want {
				t.Errorf("mapGet(%v, %q) = %v, want %v", tt.m, tt.key, got, tt.want)
			}
		})
	}
}

func TestMin(t *testing.T) {
	tests := []struct {
		name string
		a, b int
		want int
	}{
		{"a less than b", 3, 5, 3},
		{"b less than a", 7, 2, 2},
		{"equal", 4, 4, 4},
		{"negative", -5, 10, -5},
		{"zero", 0, 5, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := min(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("min(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestBackoffDuration(t *testing.T) {
	tests := []struct {
		attempt int
		want    string
	}{
		{1, "1s"},
		{2, "2s"},
		{3, "4s"},
		{4, "8s"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := backoffDuration(tt.attempt)
			if got.String() != tt.want {
				t.Errorf("backoffDuration(%d) = %v, want %v", tt.attempt, got, tt.want)
			}
		})
	}
}
