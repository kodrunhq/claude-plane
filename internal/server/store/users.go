package store

import (
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters per OWASP recommendations.
const (
	argonTime    = 1
	argonMemory  = 64 * 1024 // 64 MB
	argonThreads = 4
	argonKeyLen  = 32
	argonSaltLen = 16
)

// User represents a row in the users table.
type User struct {
	UserID       string
	Email        string
	DisplayName  string
	PasswordHash string
	Role         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// HashPassword hashes a password using Argon2id with OWASP-recommended parameters.
// Returns a string in the format: $argon2id$v=19$m=65536,t=1,p=4$<salt>$<hash>
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

// VerifyPassword checks a plaintext password against an Argon2id hash string.
func VerifyPassword(password, encodedHash string) (bool, error) {
	// Parse the hash string: $argon2id$v=19$m=65536,t=1,p=4$<salt>$<hash>
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return false, fmt.Errorf("invalid hash format: expected 6 parts, got %d", len(parts))
	}

	if parts[1] != "argon2id" {
		return false, fmt.Errorf("unsupported algorithm: %s (expected argon2id)", parts[1])
	}

	var version int
	var memory uint32
	var time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, fmt.Errorf("parse version: %w", err)
	}
	if version != argon2.Version {
		return false, fmt.Errorf("unsupported argon2 version: %d (expected %d)", version, argon2.Version)
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return false, fmt.Errorf("parse params: %w", err)
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("decode salt: %w", err)
	}

	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("decode hash: %w", err)
	}

	// Bound parameters to prevent DoS via crafted hashes
	if memory > 256*1024 || time > 10 || threads > 16 {
		return false, fmt.Errorf("argon2 params exceed safe bounds: m=%d t=%d p=%d", memory, time, threads)
	}
	if len(salt) != argonSaltLen {
		return false, fmt.Errorf("invalid salt length: got %d, expected %d", len(salt), argonSaltLen)
	}
	if len(expectedHash) != argonKeyLen {
		return false, fmt.Errorf("invalid hash length: got %d, expected %d", len(expectedHash), argonKeyLen)
	}

	// Recompute hash with the same parameters
	computedHash := argon2.IDKey([]byte(password), salt, time, memory, threads, argonKeyLen)

	// Constant-time comparison
	if subtle.ConstantTimeCompare(computedHash, expectedHash) == 1 {
		return true, nil
	}
	return false, nil
}

// generateUUID generates a random UUID v4 string.
func generateUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate uuid: %w", err)
	}
	// Set version (4) and variant (RFC 4122)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

// SeedAdmin creates an admin user with the given email, password, and display name.
// The password is hashed with Argon2id. If a user with the same email already exists,
// a descriptive error is returned (no panic).
func (s *Store) SeedAdmin(email, password, displayName string) error {
	userID, err := generateUUID()
	if err != nil {
		return fmt.Errorf("generate user id: %w", err)
	}

	hash, err := HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	_, err = s.writer.Exec(`
		INSERT INTO users (user_id, email, display_name, password_hash, role)
		VALUES (?, ?, ?, ?, 'admin')
	`, userID, email, displayName, hash)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed: users.email") {
			return fmt.Errorf("admin account with email %q already exists", email)
		}
		return fmt.Errorf("insert admin user: %w", err)
	}

	return nil
}

// CreateUser inserts a new user into the database.
// Returns an error if the email already exists (UNIQUE constraint violation).
func (s *Store) CreateUser(user *User) error {
	_, err := s.writer.Exec(`
		INSERT INTO users (user_id, email, display_name, password_hash, role)
		VALUES (?, ?, ?, ?, ?)
	`, user.UserID, user.Email, user.DisplayName, user.PasswordHash, user.Role)
	if err != nil {
		return fmt.Errorf("create user %q: %w", user.Email, err)
	}
	return nil
}

// IsUniqueViolation returns true if the error is a SQLite UNIQUE constraint violation.
func IsUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// GetUserByEmail retrieves a user by email address from the reader pool.
// Returns nil if the user is not found.
func (s *Store) GetUserByEmail(email string) (*User, error) {
	var u User
	err := s.reader.QueryRow(`
		SELECT user_id, email, display_name, password_hash, role, created_at, updated_at
		FROM users WHERE email = ?
	`, email).Scan(&u.UserID, &u.Email, &u.DisplayName, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query user by email: %w", err)
	}
	return &u, nil
}
