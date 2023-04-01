package core

import (
	"fmt"
	"math"
	"time"

	"github.com/wowsims/wotlk/sim/core/stats"
)

type SpellResult struct {
	// Target of the spell.
	Target *Unit

	// Results
	Outcome HitOutcome
	Damage  float64 // Damage done by this cast.
	Threat  float64 // The amount of threat generated by this cast.

	ResistanceMultiplier float64 // Partial Resists / Armor multiplier
	PreOutcomeDamage     float64 // Damage done by this cast before Outcome is applied

	inUse bool
}

func (spell *Spell) NewResult(target *Unit) *SpellResult {
	result := &spell.resultCache
	if result.inUse {
		result = &SpellResult{}
	}

	result.Target = target
	result.Damage = 0
	result.Threat = 0
	result.Outcome = OutcomeEmpty // for blocks
	result.inUse = true

	return result
}
func (spell *Spell) DisposeResult(result *SpellResult) {
	result.inUse = false
}

func (result *SpellResult) Landed() bool {
	return result.Outcome.Matches(OutcomeLanded)
}

func (result *SpellResult) DidCrit() bool {
	return result.Outcome.Matches(OutcomeCrit)
}

func (result *SpellResult) DamageString() string {
	outcomeStr := result.Outcome.String()
	if !result.Landed() {
		return outcomeStr
	}
	return fmt.Sprintf("%s for %0.3f damage", outcomeStr, result.Damage)
}
func (result *SpellResult) HealingString() string {
	return fmt.Sprintf("%s for %0.3f healing", result.Outcome.String(), result.Damage)
}

func (spell *Spell) ThreatFromDamage(outcome HitOutcome, damage float64) float64 {
	if outcome.Matches(OutcomeLanded) {
		return (damage*spell.ThreatMultiplier + spell.FlatThreatBonus) * spell.Unit.PseudoStats.ThreatMultiplier
	} else {
		return 0
	}
}

func (spell *Spell) MeleeAttackPower() float64 {
	return spell.Unit.stats[stats.AttackPower] + spell.Unit.PseudoStats.MobTypeAttackPower
}

func (spell *Spell) RangedAttackPower(target *Unit) float64 {
	return spell.Unit.stats[stats.RangedAttackPower] +
		spell.Unit.PseudoStats.MobTypeAttackPower +
		target.PseudoStats.BonusRangedAttackPowerTaken
}

func (spell *Spell) BonusWeaponDamage() float64 {
	return spell.Unit.PseudoStats.BonusDamage
}

func (spell *Spell) ExpertisePercentage() float64 {
	expertiseRating := spell.Unit.stats[stats.Expertise] + spell.BonusExpertiseRating
	return math.Floor(expertiseRating/ExpertisePerQuarterPercentReduction) / 400
}

func (spell *Spell) PhysicalHitChance(target *Unit) float64 {
	hitRating := spell.Unit.stats[stats.MeleeHit] +
		spell.BonusHitRating +
		target.PseudoStats.BonusMeleeHitRatingTaken
	return hitRating / (MeleeHitRatingPerHitChance * 100)
}

func (spell *Spell) physicalCritRating(target *Unit) float64 {
	return spell.Unit.stats[stats.MeleeCrit] +
		spell.BonusCritRating +
		target.PseudoStats.BonusCritRatingTaken
}
func (spell *Spell) PhysicalCritChance(target *Unit, attackTable *AttackTable) float64 {
	critRating := spell.physicalCritRating(target)
	return (critRating / (CritRatingPerCritChance * 100)) - attackTable.CritSuppression
}
func (spell *Spell) PhysicalCritCheck(sim *Simulation, target *Unit, attackTable *AttackTable) bool {
	return sim.RandomFloat("Physical Crit Roll") < spell.PhysicalCritChance(target, attackTable)
}

func (spell *Spell) SpellPower() float64 {
	return spell.Unit.GetStat(stats.SpellPower) +
		spell.BonusSpellPower +
		spell.Unit.PseudoStats.MobTypeSpellPower
}

