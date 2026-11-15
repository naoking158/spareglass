// Command spareglass turns any device with a web browser into an extra
// display for this computer.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/naoking158/spareglass/internal/config"
	"github.com/naoking158/spareglass/internal/doctor"
	"github.com/naoking158/spareglass/internal/netutil"
	"github.com/naoking158/spareglass/internal/pairing"
	"github.com/naoking158/spareglass/internal/server"
	"github.com/naoking158/spareglass/internal/vdisplay"
)

// Set by GoReleaser via -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

const usage = `spareglass - use any browser as an extra display

Usage:
  spareglass [serve] [flags]   start the host (default)
  spareglass doctor            check what works on this machine
  spareglass config path       print the config file location
  spareglass config show       print the effective config
  spareglass config init       write a config file with defaults
  spareglass version           print version information

Run "spareglass serve -h" for serve flags.
`

func main() {
	args := os.Args[1:]
	cmd := "serve"
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		cmd, args = args[0], args[1:]
	}

	var err error
	switch cmd {
	case "serve":
		err = runServe(args)
	case "doctor":
		err = doctor.Run(os.Stdout, version, vdisplay.Default())
	case "config":
		err = runConfig(args)
	case "version":
		fmt.Printf("spareglass %s (commit %s, built %s)\n", version, commit, date)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func runServe(args []string) error {
	cfg, path, err := config.Load()
	if err != nil {
		return err
	}

	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	fs.StringVar(&cfg.Addr, "addr", cfg.Addr, "listen address")
	fs.IntVar(&cfg.FPS, "fps", cfg.FPS, "target frames per second")
	fs.IntVar(&cfg.BitrateKbps, "bitrate", cfg.BitrateKbps, "target video bitrate in kbps")
	fs.IntVar(&cfg.MaxClients, "max-clients", cfg.MaxClients, "maximum number of connected displays")
	fs.Float64Var(&cfg.RenderScale, "render-scale", cfg.RenderScale, "virtual display size relative to the client's CSS pixels (1 = legible, devicePixelRatio = sharp)")
	fs.StringVar(&cfg.Token, "token", cfg.Token, "fixed pairing token (default: random per run)")
	verbose := fs.Bool("v", false, "verbose logging")
	_ = fs.Parse(args)

	if err := cfg.Validate(); err != nil {
		return err
	}

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	token := cfg.Token
	if token == "" {
		token, err = pairing.NewToken()
		if err != nil {
			return err
		}
	}

	provider := vdisplay.Default()
	if err := provider.Available(); err != nil {
		log.Warn("virtual displays are not available; clients can connect but will not get a screen yet",
			"provider", provider.Name(), "err", err)
	}

	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Addr, err)
	}

	srv := server.New(server.Options{
		Config:   cfg,
		Token:    pairing.Token(token),
		Provider: provider,
		Logger:   log,
	})
	httpSrv := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() { errCh <- httpSrv.Serve(ln) }()

	printBanner(ln.Addr().(*net.TCPAddr).Port, token, path)

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}

	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	srv.Close() // removes every virtual display we created
	return nil
}

func printBanner(port int, token, cfgPath string) {
	urls := netutil.LANURLs(port, "#t="+token)
	fmt.Println()
	fmt.Println("spareglass is running.")
	if len(urls) == 0 {
		fmt.Printf("  No LAN address found. Open http://<this-computer>:%d/#t=%s on the other device.\n", port, token)
	} else {
		if qr, err := qrcode.New(urls[0], qrcode.Low); err == nil {
			fmt.Print(qr.ToSmallString(false))
		}
		fmt.Println("Open one of these on the device you want to use as a display:")
		for _, u := range urls {
			fmt.Println("  " + u)
		}
	}
	fmt.Printf("Config: %s\n", cfgPath)
	fmt.Println("Press Ctrl+C to stop.")
	fmt.Println()
}

func runConfig(args []string) error {
	if len(args) == 0 {
		return errors.New(`config needs a subcommand: path, show or init`)
	}
	switch args[0] {
	case "path":
		p, err := config.Path()
		if err != nil {
			return err
		}
		fmt.Println(p)
	case "show":
		cfg, _, err := config.Load()
		if err != nil {
			return err
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(cfg)
	case "init":
		p, err := config.Init()
		if err != nil {
			return err
		}
		fmt.Println("wrote", p)
	default:
		return fmt.Errorf("unknown config subcommand %q", args[0])
	}
	return nil
}
