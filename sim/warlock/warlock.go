package warlock

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

var TalentTreeSizes = [3]int{28, 27, 26}

type Warlock struct {
	core.Character
	Talents  *proto.WarlockTalents
	Options  *proto.Warlock_Options
	Rotation *proto.Warlock_Rotation

	procTrackers []*ProcTracker
	majorCds     []*core.MajorCooldown

	Pet *WarlockPet

	ShadowBolt         *core.Spell
	Incinerate         *core.Spell
	Immolate           *core.Spell
	UnstableAffliction *core.Spell
	Corruption         *core.Spell
	Haunt              *core.Spell
	LifeTap            *core.Spell
	DarkPact           *core.Spell
	ChaosBolt          *core.Spell
	SoulFire           *core.Spell
	Conflagrate        *core.Spell
	DrainSoul          *core.Spell
	Shadowburn         *core.Spell

	CurseOfElements      *core.Spell
	CurseOfElementsAuras core.AuraArray
	CurseOfWeakness      *core.Spell
	CurseOfWeaknessAuras core.AuraArray
	CurseOfTongues       *core.Spell
	CurseOfTonguesAuras  core.AuraArray
	CurseOfAgony         *core.Spell
	CurseOfDoom          *core.Spell
	Seed                 *core.Spell
	SeedDamageTracker    []float64

	NightfallProcAura      *core.Aura
	EradicationAura        *core.Aura
	DemonicEmpowerment     *core.Spell
	DemonicEmpowermentAura *core.Aura
	DemonicPactAura        *core.Aura
	DemonicSoulAura        *core.Aura
	Metamorphosis          *core.Spell
	MetamorphosisAura      *core.Aura
	ImmolationAura         *core.Spell
	HauntDebuffAuras       core.AuraArray
	MoltenCoreAura         *core.Aura
	DecimationAura         *core.Aura
	PyroclasmAura          *core.Aura
	BackdraftAura          *core.Aura
	EmpoweredImpAura       *core.Aura
	GlyphOfLifeTapAura     *core.Aura
	SpiritsoftheDamnedAura *core.Aura

	Infernal *InfernalPet
	Inferno  *core.Spell

	// Rotation related memory
	CorruptionRolloverPower float64
	DrainSoulRolloverPower  float64
	// The sum total of demonic pact spell power * seconds.
	DPSPAggregate  float64
	PreviousTime   time.Duration
	SpellsRotation []SpellRotation

	petStmBonusSP                float64
	masterDemonologistFireCrit   float64
	masterDemonologistShadowCrit float64

	CritDebuffCategory *core.ExclusiveCategory
}

type SpellRotation struct {
	Spell    *core.Spell
	CastIn   CastReadyness
	Priority int
}

type CastReadyness func(*core.Simulation) time.Duration

func (warlock *Warlock) GetCharacter() *core.Character {
	return &warlock.Character
}

func (warlock *Warlock) GetWarlock() *Warlock {
	return warlock
}

func (warlock *Warlock) GrandSpellstoneBonus() float64 {
	return core.TernaryFloat64(warlock.Options.WeaponImbue == proto.Warlock_Options_GrandSpellstone, 0.01, 0)
}
func (warlock *Warlock) GrandFirestoneBonus() float64 {
	return core.TernaryFloat64(warlock.Options.WeaponImbue == proto.Warlock_Options_GrandFirestone, 0.01, 0)
}

