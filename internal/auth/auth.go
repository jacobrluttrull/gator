package auth

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// MaxPasswordBytes is bcrypt's hard input limit. A longer password is bad
// input, not a server fault — password managers emit passphrases past this
// length routinely — so both surfaces check it and say so.
const MaxPasswordBytes = 72

// ErrPasswordTooLong reports a password over MaxPasswordBytes. Callers map
// it to their surface's "you sent something invalid" response.
var ErrPasswordTooLong = errors.New("password must be at most 72 bytes")

// HashPassword returns the bcrypt hash of a password; the plaintext is
// never stored. A password over MaxPasswordBytes returns
// ErrPasswordTooLong.
func HashPassword(password string) (string, error) {
	if len(password) > MaxPasswordBytes {
		return "", ErrPasswordTooLong
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		// Defence in depth: bcrypt enforces the same limit itself, and a
		// future change to it shouldn't resurface as a bare 500.
		if errors.Is(err, bcrypt.ErrPasswordTooLong) {
			return "", ErrPasswordTooLong
		}
		return "", fmt.Errorf("hashing password: %w", err)
	}
	return string(hash), nil
}

// CheckPasswordHash returns nil when password matches the bcrypt hash,
// and an error otherwise.
func CheckPasswordHash(password, hash string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