func (spell *Spell) SpellHitChance(target *Unit) float64 {
	hitRating := spell.Unit.stats[stats.SpellHit] +
		spell.BonusHitRating +
		target.PseudoStats.BonusSpellHitRatingTaken

	return hitRating / (SpellHitRatingPerHitChance * 100)
}
func (spell *Spell) SpellChanceToMiss(attackTable *AttackTable) float64 {
	return attackTable.BaseSpellMissChance - spell.SpellHitChance(attackTable.Defender)
}
func (spell *Spell) MagicHitCheck(sim *Simulation, attackTable *AttackTable) bool {
	return sim.RandomFloat("Magical Hit Roll") > spell.SpellChanceToMiss(attackTable)
}

func (spell *Spell) spellCritRating(target *Unit) float64 {
	return spell.Unit.stats[stats.SpellCrit] +
		spell.BonusCritRating +
		target.PseudoStats.BonusCritRatingTaken +
		target.PseudoStats.BonusSpellCritRatingTaken
}
func (spell *Spell) SpellCritChance(target *Unit) float64 {
	return spell.spellCritRating(target) / (CritRatingPerCritChance * 100)
}
func (spell *Spell) MagicCritCheck(sim *Simulation, target *Unit) bool {
	critChance := spell.SpellCritChance(target)
	return sim.RandomFloat("Magical Crit Roll") < critChance
}

func (spell *Spell) HealingPower(target *Unit) float64 {
	return spell.SpellPower() + target.PseudoStats.BonusHealingTaken
}
func (spell *Spell) healingCritRating() float64 {
	return spell.Unit.GetStat(stats.SpellCrit) + spell.BonusCritRating
}
func (spell *Spell) HealingCritChance() float64 {
	return spell.healingCritRating() / (CritRatingPerCritChance * 100)
}

func (spell *Spell) HealingCritCheck(sim *Simulation) bool {
	critChance := spell.HealingCritChance()
	return sim.RandomFloat("Healing Crit Roll") < critChance
}

func (spell *Spell) ApplyPostOutcomeDamageModifiers(sim *Simulation, result *SpellResult) {
	for i := range result.Target.DynamicDamageTakenModifiers {
		result.Target.DynamicDamageTakenModifiers[i](sim, spell, result)
	}
	result.Damage = MaxFloat(0, result.Damage)
}

// For spells that do no damage but still have a hit/miss check.
func (spell *Spell) CalcOutcome(sim *Simulation, target *Unit, outcomeApplier OutcomeApplier) *SpellResult {
	attackTable := spell.Unit.AttackTables[target.UnitIndex]
	result := spell.NewResult(target)

	outcomeApplier(sim, result, attackTable)
	result.Threat = spell.ThreatFromDamage(result.Outcome, result.Damage)
	return result
}

func (spell *Spell) calcDamageInternal(sim *Simulation, target *Unit, baseDamage float64, attackerMultiplier float64, isPeriodic bool, outcomeApplier OutcomeApplier) *SpellResult {
	attackTable := spell.Unit.AttackTables[target.UnitIndex]

	result := spell.NewResult(target)
	result.Damage = baseDamage

	if sim.Log == nil {
		result.Damage *= attackerMultiplier
		result.applyTargetModifiers(spell, attackTable, isPeriodic)
		result.applyResistances(sim, spell, isPeriodic, attackTable)
		outcomeApplier(sim, result, attackTable)
		spell.ApplyPostOutcomeDamageModifiers(sim, result)
	} else {
		result.Damage *= attackerMultiplier
		afterAttackMods := result.Damage
		result.applyTargetModifiers(spell, attackTable, isPeriodic)
		afterTargetMods := result.Damage
		result.applyResistances(sim, spell, isPeriodic, attackTable)
		afterResistances := result.Damage
		outcomeApplier(sim, result, attackTable)
		afterOutcome := result.Damage
		spell.ApplyPostOutcomeDamageModifiers(sim, result)
		afterPostOutcome := result.Damage

		spell.Unit.Log(
			sim,
			"%s %s [DEBUG] MAP: %0.01f, RAP: %0.01f, SP: %0.01f, BaseDamage:%0.01f, AfterAttackerMods:%0.01f, AfterTargetMods:%0.01f, AfterResistances:%0.01f, AfterOutcome:%0.01f, AfterPostOutcome:%0.01f",
			target.LogLabel(), spell.ActionID, spell.Unit.GetStat(stats.AttackPower), spell.Unit.GetStat(stats.RangedAttackPower), spell.Unit.GetStat(stats.SpellPower), baseDamage, afterAttackMods, afterTargetMods, afterResistances, afterOutcome, afterPostOutcome)
	}

	result.Threat = spell.ThreatFromDamage(result.Outcome, result.Damage)

	return result
}
func (spell *Spell) CalcDamage(sim *Simulation, target *Unit, baseDamage float64, outcomeApplier OutcomeApplier) *SpellResult {
	attackerMultiplier := spell.AttackerDamageMultiplier(spell.Unit.AttackTables[target.UnitIndex])
	return spell.calcDamageInternal(sim, target, baseDamage, attackerMultiplier, false, outcomeApplier)
}
func (spell *Spell) CalcPeriodicDamage(sim *Simulation, target *Unit, baseDamage float64, outcomeApplier OutcomeApplier) *SpellResult {
	attackerMultiplier := spell.AttackerDamageMultiplier(spell.Unit.AttackTables[target.UnitIndex])
	return spell.calcDamageInternal(sim, target, baseDamage, attackerMultiplier, true, outcomeApplier)
}
func (dot *Dot) CalcSnapshotDamage(sim *Simulation, target *Unit, outcomeApplier OutcomeApplier) *SpellResult {
	return dot.Spell.calcDamageInternal(sim, target, dot.SnapshotBaseDamage, dot.SnapshotAttackerMultiplier, true, outcomeApplier)
}

