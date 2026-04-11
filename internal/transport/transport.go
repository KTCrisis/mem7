package transport

import (
	"context"
	"encoding/json"
)

// Transport is the abstraction through which the MCP tool layer
// invokes business methods. Implementations route the call locally
// (in-process) or to a remote mem7 backend over HTTP ; the caller
// never has to know which.
//
// Call returns the raw JSON "result" field of a JSON-RPC response.
// Protocol-level errors (method not found, invalid params, network
// failure) are returned as the error ; tool-level errors are carried
// inside the result envelope with isError=true.
type Transport interface {
	Call(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error)
}
