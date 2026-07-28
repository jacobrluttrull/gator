package auth

import "golang.org/x/crypto/bcrypt"

// HashPassword returns the bcrypt hash of a password; the plaintext is
// never stored.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPasswordHash returns nil when password matches the bcrypt hash,
// and an error otherwise.
func CheckPasswordHash(password, hash string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