func (spell *Spell) DealOutcome(sim *Simulation, result *SpellResult) {
	spell.DealDamage(sim, result)
}
func (spell *Spell) CalcAndDealOutcome(sim *Simulation, target *Unit, outcomeApplier OutcomeApplier) *SpellResult {
	result := spell.CalcOutcome(sim, target, outcomeApplier)
	spell.DealDamage(sim, result)
	return result
}

// Applies the fully computed spell result to the sim.
func (spell *Spell) dealDamageInternal(sim *Simulation, isPeriodic bool, result *SpellResult) {
	spell.SpellMetrics[result.Target.UnitIndex].TotalDamage += result.Damage
	spell.SpellMetrics[result.Target.UnitIndex].TotalThreat += result.Threat

	// Mark total damage done in raid so far for health based fights.
	// Don't include damage done by EnemyUnits to Players
	if result.Target.Type == EnemyUnit {
		sim.Encounter.DamageTaken += result.Damage
	}

	if sim.Log != nil {
		if isPeriodic {
			spell.Unit.Log(sim, "%s %s tick %s. (Threat: %0.3f)", result.Target.LogLabel(), spell.ActionID, result.DamageString(), result.Threat)
		} else {
			spell.Unit.Log(sim, "%s %s %s. (Threat: %0.3f)", result.Target.LogLabel(), spell.ActionID, result.DamageString(), result.Threat)
		}
	}

	if isPeriodic {
		spell.Unit.OnPeriodicDamageDealt(sim, spell, result)
		result.Target.OnPeriodicDamageTaken(sim, spell, result)
	} else {
		spell.Unit.OnSpellHitDealt(sim, spell, result)
		result.Target.OnSpellHitTaken(sim, spell, result)
	}

	spell.DisposeResult(result)
}
func (spell *Spell) DealDamage(sim *Simulation, result *SpellResult) {
	spell.dealDamageInternal(sim, false, result)
}
func (spell *Spell) DealPeriodicDamage(sim *Simulation, result *SpellResult) {
	spell.dealDamageInternal(sim, true, result)
}

func (spell *Spell) CalcAndDealDamage(sim *Simulation, target *Unit, baseDamage float64, outcomeApplier OutcomeApplier) *SpellResult {
	result := spell.CalcDamage(sim, target, baseDamage, outcomeApplier)
	spell.DealDamage(sim, result)
	return result
}
func (spell *Spell) CalcAndDealPeriodicDamage(sim *Simulation, target *Unit, baseDamage float64, outcomeApplier OutcomeApplier) *SpellResult {
	result := spell.CalcPeriodicDamage(sim, target, baseDamage, outcomeApplier)
	spell.DealPeriodicDamage(sim, result)
	return result
}
func (dot *Dot) CalcAndDealPeriodicSnapshotDamage(sim *Simulation, target *Unit, outcomeApplier OutcomeApplier) *SpellResult {
	result := dot.CalcSnapshotDamage(sim, target, outcomeApplier)
	dot.Spell.DealPeriodicDamage(sim, result)
	return result
}

