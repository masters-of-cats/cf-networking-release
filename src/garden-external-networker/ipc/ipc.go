package ipc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"garden-external-networker/manager"
	"io"
	"net"

	"code.cloudfoundry.org/netplugin-shim/message"

	"golang.org/x/sys/unix"
)

type Mux struct {
	Up   func(handle string, inputs manager.UpInputs) (*manager.UpOutputs, error)
	Down func(handle string) error
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

func (m *Mux) HandleWithSocket(socketPath string) error {
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}

	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}

		fd, err := getFD(conn)
		if err != nil {
			return err
		}

		msg, err := decodeMsg(conn)
		if err != nil {
			panic(err)
		}

		jsonMessage, err := json.Marshal(struct {
			message.Message
			NetNsFd uintptr
		}{
			Message: msg,
			NetNsFd: fd,
		})
		if err != nil {
			panic(err)
		}

		stdIn := bytes.NewReader(jsonMessage)

		m.Handle(msg.Command, msg.Handle, stdIn, conn)

		conn.Close()
	}
}

func getFD(conn net.Conn) (uintptr, error) {
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
