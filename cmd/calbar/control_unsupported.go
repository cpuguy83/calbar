//go:build !linux

package main

import "fmt"

type controlServer struct{}

func startControlServer(app *App) (*controlServer, error) {
	return &controlServer{}, nil
}

func (s *controlServer) Close() {}

func sendControlCommand(command string) error {
	if _, ok := controlCommandMethods[command]; !ok {
		return fmt.Errorf("unknown command %q", command)
	}
	return fmt.Errorf("control commands are not implemented on this platform")
}
