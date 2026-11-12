package httpapi

import "testing"

func TestCSVTextNeutralizesFormulaPrefixes(t *testing.T) {
	cases := map[string]string{
		"=1+1":        "'=1+1",
		"+cmd":        "'+cmd",
		"-2+3":        "'-2+3",
		"@SUM(A1:A2)": "'@SUM(A1:A2)",
		"\tawal":      "'\tawal",
		"\rawal":      "'\rawal",
		"Nama Biasa":  "Nama Biasa",
		"":            "",
	}
	for input, want := range cases {
		if got := csvText(input); got != want {
			t.Fatalf("csvText(%q) = %q, mau %q", input, got, want)
		}
	}
}
