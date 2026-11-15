package core

import (
	"strings"
	"testing"
)

func TestRedact(t *testing.T) {
	resetSecrets()
	t.Cleanup(resetSecrets)
	rememberSecret("tok-123456")
	rememberSecret("tok-123456-longer")
	rememberSecret("abc") // too short to redact

	for in, want := range map[string]string{
		`Get "https://h/a.csv?token=tok-123456&x=1": 404`:   `Get "https://h/a.csv?token=[redacted]&x=[redacted]": 404`,
		"value tok-123456-longer here":                      "value [redacted] here",
		"https://user:pw@example.com/x has userinfo":        "https://[redacted]@example.com/x has userinfo",
		"git://example.com/r.git?ref=a#frag stays readable": "git://example.com/r.git?ref=[redacted]#frag stays readable",
		"no url, abc stays":                                 "no url, abc stays",
		"":                                                  "",
	} {
		if got := redact(in); got != want {
			t.Errorf("redact(%q)\n got %q\nwant %q", in, got, want)
		}
	}
}

func TestExpansionRemembersSubstitutedValues(t *testing.T) {
	resetSecrets()
	t.Cleanup(resetSecrets)
	t.Setenv("DATUM_TEST_REMEMBERED", "remember-me-please")
	if _, err := expandEnvValue("x${DATUM_TEST_REMEMBERED}y", false); err != nil {
		t.Fatal(err)
	}
	if got := redact("a remember-me-please b"); strings.Contains(got, "remember-me") {
		t.Errorf("redact() = %q", got)
	}
}

func TestLenientExpansionKeepsUnsetReferences(t *testing.T) {
	t.Setenv("DATUM_TEST_SET", "v")
	got, err := expandEnvValue("${DATUM_TEST_SET}/${DATUM_TEST_DEFINITELY_UNSET_25}/$${LIT}", true)
	if err != nil || got != "v/${DATUM_TEST_DEFINITELY_UNSET_25}/${LIT}" {
		t.Errorf("expandEnvValue(lenient) = %q, %v", got, err)
	}
	if _, err := expandEnvValue("${BAD-NAME}", true); err == nil {
		t.Error("a malformed reference must fail even when lenient")
	}
}

func TestEnvReferences(t *testing.T) {
	got := envReferences("a ${ONE} $${ESCAPED} ${TWO}${THREE} $PLAIN ${UNTERMINATED")
	if strings.Join(got, ",") != "ONE,TWO,THREE" {
		t.Errorf("envReferences() = %v", got)
	}
}
