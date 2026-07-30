package main

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

// Passwords are stored as PBKDF2-HMAC-SHA256 (stdlib only) in the format
// "pbkdf2-sha256$<iterations>$<salt_b64>$<hash_b64>".
const (
	passwordHashAlgorithm  = "pbkdf2-sha256"
	passwordHashIterations = 210_000
	passwordHashSaltBytes  = 16
	passwordHashKeyBytes   = 32
	passwordMinLength      = 10
)

func hashPassword(password string) (string, error) {
	salt := make([]byte, passwordHashSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, passwordHashIterations, passwordHashKeyBytes)
	if err != nil {
		return "", fmt.Errorf("derive key: %w", err)
	}
	b64 := base64.RawStdEncoding
	return passwordHashAlgorithm + "$" + strconv.Itoa(passwordHashIterations) + "$" + b64.EncodeToString(salt) + "$" + b64.EncodeToString(key), nil
}

func verifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != passwordHashAlgorithm {
		return false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations < 1 {
		return false
	}
	b64 := base64.RawStdEncoding
	salt, err := b64.DecodeString(parts[2])
	if err != nil {
		return false
	}
	expected, err := b64.DecodeString(parts[3])
	if err != nil {
		return false
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, iterations, len(expected))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(key, expected) == 1
}
