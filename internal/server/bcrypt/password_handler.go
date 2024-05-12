// Package bcrypt can be used to hash data.
package bcrypt

import "golang.org/x/crypto/bcrypt"

// PasswordHandler hashes and checks passwords.
type PasswordHandler struct {
	// cost represents the number of rounds, log 2, that are made when hashing a password.
	cost int
}

// NewPasswordHandler creates a bcrypt PasswordHandler with a default hash cost.
func NewPasswordHandler() *PasswordHandler {
	ph := PasswordHandler{
		cost: bcrypt.DefaultCost,
	}
	return &ph
}

// Hash generates the password with the provided cost.
func (ph PasswordHandler) Hash(password []byte) (hashedPassword []byte, err error) {
	return bcrypt.GenerateFromPassword(password, ph.cost)
}

// IsCorrectPassword determines if the hashed password to the password that is not hashed.
// If they match, true is returned.
// Otherwise, false is returned with any unexpected errors.
func (ph PasswordHandler) IsCorrectPassword(hashedPassword, password []byte) (ok bool, err error) {
	err = bcrypt.CompareHashAndPassword(hashedPassword, password)
	switch err {
	case nil:
		return true, nil
	case bcrypt.ErrMismatchedHashAndPassword:
		return false, nil
	}
	return false, err
}
