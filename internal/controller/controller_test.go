package controller_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/thatsimonsguy/hvac-controller/internal/controller"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
	"github.com/thatsimonsguy/hvac-controller/internal/state"
)

func TestGetHeatSources(t *testing.T) {
	t.Run("returns primary and secondary heat pumps correctly", func(t *testing.T) {
		st := &state.SystemState{
			HeatPumps: []model.HeatPump{
				{
					Name:      "hp1",
					IsPrimary: true,
				},
				{
					Name: "hp2",
				},
			},
			Boilers: []model.Boiler{
				{
					Name: "boiler1",
				},
			},
		}

		sources := controller.GetHeatSources(st)

		assert.NotNil(t, sources.Primary)
		assert.Equal(t, "hp1", sources.Primary.Name)
		assert.NotNil(t, sources.Secondary)
		assert.Equal(t, "hp2", sources.Secondary.Name)
		assert.NotNil(t, sources.Tertiary)
		assert.Equal(t, "boiler1", sources.Tertiary.Name)
	})

	t.Run("panics on multiple primaries", func(t *testing.T) {
		st := &state.SystemState{
			HeatPumps: []model.HeatPump{
				{
					Name:      "hp1",
					IsPrimary: true,
				},
				{
					Name:      "hp2",
					IsPrimary: true,
				},
			},
		}

		assert.Panics(t, func() {
			controller.GetHeatSources(st)
		})
	})

	t.Run("returns nil tertiary when no boiler", func(t *testing.T) {
		st := &state.SystemState{
			HeatPumps: []model.HeatPump{
				{
					Name:      "hp1",
					IsPrimary: true,
				},
				{
					Name: "hp2",
				},
			},
			Boilers: []model.Boiler{},
		}

		sources := controller.GetHeatSources(st)
		assert.Nil(t, sources.Tertiary)
	})
}
