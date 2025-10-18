package main

import (
	"errors"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/energye/systray"

	_ "embed"
)

var (
	ignoreQuit bool
)

//go:embed Icon.ico
var iconData []byte

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile | log.Lmicroseconds)
	flag.BoolVar(&ignoreQuit, "ignore-quit", false, "ignore quit RPC")
	flag.Parse()
}

func main() {
	defer cleanup()

	// Check if server is already running
	if isRunning() {
		log.Println("already server running, exiting")
		return
	}
	// Start IPC server
	srv := NewServer()
	defer srv.Close()

	// Handle SIGINT and SIGTERM for graceful shutdown
	signalHandler(srv)

	go systray.Run(onReady(srv), onExit)

	// Run the server
	if err := srv.Run(); err != nil && !errors.Is(err, net.ErrClosed) {
		log.Fatalf("server: %v", err)
	}
}
func onReady(srv *Server) func() {
	return func() {
		systray.SetIcon(iconData)
		systray.SetTitle("goCAN Gateway")
		systray.SetTooltip("goCAN Gateway")
		systray.SetOnClick(func(menu systray.IMenu) {
			menu.ShowMenu()
		})
		systray.SetOnRClick(func(menu systray.IMenu) {
			menu.ShowMenu()
		})
		mQuit := systray.AddMenuItem("Quit", "Quit")
		mQuit.Click(func() {
			if err := srv.Close(); err != nil {
				log.Fatalf("failed to close server: %v", err)
			}
		})
		// Sets the icon of a menu item.
		//mQuit.SetIcon(icon.Data)
	}
}

func onExit() {
	// clean up here
}

// signalHandler sets up a signal handler to gracefully shut down the server on SIGINT or SIGTERM.
func signalHandler(srv *Server) {
	sigChan := make(chan os.Signal, 2)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Shutting down server")
		if err := srv.Close(); err != nil {
			log.Fatalf("failed to close server: %v", err)
		}
	}()
}
