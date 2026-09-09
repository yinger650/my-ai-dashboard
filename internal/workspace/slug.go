package workspace

import (
	"crypto/rand"
	"fmt"
	"io"
	"regexp"
)

const slugLen = 8

var slugRe = regexp.MustCompile(`^[a-z0-9]{8}$`)

const slugAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

// ValidSlug reports whether s is an 8-char workspace routing key.
func ValidSlug(s string) bool {
	return slugRe.MatchString(s)
}

func randomSlug() (string, error) {
	buf := make([]byte, slugLen)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return "", fmt.Errorf("slug rand: %w", err)
	}
	out := make([]byte, slugLen)
	for i, b := range buf {
		out[i] = slugAlphabet[int(b)%len(slugAlphabet)]
	}
	return string(out), nil
}