func (warlock *Warlock) Initialize() {

	warlock.registerIncinerateSpell()
	warlock.registerShadowBoltSpell()
	warlock.registerImmolateSpell()
	warlock.registerCorruptionSpell()
	warlock.registerCurseOfElementsSpell()
	warlock.registerCurseOfWeaknessSpell()
	warlock.registerCurseOfTonguesSpell()
	warlock.registerCurseOfAgonySpell()
	warlock.registerCurseOfDoomSpell()
	warlock.registerLifeTapSpell()
	warlock.registerSeedSpell()
	warlock.registerSoulFireSpell()
	warlock.registerUnstableAfflictionSpell()
	warlock.registerDrainSoulSpell()
	warlock.registerConflagrateSpell()
	warlock.registerHauntSpell()
	warlock.registerChaosBoltSpell()

	warlock.registerDemonicEmpowermentSpell()
	if warlock.Talents.Metamorphosis {
		warlock.registerMetamorphosisSpell()
		warlock.registerImmolationAuraSpell()
	}
	warlock.registerDarkPactSpell()
	warlock.registerShadowBurnSpell()
	warlock.registerInfernoSpell()

	warlock.defineRotation()

	precastSpell := warlock.ShadowBolt
	if warlock.Rotation.Type == proto.Warlock_Rotation_Destruction {
		precastSpell = warlock.SoulFire
	}
	// Do this post-finalize so cast speed is updated with new stats
	warlock.Env.RegisterPostFinalizeEffect(func() {
		precastSpellAt := -warlock.ApplyCastSpeedForSpell(precastSpell.DefaultCast.CastTime, precastSpell)

		warlock.RegisterPrepullAction(precastSpellAt, func(sim *core.Simulation) {
			precastSpell.Cast(sim, warlock.CurrentTarget)
		})
		if warlock.GlyphOfLifeTapAura != nil || warlock.SpiritsoftheDamnedAura != nil {
			warlock.RegisterPrepullAction(precastSpellAt-warlock.SpellGCD(), func(sim *core.Simulation) {
				warlock.LifeTap.Cast(sim, nil)
			})
		}
	})
}

func (warlock *Warlock) AddRaidBuffs(raidBuffs *proto.RaidBuffs) {
	raidBuffs.BloodPact = core.MaxTristate(raidBuffs.BloodPact, core.MakeTristateValue(
		warlock.Options.Summon == proto.Warlock_Options_Imp,
		warlock.Talents.ImprovedImp == 2,
	))

	raidBuffs.FelIntelligence = core.MaxTristate(raidBuffs.FelIntelligence, core.MakeTristateValue(
		warlock.Options.Summon == proto.Warlock_Options_Felhunter,
		warlock.Talents.ImprovedFelhunter == 2,
	))
}

func (warlock *Warlock) Reset(sim *core.Simulation) {
	if sim.CurrentTime == 0 {
		warlock.petStmBonusSP = 0
	}
}

func NewWarlock(character core.Character, options *proto.Player) *Warlock {
	warlockOptions := options.GetWarlock()

	warlock := &Warlock{
		Character: character,
		Talents:   &proto.WarlockTalents{},
		Options:   warlockOptions.Options,
		Rotation:  warlockOptions.Rotation,
		// manaTracker:           common.NewManaSpendingRateTracker(),
	}
	core.FillTalentsProto(warlock.Talents.ProtoReflect(), options.TalentsString, TalentTreeSizes)
	warlock.EnableManaBar()

	warlock.AddStatDependency(stats.Strength, stats.AttackPower, 1)

	if warlock.Options.Armor == proto.Warlock_Options_FelArmor {
		demonicAegisMultiplier := 1 + float64(warlock.Talents.DemonicAegis)*0.1
		amount := 180.0 * demonicAegisMultiplier
		warlock.AddStat(stats.SpellPower, amount)
		warlock.AddStatDependency(stats.Spirit, stats.SpellPower, 0.3*demonicAegisMultiplier)
	}

	if warlock.Options.Summon != proto.Warlock_Options_NoSummon {
		warlock.Pet = warlock.NewWarlockPet()
	}

	if warlock.Rotation.UseInfernal {
		warlock.Infernal = warlock.NewInfernal()
	}

	warlock.applyWeaponImbue()

	return warlock
}