func (spell *Spell) calcHealingInternal(sim *Simulation, target *Unit, baseHealing float64, casterMultiplier float64, outcomeApplier OutcomeApplier) *SpellResult {
	attackTable := spell.Unit.AttackTables[target.UnitIndex]

	result := spell.NewResult(target)
	result.Damage = baseHealing

	if sim.Log == nil {
		result.Damage *= casterMultiplier
		result.Damage = spell.applyTargetHealingModifiers(result.Damage, attackTable)
		outcomeApplier(sim, result, attackTable)
	} else {
		result.Damage *= casterMultiplier
		afterCasterMods := result.Damage
		result.Damage = spell.applyTargetHealingModifiers(result.Damage, attackTable)
		afterTargetMods := result.Damage
		outcomeApplier(sim, result, attackTable)
		afterOutcome := result.Damage

		spell.Unit.Log(
			sim,
			"%s %s [DEBUG] HealingPower: %0.01f, BaseHealing:%0.01f, AfterCasterMods:%0.01f, AfterTargetMods:%0.01f, AfterOutcome:%0.01f",
			target.LogLabel(), spell.ActionID, spell.HealingPower(target), baseHealing, afterCasterMods, afterTargetMods, afterOutcome)
	}

	result.Threat = spell.ThreatFromDamage(result.Outcome, result.Damage)

	return result
}
func (spell *Spell) CalcHealing(sim *Simulation, target *Unit, baseHealing float64, outcomeApplier OutcomeApplier) *SpellResult {
	return spell.calcHealingInternal(sim, target, baseHealing, spell.CasterHealingMultiplier(), outcomeApplier)
}
func (dot *Dot) CalcSnapshotHealing(sim *Simulation, target *Unit, outcomeApplier OutcomeApplier) *SpellResult {
	return dot.Spell.calcHealingInternal(sim, target, dot.SnapshotBaseDamage, dot.SnapshotAttackerMultiplier, outcomeApplier)
}

// Applies the fully computed spell result to the sim.
func (spell *Spell) dealHealingInternal(sim *Simulation, isPeriodic bool, result *SpellResult) {
	spell.SpellMetrics[result.Target.UnitIndex].TotalHealing += result.Damage
	spell.SpellMetrics[result.Target.UnitIndex].TotalThreat += result.Threat
	if result.Target.HasHealthBar() {
		result.Target.GainHealth(sim, result.Damage, spell.HealthMetrics(result.Target))
	}

	if sim.Log != nil {
		if isPeriodic {
			spell.Unit.Log(sim, "%s %s tick %s. (Threat: %0.3f)", result.Target.LogLabel(), spell.ActionID, result.HealingString(), result.Threat)
		} else {
			spell.Unit.Log(sim, "%s %s %s. (Threat: %0.3f)", result.Target.LogLabel(), spell.ActionID, result.HealingString(), result.Threat)
		}
	}

	if isPeriodic {
		spell.Unit.OnPeriodicHealDealt(sim, spell, result)
		result.Target.OnPeriodicHealTaken(sim, spell, result)
	} else {
		spell.Unit.OnHealDealt(sim, spell, result)
		result.Target.OnHealTaken(sim, spell, result)
	}

	spell.DisposeResult(result)
}
func (spell *Spell) DealHealing(sim *Simulation, result *SpellResult) {
	spell.dealHealingInternal(sim, false, result)
}
func (spell *Spell) DealPeriodicHealing(sim *Simulation, result *SpellResult) {
	spell.dealHealingInternal(sim, true, result)
}

