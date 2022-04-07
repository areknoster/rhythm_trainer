package domain

import (
	"fmt"
	"math"
	"sync"

	"time"
)

type Measure []bool

type ExerciseConfig struct {
	TracksRhythm []Measure
	BPM          int
	MeasuresNo   int
	// OutBeat should be in [MinOutBeat, MaxOutBeat] range and indicates what part of a beat should an output be turned on to indicate rhythm
	OutBeat float64
	// InBeat should be in [MinInBeat, MaxInBeat] range and indicates what part of a beat should an input be turned on to indicate rhythm
	InBeat float64
}

// UserBeat starts to listen half beat before given note and finishes half beat after it.  It is normalized to [-1.0, 1.0] range.
// 0.0 means that the user hit given note perfectly
type UserBeat struct {
	Track     int
	Measure   int
	Note      int
	Precision float64
}

type ExerciseResult struct {
	measuresNo  int
	inputs      []UserBeat
	trackRhythm []Measure
}

func (er ExerciseResult) Score() float64 {
	type beatRef struct {
		Track   int
		Measure int
		Note    int
	}
	beatsSet := map[beatRef]float64{}

	for _, in := range er.inputs {
		br := beatRef{
			Track:   in.Track,
			Measure: in.Measure,
			Note:    in.Note,
		}
		beatsSet[br] = math.Max(math.Abs(beatsSet[br]), in.Precision) // if there are multiple beats, choose the worst
	}
	const maxScore = 1.0
	score := maxScore
	faultCoef := maxScore / float64(er.measuresNo*len(er.trackRhythm)*len(er.trackRhythm[0]))

	for measure := 1; measure <= er.measuresNo; measure++ {
		for trackIndex, track := range er.trackRhythm {
			for note, expectsBeat := range track {
				note := note + 1
				br := beatRef{
					Track:   trackIndex,
					Measure: measure,
					Note:    note,
				}
				prec, beatWasThere := beatsSet[br]

				switch {
				case expectsBeat && beatWasThere:
					score -= faultCoef * prec
				case expectsBeat && !beatWasThere,
					!expectsBeat && beatWasThere:
					score -= faultCoef
				}

			}
		}
	}

	return score
}

type Track struct {
	In  Input
	Out Output
}

type RhythmTrainer struct {
	tracks    []Track
	metronome Output
}

// NewRhythmTrainer initializes Rhythm trainer with given set of tracks.
func NewRhythmTrainer(metronome Output, tracks ...Track) *RhythmTrainer {
	return &RhythmTrainer{
		tracks:    tracks,
		metronome: metronome,
	}
}

func (rt *RhythmTrainer) RunExercise(ec ExerciseConfig) (ExerciseResult, error) {
	measureLen, err := rt.validateRhythmTracks(ec.TracksRhythm)
	if err != nil {
		return ExerciseResult{}, fmt.Errorf("invalid tracks in config: %w", err)
	}

	inputs := make([]Input, len(ec.TracksRhythm))
	for i := range ec.TracksRhythm {
		inputs[i] = rt.tracks[i].In
	}

	bm := newBeatManager(ec, inputs)

	ticks := bm.startHalfBeatTicker()

	halfBeat := 0 // count them from 1, so we increment at the beginning

	var note int
	for range ticks {
		halfBeat++
		if halfBeat%2 == 1 {
			fmt.Println("metronome")
			bm.outBeat(rt.metronome)
		}
		if halfBeat < 2*measureLen {
			continue // intro
		}
		if halfBeat == 2*measureLen {
			fmt.Println("next note")
			bm.listen()
		}

		if halfBeat%2 == 0 { // start listening to note halfbeat before it appears
			note = bm.nextNote()

		} else {
			for i, rhythm := range ec.TracksRhythm {
				if rhythm[note-1] {
					fmt.Printf("indicate note %d track %d\n", note, i)
					bm.outBeat(rt.tracks[i].Out)
				}
			}
		}
	}
	fmt.Println("stop listening")

	return ExerciseResult{
		measuresNo:  ec.MeasuresNo,
		inputs:      bm.userInputs,
		trackRhythm: ec.TracksRhythm,
	}, nil
}

