package control

import (
	"errors"
	"testing"
)

func TestValidateImageCommands(t *testing.T) {
	ok := []struct {
		name string
		p    map[string]any
	}{
		{CmdExposureLock, map[string]any{"locked": true}},
		{CmdExposureLock, map[string]any{"locked": false}},
		{CmdExposureCompensation, map[string]any{"value": -1.5}},
		{CmdExposureCompensation, map[string]any{"value": 0.0}},
		{CmdWhiteBalanceLock, map[string]any{"locked": true}},
		{CmdWhiteBalanceTemp, map[string]any{"kelvin": 3200.0}},
	}
	for _, c := range ok {
		if err := ValidateCommand(c.name, c.p); err != nil {
			t.Fatalf("%s %v: %v", c.name, c.p, err)
		}
	}
	bad := []struct {
		name string
		p    map[string]any
	}{
		{CmdExposureLock, map[string]any{}},
		{CmdExposureLock, map[string]any{"locked": "sim"}},
		{CmdExposureCompensation, map[string]any{"value": 11.0}},
		{CmdExposureCompensation, map[string]any{"value": "1"}},
		{CmdWhiteBalanceLock, nil},
		{CmdWhiteBalanceTemp, map[string]any{"kelvin": 800.0}},
		{CmdWhiteBalanceTemp, map[string]any{"kelvin": 20000.0}},
	}
	for _, c := range bad {
		if err := ValidateCommand(c.name, c.p); !errors.Is(err, ErrInvalidCommand) {
			t.Fatalf("%s %v deveria ser recusado, veio %v", c.name, c.p, err)
		}
	}
}
