package query

import (
	"errors"
	"strconv"
	"time"

	"github.com/mcstatus-io/mcutil/v4/response"
	"github.com/woozymasta/a2s/pkg/a2s"
)

type ServerType int

const (
	ServerAuto            ServerType = iota // 0 — перебор всех (по умолчанию)
	ServerSource                            // 1 — только A2S (Source/GoldSrc/etc.)
	ServerMinecraftModern                   // 2 — только Minecraft 1.7+
	ServerMinecraftLegacy                   // 3 — только Minecraft ≤1.6
)

type Server struct {
	Name     string         `json:"name"`
	Icon     string         `json:"icon"`
	Map      string         `json:"map"`
	Game     string         `json:"app_id"`
	Players  []ServerPlayer `json:"players"`
	Protocol int64          `json:"protocol"`
	// TODO: Добавь Version (по protocol - для майнкрафт
	// TODO: и по Info->version для a2s:source/goldsrc)
	OnlinePlayers int64 `json:"online_players"`
	MaxPlayers    int64 `json:"max_players"`
}
type ServerPlayer struct {
	Name  string `json:"name"`
	Index string `json:"index"`
}

const (
	DefaultTimeout       = 5 * time.Second
	DefaultMinecraftPort = 25565
)

var (
	ErrTimeout        = errors.New("query timeout exceeded")
	ErrProtocolFailed = errors.New("protocol query failed")
)

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
