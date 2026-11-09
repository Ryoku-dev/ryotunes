package ltproto

import (
	"crypto/rand"
	"fmt"
	"io"
	"strings"
)

const (
	// RoomCodeLength is the fixed room-code length.
	RoomCodeLength   = 8
	roomCodeAlphabet = "1234567890QWERTYUPASDFGHJKLZXCVBNM"
)

// GenerateRoomCode returns a cryptographically random, visually unambiguous room code.
func GenerateRoomCode() (string, error) {
	return GenerateRoomCodeFrom(rand.Reader)
}

// GenerateRoomCodeFrom returns a room code using random bytes from source.
func GenerateRoomCodeFrom(source io.Reader) (string, error) {
	result := make([]byte, RoomCodeLength)
	limit := byte(256 - 256%len(roomCodeAlphabet))
	buffer := make([]byte, 1)
	for index := range result {
		for {
			if _, err := io.ReadFull(source, buffer); err != nil {
				return "", fmt.Errorf("generate room code: %w", err)
			}
			if buffer[0] < limit {
				result[index] = roomCodeAlphabet[int(buffer[0])%len(roomCodeAlphabet)]
				break
			}
		}
	}
	return string(result), nil
}

// NormalizeRoomCode trims and uppercases a user-entered room code.
func NormalizeRoomCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// ValidRoomCode reports whether code has the exact relay room-code shape.
func ValidRoomCode(code string) bool {
	if len(code) != RoomCodeLength {
		return false
	}
	for _, character := range code {
		if !strings.ContainsRune(roomCodeAlphabet, character) {
			return false
		}
	}
	return true
}
