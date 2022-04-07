package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/warthog618/gpiod"
	"github.com/warthog618/gpiod/device/rpi"
)

// This example drives GPIO 22, which is pin J8-15 on a Raspberry Pi.
// The pin is toggled high and low at 1Hz with a 50% duty cycle.
// Do not run this on a device which has this pin externally driven.
func main() {

	in, err := gpiod.RequestLine("gpiochip0", rpi.GPIO18, gpiod.AsInput)
	if err != nil {
		panic(err)
	}
	defer in.Close()

	out, err := gpiod.RequestLine("gpiochip0", rpi.GPIO27, gpiod.AsOutput())
	if err != nil {
		panic(err)
	}
	defer out.Close()

	// capture exit signals to ensure pin is reverted to input on exit.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)

	for {
		select {
		case <-time.After(time.Second):
			val, err := in.Value()
			if err != nil {
				panic(err)
			}
			fmt.Printf("read %v", val)
			err = out.SetValue(val)
			if err != nil {
				panic(err)
			}
		case <-quit:
			return
		}
	}
}
