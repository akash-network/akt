package mcp

import "testing"

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