func (spell *Spell) CalcAndDealHealing(sim *Simulation, target *Unit, baseHealing float64, outcomeApplier OutcomeApplier) *SpellResult {
	result := spell.CalcHealing(sim, target, baseHealing, outcomeApplier)
	spell.DealHealing(sim, result)
	return result
}
func (spell *Spell) CalcAndDealPeriodicHealing(sim *Simulation, target *Unit, baseHealing float64, outcomeApplier OutcomeApplier) *SpellResult {
	// This is currently identical to CalcAndDealHealing, but keeping it separate in case they become different in the future.
	return spell.CalcAndDealHealing(sim, target, baseHealing, outcomeApplier)
}
func (dot *Dot) CalcAndDealPeriodicSnapshotHealing(sim *Simulation, target *Unit, outcomeApplier OutcomeApplier) *SpellResult {
	result := dot.CalcSnapshotHealing(sim, target, outcomeApplier)
	dot.Spell.DealPeriodicHealing(sim, result)
	return result
}

func (spell *Spell) WaitTravelTime(sim *Simulation, callback func(*Simulation)) {
	travelTime := time.Duration(float64(time.Second) * spell.Unit.DistanceFromTarget / spell.MissileSpeed)
	StartDelayedAction(sim, DelayedActionOptions{
		DoAt:     sim.CurrentTime + travelTime,
		OnAction: callback,
	})
}

// Returns the combined attacker modifiers.
func (spell *Spell) AttackerDamageMultiplier(attackTable *AttackTable) float64 {
	return spell.attackerDamageMultiplierInternal(attackTable) *
		spell.DamageMultiplier *
		spell.DamageMultiplierAdditive
}
func (spell *Spell) attackerDamageMultiplierInternal(attackTable *AttackTable) float64 {
	if spell.Flags.Matches(SpellFlagIgnoreAttackerModifiers) {
		return 1
	}

	ps := spell.Unit.PseudoStats
	return ps.DamageDealtMultiplier *
		ps.SchoolDamageDealtMultiplier[spell.SchoolIndex] *
		attackTable.DamageDealtMultiplier
}

func (result *SpellResult) applyTargetModifiers(spell *Spell, attackTable *AttackTable, isPeriodic bool) {
	if spell.Flags.Matches(SpellFlagIgnoreTargetModifiers) {
		return
	}

	if spell.SpellSchool.Matches(SpellSchoolPhysical) && spell.Flags.Matches(SpellFlagIncludeTargetBonusDamage) {
		result.Damage += attackTable.Defender.PseudoStats.BonusPhysicalDamageTaken
	}

	result.Damage *= spell.TargetDamageMultiplier(attackTable, isPeriodic)
}
func (spell *Spell) TargetDamageMultiplier(attackTable *AttackTable, isPeriodic bool) float64 {
	if spell.Flags.Matches(SpellFlagIgnoreTargetModifiers) {
		return 1
	}

	multiplier := attackTable.Defender.PseudoStats.DamageTakenMultiplier *
		attackTable.Defender.PseudoStats.SchoolDamageTakenMultiplier[spell.SchoolIndex] *
		attackTable.DamageTakenMultiplier

	if spell.Flags.Matches(SpellFlagDisease) {
		multiplier *= attackTable.Defender.PseudoStats.DiseaseDamageTakenMultiplier
	}

	if spell.Flags.Matches(SpellFlagHauntSE) {
		multiplier *= attackTable.HauntSEDamageTakenMultiplier
	}

	if spell.SpellSchool.Matches(SpellSchoolNature) {
		multiplier *= attackTable.NatureDamageTakenMultiplier
	} else if isPeriodic && spell.SpellSchool.Matches(SpellSchoolPhysical) {
		multiplier *= attackTable.Defender.PseudoStats.PeriodicPhysicalDamageTakenMultiplier
	}

	return multiplier
}

func (spell *Spell) CasterHealingMultiplier() float64 {
	if spell.Flags.Matches(SpellFlagIgnoreAttackerModifiers) {
		return 1
	}

	return spell.DamageMultiplier * spell.DamageMultiplierAdditive
}
func (spell *Spell) applyTargetHealingModifiers(damage float64, attackTable *AttackTable) float64 {
	if spell.Flags.Matches(SpellFlagIgnoreTargetModifiers) {
		return damage
	}

	return damage *
		attackTable.Defender.PseudoStats.HealingTakenMultiplier *
		attackTable.HealingDealtMultiplier
}
