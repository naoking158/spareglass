package pairing

import "testing"

func TestToken(t *testing.T) {
	s, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	tok := Token(s)
	if !tok.Check(s) {
		t.Fatal("token should match itself")
	}
	if tok.Check("") || tok.Check(s+"x") {
		t.Fatal("token should not match other values")
	}
}
