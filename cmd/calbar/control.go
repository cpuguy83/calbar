package main

import (
	"fmt"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
)

const (
	controlBusName   = "com.github.cpuguy83.CalBar"
	controlPath      = "/com/github/cpuguy83/CalBar"
	controlInterface = "com.github.cpuguy83.CalBar"
)

type controlService struct {
	app *App
}

func (s *controlService) Show() *dbus.Error {
	s.app.ui.Show()
	return nil
}

func (s *controlService) Hide() *dbus.Error {
	s.app.ui.Hide()
	return nil
}

func (s *controlService) Toggle() *dbus.Error {
	s.app.ui.Toggle()
	return nil
}

func (s *controlService) Search() *dbus.Error {
	s.app.ui.Search()
	return nil
}

func (s *controlService) Sync() *dbus.Error {
	s.app.triggerSync()
	return nil
}

func (s *controlService) Quit() *dbus.Error {
	s.app.Quit()
	return nil
}

type controlServer struct {
	conn *dbus.Conn
}

func startControlServer(app *App) (*controlServer, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("connect to session bus: %w", err)
	}

	reply, err := conn.RequestName(controlBusName, dbus.NameFlagDoNotQueue)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("request control bus name: %w", err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		conn.Close()
		return nil, fmt.Errorf("control bus name already owned: %s", controlBusName)
	}

	service := &controlService{app: app}
	if err := conn.Export(service, dbus.ObjectPath(controlPath), controlInterface); err != nil {
		conn.Close()
		return nil, fmt.Errorf("export control interface: %w", err)
	}

	node := &introspect.Node{
		Name: controlPath,
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			{Name: controlInterface, Methods: controlMethods},
		},
	}
	if err := conn.Export(introspect.NewIntrospectable(node), dbus.ObjectPath(controlPath), "org.freedesktop.DBus.Introspectable"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("export control introspection: %w", err)
	}

	return &controlServer{conn: conn}, nil
}

func (s *controlServer) Close() {
	if s == nil || s.conn == nil {
		return
	}
	s.conn.Close()
}

func sendControlCommand(command string) error {
	method, ok := controlCommandMethods[command]
	if !ok {
		return fmt.Errorf("unknown command %q", command)
	}

	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return fmt.Errorf("connect to session bus: %w", err)
	}
	defer conn.Close()

	obj := conn.Object(controlBusName, dbus.ObjectPath(controlPath))
	call := obj.Call(controlInterface+"."+method, 0)
	if call.Err != nil {
		return fmt.Errorf("send %s command: %w", command, call.Err)
	}

	return nil
}

var controlCommandMethods = map[string]string{
	"show":   "Show",
	"hide":   "Hide",
	"toggle": "Toggle",
	"search": "Search",
	"sync":   "Sync",
	"quit":   "Quit",
}

var controlCommandNames = []string{
	"show",
	"hide",
	"toggle",
	"search",
	"sync",
	"quit",
}

var controlCommandDescriptions = map[string]string{
	"show":   "Show the configured CalBar UI",
	"hide":   "Hide the configured CalBar UI",
	"toggle": "Toggle the configured CalBar UI",
	"search": "Show the configured CalBar UI and focus search when supported",
	"sync":   "Trigger a calendar sync",
	"quit":   "Quit the running CalBar instance",
}

var controlMethods = []introspect.Method{
	{Name: "Show"},
	{Name: "Hide"},
	{Name: "Toggle"},
	{Name: "Search"},
	{Name: "Sync"},
	{Name: "Quit"},
}
