package main

import (
	"reflect"
	"testing"
)

// TestClassify pins down FR-18 exactly: every reserved word dispatches to
// its subcommand, `--` forces literal capture even over a reserved word or
// a leading dash, and version has three spellings.
func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want dispatch
	}{
		{"bare", nil, dispatch{action: actionDefaultList}},
		{
			"plain capture", []string{"buy milk", "errand"},
			dispatch{action: actionCapture, args: []string{"buy milk", "errand"}},
		},
		{
			"reserved word dispatches to subcommand",
			[]string{"list", "--tag", "foo"},
			dispatch{action: actionSubcommand, args: []string{"list", "--tag", "foo"}},
		},
		{
			"one-word note matching a reserved word requires --",
			[]string{"list"},
			dispatch{action: actionSubcommand, args: []string{"list"}},
		},
		{
			"-- escapes a reserved word into literal capture",
			[]string{"--", "list"},
			dispatch{action: actionCapture, args: []string{"list"}},
		},
		{
			"-- escapes a leading-dash body into literal capture",
			[]string{"--", "-1 story point"},
			dispatch{action: actionCapture, args: []string{"-1 story point"}},
		},
		{
			"-- with tags after the captured body",
			[]string{"--", "list", "meta"},
			dispatch{action: actionCapture, args: []string{"list", "meta"}},
		},
		{"version long", []string{"--version"}, dispatch{action: actionVersion}},
		{"version short", []string{"-v"}, dispatch{action: actionVersion}},
		{"version word", []string{"version"}, dispatch{action: actionVersion}},
		{
			"help routes to subcommand tree, not capture",
			[]string{"--help"},
			dispatch{action: actionSubcommand, args: []string{"--help"}},
		},
	}

	// Exercise every reserved word generically too, so adding one to the
	// map without a dedicated case above still gets covered.
	for word := range reserved {
		cases = append(cases, struct {
			name string
			args []string
			want dispatch
		}{
			name: "reserved: " + word,
			args: []string{word},
			want: dispatch{action: actionSubcommand, args: []string{word}},
		})
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classify(tc.args)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("classify(%v) = %+v, want %+v", tc.args, got, tc.want)
			}
		})
	}
}
