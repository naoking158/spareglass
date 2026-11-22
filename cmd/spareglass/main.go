// Command spareglass turns any device with a web browser into an extra
// display for this computer.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	"github.com/naoking158/spareglass/internal/media"
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
  spareglass install-driver    install the bundled virtual display driver (Windows)
  spareglass config path       print the config file location
  spareglass config show       print the effective config
  spareglass config init       write a config file with defaults
  spareglass devices           list browsers allowed to connect
  spareglass devices revoke ID forget one browser
  spareglass devices clear     forget every browser
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
	case "install-driver":
		err = runInstallDriver()
	case "config":
		err = runConfig(args)
	case "devices":
		err = runDevices(args)
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
		holdWindow()
		os.Exit(1)
	}
}

func runServe(args []string) error {
	cfg, path, err := config.Load()
	if err != nil {
		return err
	}

	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	fs.StringVar(&cfg.Addr, "addr", cfg.Addr, "listen address (default: port 80, else 7878)")
	fs.IntVar(&cfg.FPS, "fps", cfg.FPS, "target frames per second")
	fs.IntVar(&cfg.BitrateKbps, "bitrate", cfg.BitrateKbps, "target video bitrate in kbps")
	fs.IntVar(&cfg.MaxClients, "max-clients", cfg.MaxClients, "maximum number of connected displays")
	fs.Float64Var(&cfg.RenderScale, "render-scale", cfg.RenderScale, "virtual display size relative to the client's CSS pixels (1 = legible, devicePixelRatio = sharp)")
	verbose := fs.Bool("v", false, "verbose logging")
	testPattern := fs.Bool("test-pattern", false, "stream ffmpeg's test pattern instead of the virtual display (to check the browser side)")
	noOpen := fs.Bool("no-open", false, "do not open the host page in the default browser")
	_ = fs.Parse(args)

	if err := cfg.Validate(); err != nil {
		return err
	}

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(log)

	devicesPath, err := config.DevicesPath()
	if err != nil {
		return err
	}
	store, err := pairing.OpenStore(devicesPath)
	if err != nil {
		return err
	}

	release, err := claimInstance()
	if err != nil {
		return err
	}
	defer release()

	provider := vdisplay.Default()
	if !*testPattern {
		installDriverIfMissing(provider)
	}
	var displayProblem string
	if err := provider.Available(); err != nil {
		displayProblem = err.Error()
		log.Warn("virtual displays are not available; clients can connect but will not get a screen yet",
			"provider", provider.Name(), "err", err)
	} else if err := provider.CloseAll(); err != nil {
		log.Warn("removing leftover virtual displays", "err", err)
	}

	ln, err := listen(cfg.Addr, log)
	if err != nil {
		return err
	}
	hostLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen for the host page: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	ctx, quit := context.WithCancel(ctx) // the host page's Quit; signals stay handled during shutdown
	defer quit()

	pairer := pairing.NewPairer(ctx, store, pairing.NewTerminalApprover(os.Stdin, os.Stdout))
	var factory media.Factory // nil: the platform pipeline
	if *testPattern {
		factory = func(_ string, fps, _ int) (media.Pipeline, error) {
			return media.NewFFmpegPipeline(media.TestPatternArgs(1280, 720, fps), fps)
		}
	}
	srv := server.New(server.Options{
		Config:      cfg,
		Pairer:      pairer,
		Provider:    provider,
		NewPipeline: factory,
		Logger:      log,
	})
	httpSrv := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}

	port := ln.Addr().(*net.TCPAddr).Port
	token := randomToken()
	hostSrv := &http.Server{Handler: srv.HostHandler(server.HostOptions{
		Token:          token,
		URLs:           func() []string { return netutil.LANURLs(port, "/1") },
		DisplayProblem: displayProblem,
		Stop:           quit,
	}), ReadHeaderTimeout: 10 * time.Second}

	errCh := make(chan error, 2)
	go func() { errCh <- httpSrv.Serve(ln) }()
	go func() { errCh <- hostSrv.Serve(hostLn) }()

	hostURL := fmt.Sprintf("http://127.0.0.1:%d/#%s", hostLn.Addr().(*net.TCPAddr).Port, token)
	printBanner(port, netutil.LANURLs(port, "/1"), cfg.MaxClients, len(store.List()), path, hostURL)
	if !*noOpen {
		if err := openBrowser(hostURL); err != nil {
			log.Warn("could not open the host page; open it yourself", "url", hostURL, "err", err)
		}
	}

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
	_ = hostSrv.Shutdown(shutdownCtx)
	srv.Close() // removes every virtual display we created
	return nil
}

func randomToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// listen binds addr, or port 80 then 7878 when addr is empty.
func listen(addr string, log *slog.Logger) (net.Listener, error) {
	if addr != "" {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return nil, fmt.Errorf("listen on %s: %w", addr, err)
		}
		return ln, nil
	}
	ln, err := net.Listen("tcp", ":80")
	if err == nil {
		return ln, nil
	}
	log.Info("port 80 unavailable, using 7878", "err", err)
	ln, err = net.Listen("tcp", ":7878")
	if err != nil {
		return nil, fmt.Errorf("listen on :80 and :7878 failed: %w", err)
	}
	return ln, nil
}

func printBanner(port int, urls []string, displays, paired int, cfgPath, hostURL string) {
	fmt.Println()
	fmt.Println("spareglass is running.")
	fmt.Println("Host page (approve devices, quit): " + hostURL)
	if len(urls) == 0 {
		host := "<this-computer-ip>"
		if port != 80 {
			host = fmt.Sprintf("%s:%d", host, port)
		}
		fmt.Printf("  No LAN address found. Open http://%s/1 on the other device.\n", host)
	} else {
		if qr, err := qrcode.New(urls[0], qrcode.Low); err == nil {
			fmt.Print(qr.ToSmallString(false))
		}
		fmt.Println("On the device you want to use as a display, open:")
		for _, u := range urls {
			fmt.Println("  " + u)
		}
		if displays > 1 {
			fmt.Printf("  (/1 to /%d choose the display)\n", displays)
		}
	}
	fmt.Printf("New browsers ask for approval on the host page or here. Paired browsers: %d (manage on the host page or with \"spareglass devices\").\n", paired)
	fmt.Printf("Config: %s\n", cfgPath)
	fmt.Println("Press Ctrl+C to stop.")
	fmt.Println()
}

func runDevices(args []string) error {
	p, err := config.DevicesPath()
	if err != nil {
		return err
	}
	store, err := pairing.OpenStore(p)
	if err != nil {
		return err
	}
	if len(args) == 0 || args[0] == "list" {
		list := store.List()
		if len(list) == 0 {
			fmt.Println("No paired browsers.")
			return nil
		}
		for _, d := range list {
			fmt.Printf("%s  %-28s  paired %s\n", d.ID, d.Name, d.Created.Local().Format("2006-01-02 15:04"))
		}
		return nil
	}
	switch args[0] {
	case "revoke":
		if len(args) < 2 {
			return errors.New("usage: spareglass devices revoke ID")
		}
		if err := store.Remove(args[1]); err != nil {
			return err
		}
		fmt.Println("revoked", args[1])
	case "clear":
		if err := store.Clear(); err != nil {
			return err
		}
		fmt.Println("All browsers forgotten.")
	default:
		return fmt.Errorf("unknown devices subcommand %q", args[0])
	}
	return nil
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
