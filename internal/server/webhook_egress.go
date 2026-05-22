package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"

	"godrive/internal/config"
)

type webhookEgressPolicy struct {
	allowHTTP    bool
	allowPrivate bool
}

func webhookPolicyFromConfig(cfg config.Config) webhookEgressPolicy {
	return webhookEgressPolicy{
		allowHTTP:    cfg.WebhookAllowHTTP,
		allowPrivate: cfg.WebhookAllowPrivate,
	}
}

func validateWebhookURL(raw string, policy webhookEgressPolicy) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("webhook url must be absolute")
	}
	switch parsed.Scheme {
	case "https":
	case "http":
		if !policy.allowHTTP {
			return nil, errors.New("webhook url must use https")
		}
	default:
		return nil, errors.New("webhook url must use http or https")
	}
	if parsed.User != nil {
		return nil, errors.New("webhook url must not contain userinfo")
	}
	if parsed.Fragment != "" {
		return nil, errors.New("webhook url must not contain a fragment")
	}
	host := parsed.Hostname()
	if host == "" {
		return nil, errors.New("webhook url host is required")
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if err := validateWebhookIP(ip, policy); err != nil {
			return nil, err
		}
	}
	return parsed, nil
}

func webhookHTTPClient(policy webhookEgressPolicy) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	dialer := &net.Dialer{Timeout: webhookDeliveryTimeout}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := webhookResolveHost(ctx, host)
		if err != nil {
			return nil, err
		}
		for _, ip := range ips {
			if err := validateWebhookIP(ip, policy); err != nil {
				return nil, err
			}
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("webhook host %q resolved to no addresses", host)
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
	}
	return &http.Client{
		Transport: transport,
		Timeout:   webhookDeliveryTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("webhook redirect limit exceeded")
			}
			if _, err := validateWebhookURL(req.URL.String(), policy); err != nil {
				return err
			}
			return nil
		},
	}
}

func webhookResolveHost(ctx context.Context, host string) ([]netip.Addr, error) {
	if ip, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{ip}, nil
	}
	records, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	out := make([]netip.Addr, 0, len(records))
	for _, record := range records {
		ip, ok := netip.AddrFromSlice(record.IP)
		if !ok {
			continue
		}
		out = append(out, ip.Unmap())
	}
	return out, nil
}

func validateWebhookIP(ip netip.Addr, policy webhookEgressPolicy) error {
	ip = ip.Unmap()
	if !ip.IsValid() {
		return errors.New("webhook destination IP is invalid")
	}
	if policy.allowPrivate {
		return nil
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return fmt.Errorf("webhook destination %s is not allowed", ip)
	}
	if ip.Is4() {
		if blocked := blockedIPv4WebhookRange(ip); blocked {
			return fmt.Errorf("webhook destination %s is not allowed", ip)
		}
	}
	return nil
}

func blockedIPv4WebhookRange(ip netip.Addr) bool {
	ranges := []netip.Prefix{
		netip.MustParsePrefix("0.0.0.0/8"),
		netip.MustParsePrefix("100.64.0.0/10"),
		netip.MustParsePrefix("127.0.0.0/8"),
		netip.MustParsePrefix("169.254.0.0/16"),
		netip.MustParsePrefix("192.0.0.0/24"),
		netip.MustParsePrefix("192.0.2.0/24"),
		netip.MustParsePrefix("198.18.0.0/15"),
		netip.MustParsePrefix("198.51.100.0/24"),
		netip.MustParsePrefix("203.0.113.0/24"),
		netip.MustParsePrefix("224.0.0.0/4"),
		netip.MustParsePrefix("240.0.0.0/4"),
	}
	for _, prefix := range ranges {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}
