package signature

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

const Header = "HashSHA256"

// Calculate returns the hex-encoded HMAC-SHA256 signature of value.
func Calculate(value []byte, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write(value)
	return hex.EncodeToString(mac.Sum(nil))
}

// Valid compares a received signature with the expected value in constant time.
func Valid(value []byte, key, received string) bool {
	expected, err := hex.DecodeString(Calculate(value, key))
	if err != nil {
		return false
	}
	actual, err := hex.DecodeString(received)
	if err != nil {
		return false
	}
	return hmac.Equal(actual, expected)
}
