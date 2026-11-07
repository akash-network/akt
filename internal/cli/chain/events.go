package cli

import (
	"context"
	"errors"

	flagdefs "pkg.akt.dev/akt/internal/flags"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"pkg.akt.dev/akt/internal/capability"
	"pkg.akt.dev/akt/internal/output"

	"pkg.akt.dev/go/util/events"
	"pkg.akt.dev/go/util/pubsub"
)

// EventsCmd prints out events in real time
func EventsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:         "events",
		Short:       "Prints out akash events in real time",
		Args:        cobra.NoArgs,
		Example:     "akt events --node http://localhost:26657",
		Annotations: map[string]string{capability.AnnotationKey: string(capability.ChainQuery)},
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunForeverWithContext(cmd.Context(), func(ctx context.Context) error {
				return getEvents(ctx, cmd, args)
			})
		},
	}

	cmd.Flags().String(flagdefs.FlagNode, "tcp://localhost:26657", "The node address")

	return cmd
}

func getEvents(ctx context.Context, cmd *cobra.Command, _ []string) error {
	cctx, err := GetClientQueryContext(cmd)
	if err != nil {
		return err
	}

	node, err := cctx.GetNode()
	if err != nil {
		return err
	}

	bus := pubsub.NewBus()
	defer bus.Close()

	group, ctx := errgroup.WithContext(ctx)

	subscriber, err := bus.Subscribe()
	if err != nil {
		return err
	}

	evtSvc, err := events.NewEvents(ctx, node, "akash-cli", bus)
	if err != nil {
		return err
	}

	group.Go(func() error {
		<-ctx.Done()
		evtSvc.Shutdown()

		return nil
	})

	group.Go(func() error {
		for {
			select {
			case <-subscriber.Done():
				return nil
			case ev := <-subscriber.Events():
				var err error
				if output.FormatFromCmd(cmd) == output.FormatTable {
					err = output.FprintJSONSemantics(cmd.OutOrStdout(), output.FormatJSON, ev)
				} else {
					err = output.PrintStreamRecord(cmd, ev, "")
				}
				if err != nil {
					return err
				}
			}
		}
	})

	err = group.Wait()
	if err != nil && !errors.Is(err, context.Canceled) {
		return err
	}

	return nil
}
