package auth

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	totpPeriod  = 30
	totpDigits  = 6
	totpWindow  = 1
	recoveryN   = 10
	recoveryLen = 8
)

// GenerateTOTPSecret returns a new base32-encoded TOTP secret (no padding).
func GenerateTOTPSecret() (string, error) {
	b, err := randBytes(20)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(base32.StdEncoding.EncodeToString(b), "="), nil
}

// OTPAuthURL builds an otpauth:// URI for authenticator apps.
func OTPAuthURL(issuer, account, secret string) string {
	v := url.Values{}
	v.Set("secret", secret)
	v.Set("issuer", issuer)
	v.Set("algorithm", "SHA1")
	v.Set("digits", fmt.Sprintf("%d", totpDigits))
	v.Set("period", fmt.Sprintf("%d", totpPeriod))
	label := url.PathEscape(issuer + ":" + account)
	return "otpauth://totp/" + label + "?" + v.Encode()
}

// VerifyTOTP reports whether code is valid for secret at time t (±1 period).
func VerifyTOTP(secret, code string, t time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != totpDigits {
		return false
	}
	key, err := decodeBase32(secret)
	if err != nil {
		return false
	}
	step := t.Unix() / totpPeriod
	for d := -totpWindow; d <= totpWindow; d++ {
		if ConstantTimeEqual(hotp(key, uint64(step+int64(d))), code) {
			return true
		}
	}
	return false
}

func decodeBase32(s string) ([]byte, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if pad := len(s) % 8; pad != 0 {
		s += strings.Repeat("=", 8-pad)
	}
	return base32.StdEncoding.DecodeString(s)
}

func hotp(key []byte, counter uint64) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(buf[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	bin := (int(sum[off])&0x7f)<<24 | int(sum[off+1])<<16 | int(sum[off+2])<<8 | int(sum[off+3])
	mod := 1
	for i := 0; i < totpDigits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", totpDigits, bin%mod)
}

// GenerateRecoveryCodes returns plaintext codes and their SHA-256 hashes.
func GenerateRecoveryCodes() (plain []string, hashes []string, err error) {
	plain = make([]string, recoveryN)
	hashes = make([]string, recoveryN)
	for i := 0; i < recoveryN; i++ {
		b, err := randBytes(recoveryLen)
		if err != nil {
			return nil, nil, err
		}
		p := strings.ToUpper(fmt.Sprintf("%x-%x", b[:4], b[4:]))
		plain[i] = p
		hashes[i] = HashToken(normalizeRecovery(p))
	}
	return plain, hashes, nil
}

func normalizeRecovery(s string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
}

// MatchRecoveryHash returns the index of a matching recovery code hash, or -1.
func MatchRecoveryHash(code string, hashes []string) int {
	want := HashToken(normalizeRecovery(code))
	for i, h := range hashes {
		if h != "" && ConstantTimeEqual(h, want) {
			return i
		}
	}
	return -1
}

// LooksLikeTOTP reports whether s is a 6-digit code.
func LooksLikeTOTP(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) != totpDigits {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// CurrentTOTP returns the TOTP code for secret at time t (no window).
func CurrentTOTP(secret string, t time.Time) string {
	key, err := decodeBase32(secret)
	if err != nil {
		return ""
	}
	return hotp(key, uint64(t.Unix()/totpPeriod))
}
