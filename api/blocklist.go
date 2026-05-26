package main

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

var privateRanges = func() []net.IPNet {
	cidrs := []string{
		"127.0.0.0/8",    // loopback
		"10.0.0.0/8",     // RFC1918
		"172.16.0.0/12",  // RFC1918
		"192.168.0.0/16", // RFC1918
		"169.254.0.0/16", // link-local / cloud metadata (AWS, GCP)
		"0.0.0.0/8",      // "this" network
		"::1/128",         // IPv6 loopback
		"fc00::/7",        // IPv6 unique local
		"fe80::/10",       // IPv6 link-local
	}
	var ranges []net.IPNet
	for _, cidr := range cidrs {
		_, ipNet, _ := net.ParseCIDR(cidr)
		ranges = append(ranges, *ipNet)
	}
	return ranges
}()

var blockedHosts = map[string]bool{
	"localhost":                true,
	"metadata.google.internal": true, // GCP metadata
}

var errBlockedURL = errors.New("url not allowed")

// isBlockedURL returns errBlockedURL if the URL targets a private, loopback,
// link-local, or known metadata address. Resolves the hostname to catch
// DNS-based redirects to private IPs.
func isBlockedURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return errBlockedURL
	}

	host := strings.ToLower(parsed.Hostname()) // strips port if present
	if host == "" {
		return errBlockedURL
	}

	if blockedHosts[host] {
		return errBlockedURL
	}

	// Literal IP — check directly without DNS.
	if ip := net.ParseIP(host); ip != nil {
		return checkIP(ip)
	}

	// Resolve hostname and check every returned address.
	addrs, err := net.LookupHost(host)
	if err != nil {
		// Unresolvable hosts are rejected — they can't be legitimate targets.
		return errBlockedURL
	}
	for _, addr := range addrs {
		if ip := net.ParseIP(addr); ip != nil {
			if err := checkIP(ip); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkIP(ip net.IP) error {
	for _, r := range privateRanges {
		if r.Contains(ip) {
			return errBlockedURL
		}
	}
	return nil
}
