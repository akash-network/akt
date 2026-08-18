package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	chainclient "pkg.akt.dev/go/node/client/v1beta3"

	aktconsole "pkg.akt.dev/akt/internal/console"
)

// mustBool reads an annotation pointer, failing when the hint was never set.
//
// "unset" and "false" are not the same thing here: the MCP spec treats an
// absent hint as a default, and those defaults (destructiveHint=true,
// readOnlyHint=false) are the opposite of what a query needs. Leaving them
// nil is exactly the bug these tests exist to catch.
func mustBool(t *testing.T, field string, p *bool) bool {
	t.Helper()

	if p == nil {
		t.Fatalf("%s is unset; the MCP default would apply instead", field)
		return false
	}

	return *p
}

// TestQueryToolAnnotations pins what a read-only tool advertises. Clients
// decide what to auto-approve from these hints, so an unannotated query is
// presented to the user as a destructive operation on every call.
func TestQueryToolAnnotations(t *testing.T) {
	a := queryToolAnnotations()

	if !mustBool(t, "readOnlyHint", a.ReadOnlyHint) {
		t.Error("a query must be readOnlyHint=true")
	}
	if mustBool(t, "destructiveHint", a.DestructiveHint) {
		t.Error("a query must be destructiveHint=false")
	}
	if !mustBool(t, "idempotentHint", a.IdempotentHint) {
		t.Error("a query must be idempotentHint=true")
	}
	// Every tool reaches a chain node, a provider gateway or the Console API.
	if !mustBool(t, "openWorldHint", a.OpenWorldHint) {
		t.Error("a query must be openWorldHint=true")
	}
}

// TestWriteToolAnnotations pins the inverse: these broadcast transactions or
// spend credits, so a client should confirm them rather than auto-approve.
func TestWriteToolAnnotations(t *testing.T) {
	a := writeToolAnnotations()

	if mustBool(t, "readOnlyHint", a.ReadOnlyHint) {
		t.Error("a write must be readOnlyHint=false")
	}
	if !mustBool(t, "destructiveHint", a.DestructiveHint) {
		t.Error("a write must be destructiveHint=true")
	}
	if mustBool(t, "idempotentHint", a.IdempotentHint) {
		t.Error("a write must be idempotentHint=false")
	}
}

// TestQueryAndWriteAnnotationsDiffer guards the distinction itself. Before
// annotations were set at all, every tool looked identical to a client --
// all destructive -- leaving no way to tell a balance check from a
// deployment close.
func TestQueryAndWriteAnnotationsDiffer(t *testing.T) {
	q, w := queryToolAnnotations(), writeToolAnnotations()

	if *q.ReadOnlyHint == *w.ReadOnlyHint {
		t.Error("read and write tools must not advertise the same readOnlyHint")
	}
	if *q.DestructiveHint == *w.DestructiveHint {
		t.Error("read and write tools must not advertise the same destructiveHint")
	}
}

// TestNewWithoutChainOrConsoleFails covers the case where neither rail is
// usable. The chain client constructors accept an empty context and return
// something that fails on every call, so without an explicit endpoint check
// the server would start and advertise 19 chain tools that cannot work --
// and the "nothing available" error could never fire.
func TestNewWithoutChainOrConsoleFails(t *testing.T) {
	_, err := New(context.Background(), sdkclient.Context{}, "jwt", false, nil)
	if err == nil {
		t.Fatal("expected an error when there is no chain endpoint and no Console key")
	}

	for _, want := range []string{"no tools available", "Console API key"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got: %v", want, err)
		}
	}
}

// TestNewWithConsoleOnlySucceeds is the point of the change: a managed setup
// has an API key and no wallet or RPC endpoint, and must still get a server.
func TestNewWithConsoleOnlySucceeds(t *testing.T) {
	s, err := New(context.Background(), sdkclient.Context{}, "jwt", false, aktconsole.New("", "key"))
	if err != nil {
		t.Fatalf("a Console key alone must be enough to start: %v", err)
	}
	if s == nil {
		t.Fatal("expected a server")
	}
}

func TestRegisterChainToolsReturnsLightClientFailure(t *testing.T) {
	sentinel := errors.New("light client failed")
	err := (&Server{}).registerChainToolsWithLightClient(
		context.Background(),
		sdkclient.Context{}.WithNodeURI("https://rpc.example.test"),
		"jwt",
		false,
		func(sdkclient.Context) (chainclient.LightClient, error) {
			return nil, sentinel
		},
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("light client error = %v", err)
	}
}
