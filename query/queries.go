package query

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"time"

	"github.com/mcstatus-io/mcutil/v4/response"
	"github.com/mcstatus-io/mcutil/v4/status"
	"github.com/woozymasta/a2s/pkg/a2s"
)

type Server struct {
	Name          string         `json:"name"`
	Icon          string         `json:"icon"`
	Map           string         `json:"map"`
	Game          string         `json:"app_id"`
	Players       []ServerPlayer `json:"players"`
	Protocol      int64          `json:"protocol"`
	OnlinePlayers int64          `json:"online_players"`
	MaxPlayers    int64          `json:"max_players"`
}

type ServerPlayer struct {
	Name  string `json:"name"`
	Index string `json:"index"`
}

func serverFromMinecraft(resp response.StatusModern) Server {
	s := Server{
		Name:     resp.Version.Name.Clean,
		Map:      "Minecraft",
		Game:     "Minecraft",
		Protocol: resp.Version.Protocol,
	}
	if resp.Favicon != nil {
		s.Icon = *resp.Favicon
	}
	if resp.Players.Online != nil {
		s.OnlinePlayers = *resp.Players.Online
	}
	if resp.Players.Max != nil {
		s.MaxPlayers = *resp.Players.Max
	}
	s.Players = make([]ServerPlayer, 0, len(resp.Players.Sample))
	for _, p := range resp.Players.Sample {
		s.Players = append(s.Players, ServerPlayer{Name: p.Name.Clean, Index: p.ID})
	}
	return s
}

func serverFromMinecraftOld(resp response.StatusLegacy) Server {
	return Server{
		Name:          resp.Version.Name.Clean,
		Map:           "Minecraft",
		Game:          "MinecraftOld",
		Protocol:      resp.Version.Protocol,
		OnlinePlayers: resp.Players.Online,
		MaxPlayers:    resp.Players.Max,
		Players:       []ServerPlayer{},
	}
}

func serverFromSource(info a2s.Info, players []a2s.Player) Server {
	s := Server{
		Name:          info.Name,
		Map:           info.Map,
		Game:          strconv.FormatUint(uint64(info.AppID), 10),
		Protocol:      int64(info.Protocol),
		OnlinePlayers: int64(info.Players),
		MaxPlayers:    int64(info.MaxPlayers),
		Players:       make([]ServerPlayer, 0, len(players)),
	}
	for _, p := range players {
		s.Players = append(s.Players, ServerPlayer{
			Name:  p.Name,
			Index: strconv.FormatUint(uint64(p.Index), 10),
		})
	}
	return s
}

// splitHostPort разбирает "host:port". Если порт не указан — подставляет
// DefaultMinecraftPort (25565), как это делает клиент Minecraft.
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

// isDialError — true, если TCP-соединение не установилось вообще
// (DNS, refused, dial timeout). false — коннект прошёл и удалённая
// сторона что-то ответила (EOF, RST и т.п.).
func isDialError(err error) bool {
	var opErr *net.OpError
	return errors.As(err, &opErr) && opErr.Op == "dial"
}

func queryMinecraft(ctx context.Context, host string, port uint16) (Server, error) {
	resp, err := status.Modern(ctx, host, port)
	if err == nil {
		return serverFromMinecraft(*resp), nil
	}
	if ctx.Err() != nil {
		return Server{}, fmt.Errorf("modern: %w", err)
	}

	respLegacy, errLegacy := status.Legacy(ctx, host, port)
	if errLegacy != nil {
		return Server{}, errors.Join(
			fmt.Errorf("modern: %w", err),
			fmt.Errorf("legacy: %w", errLegacy),
		)
	}
	return serverFromMinecraftOld(*respLegacy), nil
}

func querySource(ctx context.Context, host string, port uint16) (Server, error) {
	ip, err := resolveIPv4Pref(ctx, host)
	if err != nil {
		return Server{}, err
	}
	client, err := a2s.New(ip, int(port))
	if err != nil {
		return Server{}, err
	}
	defer client.Close()
	defer context.AfterFunc(ctx, func() { client.Close() })()

	info, err := client.GetInfo(ctx)
	if err != nil {
		return Server{}, fmt.Errorf("%w: %w", ErrProtocolFailed, err)
	}
	players, err := client.GetPlayers(ctx)
	if err != nil {
		return Server{}, fmt.Errorf("%w: %w", ErrProtocolFailed, err)
	}
	return serverFromSource(*info, players), nil
}

// QueryServer опрашивает Minecraft и Source параллельно, возвращает
// первый успешный ответ. Если Minecraft установил TCP и получил ответ
// (не dial-ошибка) — Source не дожидается: порт точно занят.
func QueryServer(addrStr string, timeout time.Duration) (Server, error) {
	host, port, err := splitHostPort(addrStr)
	if err != nil {
		return Server{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	type result struct {
		s   Server
		err error
		mc  bool
	}
	ch := make(chan result, 2)
	go func() { s, err := queryMinecraft(ctx, host, port); ch <- result{s, err, true} }()
	go func() { s, err := querySource(ctx, host, port); ch <- result{s, err, false} }()

	errs := make([]error, 0, 2)
	for range 2 {
		r := <-ch
		if r.err == nil {
			cancel()
			return r.s, nil
		}
		if r.mc && !isDialError(r.err) {
			cancel()
			return Server{}, r.err
		}
		errs = append(errs, r.err)
	}

	joined := errors.Join(errs...)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		joined = fmt.Errorf("%w: %w", ErrTimeout, joined)
	}
	return Server{}, joined
}
