package warrior

import (
	"github.com/wowsims/wotlk/sim/core"
)

// 319857
func (warrior *Warrior) registerCircularAttackSpell() {
	numHits := min(3, warrior.Env.GetNumTargets())
	results := make([]*core.SpellResult, numHits)

	warrior.CircularAttackOH = warrior.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 319857},
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskMeleeOHSpecial,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage | core.SpellFlagNoOnCastComplete | SpellFlagBloodsurge,

		DamageMultiplier: 1,
		CritMultiplier:   warrior.critMultiplier(oh),
		ThreatMultiplier: 1.0,
	},
	)

	warrior.CircularAttack = warrior.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 319857},
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagIncludeTargetBonusDamage | SpellFlagBloodsurge,

		RageCost: core.RageCostOptions{
			Cost: 0,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: 0,
			},
			IgnoreHaste: true,
		},

		DamageMultiplier: 1,
		CritMultiplier:   warrior.critMultiplier(mh),
		ThreatMultiplier: 1.25,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			curTarget := target
			for hitIndex := int32(0); hitIndex < numHits; hitIndex++ {
				baseDamage := 0 +
					spell.Unit.MHWeaponDamage(sim, spell.MeleeAttackPower()) +
					spell.BonusWeaponDamage()
				results[hitIndex] = spell.CalcDamage(sim, curTarget, baseDamage, spell.OutcomeMeleeWeaponSpecialHitAndCrit)

				curTarget = sim.Environment.NextTargetUnit(curTarget)
			}

			curTarget = target
			for hitIndex := int32(0); hitIndex < numHits; hitIndex++ {
				spell.DealDamage(sim, results[hitIndex])
				curTarget = sim.Environment.NextTargetUnit(curTarget)
			}

			if warrior.WhirlwindOH != nil {
				curTarget = target
				for hitIndex := int32(0); hitIndex < numHits; hitIndex++ {
					baseDamage := 0 +
						spell.Unit.OHWeaponDamage(sim, spell.MeleeAttackPower()) +
						spell.BonusWeaponDamage()
					results[hitIndex] = warrior.CircularAttackOH.CalcDamage(sim, curTarget, baseDamage, warrior.CircularAttackOH.OutcomeMeleeWeaponSpecialHitAndCrit)

					curTarget = sim.Environment.NextTargetUnit(curTarget)
				}

				curTarget = target
				for hitIndex := int32(0); hitIndex < numHits; hitIndex++ {
					warrior.CircularAttackOH.DealDamage(sim, results[hitIndex])
					curTarget = sim.Environment.NextTargetUnit(curTarget)
				}
			}
		},
	})
}

func (warrior *Warrior) CanCircularAttack(sim *core.Simulation) bool {
	if warrior.HasActiveAura("Pouring out anger") {
		return true
	} else {
		return false
	}
}
