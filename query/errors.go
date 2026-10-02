package query

import "errors"

var (
	ErrTimeout        = errors.New("query timeout exceeded")
	ErrProtocolFailed = errors.New("protocol query failed")
)
