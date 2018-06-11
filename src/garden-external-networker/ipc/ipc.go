package ipc

import (
	"encoding/json"
	"fmt"
	"garden-external-networker/manager"
	"io"
	"net"
	"os"
	"os/signal"
	"syscall"

	"code.cloudfoundry.org/netplugin-shim/message"
)

//go:generate counterfeiter . SocketManager
type SocketManager interface {
	ReadFileDescriptor(net.Conn) (uintptr, error)
	ReadMessage(io.Reader) (message.Message, error)
}

type upFunction func(handle string, inputs manager.UpInputs) (*manager.UpOutputs, error)
type downFunction func(handle string) error

type Mux struct {
	Up            upFunction
	Down          downFunction
	SocketManager SocketManager
	KillChannel   chan os.Signal
}

func NewMux(up upFunction, down downFunction) *Mux {
	return &Mux{
		SocketManager: new(UnixSocketManager),
		Up:            up,
		Down:          down,
		KillChannel:   make(chan os.Signal, 1),
	}
}

func (m *Mux) Handle(action string, handle string, stdin io.Reader, stdout io.Writer) error {
	if handle == "" {
		return fmt.Errorf("missing handle")
	}

	switch action {
	case "up":
		var inputs manager.UpInputs
		if err := json.NewDecoder(stdin).Decode(&inputs); err != nil {
			return err
		}
		outputs, err := m.Up(handle, inputs)
		if err != nil {
			return err
		}
		if err := json.NewEncoder(stdout).Encode(outputs); err != nil {
			return err
		}
	case "down":
		err := m.Down(handle)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unrecognized action: %s", action)
	}
	return nil
}

func (m *Mux) HandleWithSocket(logger io.Writer, socketPath string) error {
	fmt.Fprint(logger, "handle with socket")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}
	closeOnKill(logger, listener, m.KillChannel)

	for {
		connection, err := listener.Accept()
		if err != nil {
			fmt.Fprintf(logger, "Failed to accept connection: %s", err.Error())
			continue
		}

		_, err = m.SocketManager.ReadFileDescriptor(connection)
		if err != nil {
			sendError(connection, err)
			continue
		}

		msg, err := m.SocketManager.ReadMessage(connection)
		if err != nil {
			sendError(connection, err)
			continue
		}

		if msg.Command == "up" {
			response := `{
			"properties": {
				"garden.network.container-ip": "169.254.1.2",
				"garden.network.host-ip": "255.255.255.255",
				"garden.network.mapped-ports": "[{\"HostPort\":12345,\"ContainerPort\":7000},{\"HostPort\":60000,\"ContainerPort\":7000}]"
			},
			"dns_servers": [
				"1.2.3.4"
			],
			"search_domains": [
				"pivotal.io",
				"foo.bar",
				"baz.me"
			]
		}`

			connection.Write([]byte(response))
		}

		if err := connection.Close(); err != nil {
			fmt.Fprintf(logger, "Failed to close the connection: %s", err.Error())
		}
	}

}

// TODO: Proper send error testing
func sendError(writer io.Writer, err error) {
}

func closeOnKill(logger io.Writer, closer io.Closer, signalChannel chan os.Signal) {
	signal.Notify(signalChannel, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-signalChannel
		if err := closer.Close(); err != nil {
			fmt.Fprintln(logger, err)
		}
		os.Exit(0)
	}()
}

type SocketRequestErrorHandler interface {
	HandleError(writer io.Writer, err error)
}
