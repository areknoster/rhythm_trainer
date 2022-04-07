package domain_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/areknoster/rhythm_trainer/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockIO struct {
	v *uint32
}

func newMockIO() *mockIO {
	var v = uint32(0)
	return &mockIO{
		v: &v,
	}
}

func (m *mockIO) Value() bool {
	return atomic.LoadUint32(m.v) == 1
}

func (m *mockIO) Set(v bool) {
	if v {
		atomic.StoreUint32(m.v, 1)
		return
	}
	atomic.StoreUint32(m.v, 0)
}

func TestMockIO(t *testing.T) {
	m := newMockIO()
	assert.Equal(t, false, m.Value())
	m.Set(true)
	assert.Equal(t, true, m.Value())
	m.Set(false)
	assert.Equal(t, false, m.Value())
}

type mockTrack struct {
	in  *mockIO
	out *mockIO
}

func newMockTrack() mockTrack {
	return mockTrack{
		in:  newMockIO(),
		out: newMockIO(),
	}
}

func (mt mockTrack) Track() Track {
	return Track{
		In:  mt.in,
		Out: mt.out,
	}
}

func TestRhythmTrainer(t *testing.T) {
	const (
		measuresNo  = 12
		measureLen  = 4
		bpm         = 60 * measuresNo * 4      // so that all measures are finished in one second
		inputLength = time.Minute / (bpm * 10) // 1/10th of beat is proper input
	)
	methronome := newMockIO()
	mockTracks := []mockTrack{
		newMockTrack(),
		newMockTrack(),
		newMockTrack(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	beatsChan := make(chan struct{}, 1)
	go func() { // sends beats based on metronome to channel
		pv := methronome.Value()
		for {
			select {
			case <-ctx.Done():
				t.Log("stop metronome listener")
				return
			default:
				cv := methronome.Value()
				if !pv && cv {
					beatsChan <- struct{}{}
				}
				pv = cv
				time.Sleep(10 * time.Microsecond)
			}
		}
	}()

	rt := NewRhythmTrainer(
		methronome,
		mockTracks[0].Track(),
		mockTracks[1].Track(),
		mockTracks[2].Track(),
	)

	t.Run("When the input almost perfectly matches rhythm, return > 0.99 score", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		t.Cleanup(cancel)

		givenTracksRhythm := []Measure{
			{true, false, false, false},
			{true, false, true, false},
			{false, true, false, false},
		}
		go func() { // simulates perfect user input
			for i := 0; i < measureLen; i++ { //  intro beats
				select {
				case <-ctx.Done():
					t.Error("cancelled ctx on intro beat phase")
				case <-beatsChan:
					t.Logf("Intro beat %d", i)
				}
			}

			for i := 0; i < measureLen*measuresNo; i++ { //  exercise
				select {
				case <-ctx.Done():
					t.Error("cancelled ctx during exercise")
				case <-beatsChan:
					beatInMeasure := i % measureLen
					for trackIndex, rhythm := range givenTracksRhythm {
						mockTracks[trackIndex].in.Set(rhythm[beatInMeasure])
					}
					time.Sleep(inputLength)
					for _, track := range mockTracks { // turn off all, we finished our click
						track.in.Set(false)
					}
				}
			}
		}()

		result, err := rt.RunExercise(ExerciseConfig{
			TracksRhythm: givenTracksRhythm,
			BPM:          bpm,
			MeasuresNo:   measuresNo,
			InBeat:       0.05,
			OutBeat:      0.2,
		})
		require.NoError(t, err)
		t.Log("score:", result.Score())
		assert.Greater(t, result.Score(), 0.99)
	})

	t.Run("When no input is done and we expect all beats, the score should be below 0.05", func(t *testing.T) {
		givenTracksRhythm := []Measure{
			{true, true, true, true},
			{true, true, true, true},
			{true, true, true, true},
		}
		result, err := rt.RunExercise(ExerciseConfig{
			TracksRhythm: givenTracksRhythm,
			BPM:          bpm,
			MeasuresNo:   measuresNo,
		})
		require.NoError(t, err)
		t.Log("score:", result.Score())
		assert.Less(t, result.Score(), 0.05)
	})

	t.Run("When the number of tracks is greater than the number of IO setup, return an error", func(t *testing.T) {
		_, err := rt.RunExercise(ExerciseConfig{
			TracksRhythm: []Measure{
				{true},
				{true},
				{true},
				{true},
			},
			BPM:        bpm,
			MeasuresNo: measuresNo,
		})
		require.Error(t, err)
	})

	t.Run("When not all tracks are the same length, return an error", func(t *testing.T) {
		_, err := rt.RunExercise(ExerciseConfig{
			TracksRhythm: []Measure{
				{true},
				{true, false},
			},
			BPM:        bpm,
			MeasuresNo: measuresNo,
		})
		require.Error(t, err)
	})

}
