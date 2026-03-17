package store

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func generateKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return key
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	key := generateKey(t)
	plaintext := []byte("hello, world! this is a secret message.")

	ciphertext, nonce, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if len(ciphertext) == 0 {
		t.Fatal("expected non-empty ciphertext")
	}
	if len(nonce) == 0 {
		t.Fatal("expected non-empty nonce")
	}

	decrypted, err := Decrypt(ciphertext, nonce, key)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("decrypted = %q, want %q", decrypted, plaintext)
	}
}

func TestEncrypt_WrongKeyLength(t *testing.T) {
	shortKey := make([]byte, 16)
	_, _, err := Encrypt([]byte("test"), shortKey)
	if err == nil {
		t.Fatal("expected error for 16-byte key, got nil")
	}
}

func TestDecrypt_WrongKeyLength(t *testing.T) {
	shortKey := make([]byte, 16)
	_, err := Decrypt([]byte("ciphertext"), []byte("nonce-12bytes"), shortKey)
	if err == nil {
		t.Fatal("expected error for 16-byte key, got nil")
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	keyA := generateKey(t)
	keyB := generateKey(t)
	plaintext := []byte("secret data")

	ciphertext, nonce, err := Encrypt(plaintext, keyA)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	_, err = Decrypt(ciphertext, nonce, keyB)
	if err == nil {
		t.Fatal("expected error when decrypting with wrong key, got nil")
	}
}

func TestDecrypt_TamperedCiphertext(t *testing.T) {
	key := generateKey(t)
	plaintext := []byte("important data")

	ciphertext, nonce, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Flip a byte in the middle of the ciphertext.
	if len(ciphertext) > 2 {
		ciphertext[len(ciphertext)/2] ^= 0xFF
	}

	_, err = Decrypt(ciphertext, nonce, key)
	if err == nil {
		t.Fatal("expected error for tampered ciphertext, got nil")
	}
}

func TestDecrypt_TruncatedCiphertext(t *testing.T) {
	key := generateKey(t)
	plaintext := []byte("test data")

	_, nonce, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Use truncated ciphertext (shorter than the GCM tag).
	truncated := []byte{0x01, 0x02}

	_, err = Decrypt(truncated, nonce, key)
	if err == nil {
		t.Fatal("expected error for truncated ciphertext, got nil")
	}
}

func TestEncryptDecrypt_EmptyPlaintext(t *testing.T) {
	key := generateKey(t)
	plaintext := []byte("")

	ciphertext, nonce, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if len(ciphertext) == 0 {
		t.Fatal("expected non-empty ciphertext even for empty plaintext (GCM tag)")
	}

	decrypted, err := Decrypt(ciphertext, nonce, key)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if len(decrypted) != 0 {
		t.Errorf("expected empty plaintext, got %q", decrypted)
	}
}

func TestEncrypt_ProducesDifferentCiphertexts(t *testing.T) {
	key := generateKey(t)
	plaintext := []byte("same input")

	ct1, _, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt 1: %v", err)
	}

	ct2, _, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt 2: %v", err)
	}

	if bytes.Equal(ct1, ct2) {
		t.Error("two encryptions of the same plaintext should produce different ciphertexts (random nonce)")
	}
}
