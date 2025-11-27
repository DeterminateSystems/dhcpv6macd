package main

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"sync"
	"time"

	"github.com/looplab/fsm"
)

type Machines struct {
	mu       sync.RWMutex
	broker   *Broker
	machines map[MACKey]*Machine
}

func NewMachines(broker *Broker) *Machines {
	return &Machines{
		mu:       sync.RWMutex{},
		broker:   broker,
		machines: make(map[MACKey]*Machine),
	}
}

func (m *Machines) GetOrInitMachine(mac net.HardwareAddr) *Machine {
	key := MACKey(mac.String())

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.machines[key] == nil {
		m.machines[key] = NewMachine(mac, m.broker)
	}

	return m.machines[key]
}

// Lookup the machine stats
func (m *Machines) GetMachine(mac net.HardwareAddr) *Machine {
	key := MACKey(mac.String())

	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.machines[key]
}

func (m *Machines) MarshalJSON() ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return json.Marshal(m.machines)
}

type Machine struct {
	mu          sync.RWMutex
	Mac         MAC
	IPv6Address net.IP
	fsm         *fsm.FSM
	Events      *Ring[Event]
	broker      *Broker
}

type MAC net.HardwareAddr
type MACKey string

type IdentifiedEvent struct {
	Mac         MAC    `json:"mac"`
	IPv6Address net.IP `json:"ipv6_address"`
	Event       Event  `json:"event"`
}

type Event struct {
	Event     string      `json:"event"`
	Timestamp string      `json:"timestamp"`
	Repeated  bool        `json:"repeat_event"`
	Detail    interface{} `json:"detail"`
}

var bogusTimestamp *string

func makeTimeBogus() {
	bogus := "bogustime"
	bogusTimestamp = &bogus
}

func (m MAC) MarshalJSON() ([]byte, error) {
	return json.Marshal(net.HardwareAddr(m).String())
}

func (m MAC) String() string {
	return net.HardwareAddr(m).String()
}

func NewEvent(event string, repeat bool, detail interface{}) Event {
	ev := Event{
		Event:    event,
		Repeated: repeat,
		Detail:   detail,
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
			{Name: "serve_ipxe_over_tftp", Src: []string{"point_pxe_to_ipxe_over_tftp"}, Dst: "serve_ipxe_over_tftp"},
			{Name: "point_ipxe_to_http_boot", Src: []string{"serve_ipxe_over_tftp"}, Dst: "point_ipxe_to_http_boot"},

			{Name: "http_fetch_uki", Src: []string{"http_boot", "point_ipxe_to_http_boot"}, Dst: "http_fetch_uki"},

			{Name: "os_init", Src: []string{"http_fetch_uki"}, Dst: "os_init"},
		},
		fsm.Callbacks{
			"enter_state": func(_ context.Context, e *fsm.Event) {
				var arg interface{}
				if len(e.Args) == 0 {
					arg = nil
				} else if len(e.Args) == 1 {
					arg = e.Args[0]
				} else {
					log.Println("Entered a state where the event had plural arguments", machine.Mac, e)
					arg = e.Args
				}
				machine.Events.Push(NewEvent(e.Dst, false, arg))
			},
		},
	)

	init := NewEvent("init", false, nil)
	machine.Events.Push(init)
	broker.Publish(IdentifiedEvent{
		Mac:   MAC(mac),
		Event: init,
	})

	return &machine
}

func (m *Machine) Can(event string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.fsm.Can(event)
}

func (m *Machine) Cannot(event string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.fsm.Cannot(event)
}

func (m *Machine) Event(ctx context.Context, event string, detail interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var repeat bool

	if m.fsm.Is(event) {
		repeat = true
	} else {
		repeat = false
	}

	identifiedEvent := IdentifiedEvent{
		Mac:         m.Mac,
		IPv6Address: m.IPv6Address,
		Event:       NewEvent(event, repeat, detail),
	}

	if repeat {
		// Emulate the FSM always allowing an event to transition into itself
		m.broker.Publish(identifiedEvent)
		return nil
	} else if m.fsm.Cannot(event) {
		m.resetToWithoutLocking(event, detail)
		return nil
	} else {
		err := m.fsm.Event(ctx, event, detail)
		if err == nil {
			m.broker.Publish(identifiedEvent)
		}
		return err
	}
}

func (m *Machine) resetToWithoutLocking(event string, detail interface{}) {
	jump := NewEvent("jump_to", false, nil)
	m.Events.Push(jump)

	m.broker.Publish(IdentifiedEvent{
		Mac:         m.Mac,
		IPv6Address: m.IPv6Address,
		Event:       jump,
	})

	m.fsm.SetState(event)

	ev := NewEvent(event, false, detail)
	m.Events.Push(ev)

	m.broker.Publish(IdentifiedEvent{
		Mac:         m.Mac,
		IPv6Address: m.IPv6Address,
		Event:       ev,
	})
}
