package rogue

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

type PriorityAction int32

const (
	Skip PriorityAction = iota
	Build
	Cast
	Wait
)

type GetAction func(*core.Simulation, *Rogue) PriorityAction
type DoAction func(*core.Simulation, *Rogue) bool

type assassinationPrio struct {
	check GetAction
	cast  DoAction
	cost  float64
}

func (rogue *Rogue) targetHasBleed(sim *core.Simulation) bool {
	return rogue.bleedCategory.AnyActive() || rogue.CurrentTarget.HasActiveAuraWithTag(RogueBleedTag)
}

func (rogue *Rogue) setupAssassinationRotation(sim *core.Simulation) {
	rogue.assassinationPrios = make([]assassinationPrio, 0)
	rogue.bleedCategory = rogue.CurrentTarget.GetExclusiveEffectCategory(core.BleedEffectCategory)

	// Garrote
	if rogue.Rotation.OpenWithGarrote {
		hasCastGarrote := false
		rogue.assassinationPrios = append(rogue.assassinationPrios, assassinationPrio{
			func(s *core.Simulation, r *Rogue) PriorityAction {
				if hasCastGarrote {
					return Skip
				}
				if rogue.CurrentEnergy() > rogue.Garrote.DefaultCast.Cost {
					return Cast
				}
				return Wait
			},
			func(s *core.Simulation, r *Rogue) bool {
				casted := r.Garrote.Cast(sim, rogue.CurrentTarget)
				if casted {
					hasCastGarrote = true
				}
				return casted
			},
			rogue.Garrote.DefaultCast.Cost,
		})
	}

	// Slice And Dice
	rogue.assassinationPrios = append(rogue.assassinationPrios, assassinationPrio{
		func(s *core.Simulation, r *Rogue) PriorityAction {
			if rogue.SliceAndDiceAura.IsActive() {
				return Skip
			}
			if rogue.ComboPoints() > 0 && rogue.CurrentEnergy() > rogue.SliceAndDice.DefaultCast.Cost {
				return Cast
			}
			if rogue.ComboPoints() < 1 && rogue.CurrentEnergy() > rogue.Builder.DefaultCast.Cost {
				return Build
			}
			return Wait
		},
		func(s *core.Simulation, r *Rogue) bool {
			return rogue.SliceAndDice.Cast(sim, rogue.CurrentTarget)
		},
		rogue.SliceAndDice.DefaultCast.Cost,
	})

	// Hunger while planning
	if rogue.Talents.HungerForBlood {
		rogue.assassinationPrios = append(rogue.assassinationPrios, assassinationPrio{
			func(s *core.Simulation, r *Rogue) PriorityAction {

				prioExpose := rogue.Rotation.ExposeArmorFrequency == proto.Rogue_Rotation_Once ||
					rogue.Rotation.ExposeArmorFrequency == proto.Rogue_Rotation_Maintain
				if prioExpose && !r.ExposeArmorAuras.Get(rogue.CurrentTarget).IsActive() {
					return Skip
				}

				if r.HungerForBloodAura.IsActive() {
					return Skip
				}

				if !r.targetHasBleed(s) {
					return Skip
				}

				if r.targetHasBleed(s) && r.CurrentEnergy() > r.HungerForBlood.DefaultCast.Cost {
					return Cast
				}
				return Wait
			},
			func(s *core.Simulation, r *Rogue) bool {
				return rogue.HungerForBlood.Cast(sim, rogue.CurrentTarget)
			},
			rogue.HungerForBlood.DefaultCast.Cost,
		})
	}

	// Expose armor
	if rogue.Rotation.ExposeArmorFrequency == proto.Rogue_Rotation_Once ||
		rogue.Rotation.ExposeArmorFrequency == proto.Rogue_Rotation_Maintain {
		hasCastExpose := false
		rogue.assassinationPrios = append(rogue.assassinationPrios, assassinationPrio{
			func(s *core.Simulation, r *Rogue) PriorityAction {
				if hasCastExpose && rogue.Rotation.ExposeArmorFrequency == proto.Rogue_Rotation_Once {
					return Skip
				}
				timeLeft := rogue.ExposeArmorAuras.Get(rogue.CurrentTarget).RemainingDuration(sim)
				minPoints := core.MaxInt32(1, core.MinInt32(rogue.Rotation.MinimumComboPointsExposeArmor, 5))
				if rogue.Rotation.ExposeArmorFrequency != proto.Rogue_Rotation_Once {
					minPoints = 1
				}
				if timeLeft <= 0 {
					if rogue.ComboPoints() < minPoints {
						if rogue.CurrentEnergy() >= rogue.Builder.DefaultCast.Cost {
							return Build
						} else {
							return Wait
						}
					} else {
						if rogue.CurrentEnergy() >= rogue.ExposeArmor.DefaultCast.Cost {
							return Cast
						} else {
							return Wait
						}
					}
				} else {
					energyGained := rogue.getExpectedEnergyPerSecond() * timeLeft.Seconds()
					cpGenerated := energyGained / rogue.Builder.DefaultCast.Cost
					currentCp := float64(rogue.ComboPoints())
					if currentCp+cpGenerated > 5 {
						return Skip
					} else {
						if currentCp < 5 {
							if rogue.CurrentEnergy() >= rogue.Builder.DefaultCast.Cost {
								return Build
							}
						}
						return Wait
					}
				}
			},
			func(s *core.Simulation, r *Rogue) bool {
				casted := r.ExposeArmor.Cast(sim, r.CurrentTarget)
				if casted {
					hasCastExpose = true
				}
				return casted
			},
			rogue.ExposeArmor.DefaultCast.Cost,
		})
	}

	// Rupture for Bleed
	if rogue.Rotation.RuptureForBleed {
		rogue.assassinationPrios = append(rogue.assassinationPrios, assassinationPrio{
			func(s *core.Simulation, r *Rogue) PriorityAction {
				if r.targetHasBleed(s) {
					return Skip
				}
				if rogue.HungerForBloodAura.IsActive() {
					return Skip
				}
				if rogue.ComboPoints() > 0 && rogue.CurrentEnergy() >= rogue.Rupture.DefaultCast.Cost {
					return Cast
				}
				if rogue.ComboPoints() < 1 && rogue.CurrentEnergy() >= rogue.Builder.DefaultCast.Cost {
					return Build
				}
				return Wait
			},
			func(s *core.Simulation, r *Rogue) bool {
				return rogue.Rupture.Cast(sim, rogue.CurrentTarget)
			},
			rogue.Rupture.DefaultCast.Cost,
		})
	}

	// Hunger for Blood
	if rogue.Talents.HungerForBlood {
		rogue.assassinationPrios = append(rogue.assassinationPrios, assassinationPrio{
			func(s *core.Simulation, r *Rogue) PriorityAction {

				if r.HungerForBloodAura.IsActive() {
					return Skip
				}

				if !r.targetHasBleed(s) {
					return Skip
				}

				if r.targetHasBleed(s) && r.CurrentEnergy() > r.HungerForBlood.DefaultCast.Cost {
					return Cast
				}
				return Wait
			},
			func(s *core.Simulation, r *Rogue) bool {
				return rogue.HungerForBlood.Cast(sim, rogue.CurrentTarget)
			},
			rogue.HungerForBlood.DefaultCast.Cost,
		})
	}

	// TODO I'd assume this should only be used once, to re-enable MCDs when sensible?
	// Enable CDs
	rogue.assassinationPrios = append(rogue.assassinationPrios, assassinationPrio{
		func(s *core.Simulation, r *Rogue) PriorityAction {
			if r.allMCDsDisabled {
				for _, mcd := range r.GetMajorCooldowns() {
					mcd.Enable()
				}
				r.allMCDsDisabled = false
			}
			return Skip
		},
		func(s *core.Simulation, r *Rogue) bool {
			return false
		},
		0,
	})

	// Rupture
	if rogue.Rotation.AssassinationFinisherPriority == proto.Rogue_Rotation_RuptureEnvenom {
		rogue.assassinationPrios = append(rogue.assassinationPrios, assassinationPrio{
			func(s *core.Simulation, r *Rogue) PriorityAction {
				if r.Rupture.CurDot().IsActive() || s.GetRemainingDuration() < time.Second*18 {
					return Skip
				}
				if rogue.ComboPoints() > 3 && rogue.CurrentEnergy() >= rogue.Rupture.DefaultCast.Cost {
					return Cast
				}
				if rogue.ComboPoints() < 4 && rogue.CurrentEnergy() >= rogue.Builder.DefaultCast.Cost {
					return Build
				}
				return Wait

			},
			func(s *core.Simulation, r *Rogue) bool {
				return r.Rupture.Cast(s, r.CurrentTarget)
			},
			rogue.Rupture.DefaultCast.Cost,
		})
	}

	// Envenom
	rogue.assassinationPrios = append(rogue.assassinationPrios, assassinationPrio{
		func(s *core.Simulation, r *Rogue) PriorityAction {
			minimumCP := int32(4)
			if r.ComboPoints() >= minimumCP {
				// Don't pool when fight is about to end
				fightEndsSoon := false
				energyNeeded := r.Envenom.DefaultCast.Cost
				if s.GetRemainingDuration() <= time.Second*6 {
					fightEndsSoon = true
				} else {
					energyNeeded = r.getEnvenomThreshold(s)
				}
				if r.CurrentEnergy() >= energyNeeded {
					if !fightEndsSoon && r.EnvenomAura.IsActive() && r.CurrentEnergy() < (r.maxEnergy-16) {
						return Wait
					}
					return Cast
				}
			}
			if r.ComboPoints() < 4 && r.CurrentEnergy() >= r.Builder.DefaultCast.Cost {
				return Build
			}
			return Wait
		},
		func(s *core.Simulation, r *Rogue) bool {
			return rogue.Envenom.Cast(sim, rogue.CurrentTarget)
		},
		rogue.Envenom.DefaultCast.Cost,
	})
}

