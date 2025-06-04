package models

import (
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-signals"
)

type ModelSignalFlag uint

func (f *ModelSignalFlag) set(flag ModelSignalFlag) {
	*f |= flag
}

func (f ModelSignalFlag) True(flag ModelSignalFlag) bool {
	return f&flag == flag
}

const (
	ModelSignalFlagNone ModelSignalFlag = 0
	FlagModelReset      ModelSignalFlag = iota
	FlagModelSetup
	FlagProxySetup
)

type ModelSignalInfo struct {
	Flags ModelSignalFlag
	Data  map[string]any
}

type ModelSignal struct {
	SignalInfo ModelSignalInfo
	Model      *Model
	Object     attrs.Definer
}

var (
	model_signal_pool = signals.NewPool[ModelSignal]()

	SIGNAL_MODEL_SETUP = model_signal_pool.Get("models.Model.setup")
)
