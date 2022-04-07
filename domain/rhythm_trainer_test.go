package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClamp(t *testing.T){
	assert.Equal(t, 0.5, clampFunc(0.1, 0.5)(1.0))
	assert.Equal(t, 0.1, clampFunc(0.1, 0.5)(0.0))
}