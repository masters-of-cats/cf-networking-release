package ipc

import (
	"encoding/json"
	"errors"
	"io"
	"net"

	"code.cloudfoundry.org/netplugin-shim/message"
	"golang.org/x/sys/unix"
)

type UnixSocketManager struct{}

func (sm UnixSocketManager) ReadFileDescriptor(connection net.Conn) (uintptr, error) {
	unixconn, ok := connection.(*net.UnixConn)
	if !ok {
		return 0, errors.New("failed to cast connection to unixconn")
	}

	return receiveFileDescriptor(unixconn)
}

func (sm UnixSocketManager) ReadMessage(reader io.Reader) (message.Message, error) {
	var content message.Message
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&content); err != nil {
		return message.Message{}, err
	}
	return content, nil
}

func receiveFileDescriptor(conn *net.UnixConn) (uintptr, error) {
	controlMessageBytesSpace := unix.CmsgSpace(4)

	controlMessageBytes := make([]byte, controlMessageBytesSpace)
	_, readSocketControlMessageBytes, _, _, err := conn.ReadMsgUnix(nil, controlMessageBytes)
	if err != nil {
		return 0, err
	}

	if readSocketControlMessageBytes > controlMessageBytesSpace {
		return 0, errors.New("received too many bytes from socket control message")
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
