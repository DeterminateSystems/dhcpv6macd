package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"testing"
)

func expectEvent(t *testing.T, subscriber chan IdentifiedEvent, event string) {
	select {
	case v := <-subscriber:
		if v.Event.Event != event || v.Event.Repeated != false {
			t.Fatalf("Expected to get the %s event, got: %v", event, v)
		}
	default:
		t.Fatalf("Expected to get the %s event, got nothing", event)
	}
}

func expectRepeatEvent(t *testing.T, subscriber chan IdentifiedEvent, event string) {
	select {
	case v := <-subscriber:
		if v.Event.Event != event || v.Event.Repeated != true {
			t.Fatalf("Expected to get the repeated %s event, got: %v (repeated: %v)", event, v, v.Event.Repeated)
		}
	default:
		t.Fatalf("Expected to get the %s event, got nothing", event)
	}
}

func expectNoEvent(t *testing.T, subscriber chan IdentifiedEvent) {
	select {
	case v := <-subscriber:
		t.Fatalf("Expected to not receive any events, but got: %v", v)
	default:
		// Great!
	}
}

func TestTransitionsHttpBootMatchEvents(t *testing.T) {
	makeTimeBogus()

	mac := net.HardwareAddr{0x04, 0x42, 0x1a, 0x03, 0x9b, 0x20}
	broker := NewBroker()
	subscriber, unsubscribe := broker.Subscribe()
	defer unsubscribe()

	machine := NewMachine(mac, broker)

	{
		expectEvent(t, subscriber, "init")

		want := "[{init bogustime false <nil>}]"
		got := fmt.Sprint(machine.Events.Slice())
		if got != want {
			t.Fatalf("Wanted %s, got %s", want, got)
		}
	}

	{
		machine.Event(context.Background(), "firmware_init", nil)

		expectEvent(t, subscriber, "firmware_init")

		want := "[{init bogustime false <nil>} {firmware_init bogustime false <nil>}]"
		got := fmt.Sprint(machine.Events.Slice())
		if got != want {
			t.Fatalf("Wanted %s, got %s", want, got)
		}
	}

	{
		machine.Event(context.Background(), "http_boot", nil)
		expectEvent(t, subscriber, "http_boot")

		want := "[{init bogustime false <nil>} {firmware_init bogustime false <nil>} {http_boot bogustime false <nil>}]"
		got := fmt.Sprint(machine.Events.Slice())
		if got != want {
			t.Fatalf("Wanted %s, got %s", want, got)
		}
	}

	expectNoEvent(t, subscriber)
}

