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
		if v.Event.Event != event {
			t.Fatalf("Expected to get the %s event, got: %v", event, v)
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

		want := "[{init bogustime}]"
		if fmt.Sprint(machine.Events.Slice()) != want {
			t.Fatalf("Wanted %s, got %s", want, machine.Events.Slice())
		}
	}

	{
		machine.Event(context.Background(), "firmware_init")

		expectEvent(t, subscriber, "firmware_init")

		want := "[{init bogustime} {firmware_init bogustime}]"
		if fmt.Sprint(machine.Events.Slice()) != want {
			t.Fatalf("Wanted %s, got %s", want, machine.Events.Slice())
		}
	}

	{
		machine.Event(context.Background(), "http_boot")
		expectEvent(t, subscriber, "http_boot")

		want := "[{init bogustime} {firmware_init bogustime} {http_boot bogustime}]"
		if fmt.Sprint(machine.Events.Slice()) != want {
			t.Fatalf("Wanted %s, got %s", want, machine.Events.Slice())
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

		want := "[{init bogustime}]"
		if fmt.Sprint(machine.Events.Slice()) != want {
			t.Fatalf("Wanted %s, got %s", want, machine.Events.Slice())
		}
	}

	{
		machine.Event(context.Background(), "firmware_init")
		expectEvent(t, subscriber, "firmware_init")

		want := "[{init bogustime} {firmware_init bogustime}]"
		if fmt.Sprint(machine.Events.Slice()) != want {
			t.Fatalf("Wanted %s, got %s", want, machine.Events.Slice())
		}
	}

	{
		machine.Event(context.Background(), "point_pxe_to_ipxe_over_tftp")
		expectEvent(t, subscriber, "point_pxe_to_ipxe_over_tftp")

		want := "[{init bogustime} {firmware_init bogustime} {point_pxe_to_ipxe_over_tftp bogustime}]"
		if fmt.Sprint(machine.Events.Slice()) != want {
			t.Fatalf("Wanted %s, got %s", want, machine.Events.Slice())
		}
	}

	{
		machine.Event(context.Background(), "served_ipxe_over_tftp")
		expectEvent(t, subscriber, "served_ipxe_over_tftp")

		want := "[{init bogustime} {firmware_init bogustime} {point_pxe_to_ipxe_over_tftp bogustime} {served_ipxe_over_tftp bogustime}]"
		if fmt.Sprint(machine.Events.Slice()) != want {
			t.Fatalf("Wanted %s, got %s", want, machine.Events.Slice())
		}
	}

	{
		machine.Event(context.Background(), "point_ipxe_to_http_boot")
		expectEvent(t, subscriber, "point_ipxe_to_http_boot")

		want := "[{init bogustime} {firmware_init bogustime} {point_pxe_to_ipxe_over_tftp bogustime} {served_ipxe_over_tftp bogustime} {point_ipxe_to_http_boot bogustime}]"
		if fmt.Sprint(machine.Events.Slice()) != want {
			t.Fatalf("Wanted %s, got %s", want, machine.Events.Slice())
		}
	}

	{
		machine.Event(context.Background(), "os_init")
		expectEvent(t, subscriber, "os_init")

		want := "[{init bogustime} {firmware_init bogustime} {point_pxe_to_ipxe_over_tftp bogustime} {served_ipxe_over_tftp bogustime} {point_ipxe_to_http_boot bogustime} {os_init bogustime}]"
		if fmt.Sprint(machine.Events.Slice()) != want {
			t.Fatalf("Wanted %s, got %s", want, machine.Events.Slice())
		}
	}

	{
		machine.Event(context.Background(), "os_init")
		expectNoEvent(t, subscriber)

		want := "[{init bogustime} {firmware_init bogustime} {point_pxe_to_ipxe_over_tftp bogustime} {served_ipxe_over_tftp bogustime} {point_ipxe_to_http_boot bogustime} {os_init bogustime}]"
		if fmt.Sprint(machine.Events.Slice()) != want {
			t.Fatalf("Wanted %s, got %s", want, machine.Events.Slice())
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

		want := "[{init bogustime}]"
		if fmt.Sprint(machine.Events.Slice()) != want {
			t.Fatalf("Wanted %s, got %s", want, machine.Events.Slice())
		}
	}

	{
		machine.Event(context.Background(), "served_ipxe_over_tftp")
		expectEvent(t, subscriber, "jump_to")

		expectEvent(t, subscriber, "served_ipxe_over_tftp")

		expectNoEvent(t, subscriber)

		want := "[{init bogustime} {jump_to bogustime} {served_ipxe_over_tftp bogustime}]"
		if fmt.Sprint(machine.Events.Slice()) != want {
			t.Fatalf("Wanted %s, got %s", want, machine.Events.Slice())
		}
	}

	expectNoEvent(t, subscriber)
}

func TestEncodeMachines(t *testing.T) {
	makeTimeBogus()

	mac := net.HardwareAddr{0x04, 0x42, 0x1a, 0x03, 0x9b, 0x20}
	broker := NewBroker()
	machines := NewMachines(broker)

	machines.GetOrInitMachine(mac).Event(context.Background(), "http_boot")

	jsonbytes, err := json.Marshal(machines)
	if err != nil {
		t.Fatalf("Marshal failure: %v", err)
	}
	json := string(jsonbytes)

	want := "{\"04:42:1a:03:9b:20\":{\"Mac\":\"04:42:1a:03:9b:20\",\"Events\":[{\"event\":\"init\",\"timestamp\":\"bogustime\"},{\"event\":\"http_boot\",\"timestamp\":\"bogustime\"}]}}"
	if json != want {
		t.Fatalf("Wanted %s,\ngot: %s", want, json)
	}
}
