package models

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestNormalizeAvatarBase64SizeLimit(t *testing.T) {
	t.Run("accepts exactly ten mebibytes", func(t *testing.T) {
		encoded := base64.StdEncoding.EncodeToString(make([]byte, MaxAvatarBytes))

		if _, err := NormalizeAvatarBase64(encoded); err != nil {
			t.Fatalf("NormalizeAvatarBase64() error = %v", err)
		}
	})

	t.Run("rejects content over ten mebibytes", func(t *testing.T) {
		encoded := base64.StdEncoding.EncodeToString(make([]byte, MaxAvatarBytes+1))

		_, err := NormalizeAvatarBase64("data:image/png;base64," + encoded)
		if !errors.Is(err, ErrAvatarTooLarge) {
			t.Fatalf("NormalizeAvatarBase64() error = %v, want ErrAvatarTooLarge", err)
		}
	})

	t.Run("still rejects invalid base64", func(t *testing.T) {
		_, err := NormalizeAvatarBase64(strings.Repeat("!", 32))
		if !errors.Is(err, ErrInvalidAvatarBase64) {
			t.Fatalf("NormalizeAvatarBase64() error = %v, want ErrInvalidAvatarBase64", err)
		}
	})
}
