package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	_ "github.com/roffe/gocan/adapter"
	"github.com/roffe/gocan/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

var (
	ignoreQuit bool
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile | log.Lmicroseconds)
	flag.BoolVar(&ignoreQuit, "ignore-quit", false, "ignore quit RPC")
	flag.Parse()
}

func fileExists(name string) bool {
	_, err := os.Stat(name)
	return !os.IsNotExist(err)
}

func isRunning(socketFile string) bool {
	if fileExists(socketFile) {
		conn, err := grpc.NewClient(
			"unix:"+socketFile,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			log.Println(err)
			os.Remove(socketFile)
			return false
		}
		defer conn.Close()
		client := proto.NewGocanClient(conn)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		resp, err := client.SendCommand(ctx, &proto.Command{Data: []byte("ping")})
		if err != nil {
			os.Remove(socketFile)
			return false
		}
		if bytes.Equal(resp.GetData(), []byte("pong")) {
			return true
		}
		os.Remove(socketFile)
		return false
	}
	return false
}

func main() {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		log.Fatalf("failed to get user cache dir: %v", err)
	}

	socketFile := filepath.Join(cacheDir, "gocan.sock")
	if isRunning(socketFile) {
		log.Printf("already server listening at %s", socketFile)
		return
	}

	defer os.Remove(socketFile)

	// Start IPC server
	srv := NewServer(socketFile)
	defer srv.Close()

	sigChan := make(chan os.Signal, 2)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Shutting down server")
		if err := srv.Close(); err != nil {
			log.Fatalf("failed to close server: %v", err)
		}
	}()

	if err := srv.Run(); err != nil && !errors.Is(err, net.ErrClosed) {
		log.Fatalf("server: %v", err)
	}

}

/*
	func main() {
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			log.Fatalf("failed to get user cache dir: %v", err)
		}

		socketFile := filepath.Join(cacheDir, "gocan.sock")
		if isRunning(socketFile) {
			log.Printf("already server listening at %s", socketFile)
			return
		}

		defer os.Remove(socketFile)

		// Start IPC server
		srv := NewServer(socketFile)
		defer srv.Close()

		sigChan := make(chan os.Signal, 2)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigChan
			log.Println("Shutting down server")
			if err := srv.Close(); err != nil {
				log.Fatalf("failed to close server: %v", err)
			}
		}()
		go func() {
			if err := srv.Run(); err != nil {
				log.Fatalf("server: %v", err)
			}
		}()

		a := app.NewWithID("com.roffe.cangw")
		if desk, ok := a.(desktop.App); ok {

			m := fyne.NewMenu("",
				fyne.NewMenuItem("Show", func() {
					w := a.NewWindow("GoCAN Gateway")
					output := widget.NewLabel("")
					adapterList := widget.NewSelect(adapter.List(), func(s string) {
						log.Printf("selected adapter: %v", s)
						ad := adapter.GetAdapterMap()[s]
						if ad != nil {
							var out strings.Builder
							out.WriteString("Description: " + ad.Description + "\n")
							out.WriteString("Capabilities:\n")
							out.WriteString("  HSCAN: " + strconv.FormatBool(ad.Capabilities.HSCAN) + "\n")
							out.WriteString("  KLine: " + strconv.FormatBool(ad.Capabilities.KLine) + "\n")
							out.WriteString("  SWCAN: " + strconv.FormatBool(ad.Capabilities.SWCAN) + "\n")
							out.WriteString("RequiresSerialPort: " + strconv.FormatBool(ad.RequiresSerialPort) + "\n")
							output.SetText(out.String())
						}
					})

					w.SetContent(container.NewBorder(
						container.NewBorder(nil, nil, widget.NewLabel("Info"), nil,
							adapterList,
						),
						nil,
						nil,
						nil,
						output,
					))
					w.Resize(fyne.Size{Width: 350, Height: 125})
					w.Show()
				}))
			desk.SetSystemTrayMenu(m)
		}
		a.Run()
		log.Println("Exiting")
	}
*/
var _ proto.GocanServer = (*Server)(nil)

type Server struct {
	proto.UnimplementedGocanServer

	l net.Listener
}

func NewServer(socketFile string) *Server {
	l, err := net.Listen("unix", socketFile)
	if err != nil {
		log.Fatal(err)
	}
	srv := &Server{l: l}

	return srv
}

