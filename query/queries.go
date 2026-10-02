package query

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mcstatus-io/mcutil/v4/status"
	"github.com/woozymasta/a2s/pkg/a2s"
)

// queryMinecraftModern — одиночный Modern-запрос, одна ошибка, один коннект.
func queryMinecraftModern(ctx context.Context, host string, port uint16) (Server, error) {
	resp, err := status.Modern(ctx, host, port)
	if err != nil {
		return Server{}, fmt.Errorf("modern: %w", err)
	}
	return serverFromMinecraft(*resp), nil
}

// queryMinecraftLegacy — одиночный Legacy-запрос, одна ошибка, один коннект.
func queryMinecraftLegacy(ctx context.Context, host string, port uint16) (Server, error) {
	resp, err := status.Legacy(ctx, host, port)
	if err != nil {
		return Server{}, fmt.Errorf("legacy: %w", err)
	}
	return serverFromMinecraftOld(*resp), nil
}

// queryMinecraft — auto: Modern и Legacy параллельно, первый успех побеждает.
// Проигравший глушится через cancel. Используется только в ServerAuto.
func queryMinecraft(ctx context.Context, host string, port uint16) (Server, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type mcResult struct {
		s   Server
		err error
	}
	ch := make(chan mcResult, 2)

	go func() {
		s, err := queryMinecraftModern(ctx, host, port)
		ch <- mcResult{s: s, err: err}
	}()
	go func() {
		s, err := queryMinecraftLegacy(ctx, host, port)
		ch <- mcResult{s: s, err: err}
	}()

	errs := make([]error, 0, 2)
	for range 2 {
		r := <-ch
		if r.err == nil {
			cancel()
			return r.s, nil
		}
		errs = append(errs, r.err)
	}
	return Server{}, errors.Join(errs...)
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

	stop := context.AfterFunc(ctx, func() { client.Close() })
	defer stop()

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

// QueryServer — точка входа.
//
// Без третьего аргумента (или с ServerAuto) работает в auto-режиме:
// Minecraft (Modern|Legacy) и Source опрашиваются параллельно, побеждает
// первый успех. Если Minecraft установил TCP и получил ответ — Source
// не дожидается: порт точно занят.
//
// Предпочтительно использовать ServerType, чтобы anti-ddos не банил.
// Я пока хз как разобраться с этой проблемой нормально, но поху...
func QueryServer(addrStr string, timeout time.Duration, st ...ServerType) (Server, error) {
	host, port, err := splitHostPort(addrStr)
	if err != nil {
		return Server{}, err
	}

	kind := ServerAuto
	if len(st) > 0 {
		kind = st[0]
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Явный тип — 1 коннект, никаких гонок и лишних сокетов.
	switch kind {
	case ServerSource:
		return querySource(ctx, host, port)
	case ServerMinecraftModern:
		return queryMinecraftModern(ctx, host, port)
	case ServerMinecraftLegacy:
		return queryMinecraftLegacy(ctx, host, port)
	}

	// ServerAuto (или неизвестное значение) — параллельный перебор.
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