func RegisterWarlock() {
	core.RegisterAgentFactory(
		proto.Player_Warlock{},
		proto.Spec_SpecWarlock,
		func(character core.Character, options *proto.Player) core.Agent {
			return NewWarlock(character, options)
		},
		func(player *proto.Player, spec interface{}) {
			playerSpec, ok := spec.(*proto.Player_Warlock)
			if !ok {
				panic("Invalid spec value for Warlock!")
			}
			player.Spec = playerSpec
		},
	)
}

func init() {
	core.BaseStats[core.BaseStatsKey{Race: proto.Race_RaceSindorei, Class: proto.Class_ClassWarlock}] = stats.Stats{
		stats.Health:    7164,
		stats.Strength:  56,
		stats.Agility:   69,
		stats.Stamina:   89,
		stats.Intellect: 162,
		stats.Spirit:    164,
		stats.Mana:      3856,
		stats.SpellCrit: 1.697 * core.CritRatingPerCritChance,
		// Not sure how stats modify the crit chance.
		// stats.MeleeCrit:   4.43 * core.CritRatingPerCritChance,
	}
	core.BaseStats[core.BaseStatsKey{Race: proto.Race_RaceOrc, Class: proto.Class_ClassWarlock}] = stats.Stats{
		stats.Health:    7164,
		stats.Strength:  62,
		stats.Agility:   64,
		stats.Stamina:   90,
		stats.Intellect: 156,
		stats.Spirit:    168,
		stats.Mana:      3856,
		stats.SpellCrit: 1.697 * core.CritRatingPerCritChance,
		// Not sure how stats modify the crit chance.
		// stats.MeleeCrit:   4.43 * core.CritRatingPerCritChance,
	}
	core.BaseStats[core.BaseStatsKey{Race: proto.Race_RaceUndead, Class: proto.Class_ClassWarlock}] = stats.Stats{
		stats.Health:    7164,
		stats.Strength:  58,
		stats.Agility:   65,
		stats.Stamina:   89,
		stats.Intellect: 157,
		stats.Spirit:    171,
		stats.Mana:      3856,
		stats.SpellCrit: 1.697 * core.CritRatingPerCritChance,
		// Not sure how stats modify the crit chance.
		// stats.MeleeCrit:   4.43 * core.CritRatingPerCritChance,
	}
	core.BaseStats[core.BaseStatsKey{Race: proto.Race_RaceHuman, Class: proto.Class_ClassWarlock}] = stats.Stats{
		stats.Health:    7164,
		stats.Strength:  59,
		stats.Agility:   67,
		stats.Stamina:   89,
		stats.Intellect: 159,
		stats.Spirit:    166, // racial makes this 170
		stats.Mana:      3856,
		stats.SpellCrit: 1.697 * core.CritRatingPerCritChance,
		// Not sure how stats modify the crit chance.
		// stats.MeleeCrit:   4.43 * core.CritRatingPerCritChance,
	}
	core.BaseStats[core.BaseStatsKey{Race: proto.Race_RaceGnome, Class: proto.Class_ClassWarlock}] = stats.Stats{
		stats.Health:    7164,
		stats.Strength:  54,
		stats.Agility:   69,
		stats.Stamina:   89,
		stats.Intellect: 162, // racial makes this 170
		stats.Spirit:    166,
		stats.Mana:      3856,
		stats.SpellCrit: 1.697 * core.CritRatingPerCritChance,
		// Not sure how stats modify the crit chance.
		// stats.MeleeCrit:   4.43 * core.CritRatingPerCritChance,
	}
}

// Agent is a generic way to access underlying warlock on any of the agents.
type WarlockAgent interface {
	GetWarlock() *Warlock
}

func (warlock *Warlock) HasMajorGlyph(glyph proto.WarlockMajorGlyph) bool {
	return warlock.HasGlyph(int32(glyph))
}

func (warlock *Warlock) HasMinorGlyph(glyph proto.WarlockMinorGlyph) bool {
	return warlock.HasGlyph(int32(glyph))
}
