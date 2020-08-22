package neterror

import (
	"io"
	"net"
	"strings"
)

// NetCode network error code
type NetCode int

const (
	// NetSuccess network operation success
	NetSuccess NetCode = iota
	// NetConTimeout connection timeout
	NetConTimeout
	// NetConRefused connection refused
	NetConRefused
	// NetReset connection reset
	NetReset
	// NetAddrLookup Address lookup
	NetAddrLookup
	// NetConEOF Connection closed
	NetConEOF
	// OtherError other error
	OtherError
)

// String get NetCode string representation
func (c NetCode) String() string {
	return [...]string{"success", "connection timeout", "connection refused", "connection reset",
		"address lookup error", "connection eof", "other"}[c]
}

// GetNetCode try to resolve NetError, if not possible return OtherError
func GetNetCode(err error) NetCode {
	if err == nil {
		return NetSuccess
	} else if err == io.EOF {
		return NetConEOF
	}
	netErr, ok := err.(net.Error)
	if ok {
		if netErr.Timeout() {
			return NetConTimeout
		} else if strings.Contains(err.Error(), " lookup ") {
			return NetAddrLookup
		} else if strings.HasSuffix(err.Error(), ": connection refused") {
			return NetConRefused
		} else if strings.HasSuffix(err.Error(), ": broken pipe") ||
			strings.HasSuffix(err.Error(), "EOF") {
			return NetConEOF
		} else if strings.HasSuffix(err.Error(), ": connection reset by peer") {
			return NetReset
		}
	}
	return OtherError
}

// NetError network error
type NetError struct {
	code NetCode
}

// NewNetError try to resolve NetError, if not possible return original error
func NewNetError(err error) error {
	if err == nil {
		return nil
	}
	netCode := GetNetCode(err)
	if netCode == OtherError {
		return err
	}
	return &NetError{code: netCode}

}

// Error get error description
func (n *NetError) Error() string {
	return n.code.String()
}