func TestTransitionsPxeBootMatchEvents(t *testing.T) {
	makeTimeBogus()

	mac := net.HardwareAddr{0x04, 0x42, 0x1a, 0x03, 0x9b, 0x20}
	broker := NewBroker()
	subscriber, unsubscribe := broker.Subscribe()
	defer unsubscribe()

	machine := NewMachine(mac, broker)

	{
		expectEvent(t, subscriber, "init")

		want := "[{init bogustime false <nil>}]"
		got := fmt.Sprint(machine.Events.Slice())
		if got != want {
			t.Fatalf("Wanted %s, got %s", want, got)
		}
	}

	{
		machine.Event(context.Background(), "firmware_init", nil)
		expectEvent(t, subscriber, "firmware_init")

		want := "[{init bogustime false <nil>} {firmware_init bogustime false <nil>}]"
		got := fmt.Sprint(machine.Events.Slice())
		if got != want {
			t.Fatalf("Wanted %s, got %s", want, got)
		}
	}

	{
		machine.Event(context.Background(), "point_pxe_to_ipxe_over_tftp", nil)
		expectEvent(t, subscriber, "point_pxe_to_ipxe_over_tftp")

		want := "[{init bogustime false <nil>} {firmware_init bogustime false <nil>} {point_pxe_to_ipxe_over_tftp bogustime false <nil>}]"
		got := fmt.Sprint(machine.Events.Slice())
		if got != want {
			t.Fatalf("Wanted %s, got %s", want, got)
		}
	}

	{
		machine.Event(context.Background(), "serve_ipxe_over_tftp", nil)
		expectEvent(t, subscriber, "serve_ipxe_over_tftp")

		want := "[{init bogustime false <nil>} {firmware_init bogustime false <nil>} {point_pxe_to_ipxe_over_tftp bogustime false <nil>} {serve_ipxe_over_tftp bogustime false <nil>}]"
		got := fmt.Sprint(machine.Events.Slice())
		if got != want {
			t.Fatalf("Wanted %s, got %s", want, got)
		}
	}

	{
		machine.Event(context.Background(), "point_ipxe_to_http_boot", nil)
		expectEvent(t, subscriber, "point_ipxe_to_http_boot")

		want := "[{init bogustime false <nil>} {firmware_init bogustime false <nil>} {point_pxe_to_ipxe_over_tftp bogustime false <nil>} {serve_ipxe_over_tftp bogustime false <nil>} {point_ipxe_to_http_boot bogustime false <nil>}]"
		got := fmt.Sprint(machine.Events.Slice())
		if got != want {
			t.Fatalf("Wanted %s, got %s", want, got)
		}
	}

	{
		machine.Event(context.Background(), "http_fetch_uki", nil)
		expectEvent(t, subscriber, "http_fetch_uki")

		want := "[{init bogustime false <nil>} {firmware_init bogustime false <nil>} {point_pxe_to_ipxe_over_tftp bogustime false <nil>} {serve_ipxe_over_tftp bogustime false <nil>} {point_ipxe_to_http_boot bogustime false <nil>} {http_fetch_uki bogustime false <nil>}]"
		got := fmt.Sprint(machine.Events.Slice())
		if got != want {
			t.Fatalf("Wanted %s, got %s", want, got)
		}
	}

	{
		machine.Event(context.Background(), "os_init", nil)
		expectEvent(t, subscriber, "os_init")

		want := "[{init bogustime false <nil>} {firmware_init bogustime false <nil>} {point_pxe_to_ipxe_over_tftp bogustime false <nil>} {serve_ipxe_over_tftp bogustime false <nil>} {point_ipxe_to_http_boot bogustime false <nil>} {http_fetch_uki bogustime false <nil>} {os_init bogustime false <nil>}]"
		got := fmt.Sprint(machine.Events.Slice())
		if got != want {
			t.Fatalf("Wanted %s, got %s", want, got)
		}
	}

	{
		machine.Event(context.Background(), "os_init", nil)
		expectRepeatEvent(t, subscriber, "os_init")

		want := "[{init bogustime false <nil>} {firmware_init bogustime false <nil>} {point_pxe_to_ipxe_over_tftp bogustime false <nil>} {serve_ipxe_over_tftp bogustime false <nil>} {point_ipxe_to_http_boot bogustime false <nil>} {http_fetch_uki bogustime false <nil>} {os_init bogustime false <nil>}]"
		got := fmt.Sprint(machine.Events.Slice())
		if got != want {
			t.Fatalf("Wanted %s, got %s", want, got)
		}
	}

	expectNoEvent(t, subscriber)
}

func TestTransitionsJumpTo(t *testing.T) {
	makeTimeBogus()

	mac := net.HardwareAddr{0x04, 0x42, 0x1a, 0x03, 0x9b, 0x20}
	broker := NewBroker()
	subscriber, unsubscribe := broker.Subscribe()
	defer unsubscribe()

	machine := NewMachine(mac, broker)

	{
		expectEvent(t, subscriber, "init")

		want := "[{init bogustime false <nil>}]"
		got := fmt.Sprint(machine.Events.Slice())
		if got != want {
			t.Fatalf("Wanted %s, got %s", want, got)
		}
	}

	{
		machine.Event(context.Background(), "serve_ipxe_over_tftp", nil)
		expectEvent(t, subscriber, "jump_to")

		expectEvent(t, subscriber, "serve_ipxe_over_tftp")

		expectNoEvent(t, subscriber)

		want := "[{init bogustime false <nil>} {jump_to bogustime false <nil>} {serve_ipxe_over_tftp bogustime false <nil>}]"
		got := fmt.Sprint(machine.Events.Slice())
		if got != want {
			t.Fatalf("Wanted %s, got %s", want, got)
		}
	}

	expectNoEvent(t, subscriber)
}

func TestEncodeMachines(t *testing.T) {
	makeTimeBogus()

	mac := net.HardwareAddr{0x04, 0x42, 0x1a, 0x03, 0x9b, 0x20}
	broker := NewBroker()
	machines := NewMachines(broker)

	machines.GetOrInitMachine(mac).Event(context.Background(), "http_boot", nil)

	jsonbytes, err := json.Marshal(machines)
	if err != nil {
		t.Fatalf("Marshal failure: %v", err)
	}
	json := string(jsonbytes)

	want := "{\"04:42:1a:03:9b:20\":{\"Mac\":\"04:42:1a:03:9b:20\",\"IPv6Address\":\"\",\"Events\":[{\"event\":\"init\",\"timestamp\":\"bogustime\",\"repeat_event\":false,\"detail\":null},{\"event\":\"http_boot\",\"timestamp\":\"bogustime\",\"repeat_event\":false,\"detail\":null}]}}"
	if json != want {
		t.Fatalf("Wanted %s,\ngot: %s", want, json)
	}
}
