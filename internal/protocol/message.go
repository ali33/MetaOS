package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
)

var ErrBadMessage = errors.New("protocol: bad message")

const (
	TypeOpen     = "open"
	TypeReady    = "ready"
	TypeClose    = "close"
	TypeError    = "error"
	TypePing     = "ping"
	TypePong     = "pong"
	TypeHello    = "hello"    // bridge → ws, ngay khi khởi động
	TypeDetached = "detached" // ws → bridge, WebSocket vừa rớt
	TypeAttached = "attached" // ws → bridge, WebSocket mới đã nối
	TypeChannels = "channels" // bridge → client, trả lời attached: các kênh tiếp quản được (D17)
)

const (
	CodeAccessDenied  = "access-denied"
	CodeNotFound      = "not-found"
	CodeInvalidParams = "invalid-params"
	CodeUnsupported   = "unsupported"
	CodeInternal      = "internal"
	CodeSudoRequired  = "sudo-required"
	CodeSudoFailed    = "sudo-failed"
)

type Message struct {
	Ch   string          `json:"ch"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

type OpenData struct {
	Ch     string          `json:"ch"`
	Kind   string          `json:"kind"`
	Params json.RawMessage `json:"params,omitempty"`
}

type ReadyData struct {
	Ch string `json:"ch"`
}

type CloseData struct {
	Ch       string `json:"ch"`
	Reason   string `json:"reason"`
	ExitCode *int   `json:"exitCode,omitempty"`
}

type ErrorData struct {
	Ch      string `json:"ch"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ChannelInfo struct {
	Ch   string `json:"ch"`
	Kind string `json:"kind"`
}

type HelloData struct {
	User     string `json:"user"`
	UID      int    `json:"uid"`
	GID      int    `json:"gid"`
	Home     string `json:"home"`
	Hostname string `json:"hostname"`
	Version  string `json:"version"`
}

func DecodeMessage(b []byte) (Message, error) {
	if len(b) > MaxFrame {
		return Message{}, ErrFrameTooLarge
	}
	var m Message
	if err := json.Unmarshal(b, &m); err != nil {
		return Message{}, fmt.Errorf("%w: %v", ErrBadMessage, err)
	}
	if m.Type == "" {
		return Message{}, fmt.Errorf("%w: missing type", ErrBadMessage)
	}
	if m.Ch != "" {
		if err := ValidateChannelID(m.Ch); err != nil {
			return Message{}, fmt.Errorf("%w: %v", ErrBadMessage, err)
		}
	}
	return m, nil
}

func (m Message) Encode() ([]byte, error) { return json.Marshal(m) }

// Control dựng thông điệp điều khiển (ch rỗng). data phải marshal được; lỗi
// marshal ở đây là lỗi lập trình nên panic.
func Control(typ string, data any) Message {
	raw, err := json.Marshal(data)
	if err != nil {
		panic(fmt.Sprintf("protocol.Control(%s): %v", typ, err))
	}
	return Message{Type: typ, Data: raw}
}

func CodeFromErr(err error) string {
	switch {
	case errors.Is(err, fs.ErrPermission):
		return CodeAccessDenied
	case errors.Is(err, fs.ErrNotExist):
		return CodeNotFound
	default:
		return CodeInternal
	}
}