func (r *Rogue) getEnvenomThreshold(s *core.Simulation) float64 {
	hasOverkill := r.OverkillAura.RemainingDuration(s) > time.Duration(3)*time.Second
	energyNeeded := core.MinFloat(r.maxEnergy, float64(r.Rotation.EnvenomEnergyThreshold))
	if r.ComboPoints() == 5 {
		if hasOverkill {
			energyNeeded = core.MinFloat(energyNeeded, float64(r.Rotation.EnvenomEnergyThresholdOverkillMin))
		} else {
			energyNeeded = core.MinFloat(energyNeeded, float64(r.Rotation.EnvenomEnergyThresholdMin))
		}
	} else if hasOverkill {
		energyNeeded = core.MinFloat(r.maxEnergy, float64(r.Rotation.EnvenomEnergyThresholdOverkill))
	}
	energyNeeded = core.MaxFloat(r.Envenom.DefaultCast.Cost, energyNeeded)
	return energyNeeded
}

func (rogue *Rogue) doAssassinationRotation(sim *core.Simulation) {

	prioIndex := 0
	for prioIndex < len(rogue.assassinationPrios) {
		prio := rogue.assassinationPrios[prioIndex]
		switch prio.check(sim, rogue) {
		case Skip:
			prioIndex += 1
		case Build:
			if rogue.GCD.IsReady(sim) {
				if !rogue.Builder.Cast(sim, rogue.CurrentTarget) {
					rogue.WaitForEnergy(sim, rogue.Builder.DefaultCast.Cost)
					return
				}
			}
			rogue.DoNothing()
			return
		case Cast:
			if rogue.GCD.IsReady(sim) {
				if !prio.cast(sim, rogue) {
					rogue.WaitForEnergy(sim, prio.cost)
					return
				}
			}
			rogue.DoNothing()
			return
		case Wait:
			rogue.DoNothing()
			return
		}
	}
	rogue.DoNothing()
}

func (rogue *Rogue) OnCanAct(sim *core.Simulation) {
	if rogue.KillingSpreeAura.IsActive() {
		rogue.DoNothing()
		return
	}
	rogue.TryUseCooldowns(sim)
	if rogue.GCD.IsReady(sim) {
		rogue.doAssassinationRotation(sim)
	}
}
