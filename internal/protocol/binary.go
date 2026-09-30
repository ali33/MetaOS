package protocol

import "errors"

var (
	ErrShortFrame   = errors.New("protocol: short binary frame")
	ErrBadChannelID = errors.New("protocol: bad channel id")
)

func ValidateChannelID(id string) error {
	if len(id) < 1 || len(id) > 64 {
		return ErrBadChannelID
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return ErrBadChannelID
		}
	}
	return nil
}

func EncodeBinary(ch string, p []byte) ([]byte, error) {
	if err := ValidateChannelID(ch); err != nil {
		return nil, err
	}
	b := make([]byte, 1+len(ch)+len(p))
	b[0] = byte(len(ch))
	copy(b[1:], ch)
	copy(b[1+len(ch):], p)
	return b, nil
}

func DecodeBinary(b []byte) (string, []byte, error) {
	if len(b) < 1 {
		return "", nil, ErrShortFrame
	}
	n := int(b[0])
	if n == 0 || len(b) < 1+n {
		return "", nil, ErrShortFrame
	}
	ch := string(b[1 : 1+n])
	if err := ValidateChannelID(ch); err != nil {
		return "", nil, err
	}
	return ch, b[1+n:], nil
}
