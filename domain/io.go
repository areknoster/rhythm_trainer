package domain

import "sync/atomic"

type Input interface {
	Value() bool
}

type Output interface {
	Set(bool)
}

type memIO struct {
	v *uint32
}

func newMemIO() *memIO {
	var v = uint32(0)
	return &memIO{
		v: &v,
	}
}

func (m *memIO) Value() bool {
	return atomic.LoadUint32(m.v) == 1
}

func (m *memIO) Set(v bool) {
	if v {
		atomic.StoreUint32(m.v, 1)
		return
	}
	atomic.StoreUint32(m.v, 0)
}