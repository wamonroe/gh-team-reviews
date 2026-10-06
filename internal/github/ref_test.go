package github

import "testing"

func TestParseRef(t *testing.T) {
	valid := map[string]Ref{
		"https://github.com/acme/api/pull/42":               {Repo: "acme/api", Number: 42},
		"https://github.com/acme/api/pull/42/files":         {Repo: "acme/api", Number: 42},
		"https://github.com/acme/api/pull/42#discussion_r1": {Repo: "acme/api", Number: 42},
		"https://ghe.example.com/acme/api.v2/pull/7?w=1":    {Repo: "acme/api.v2", Number: 7},
		"acme/api#42": {Repo: "acme/api", Number: 42},
	}
	for input, want := range valid {
		got, err := ParseRef(input)
		if err != nil || got != want {
			t.Errorf("ParseRef(%q) = %v, %v; want %v", input, got, err, want)
		}
	}

	for _, input := range []string{
		"", "42", "#42", "acme#42", "acme/api", "acme/api#0", "acme/api/extra#42",
		"https://github.com/acme/api/issues/42", "https://github.com/acme/api/pull/",
	} {
		if got, err := ParseRef(input); err == nil {
			t.Errorf("ParseRef(%q) = %v, want an error", input, got)
		}
	}
}