var kaep = keepalive.EnforcementPolicy{
	MinTime:             5 * time.Second, // If a client pings more than once every 5 seconds, terminate the connection
	PermitWithoutStream: true,            // Allow pings even when there are no active streams
}

var kasp = keepalive.ServerParameters{
	MaxConnectionIdle:     15 * time.Second, // If a client is idle for 15 seconds, send a GOAWAY
	MaxConnectionAge:      0,                // If any connection is alive for more than 30 seconds, send a GOAWAY
	MaxConnectionAgeGrace: 5 * time.Second,  // Allow 5 seconds for pending RPCs to complete before forcibly closing connections
	Time:                  5 * time.Second,  // Ping the client if it is idle for 5 seconds to ensure the connection is still active
	Timeout:               3 * time.Second,  // Wait 1 second for the ping ack before assuming the connection is dead
}

func (s *Server) Run() error {
	sg := grpc.NewServer(grpc.KeepaliveEnforcementPolicy(kaep), grpc.KeepaliveParams(kasp))
	proto.RegisterGocanServer(sg, s)
	log.Printf("server listening at %v", s.l.Addr())
	if err := sg.Serve(s.l); err != nil {
		return fmt.Errorf("failed to serve: %w", err)
	}
	return nil
}

func (s *Server) Close() error {
	return s.l.Close()
}

// ##############################
/*
const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPauseAndContinue

type gocanGatewayService struct {
	c      *Server
	r      <-chan svc.ChangeRequest
	status chan<- svc.Status

	socketFile string
}

func (m *gocanGatewayService) Execute(args []string, r <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}

	m.r = r
	m.status = status

	var err error
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		log.Fatalf("failed to get user cache dir: %v", err)
	}

	m.socketFile = filepath.Join(cacheDir, "gocan.sock")

	defer os.Remove(m.socketFile)

	m.c, err = m.startGateway()
	if err != nil {
		log.Printf("failed to start gateway: %v", err)
		status <- svc.Status{State: svc.StopPending}
		time.Sleep(100 * time.Millisecond)
		return false, 1
	}

	status <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	sigChan := make(chan os.Signal, 2)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Shutting down server")
		if err := m.c.Close(); err != nil {
			log.Fatalf("failed to close server: %v", err)
		}
	}()

	for c := range r {
		switch c.Cmd {
		case svc.Interrogate:
			log.Println("Interrogate...!", c.CurrentStatus)
			status <- c.CurrentStatus
			continue
		case svc.Stop, svc.Shutdown:
			log.Print("Shutting service...!")
			status <- svc.Status{State: svc.StopPending}
			time.Sleep(100 * time.Millisecond)
			return false, 1
		case svc.Pause:
			log.Print("Pausing service...!")
			m.c.Close()
			m.c = nil
			status <- svc.Status{State: svc.Paused, Accepts: cmdsAccepted}
			continue
		case svc.Continue:
			log.Print("Continuing service...!")
			m.c, err = m.startGateway()
			if err != nil {
				log.Printf("failed to start gateway: %v", err)
				status <- svc.Status{State: svc.StopPending}
				continue
			}

			status <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
			continue
		default:
			log.Printf("Unexpected service control request #%d", c)
			continue
		}
	}
	status <- svc.Status{State: svc.StopPending}
	return false, 1
}

func (m *gocanGatewayService) startGateway() (*Server, error) {

	// Start IPC server
	srv := NewServer(m.socketFile)
	//defer srv.Close()

	go func() {
		if err := srv.Run(); err != nil {
			log.Fatalf("server: %v", err)
		}
	}()
	return srv, nil
}

func runService(name string, isDebug bool) {
	if isDebug {
		err := debug.Run(name, &gocanGatewayService{})
		if err != nil {
			log.Fatalln("Error running service in debug mode.")
		}
	} else {
		err := svc.Run(name, &gocanGatewayService{})
		if err != nil {
			log.Fatalln("Error running service in Service Control mode.")
		}
	}
}

func main2() {
	f, err := os.OpenFile("E:\\gocan_gateway_debug.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalln(fmt.Errorf("error opening file: %v", err))
	}
	defer f.Close()

	log.SetOutput(f)
	runService("myservice", true)
}
*/
