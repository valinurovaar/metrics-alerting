package signature

import "testing"

func TestCalculateAndValid(t *testing.T) {
	const (
		key  = "key"
		want = "f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8"
	)
	value := []byte("The quick brown fox jumps over the lazy dog")

	if got := Calculate(value, key); got != want {
		t.Fatalf("Calculate() = %q, want %q", got, want)
	}
	if !Valid(value, key, want) {
		t.Fatal("Valid() rejected a valid signature")
	}
	if Valid([]byte("changed"), key, want) {
		t.Fatal("Valid() accepted a signature for another body")
	}
}
