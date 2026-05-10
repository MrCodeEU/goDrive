package auth

import (
	"errors"
	"testing"
)

func TestHashAndVerifyPassword(t *testing.T) {
	t.Parallel()

	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyPassword("correct horse battery staple", hash); err != nil {
		t.Fatalf("VerifyPassword correct password err = %v", err)
	}
	if err := VerifyPassword("wrong", hash); !errors.Is(err, ErrPasswordMismatch) {
		t.Fatalf("VerifyPassword wrong password err = %v, want ErrPasswordMismatch", err)
	}
}

func TestHashTokenIsStable(t *testing.T) {
	t.Parallel()

	first := HashToken("session-token")
	second := HashToken("session-token")
	if first == "" || first != second {
		t.Fatalf("HashToken was not stable")
	}
}
