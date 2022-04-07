package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/areknoster/rhythm_trainer/domain"
	"github.com/areknoster/rhythm_trainer/gpio"

	"github.com/warthog618/gpiod/device/rpi"
)

type GPIOTrack struct {
	In  *gpio.GPIOIn
	Out *gpio.GPIOOut
}

func (g GPIOTrack) ToDomain() domain.Track {
	return domain.Track{
		In:  g.In,
		Out: g.Out,
	}
}

const maxTracks = 3

// This example drives GPIO 22, which is pin J8-15 on a Raspberry Pi.
// The pin is toggled high and low at 1Hz with a 50% duty cycle.
// Do not run this on a device which has this pin externally driven.
func main() {
	metronome := gpio.NewGPIOOut(rpi.GPIO27)

	tracks := []GPIOTrack{
		{
			In:  gpio.NewGPIOIn(rpi.GPIO25),
			Out: gpio.NewGPIOOut(rpi.GPIO24),
		},
		{
			In:  gpio.NewGPIOIn(rpi.GPIO10),
			Out: gpio.NewGPIOOut(rpi.GPIO22),
		},
		{
			In:  gpio.NewGPIOIn(rpi.GPIO17),
			Out: gpio.NewGPIOOut(rpi.GPIO23),
		},
	}

	defer func() {
		for _, track := range tracks {
			track.In.Close()
			track.Out.Close()
		}
	}()

	rt := domain.NewRhythmTrainer(metronome, tracks[0].ToDomain(), tracks[1].ToDomain(), tracks[2].ToDomain())

	// capture exit signals to ensure pin is reverted to input on exit.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)

	fmt.Println("Input up to 3 tracks of the same length using '.' and '*' signs for pause and beat, e.g. .*.*")
	fmt.Println("Each line is a new track. To stop inputting lines, enter empty one")

	rhythmStrings := [maxTracks]string{}
	for i := 0; i < maxTracks; i++ {
		select {
		case <-quit:
			fmt.Println("exiting")
			return
		default:
			fmt.Scanln(&rhythmStrings[i])
		}
	}
	rhythmTracks, err := parseRhythmStrings(rhythmStrings)
	if err != nil {
		log.Print("can't parse user input", err.Error())
		return
	}

	fmt.Println("starting exercise: first you'll see introductory measure and then you'll need to press the buttons in the rhyrhm you set")
	result, err := rt.RunExercise(domain.ExerciseConfig{
		TracksRhythm: rhythmTracks,
		BPM:          100,
		MeasuresNo:   4,
		OutBeat:      0.2,
		InBeat:       0.05,
	})
	if err != nil {
		log.Print("error running exercise", err)
		return
	}
	fmt.Printf("Result: %f", result.Score())

}

func parseRhythmStrings(stringTracks [maxTracks]string) ([]domain.Measure, error) {
	l := len(stringTracks[0])

	measures := make([]domain.Measure, 0)
	for _, st := range stringTracks {
		if len(st) != l {
			return nil, fmt.Errorf("all tracks must be same length")
		}
		if len(st) == 1 {
			return measures, nil
		}

		measure := make(domain.Measure, l)
		for i, letter := range st {
			switch letter {
			case '.':
				measure[i] = false
			case '*':
				measure[i] = true
			default:
				return nil, fmt.Errorf("not allowed sign in input: %v", letter)
			}
		}
		measures = append(measures, measure)
	}
	return measures, nil

}
