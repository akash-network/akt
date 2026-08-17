package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"k8s.io/client-go/tools/remotecommand"

	mtypes "pkg.akt.dev/go/node/market/v1"
	rest "pkg.akt.dev/go/provider/client"
)

// LeaseStatusClient is the narrow gateway capability needed to verify that a
// lease exists before opening a websocket stream.
type LeaseStatusClient interface {
	LeaseStatus(context.Context, mtypes.LeaseID) (rest.LeaseStatus, error)
}

// LeaseShellClient is the gateway surface needed for a preflighted remote
// shell operation.
type LeaseShellClient interface {
	LeaseStatusClient
	LeaseShell(
		context.Context,
		mtypes.LeaseID,
		string,
		uint,
		[]string,
		io.Reader,
		io.Writer,
		io.Writer,
		bool,
		<-chan remotecommand.TerminalSize,
	) error
}

// CheckLease verifies that a gateway recognizes the lease and preserves any
// provider response detail in the returned error.
func CheckLease(ctx context.Context, client LeaseStatusClient, id mtypes.LeaseID) error {
	_, err := client.LeaseStatus(ctx, id)
	return GatewayError("query lease status", err)
}

// RunLeaseShell verifies the lease before starting the remote process and
// preserves provider response details from either boundary call.
func RunLeaseShell(
	ctx context.Context,
	client LeaseShellClient,
	id mtypes.LeaseID,
	service string,
	podIndex uint,
	command []string,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	tty bool,
	resize <-chan remotecommand.TerminalSize,
) error {
	if err := CheckLease(ctx, client, id); err != nil {
		return err
	}

	err := client.LeaseShell(ctx, id, service, podIndex, command,
		stdin, stdout, stderr, tty, resize)
	return GatewayError("open lease shell", err)
}

// StreamCloseError distinguishes a bounded stream's normal EOF from an
// interrupted follow stream. Providers commonly end one-shot websocket reads
// by closing the connection without a close frame.
func StreamCloseError(kind, reason string, follow bool) error {
	if reason == "" {
		return nil
	}
	if !follow && strings.Contains(strings.ToLower(reason), "eof") {
		return nil
	}

	return fmt.Errorf("%s stream closed: %s", kind, reason)
}

// ConsumeStream drains both provider channels before interpreting the close
// reason. The gateway library delivers records and close metadata on separate
// channels, so returning when either closes can drop records or hide a follow
// failure depending on select scheduling.
func ConsumeStream[T any](
	ctx context.Context,
	kind string,
	stream <-chan T,
	onClose <-chan string,
	follow bool,
	emit func(T) error,
) error {
	closeReason := ""
	for stream != nil || onClose != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case record, ok := <-stream:
			if !ok {
				stream = nil
				continue
			}
			if err := emit(record); err != nil {
				return err
			}
		case reason, ok := <-onClose:
			if !ok {
				onClose = nil
				continue
			}
			if reason != "" {
				closeReason = reason
			}
		}
	}

	return StreamCloseError(kind, closeReason, follow)
}

// ValidateLogTail rejects values the gateway boundary cannot honor.
func ValidateLogTail(follow bool, tail int64) error {
	if tail < -1 {
		return fmt.Errorf("--tail must be -1 or greater")
	}
	if follow && tail >= 0 {
		return fmt.Errorf("--tail cannot be combined with --follow")
	}

	return nil
}

// RetainTail appends one record while retaining at most limit records.
func RetainTail[T any](records []T, record T, limit int64) []T {
	if limit <= 0 {
		return nil
	}

	records = append(records, record)
	if int64(len(records)) > limit {
		records = records[len(records)-int(limit):]
	}

	return records
}

// MatchesService reports whether a provider log pod belongs to service.
func MatchesService(name, service string) bool {
	if service == "" {
		return true
	}

	podPrefix := service + "-"
	return name == service || (strings.HasPrefix(name, podPrefix) && len(name) > len(podPrefix))
}

// HoldEOF returns a reader that withholds a local EOF until ctx is done. The
// upstream provider shell client treats stdin EOF as an operation error even
// after the remote process has succeeded; returning a clean zero-byte read
// after cancellation lets its stdin goroutine exit through the context path.
func HoldEOF(ctx context.Context, reader io.Reader) io.Reader {
	if reader == nil {
		return nil
	}

	return &holdEOFReader{ctx: ctx, reader: reader}
}

// SelectShellStdin chooses whether a shell invocation advertises stdin to the
// provider. Interactive shells and piped commands attach automatically. An
// explicit command launched from a terminal detaches by default so a provider
// cannot keep the completed command open waiting for terminal input.
func SelectShellStdin(
	ctx context.Context,
	reader io.Reader,
	interactive bool,
	inputIsTerminal bool,
	overrideSet bool,
	overrideValue bool,
) io.Reader {
	if reader == nil {
		return nil
	}

	attach := interactive || !inputIsTerminal
	if overrideSet {
		attach = overrideValue
	}
	if !attach {
		return nil
	}

	return HoldEOF(ctx, reader)
}

type holdEOFReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *holdEOFReader) Read(data []byte) (int, error) {
	n, err := reader.reader.Read(data)
	if !errors.Is(err, io.EOF) {
		return n, err
	}
	if n > 0 {
		return n, nil
	}

	<-reader.ctx.Done()
	return 0, nil
}

// GatewayError adds operation context to err and includes a provider's
// response body when the gateway client exposes one.
func GatewayError(action string, err error) error {
	if err == nil {
		return nil
	}

	var responseErr rest.ClientResponseError
	if errors.As(err, &responseErr) {
		if detail := strings.TrimSpace(responseErr.Message); detail != "" {
			return fmt.Errorf("%s: %w: %s", action, err, detail)
		}
	}

	return fmt.Errorf("%s: %w", action, err)
}
