package aiprovider

import (
	"regexp"
)

var redactPatterns = []*regexp.Regexp{
	regexp.MustCompile(`abp_[a-z]_[A-Za-z0-9._-]{8,}`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{8,}`),
	regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9._\-+=/]+`),
	regexp.MustCompile(`(?i)((?:api[_-]?key|token|secret|password)\s*[=:]\s*)\S+`),
}

// Redact masks tokens, bearer headers and key=value secrets before text leaves the host.
func Redact(s string) string {
	out := s
	out = redactPatterns[0].ReplaceAllString(out, "abp_*_REDACTED")
	out = redactPatterns[1].ReplaceAllString(out, "sk-REDACTED")
	out = redactPatterns[2].ReplaceAllString(out, "Bearer REDACTED")
	out = redactPatterns[3].ReplaceAllString(out, "${1}REDACTED")
	return out
}
