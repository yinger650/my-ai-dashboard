package auth

import (
	"testing"
	"time"
)

func TestTOTPRoundTrip(t *testing.T) {
	secret, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	key, err := decodeBase32(secret)
	if err != nil {
		t.Fatal(err)
	}
	code := hotp(key, uint64(now.Unix()/30))
	if !VerifyTOTP(secret, code, now) {
		t.Fatal("expected current code to verify")
	}
	if VerifyTOTP(secret, "000000", now) {
		t.Fatal("wrong code should fail")
	}
	if !LooksLikeTOTP(code) {
		t.Fatal("code should look like totp")
	}
}

func TestOTPAuthURL(t *testing.T) {
	u := OTPAuthURL("AgentBoard", "admin", "MFRGGZDFMZTWQ2LK")
	if u[:15] != "otpauth://totp/" {
		t.Fatalf("url = %s", u)
	}
}

func TestRecoveryCodes(t *testing.T) {
	plain, hashes, err := GenerateRecoveryCodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(plain) != 10 || len(hashes) != 10 {
		t.Fatal("want 10 codes")
	}
	idx := MatchRecoveryHash(plain[3], hashes)
	if idx != 3 {
		t.Fatalf("match = %d", idx)
	}
	if MatchRecoveryHash("nope", hashes) != -1 {
		t.Fatal("bad code matched")
	}
}

func TestEncryptDecrypt(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	ct, err := Encrypt(key, []byte("hello totp secret"))
	if err != nil {
		t.Fatal(err)
	}
	pt, err := Decrypt(key, ct)
	if err != nil {
		t.Fatal(err)
	}
	if string(pt) != "hello totp secret" {
		t.Fatalf("got %q", pt)
	}
}
