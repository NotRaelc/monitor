package query

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
)

// splitHostPort "host:port" -> host, port, error.
// Если порт не указан (или 0) — подставляет DefaultMinecraftPort (25565),
func splitHostPort(s string) (string, uint16, error) {
	host, portStr, err := net.SplitHostPort(s)
	if err != nil {
		var addrErr *net.AddrError
		if errors.As(err, &addrErr) && addrErr.Err == "missing port in address" {
			return s, DefaultMinecraftPort, nil
		}
		return "", 0, fmt.Errorf("invalid address %q: %w", s, err)
	}
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port %q: %w", portStr, err)
	}
	if port == 0 {
		port = DefaultMinecraftPort
	}
	return host, uint16(port), nil
}

// resolveIPv4Pref принимает хост (IP или домен) и возвращает строку-IP.
// Если хост уже IP — возвращает как есть. Иначе делает DNS-lookup с учётом
// контекста и предпочитает IPv4: голый IPv6-адрес без маршрута даёт
// тайм-аут вместо быстрой ошибки. Если IPv4 нет — вернёт первый из
// найденных адресов.
func resolveIPv4Pref(ctx context.Context, host string) (string, error) {
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.String(), nil
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return "", fmt.Errorf("dns lookup failed for %q: %w", host, err)
	}
	if len(ips) == 0 {
		return "", fmt.Errorf("no IP addresses for %q", host)
	}
	for _, ip := range ips {
		if ip.Is4() {
			return ip.String(), nil
		}
	}
	return ips[0].String(), nil
}

// isDialError принимает ошибку и говорит, провалилось ли TCP-соединение
// на этапе установки. True — DNS, connection refused, dial timeout;
// порт, возможно, мёртв. False — соединение установилось, но удалённая
// сторона ответила плохо (EOF, RST, wsarecv): порт точно живой.
func isDialError(err error) bool {
	var opErr *net.OpError
	return errors.As(err, &opErr) && opErr.Op == "dial"
}
