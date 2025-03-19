package metadata

import (
	"context"
	"fmt"
	"strings"
)

type authKey struct{}

type MD map[string][]string

// New creates an MD from a given key-value map.
// Uppercase letters are automatically converted to lowercase.
//
// Keys beginning with "zrpc-" are reserved for grpc-internal use only and may
// result in errors if set in metadata.
func New(m map[string]string) MD {
	md := make(MD, len(m))
	for k, v := range m {
		key := strings.ToLower(k)
		md[key] = append(md[key], v)
	}
	return md
}

// Pairs returns an MD formed by the mapping of key, value ...
// Pairs panics if len(kv) is odd.
// Uppercase letters are automatically converted to lowercase.
//
// Keys beginning with "zrpc-" are reserved for grpc-internal use only and may
// result in errors if set in metadata.
func Pairs(kv ...string) MD {
	if len(kv)%2 != 0 {
		panic(fmt.Sprintf("metadata: Pairs got the odd number of input pairs for metadata: %d", len(kv)))
	}

	md := make(MD, len(kv))
	for i := 0; i < len(kv); i += 2 {
		key := strings.ToLower(kv[i])
		md[key] = append(md[key], kv[i+1])
	}

	return md
}

// Len returns the number of items in md.
func (md MD) Len() int {
	return len(md)
}

// Copy returns a copy of md.
func (md MD) Copy() MD {
	out := make(MD, len(md))
	for k, v := range md {
		out[k] = copyOf(v)
	}
	return out
}

// Get obtains the values for a given key.
//
// k is converted to lowercase before searching in md.
func (md MD) Get(k string) []string {
	k = strings.ToLower(k)
	return md[k]
}

// Set sets the value of a given key with a slice of values.
//
// k is converted to lowercase before storing in md.
func (md MD) Set(k string, vals ...string) {
	if len(vals) == 0 {
		return
	}
	k = strings.ToLower(k)
	md[k] = vals
}

// Append adds the values to key k, not overwriting what was already stored at
// that key.
//
// k is converted to lowercase before storing in md.
func (md MD) Append(k string, vals ...string) {
	if len(vals) == 0 {
		return
	}
	k = strings.ToLower(k)
	md[k] = append(md[k], vals...)
}

// Delete removes the values for a given key k which is converted to lowercase
// before removing it from md.
func (md MD) Delete(k string) {
	k = strings.ToLower(k)
	delete(md, k)
}

// Join joins any number of mds into a single MD.
//
// The order of values for each key is determined by the order in which the mds
// containing those values are presented to Join.
func Join(mds ...MD) MD {
	out := MD{}
	for _, md := range mds {
		for k, v := range md {
			out[k] = append(out[k], v...)
		}
	}
	return out
}

// mdIncomingKey represents the Key used by the client to store the MD in Context
type mdIncomingKey struct{}

// mdOutgoingKey represents the Key used by the server to store the MD in Context
type mdOutgoingKey struct{}

// NewOutgoingContext is called by the user
// It creates a new context with outgoing md attached. If used
// in conjunction with AppendToOutgoingContext, NewOutgoingContext will
// overwrite any previously-appended metadata. md must not be modified after
// calling this function.
func NewOutgoingContext(md MD) context.Context {
	return context.WithValue(context.Background(), mdOutgoingKey{}, rawMD{md: md})
}

func AppendToOutgoingContext(ctx context.Context, kv ...string) context.Context {
	if len(kv)%2 == 1 {
		panic(fmt.Sprintf("metadata: AppendToOutgoingContext got an odd number of input pairs for metadata: %d", len(kv)))
	}
	md, _ := ctx.Value(mdOutgoingKey{}).(rawMD)
	added := make([][]string, len(md.added)+1)
	copy(added, md.added)
	copy(added, md.added)
	kvCopy := make([]string, 0, len(kv))
	for i := 0; i < len(kv); i += 2 {
		kvCopy = append(kvCopy, strings.ToLower(kv[i]), kv[i+1])
	}
	added[len(added)-1] = kvCopy
	return context.WithValue(ctx, mdOutgoingKey{}, rawMD{md: md.md, added: added})
}

// FromOutgoingContext is called by the user
// It returns the outgoing metadata in ctx if it exists.
func FromOutgoingContext(ctx context.Context) (MD, bool) {
	raw, ok := ctx.Value(mdOutgoingKey{}).(rawMD)
	if !ok {
		return nil, false
	}

	mdSize := len(raw.md)
	for i := range raw.added {
		mdSize += len(raw.added[i]) / 2
	}

	out := make(MD, mdSize)
	for k, v := range raw.md {
		// We need to manually convert all keys to lower case, because MD is a
		// map, and there's no guarantee that the MD attached to the context is
		// created using our helper functions.
		key := strings.ToLower(k)
		out[key] = copyOf(v)
	}

	for _, added := range raw.added {
		if len(added)%2 == 1 {
			panic(fmt.Sprintf("metadata: FromOutgoingContext got an odd number of input pairs for metadata: %d", len(added)))
		}

		for i := 0; i < len(added); i += 2 {
			key := strings.ToLower(added[i])
			out[key] = append(out[key], added[i+1])
		}
	}
	return out, ok
}

// NewInComingContext is called by the user
// It injects the MD passed from the client into the Context
// md must not be modified after calling this function.
func NewInComingContext(md MD) context.Context {
	return context.WithValue(context.Background(), mdIncomingKey{}, md)
}

// FromInComingContext is called by the zrpc automatically. User do not call!
// It parses the client MD from the context
func FromInComingContext(ctx context.Context) (MD, bool) {
	md, ok := ctx.Value(mdOutgoingKey{}).(MD)
	if !ok {
		return nil, false
	}
	out := make(MD, len(md))
	for k, v := range md {
		// We need to manually convert all keys to lower case, because MD is a
		// map, and there's no guarantee that the MD attached to the context is
		// created using our helper functions.
		key := strings.ToLower(k)
		out[key] = copyOf(v)
	}
	return out, true
}

func copyOf(v []string) []string {
	vals := make([]string, len(v))
	copy(vals, v)
	return vals
}

type rawMD struct {
	md    MD
	added [][]string
}
