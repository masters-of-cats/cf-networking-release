package ipc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"garden-external-networker/manager"
	"io"
	"net"
	"os"
	"os/signal"
	"syscall"

	"code.cloudfoundry.org/netplugin-shim/message"
	"golang.org/x/sys/unix"
)

type Mux struct {
	Up   func(handle string, inputs manager.UpInputs, netNSFD *uintptr) (*manager.UpOutputs, error)
	Down func(handle string) error
}

func (m *Mux) Handle(action string, handle string, stdin io.Reader, stdout io.Writer) error {
	return m.handle(action, handle, nil, stdin, stdout)
}

func (m *Mux) handle(action string, handle string, netNSFD *uintptr, stdin io.Reader, stdout io.Writer) error {
	if handle == "" {
		return fmt.Errorf("missing handle")
	}

	switch action {
	case "up":
		var inputs manager.UpInputs
		if err := json.NewDecoder(stdin).Decode(&inputs); err != nil {
			return err
		}
		outputs, err := m.Up(handle, inputs, netNSFD)
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
		io.WriteString(stdout, "{}")
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
	defer listener.Close()

	for {
		if err := m.handleOne(listener); err != nil {
			fmt.Fprintf(logger, "%v", err)
			continue
		}
	}
}

func (m *Mux) handleOne(listener net.Listener) error {
	connection, err := listener.Accept()
	if err != nil {
		return err
	}
	defer connection.Close()

	nsFD, err := readNsFileDescriptor(connection)
	if err != nil {
		return err
	}

	msg, err := decodeMsg(connection)
	if err != nil {
		return err
	}

	return m.handle(string(msg.Command), string(msg.Handle), newUintptr(nsFD), bytes.NewBuffer(msg.Data), connection)
}

func readNsFileDescriptor(conn net.Conn) (uintptr, error) {
	unixconn, ok := conn.(*net.UnixConn)
	if !ok {
		return 0, errors.New("failed to cast connection to unixconn")
	}

	return recvFD(unixconn)
}

func recvFD(conn *net.UnixConn) (uintptr, error) {
	controlMessageBytesSpace := unix.CmsgSpace(4)

	controlMessageBytes := make([]byte, controlMessageBytesSpace)
	_, readSocketControlMessageBytes, _, _, err := conn.ReadMsgUnix(nil, controlMessageBytes)
	if err != nil {
		return 0, err
	}

	if readSocketControlMessageBytes > controlMessageBytesSpace {
		return 0, errors.New("received too many things")
	}

	controlMessageBytes = controlMessageBytes[:readSocketControlMessageBytes]

	socketControlMessages, err := parseSocketControlMessage(controlMessageBytes)
	if err != nil {
		return 0, err
	}

	fds, err := parseUnixRights(&socketControlMessages[0])
	if err != nil {
		return 0, err
	}

	return uintptr(fds[0]), nil
}

func decodeMsg(r io.Reader) (message.Message, error) {
	var content message.Message
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(&content); err != nil {
		return message.Message{}, err
	}
	return content, nil
}

func parseUnixRights(m *unix.SocketControlMessage) ([]int, error) {
	messages, err := unix.ParseUnixRights(m)
	if err != nil {
		return nil, err
	}
	if len(messages) != 1 {
		return nil, errors.New("no messages parsed")
	}
	return messages, nil
}

func parseSocketControlMessage(b []byte) ([]unix.SocketControlMessage, error) {
	messages, err := unix.ParseSocketControlMessage(b)
	if err != nil {
		return nil, err
	}
	if len(messages) != 1 {
		return nil, errors.New("no messages parsed")
	}
	return messages, nil
}

func onInterrupt(f func()) {
	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-signalChannel
		f()
		os.Exit(0)
	}()
}

func closeOnInterrupt(logger io.Writer, closer io.Closer) {
	onInterrupt(func() {
		if err := closer.Close(); err != nil {
			fmt.Fprintln(logger, err)
		}
	})
}

type SocketRequestErrorHandler interface {
	HandleError(writer io.Writer, err error)
}

type ReadFileDescriptorFromConnection interface {
	ReadNsFileDescriptor(conn net.Conn) (uintptr, error)
}

func newUintptr(u uintptr) *uintptr {
	return &u
}
