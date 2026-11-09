package ltproto

import (
	"bytes"
	"errors"
	"testing"
)

func TestGenerateRoomCodeFrom(t *testing.T) {
	code, err := GenerateRoomCodeFrom(bytes.NewReader(make([]byte, RoomCodeLength)))
	if err != nil {
		t.Fatal(err)
	}
	if code != "11111111" {
		t.Fatalf("code = %q, want %q", code, "11111111")
	}
	if !ValidRoomCode(code) {
		t.Fatalf("generated invalid room code %q", code)
	}
}

func TestGenerateRoomCodeFromReportsEntropyFailure(t *testing.T) {
	_, err := GenerateRoomCodeFrom(errorReader{})
	if err == nil {
		t.Fatal("expected entropy error")
	}
}

func TestRoomCodeValidation(t *testing.T) {
	valid := []string{"12345678", "QWERTYUP", "ASDFGHJK", "ZXCVBNM0"}
	for _, code := range valid {
		if !ValidRoomCode(code) {
			t.Errorf("ValidRoomCode(%q) = false", code)
		}
	}
	invalid := []string{"", "1234567", "123456789", "IIIIIIII", "OOOOOOOO", "abcd1234", "1234-678"}
	for _, code := range invalid {
		if ValidRoomCode(code) {
			t.Errorf("ValidRoomCode(%q) = true", code)
		}
	}
	if normalized := NormalizeRoomCode("  asdfghjk  "); normalized != "ASDFGHJK" {
		t.Fatalf("NormalizeRoomCode = %q", normalized)
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, errors.New("entropy unavailable")
}
