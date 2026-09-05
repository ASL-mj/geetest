package crypto

import (
	"fmt"
	"net"
	"strings"
)

// IPAllowed reports whether clientIP satisfies the allowlist. An empty
// allowlist allows everything. Entries are exact IPs or CIDR blocks
// separated by commas or newlines; invalid entries are skipped so a typo
// never locks every caller out.
func IPAllowed(clientIP, allowlist string) bool {
	allowlist = strings.TrimSpace(allowlist)
	if allowlist == "" {
		return true
	}
	ip := net.ParseIP(clientIP)
	if ip == nil {
		return false
	}
	for _, entry := range strings.FieldsFunc(allowlist, func(r rune) bool { return r == ',' || r == '\n' || r == ' ' }) {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if !strings.Contains(entry, "/") {
			if exact := net.ParseIP(entry); exact != nil && exact.Equal(ip) {
				return true
			}
			continue
		}
		_, network, err := net.ParseCIDR(entry)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// NormalizeIPAllowlist trims entries, drops empties and validates every
// entry; the first invalid entry aborts with its position so the caller can
// render a precise 422.
func NormalizeIPAllowlist(raw string) (string, error) {
	entries := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '\n' || r == ';' })
	cleaned := make([]string, 0, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if _, _, err := net.ParseCIDR(entry); err == nil {
			cleaned = append(cleaned, entry)
			continue
		}
		if net.ParseIP(entry) != nil {
			cleaned = append(cleaned, entry)
			continue
		}
		return "", fmt.Errorf("无效的 IP 或 CIDR：%s", entry)
	}
	return strings.Join(cleaned, ","), nil
}
