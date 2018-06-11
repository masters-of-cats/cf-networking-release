package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"

	"code.cloudfoundry.org/netplugin-shim/message"
	flags "github.com/jessevdk/go-flags"
	"golang.org/x/sys/unix"
)

type Args struct {
	Socket string `long:"socket"`
}

func parseArgs() (Args, error) {
	var args Args
	return args, nil
}

func main() {
	var args Args
	_, err := flags.Parse(&args)
	exitOn(err)

	conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: args.Socket, Net: "unix"})
	exitOn(err)
	defer conn.Close()

	err = writeNetNSFD(conn)
	exitOn(err)

	msg := message.Message{Command: "up", Handle: "cake", Data: nil}
	encoder := json.NewEncoder(conn)
	encoder.Encode(msg)
}

func writeNetNSFD(socket *net.UnixConn) error {
	socketControlMessage := unix.UnixRights(0)
	_, _, err := socket.WriteMsgUnix(nil, socketControlMessage, nil)
	return err
}

func exitOn(err error) {
	if err != nil {
		fmt.Fprint(os.Stderr, err.Error())
		os.Exit(1)
	}
}
