// Package channels khai báo hợp đồng chung cho mọi loại kênh chạy trong metaos-bridge.
package channels

import (
	"encoding/json"

	"github.com/ali33/MetaOS/internal/protocol"
)

type Sender interface {
	SendText(m protocol.Message) error
	SendBinary(ch string, p []byte) error
}

type OpenArgs struct {
	ID     string
	Params json.RawMessage
	Out    Sender
	Done   func()
}

type Channel interface {
	HandleText(m protocol.Message)
	HandleBinary(p []byte)
	Reattachable() bool
	Detach()
	Reattach(params json.RawMessage)
	Close(reason string)
}

type Factory func(a OpenArgs) (Channel, error)

type Error struct {
	Code string
	Err  error
}

func (e *Error) Error() string { return e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }
