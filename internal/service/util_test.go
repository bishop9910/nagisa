package service

import "testing"

// TestNormalizeBoolLiterals covers the rewrite that lets clients write boolean
// comparisons even though the AIP grammar has no boolean literals.
func TestNormalizeBoolLiterals(t *testing.T) {
	cases := []struct {
		name   string
		filter string
		want   string
	}{
		{"empty", "", ""},
		{"true becomes the bare field", "success=true", "success"},
		{"false becomes a negation", "success=false", "NOT success"},
		{"not equals true", "success!=true", "NOT success"},
		{"not equals false", "success!=false", "success"},
		{"uppercase literal", "success=TRUE", "success"},
		{"mixed case literal", "success=False", "NOT success"},
		{"spaces around the operator", "success   =   false", "NOT success"},
		{"combined with another field", "success=false AND action:\"node.\"", "NOT success AND action:\"node.\""},
		{"left of the conjunction", "success=true OR success=false", "success OR NOT success"},
		{"already bare", "success", "success"},
		{"already negated", "NOT success", "NOT success"},
		{"double negation stays intact", "NOT success=false", "NOT NOT success"},
		{"other fields are untouched", "size=true", "size=true"},
		{"quoted values are untouched", "action:\"success=true\"", "action:\"success=true\""},
		{"quoted value next to a rewrite", "name=\"a=b\" AND success=false", "name=\"a=b\" AND NOT success"},
		{"escaped quote inside a value", `action:"a\"b" AND success=true`, `action:"a\"b" AND success`},
		{"identifier prefix is not rewritten", "success_rate=true", "success_rate=true"},
		{"non boolean literal untouched", "success=1", "success=1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeBoolLiterals(tc.filter, "success"); got != tc.want {
				t.Fatalf("normalizeBoolLiterals(%q) = %q, want %q", tc.filter, got, tc.want)
			}
		})
	}
}

// TestNormalizeBoolLiteralsWithoutFields leaves every expression alone when the
// caller declares no boolean fields.
func TestNormalizeBoolLiteralsWithoutFields(t *testing.T) {
	const filter = "success=true"
	if got := normalizeBoolLiterals(filter); got != filter {
		t.Fatalf("normalizeBoolLiterals(%q) = %q, want it unchanged", filter, got)
	}
}
