package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonMemory      uint32 = 64 * 1024
	argonIterations  uint32 = 3
	argonParallelism uint8  = 4
	argonKeyLen      uint32 = 32
	saltLen                 = 16
	tokenLen                = 32
)

var (
	ErrInvalidHash      = errors.New("invalid password hash")
	ErrPasswordMismatch = errors.New("password mismatch")
)

// HashPassword returns a PHC-style Argon2id hash suitable for storing in SQLite.
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	key := argon2.IDKey([]byte(password), salt, argonIterations, argonMemory, argonParallelism, argonKeyLen)
	enc := base64.RawStdEncoding

	return fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory,
		argonIterations,
		argonParallelism,
		enc.EncodeToString(salt),
		enc.EncodeToString(key),
	), nil
}

func VerifyPassword(password, encoded string) error {
	params, salt, key, err := parsePHC(encoded)
	if err != nil {
		return err
	}

	derived := argon2.IDKey([]byte(password), salt, params.iterations, params.memory, params.parallelism, uint32(len(key)))
	if subtle.ConstantTimeCompare(derived, key) != 1 {
		return ErrPasswordMismatch
	}

	return nil
}

func RandomToken() (string, error) {
	raw := make([]byte, tokenLen)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func RandomID(bytes int) (string, error) {
	raw := make([]byte, bytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

type phcParams struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
}

func parsePHC(encoded string) (phcParams, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return phcParams{}, nil, nil, ErrInvalidHash
	}

	params := phcParams{}
	for _, part := range strings.Split(parts[3], ",") {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			return phcParams{}, nil, nil, ErrInvalidHash
		}
		n, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return phcParams{}, nil, nil, ErrInvalidHash
		}

		switch key {
		case "m":
			params.memory = uint32(n)
		case "t":
			params.iterations = uint32(n)
		case "p":
			if n > 255 {
				return phcParams{}, nil, nil, ErrInvalidHash
			}
			params.parallelism = uint8(n)
		default:
			return phcParams{}, nil, nil, ErrInvalidHash
		}
	}

	if params.memory == 0 || params.iterations == 0 || params.parallelism == 0 {
		return phcParams{}, nil, nil, ErrInvalidHash
	}

	enc := base64.RawStdEncoding
	salt, err := enc.DecodeString(parts[4])
	if err != nil {
		return phcParams{}, nil, nil, ErrInvalidHash
	}
	key, err := enc.DecodeString(parts[5])
	if err != nil {
		return phcParams{}, nil, nil, ErrInvalidHash
	}

	return params, salt, key, nil
}
