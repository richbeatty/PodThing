package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"image/png"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/richbeatty/PodThing/internal/display"
	"github.com/richbeatty/PodThing/internal/hardware"
	"github.com/richbeatty/PodThing/internal/player"
)

func main() {
	if e := run(); e != nil {
		log.Print(e)
		os.Exit(1)
	}
}
func run() error {
	demo := flag.Bool("demo", false, "simulate a library; never connect to an iPod or play audio")
	headless := flag.Bool("headless", false, "serve the device console without claiming screen or physical inputs")
	listen := flag.String("listen", "127.0.0.1:8787", "HTTP console address; use SSH forwarding for remote access")
	state := flag.String("state-dir", "podthing-state", "writable settings/report directory")
	socket := flag.String("wayland-socket", "", "Weston socket (default: XDG_RUNTIME_DIR/WAYLAND_DISPLAY)")
	screenshot := flag.String("screenshot", "", "save one 800x480 PNG and exit")
	diagnose := flag.Bool("diagnose", false, "print read-only USB/audio diagnostics and exit")
	version := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *version {
		fmt.Println(player.Version)
		return nil
	}
	a, e := player.New(*demo, *state)
	if e != nil {
		return e
	}
	defer a.Close()
	if *diagnose {
		b, e := a.Report()
		if e == nil {
			fmt.Println(string(b))
		}
		return e
	}
	p, e := player.NewPainter()
	if e != nil {
		return e
	}
	if *screenshot != "" {
		f, e := os.Create(*screenshot)
		if e != nil {
			return e
		}
		e = png.Encode(f, p.Render(a.View()))
		closeErr := f.Close()
		if e != nil {
			return e
		}
		return closeErr
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	var window *display.Window
	if !*headless {
		window, e = display.Connect(*socket)
		if e != nil {
			return fmt.Errorf("native display: %w; desktop testing can use --demo --headless", e)
		}
		defer window.Close()
		inputs := make(chan hardware.Input, 64)
		hardware.ReadInputs(ctx, func(v hardware.Input) {
			select {
			case inputs <- v:
			case <-ctx.Done():
			}
		}, a.Log)
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case v := <-inputs:
					if e := a.Action(player.Action{Name: v.Action, Value: v.Value, X: v.X, Y: v.Y}); e != nil {
						a.Log("Input action: " + e.Error())
					}
				}
			}
		}()
	}
	frames := &player.Frames{}
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	render := func() error {
		img := p.Render(a.View())
		var b bytes.Buffer
		if e := encoder.Encode(&b, img); e != nil {
			return e
		}
		frames.PNG.Store(b.Bytes())
		if window != nil {
			return window.Present(img.Pix)
		}
		return nil
	}
	if e = render(); e != nil {
		return e
	}
	listener, e := net.Listen("tcp", *listen)
	if e != nil {
		return e
	}
	server := &http.Server{Handler: a.Handler(frames), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 90 * time.Second, IdleTimeout: 30 * time.Second}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()
	defer server.Close()
	mode := "hardware"
	if *demo {
		mode = "DEMO (no physical playback)"
	}
	log.Printf("PodThing %s; %s; console bound to %s", player.Version, mode, listener.Addr())
	a.Log("PodThing started in " + mode + " mode")
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.Tick()
			}
		}
	}()
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.Scan()
			}
		}
	}()
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case e := <-serveErr:
			if e == http.ErrServerClosed {
				return nil
			}
			return e
		case <-tick.C:
			if e = render(); e != nil {
				return e
			}
		}
	}
}
