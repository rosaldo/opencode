package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/opencode-ai/opencode/internal/config"
	"github.com/opencode-ai/opencode/internal/db"
	"github.com/opencode-ai/opencode/internal/message"
	"github.com/opencode-ai/opencode/internal/pubsub"
	"github.com/opencode-ai/opencode/internal/server"
	"github.com/opencode-ai/opencode/internal/session"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run a local HTTP API over sessions and events",
	Long: `Serve exposes a loopback-only HTTP API with session/message read endpoints
and a Server-Sent Events stream. A token is required for any non-loopback bind.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		addr, _ := cmd.Flags().GetString("addr")
		token, _ := cmd.Flags().GetString("token")

		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		if c, _ := cmd.Flags().GetString("cwd"); c != "" {
			cwd = c
		}
		if _, err := config.Load(cwd, false); err != nil {
			return err
		}

		conn, err := db.Connect()
		if err != nil {
			return err
		}
		defer conn.Close()

		q := db.New(conn)
		sessions := session.NewService(q)
		messages := message.NewService(q)

		srv, err := server.New(server.Config{
			Addr:     addr,
			Token:    token,
			Sessions: sessions,
			Messages: messages,
		})
		if err != nil {
			return err
		}
		if err := srv.Start(); err != nil {
			return err
		}
		defer srv.Close()
		fmt.Printf("opencode serve listening on %s\n", srv.URL())
		if token != "" {
			fmt.Printf("web UI: %s/?token=%s\n", srv.URL(), token)
		} else {
			fmt.Printf("web UI: %s/\n", srv.URL())
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Bridge session/message lifecycle events into the SSE stream.
		go bridgeEvents(ctx, srv, sessions.Subscribe(ctx), "session")
		go bridgeEvents(ctx, srv, messages.Subscribe(ctx), "message")

		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		return nil
	},
}

func bridgeEvents[T any](ctx context.Context, srv *server.Server, ch <-chan pubsub.Event[T], kind string) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			srv.Emit(server.Event{Type: kind + "." + string(ev.Type), Payload: ev.Payload})
		}
	}
}

func init() {
	serveCmd.Flags().String("addr", "127.0.0.1:4096", "Listen address (loopback by default)")
	serveCmd.Flags().String("token", "", "Bearer token (required for non-loopback binds)")
	serveCmd.Flags().StringP("cwd", "c", "", "Working directory")
	rootCmd.AddCommand(serveCmd)
}
