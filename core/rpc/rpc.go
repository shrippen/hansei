// Package rpc exposes the service over JSON-RPC 2.0 on a Unix socket, one JSON object per
// line. Clients call methods; after "subscribe" the server also pushes "event" notifications.
//
//	→ {"jsonrpc":"2.0","id":7,"method":"decide","params":{"id":"…","path":"IT/x.md","hunk":"h1a2…","decision":"accepted"}}
//	← {"jsonrpc":"2.0","id":7,"result":{"written":true,…}}
//	← {"jsonrpc":"2.0","method":"event","params":{"kind":"batch","batch":"…"}}
package rpc

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Version is the protocol version the "hello" method reports.
const Version = 1

// Error codes (JSON-RPC 2.0).
const (
	codeParse    = -32700
	codeInvalid  = -32600
	codeMethod   = -32601
	codeParams   = -32602
	codeInternal = -32000 // application error, message is shown to people
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
	Method  string          `json:"method,omitempty"` // notifications
	Params  any             `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return e.Message }

// SocketPath is $HANSEI_SOCKET or $XDG_RUNTIME_DIR/hansei.sock.
func SocketPath() string {
	if p := os.Getenv("HANSEI_SOCKET"); p != "" {
		return p
	}
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "hansei.sock")
}
