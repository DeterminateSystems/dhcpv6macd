package main

import (
	"context"
	"encoding/json"
	"net"
	"time"

	"github.com/looplab/fsm"
)

type Machines map[MACKey]*Machine

func (m Machines) GetMachine(mac net.HardwareAddr) *Machine {
	key := MACKey(mac.String())

	if m[key] == nil {
		m[key] = NewMachine(mac, broker)
	}

	return m[key]
}

type Machine struct {
	Mac    MAC
	fsm    *fsm.FSM
	Events *Ring[Event]
	broker *Broker
}

type MAC net.HardwareAddr
type MACKey string

type IdentifiedEvent struct {
	Mac   MAC   `json:"mac"`
	Event Event `json:"event"`
}

type Event struct {
	Event     string `json:"event"`
	Timestamp string `json:"timestamp"`
}

var bogusTimestamp *string

func makeTimeBogus() {
	bogus := "bogustime"
	bogusTimestamp = &bogus
}

func (m MAC) MarshalJSON() ([]byte, error) {
	return json.Marshal(net.HardwareAddr(m).String())
}

func NewEvent(event string) Event {
	ev := Event{
		Event: event,
	}

	if bogusTimestamp == nil {
		ev.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	} else {
		ev.Timestamp = *bogusTimestamp
	}

	return ev
}

func NewMachine(mac net.HardwareAddr, broker *Broker) *Machine {
	machine := Machine{
		Mac:    MAC(mac),
		broker: broker,
		Events: NewRing[Event](50),
	}

	machine.fsm = fsm.NewFSM(
		"reset",
		fsm.Events{
			{Name: "firmware_init", Src: []string{"reset"}, Dst: "firmware_init"},

			{Name: "http_boot", Src: []string{"firmware_init", "reset"}, Dst: "http_boot"},

			{Name: "point_pxe_to_ipxe_over_tftp", Src: []string{"firmware_init", "reset"}, Dst: "point_pxe_to_ipxe_over_tftp"},
			{Name: "served_ipxe_over_tftp", Src: []string{"point_pxe_to_ipxe_over_tftp"}, Dst: "served_ipxe_over_tftp"},
			{Name: "point_ipxe_to_http_boot", Src: []string{"served_ipxe_over_tftp"}, Dst: "point_ipxe_to_http_boot"},

			{Name: "os_init", Src: []string{"http_boot", "point_ipxe_to_http_boot"}, Dst: "os_init"},
		},
		fsm.Callbacks{
			"enter_state": func(_ context.Context, e *fsm.Event) {
				machine.Events.Push(NewEvent(e.Dst))
			},
		},
	)

	init := NewEvent("init")
	machine.Events.Push(init)
	broker.Publish(IdentifiedEvent{
		Mac:   MAC(mac),
		Event: init,
	})

	return &machine
}

func (m *Machine) Can(event string) bool {
	return m.fsm.Can(event)
}

func (m *Machine) Cannot(event string) bool {
	return m.fsm.Cannot(event)
}

func (m *Machine) Event(ctx context.Context, event string, args ...interface{}) error {
	if m.fsm.Is(event) {
		// Emulate the FSM always allowing an event to transition into itself
		return nil
	}

	if m.Cannot(event) {
		m.ResetTo(event)
		return nil
	} else {
		err := m.fsm.Event(ctx, event, args...)
		if err == nil {
			m.broker.Publish(IdentifiedEvent{
				Mac:   m.Mac,
				Event: NewEvent(event),
			})
		}
		return err
	}
}

func (m *Machine) Reset() {
	event := NewEvent("reset")
	m.broker.Publish(IdentifiedEvent{
		Mac:   m.Mac,
		Event: event,
	})

	m.Events.Push(event)
	m.fsm.SetState("reset")
}

func (m *Machine) ResetTo(event string) {
	jump := NewEvent("jump_to")
	m.Events.Push(jump)

	m.broker.Publish(IdentifiedEvent{
		Mac:   m.Mac,
		Event: jump,
	})

	m.fsm.SetState(event)

	ev := NewEvent(event)
	m.Events.Push(ev)

	m.broker.Publish(IdentifiedEvent{
		Mac:   m.Mac,
		Event: ev,
	})
}
