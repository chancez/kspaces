package cli

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/chancez/kspaces/internal/logs"
	"github.com/chancez/kspaces/internal/server"
	"github.com/chancez/kspaces/internal/spaces"
)

func (a *app) serverCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Run this host's kspaces server, which kspaces starts itself when it needs one",
		Long: `The server holds this host's data, its config, spaces, worktrees and their git state, for every kspaces
process on the host and, through kspaces server proxy over ssh, for other machines. kspaces starts it when
it finds none, and it exits once it has had no client for five minutes, or its binary is replaced.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := server.SocketPath()
			if err != nil {
				return err
			}
			ln, unlock, err := server.Listen(path)
			if errors.Is(err, server.ErrRunning) {
				// Another client started one first.
				return nil
			}
			if err != nil {
				return err
			}
			defer unlock()
			ctx, cancel := signal.NotifyContext(cmd.Context(), syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
			defer cancel()
			name, _ := os.Hostname()
			s := server.New(spaces.NewLocalHost(strings.TrimSuffix(path, ".sock")+".git.json"), a.version, name)
			if exe, err := os.Executable(); err == nil {
				s.WatchExe(server.ExeChanged(exe))
			}
			logs.Debug("server listening", "socket", path)
			return s.Serve(ctx, ln)
		},
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "proxy",
		Short: "Connect stdin and stdout to this host's server, starting it if need be, as another machine does over ssh",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := server.SocketPath()
			if err != nil {
				return err
			}
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			conn, err := server.DialLocal(path, exe)
			if err != nil {
				return err
			}
			defer conn.Close()
			done := make(chan error, 2)
			go func() {
				_, err := io.Copy(conn, os.Stdin)
				done <- err
			}()
			go func() {
				_, err := io.Copy(os.Stdout, conn)
				done <- err
			}()
			return <-done
		},
	}, &cobra.Command{
		Use:   "stop",
		Short: "Ask this host's server to exit; the next kspaces to need one starts it again",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := server.SocketPath()
			if err != nil {
				return err
			}
			c := server.NewClient(func() (io.ReadWriteCloser, error) { return net.Dial("unix", path) }, a.version)
			if err := c.Exit(); err != nil {
				var op *net.OpError
				if errors.As(err, &op) {
					fmt.Fprintln(a.stdout, "no kspaces server is running")
					return nil
				}
				return err
			}
			return nil
		},
	})
	return cmd
}