func (rt *RhythmTrainer) validateRhythmTracks(tracks []Measure) (int, error) {
	if len(tracks) > len(rt.tracks) {
		return 0, fmt.Errorf("requested exercise number of tracks(%d) exceeds available number of tracks(%d)", len(tracks), len(rt.tracks))
	}
	if len(tracks) == 0 {
		return 0, fmt.Errorf("number of tracks in exercise must not be zero")
	}

	measureLen := len(tracks[0])
	for _, track := range tracks {
		if len(track) != measureLen {
			return 0, fmt.Errorf("all tracks rhythms must be the same length")
		}
	}
	return measureLen, nil
}

const (
	MinOutBeat = 0.1
	MaxOutBeat = 0.5
	MinInBeat  = 0.01
	MaxInBeat  = 0.2
)

func clampFunc(min, max float64) func(float64) float64 {
	return func(x float64) float64 {
		return math.Max(min, math.Min(max, x))
	}
}

var (
	clampOutBeat = clampFunc(MinOutBeat, MaxOutBeat)
	clampInBeat  = clampFunc(MinInBeat, MaxInBeat)
)

type beatManager struct {
	beatLen              time.Duration
	outIndicatorDuration time.Duration
	inIndicatorDuration  time.Duration

	measureLen int
	measuresNo int
	inputs     []Input
	userInputs []UserBeat

	mx                 sync.Mutex
	done               chan struct{}
	currentNoteStarted time.Time
	note               int
	measure            int
}

func newBeatManager(config ExerciseConfig, inputs []Input) beatManager {
	beatDuration := time.Minute / time.Duration(config.BPM)
	scaleBeat := func(scale float64) time.Duration {
		return time.Duration(float64(beatDuration) * scale)
	}

	return beatManager{
		beatLen:              beatDuration,
		outIndicatorDuration: scaleBeat(clampOutBeat(config.OutBeat)),
		inIndicatorDuration:  scaleBeat(clampInBeat(config.InBeat)),

		measureLen: len(config.TracksRhythm[0]),
		measuresNo: config.MeasuresNo,

		inputs: inputs,
		done:   make(chan struct{}),
	}
}

func (bm *beatManager) startHalfBeatTicker() <-chan time.Time {
	ticker := time.NewTicker(bm.beatLen / 2)
	closableTicker := make(chan time.Time)
	go func() {
		for {
			select {
			case <-bm.done:
				ticker.Stop()
				close(closableTicker)
				return
			case t := <-ticker.C:
				closableTicker <- t
			}
		}
	}()
	return closableTicker
}

func (bm *beatManager) outBeat(out Output) {
	out.Set(true)
	_ = time.AfterFunc(bm.outIndicatorDuration, func() { out.Set(false) })
}

func (bm *beatManager) nextNote() int {
	bm.mx.Lock()
	bm.note = bm.note%bm.measureLen + 1
	if bm.note == 1 {
		bm.measure++
	}
	bm.currentNoteStarted = time.Now()
	note, measure := bm.note, bm.measure
	bm.mx.Unlock()
	if measure > bm.measuresNo {
		close(bm.done)
	}
	return note
}

const bounceEffectRecheckAfter = time.Millisecond

func (bm *beatManager) listen() {
	t := time.NewTicker(bm.inIndicatorDuration / 20)
	wasUp := make([]*memIO, len(bm.inputs))
	for i := range wasUp {
		wasUp[i] = newMemIO()
	}

	go func() {
		for now := range t.C {
			for track, in := range bm.inputs {
				select {
				case <-bm.done:
					return
				default:
				}
				v := in.Value()
				if v == wasUp[track].Value() {
					continue
				}

				wasUp[track].Set(v)
				if !v {
					continue
				}
				bm.mx.Lock()
				bm.userInputs = append(bm.userInputs, UserBeat{
					Track:     track,
					Measure:   bm.measure,
					Note:      bm.note,
					Precision: float64(now.Sub(bm.currentNoteStarted)-bm.beatLen/2) / float64(bm.beatLen),
				})
				bm.mx.Unlock()

			}
		}
	}()
}
