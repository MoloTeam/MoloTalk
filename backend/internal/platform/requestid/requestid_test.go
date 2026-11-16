package requestid_test

import (
	"context"
	"strings"
	"testing"

	"github.com/oklog/ulid/v2"

	"github.com/MoloTeam/MoloTalk/backend/internal/platform/idgen"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/requestid"
)

func TestValidValues(t *testing.T) {
	t.Parallel()

	cases := map[string]bool{
		"a":                          true,
		"A":                          true,
		"0":                          true,
		strings.Repeat("a", 64):      true,
		"abc.def-ghi_jkl":            true,
		"01ARZ3NDEKTSV4RRFFQ69G5FAV": true,
		"":                           false,
		strings.Repeat("a", 65):      false,
		".abc":                       false,
		"-abc":                       false,
		"_abc":                       false,
		"a,b":                        false,
		"a b":                        false,
		"a/b":                        false,
		"идентификатор":              false,
	}

	for value, want := range cases {
		if got := requestid.Valid(value); got != want {
			t.Errorf("Valid(%q) = %v, want %v", value, got, want)
		}
	}
}

func TestCleanValueIsKept(t *testing.T) {
	t.Parallel()

	generated := ulid.MustParse("01ARZ3NDEKTSV4RRFFQ69G5FAV")

	for _, value := range []string{"a", "0123456789abcdef", strings.Repeat("a", 64)} {
		got, err := requestid.Resolve([]string{value}, idgen.NewSequence(generated))
		if err != nil {
			t.Fatalf("resolve %q: %v", value, err)
		}
		if got != value {
			t.Errorf("id = %q, want the incoming value %q", got, value)
		}
	}
}

func TestAmbiguousValuesAreReplaced(t *testing.T) {
	t.Parallel()

	generated := ulid.MustParse("01ARZ3NDEKTSV4RRFFQ69G5FAV")

	for name, values := range map[string][]string{
		"absent":     nil,
		"empty":      {""},
		"duplicated": {"first", "second"},
		"invalid":    {".leading-dot"},
	} {
		got, err := requestid.Resolve(values, idgen.NewSequence(generated))
		if err != nil {
			t.Fatalf("%s: resolve: %v", name, err)
		}
		if got != generated.String() {
			t.Errorf("%s: id = %q, want the generated value", name, got)
		}
	}
}

func TestContextRoundTrip(t *testing.T) {
	t.Parallel()

	const want = "operation-identifier"
	ctx := requestid.WithContext(context.Background(), want)

	got, ok := requestid.FromContext(ctx)
	if !ok {
		t.Fatal("identifier is missing in the context")
	}
	if got != want {
		t.Fatalf("id = %q, want %q", got, want)
	}

	if _, ok := requestid.FromContext(context.Background()); ok {
		t.Fatal("empty context returned an identifier")
	}
}
