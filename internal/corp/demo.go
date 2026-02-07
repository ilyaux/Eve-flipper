package corp

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"time"
)

// DemoCorpProvider generates realistic synthetic corporation data.
// Uses a seeded RNG for deterministic output (same demo every time).
type DemoCorpProvider struct {
	rng     *rand.Rand
	members []CorpMember
	now     time.Time
}

// NewDemoCorpProvider creates a new demo provider with deterministic seed.
func NewDemoCorpProvider() *DemoCorpProvider {
	d := &DemoCorpProvider{
		rng: rand.New(rand.NewSource(424242)),
		now: time.Now().UTC(),
	}
	d.members = d.generateMembers()
	return d
}

func (d *DemoCorpProvider) IsDemo() bool { return true }

// DemoPrices returns a PriceMap with approximate adjusted prices for demo items.
func (d *DemoCorpProvider) DemoPrices() PriceMap {
	prices := make(PriceMap)
	// Ore prices
	for _, ore := range miningOres {
		prices[ore.typeID] = ore.iskPerUnit
	}
	// Industry product prices (approximate)
	for _, prod := range industryProducts {
		switch prod.productName {
		case "Drake":
			prices[prod.productID] = 55_000_000
		case "Muninn":
			prices[prod.productID] = 220_000_000
		case "Sabre":
			prices[prod.productID] = 55_000_000
		case "Scimitar":
			prices[prod.productID] = 275_000_000
		case "Rifter":
			prices[prod.productID] = 450_000
		case "Hurricane":
			prices[prod.productID] = 70_000_000
		case "Noctis":
			prices[prod.productID] = 50_000_000
		case "Antimatter Charge S":
			prices[prod.productID] = 50
		case "Scourge Heavy Missile":
			prices[prod.productID] = 300
		case "100MN Afterburner II":
			prices[prod.productID] = 4_500_000
		}
	}
	// Trade item prices
	for _, item := range tradeItems {
		prices[item.typeID] = (item.minPrice + item.maxPrice) / 2
	}
	return prices
}

func (d *DemoCorpProvider) GetInfo() CorpInfo {
	return CorpInfo{
		CorporationID: 98000042,
		Name:          "Void Horizons",
		Ticker:        "VH0RZ",
		MemberCount:   len(d.members),
	}
}

// ============================================================
// Wallet divisions
// ============================================================

var divisionNames = []string{
	"Master Wallet",
	"Alliance Tax",
	"Market Operations",
	"Industry",
	"SRP Fund",
	"Moon Mining",
	"PI Tax",
}

func (d *DemoCorpProvider) GetWallets() ([]CorpWalletDivision, error) {
	// Deterministic balances
	balances := []float64{
		18_742_531_204.50, // Master Wallet
		3_215_887_102.30,  // Alliance Tax
		8_934_221_550.75,  // Market Operations
		5_672_104_330.20,  // Industry
		6_128_990_415.60,  // SRP Fund
		2_847_663_108.40,  // Moon Mining
		1_523_441_877.90,  // PI Tax
	}
	wallets := make([]CorpWalletDivision, 7)
	for i := 0; i < 7; i++ {
		wallets[i] = CorpWalletDivision{
			Division: i + 1,
			Name:     divisionNames[i],
			Balance:  balances[i],
		}
	}
	return wallets, nil
}

// ============================================================
// Journal
// ============================================================

// ESI ref_type constants used in journal generation.
var journalRefTypes = []struct {
	refType  string
	isIncome bool
	minISK   float64
	maxISK   float64
	weight   int // relative frequency
}{
	{"bounty_prizes", true, 5_000_000, 120_000_000, 35},
	{"market_transaction", true, 1_000_000, 500_000_000, 20},
	{"transaction_tax", false, 50_000, 25_000_000, 15},
	{"planetary_interaction", true, 500_000, 30_000_000, 10},
	{"industry_job_tax", false, 100_000, 10_000_000, 8},
	{"office_rental_fee", false, 5_000_000, 50_000_000, 3},
	{"insurance", true, 2_000_000, 80_000_000, 5},
	{"player_donation", true, 10_000_000, 500_000_000, 4},
	{"corporation_account_withdrawal", false, 50_000_000, 2_000_000_000, 3},
	{"market_escrow", false, 10_000_000, 800_000_000, 8},
	{"brokers_fee", false, 100_000, 15_000_000, 12},
	{"jump_clone_activation_fee", false, 100_000, 900_000, 2},
	{"reprocessing_tax", false, 50_000, 5_000_000, 3},
	{"contract_price", true, 5_000_000, 200_000_000, 6},
	{"moon_mining_extraction_tax", true, 10_000_000, 150_000_000, 7},
}

func (d *DemoCorpProvider) GetJournal(division int, days int) ([]CorpJournalEntry, error) {
	if days <= 0 {
		days = 90
	}
	if division < 1 || division > 7 {
		division = 1
	}

	// Seed per division for deterministic per-division output
	rng := rand.New(rand.NewSource(int64(424242 + division*1000)))

	// Build weighted ref type picker
	totalWeight := 0
	for _, rt := range journalRefTypes {
		totalWeight += rt.weight
	}

	pickRefType := func() int {
		roll := rng.Intn(totalWeight)
		cumul := 0
		for i, rt := range journalRefTypes {
			cumul += rt.weight
			if roll < cumul {
				return i
			}
		}
		return 0
	}

	var entries []CorpJournalEntry
	balance := d.walletBalance(division)
	entryID := int64(division * 10_000_000)

	for day := days - 1; day >= 0; day-- {
		date := d.now.AddDate(0, 0, -day)
		dateStr := date.Format("2006-01-02")

		// Weekdays have more activity
		entriesPerDay := 15 + rng.Intn(25)
		if date.Weekday() == time.Saturday || date.Weekday() == time.Sunday {
			entriesPerDay = 8 + rng.Intn(12)
		}

		// Division-specific filtering: e.g. SRP fund mostly has insurance/donations
		for j := 0; j < entriesPerDay; j++ {
			idx := pickRefType()
			rt := journalRefTypes[idx]

			// Apply division-specific bias
			if !d.divisionAcceptsRefType(division, rt.refType) {
				continue
			}

			amount := rt.minISK + rng.Float64()*(rt.maxISK-rt.minISK)
			// Round to reasonable precision
			amount = math.Round(amount*100) / 100

			if !rt.isIncome {
				amount = -amount
			}
			balance += amount

			// Pick random member as party
			memberIdx := rng.Intn(len(d.members))
			member := d.members[memberIdx]

			hour := rng.Intn(24)
			minute := rng.Intn(60)
			second := rng.Intn(60)
			ts := fmt.Sprintf("%sT%02d:%02d:%02dZ", dateStr, hour, minute, second)

			entryID++
			entries = append(entries, CorpJournalEntry{
				ID:              entryID,
				Date:            ts,
				RefType:         rt.refType,
				Amount:          amount,
				Balance:         math.Round(balance*100) / 100,
				Description:     d.journalDescription(rt.refType, member.Name),
				FirstPartyID:    member.CharacterID,
				FirstPartyName:  member.Name,
				SecondPartyID:   98000042, // corp ID
				SecondPartyName: "Void Horizons",
			})
		}
	}

	// Sort chronologically
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Date < entries[j].Date
	})

	return entries, nil
}

func (d *DemoCorpProvider) walletBalance(division int) float64 {
	wallets, _ := d.GetWallets()
	for _, w := range wallets {
		if w.Division == division {
			return w.Balance
		}
	}
	return 0
}

func (d *DemoCorpProvider) divisionAcceptsRefType(division int, refType string) bool {
	switch division {
	case 1: // Master Wallet — everything
		return true
	case 2: // Alliance Tax
		return refType == "player_donation" || refType == "corporation_account_withdrawal" || refType == "transaction_tax"
	case 3: // Market Operations
		return refType == "market_transaction" || refType == "market_escrow" || refType == "brokers_fee" || refType == "transaction_tax"
	case 4: // Industry
		return refType == "industry_job_tax" || refType == "market_transaction" || refType == "contract_price" || refType == "reprocessing_tax"
	case 5: // SRP Fund
		return refType == "insurance" || refType == "player_donation" || refType == "corporation_account_withdrawal"
	case 6: // Moon Mining
		return refType == "moon_mining_extraction_tax" || refType == "reprocessing_tax" || refType == "market_transaction"
	case 7: // PI Tax
		return refType == "planetary_interaction" || refType == "transaction_tax"
	}
	return true
}

func (d *DemoCorpProvider) journalDescription(refType, memberName string) string {
	switch refType {
	case "bounty_prizes":
		return fmt.Sprintf("Bounty Prizes for killing pirates in %s", d.randomSystem())
	case "market_transaction":
		return "Market Transaction"
	case "transaction_tax":
		return "Transaction Tax"
	case "planetary_interaction":
		return fmt.Sprintf("Customs Office tax from %s", memberName)
	case "industry_job_tax":
		return "Manufacturing/Research Job Tax"
	case "office_rental_fee":
		return "Office Rental Fee"
	case "insurance":
		return fmt.Sprintf("Insurance payment for %s's ship loss", memberName)
	case "player_donation":
		return fmt.Sprintf("Donation from %s", memberName)
	case "corporation_account_withdrawal":
		return fmt.Sprintf("Withdrawal by %s", memberName)
	case "market_escrow":
		return "Market Buy Order Escrow"
	case "brokers_fee":
		return "Broker's Fee"
	case "jump_clone_activation_fee":
		return "Jump Clone Activation Fee"
	case "reprocessing_tax":
		return "Reprocessing Tax"
	case "contract_price":
		return "Contract Payment"
	case "moon_mining_extraction_tax":
		return "Moon Mining Extraction Tax"
	}
	return refType
}

func (d *DemoCorpProvider) randomSystem() string {
	systems := []string{
		"Y-2ANO", "J5A-IX", "3-DMQT", "7X-02R", "PNQY-Y",
		"4-EP12", "Z9PP-H", "B-DBYQ", "KVN-36", "IGE-RI",
	}
	return systems[d.rng.Intn(len(systems))]
}

// ============================================================
// Transactions
// ============================================================

// Well-known items for realistic transactions.
var tradeItems = []struct {
	typeID   int32
	name     string
	minPrice float64
	maxPrice float64
}{
	{34, "Tritanium", 3.5, 6.0},
	{35, "Pyerite", 5.0, 12.0},
	{36, "Mexallon", 30, 70},
	{37, "Isogen", 40, 90},
	{38, "Nocxium", 300, 800},
	{39, "Zydrine", 500, 1500},
	{40, "Megacyte", 1000, 3000},
	{11399, "Morphite", 8000, 16000},
	{24690, "Drake", 45_000_000, 65_000_000},
	{11993, "Muninn", 180_000_000, 260_000_000},
	{22456, "Sabre", 40_000_000, 70_000_000},
	{11978, "Scimitar", 200_000_000, 350_000_000},
	{587, "Rifter", 300_000, 600_000},
	{17703, "Hurricane", 55_000_000, 85_000_000},
	{29984, "Noctis", 40_000_000, 60_000_000},
	{17478, "Raven", 200_000_000, 300_000_000},
	{17480, "Apocalypse", 180_000_000, 280_000_000},
	{12005, "Ishtar", 200_000_000, 350_000_000},
	{44992, "PLEX", 3_500_000, 5_000_000},
}

func (d *DemoCorpProvider) GetTransactions(division int) ([]CorpTransaction, error) {
	rng := rand.New(rand.NewSource(int64(424242 + division*2000)))

	var txns []CorpTransaction
	txnID := int64(division * 20_000_000)

	for day := 89; day >= 0; day-- {
		date := d.now.AddDate(0, 0, -day)
		dateStr := date.Format("2006-01-02")

		txnsPerDay := 5 + rng.Intn(15)
		if date.Weekday() == time.Saturday || date.Weekday() == time.Sunday {
			txnsPerDay = 2 + rng.Intn(8)
		}

		for j := 0; j < txnsPerDay; j++ {
			item := tradeItems[rng.Intn(len(tradeItems))]
			price := item.minPrice + rng.Float64()*(item.maxPrice-item.minPrice)
			qty := int32(1 + rng.Intn(100))
			if price > 1_000_000 {
				qty = int32(1 + rng.Intn(10))
			}
			isBuy := rng.Float64() < 0.45

			member := d.members[rng.Intn(len(d.members))]
			hour := rng.Intn(24)
			minute := rng.Intn(60)
			second := rng.Intn(60)

			txnID++
			txns = append(txns, CorpTransaction{
				TransactionID: txnID,
				Date:          fmt.Sprintf("%sT%02d:%02d:%02dZ", dateStr, hour, minute, second),
				TypeID:        item.typeID,
				TypeName:      item.name,
				Quantity:      qty,
				UnitPrice:     math.Round(price*100) / 100,
				IsBuy:         isBuy,
				LocationID:    60003760, // Jita 4-4
				LocationName:  "Jita IV - Moon 4 - Caldari Navy Assembly Plant",
				ClientID:      member.CharacterID,
				ClientName:    member.Name,
			})
		}
	}

	sort.Slice(txns, func(i, j int) bool {
		return txns[i].Date < txns[j].Date
	})

	return txns, nil
}

// ============================================================
// Members
// ============================================================

// Realistic EVE-style character names.
var demoNames = []string{
	"Kael Draconis", "Zara Voidwalker", "Rix Ashenmaw", "Nyx Ironheart", "Sera Blackthorn",
	"Vex Shadowborn", "Lyra Stormcrest", "Orion Darkforge", "Thane Frostwing", "Mira Starweaver",
	"Drax Nightblade", "Luna Emberveil", "Cade Ironfist", "Nova Sunstrider", "Kyra Deepvoid",
	"Jace Steelhawk", "Ember Duskfall", "Rowan Ashbloom", "Sable Voidfire", "Talon Greywind",
	"Ivy Moonshard", "Dante Stormforge", "Astra Silvertide", "Hawk Bloodraven", "Elise Darkhollow",
	"Magnus Starfall", "Quinn Frostbane", "Raven Nighthollow", "Silas Ironmaw", "Willow Emberthorn",
	"Axel Voidclaw", "Brynn Starchaser", "Cyrus Deepforge", "Dahlia Blackfire", "Echo Shadowmere",
	"Flint Ironveil", "Gaia Moonstrike", "Hector Doomhammer", "Iris Sunveil", "Juno Stormrider",
	"Kane Ashblade", "Lira Frostweaver", "Maxim Darksteel", "Nora Starbloom", "Orik Bloodforge",
	"Petra Nightwind", "Quill Emberheart", "Rex Shadowsteel", "Sage Moonfire", "Tyr Ironwing",
	"Uma Voidbloom", "Vance Stormclaw", "Wren Darkflame", "Xara Sunforge", "Yael Frostmere",
	"Zephyr Ashveil", "Aldric Steelvane", "Bella Moonrider", "Corvin Darkwing", "Daria Starbane",
	"Eryn Bloodmere", "Felix Ironstorm", "Grace Embervane", "Hugo Shadowcrest", "Isla Sunmere",
	"Jasper Voidhammer", "Kira Moonblaze", "Leon Darkstorm", "Maya Starfrost", "Nero Ashclaw",
	"Olive Nightbloom", "Pike Stonewall", "Ruby Frostfire", "Storm Ironhelm", "Terra Moonveil",
	"Ulric Darkblaze", "Vera Starweave", "Wolf Shadowmere", "Xena Bloodtide", "Yara Emberwing",
	"Zane Voidsteel", "Aria Sunblaze", "Blaze Frostclaw", "Cora Nightsteel", "Drake Moonhammer",
	"Eve Stormveil", "Finn Ashstrike", "Gwen Darkbloom",
}

type memberArchetype string

const (
	archetypeMiner         memberArchetype = "miner"
	archetypeRatter        memberArchetype = "ratter"
	archetypeIndustrialist memberArchetype = "industrialist"
	archetypeTrader        memberArchetype = "trader"
	archetypePvPer         memberArchetype = "pvper"
	archetypeCasual        memberArchetype = "casual"
)

var archetypeDistribution = []struct {
	archetype memberArchetype
	weight    int
}{
	{archetypeMiner, 25},
	{archetypeRatter, 20},
	{archetypeIndustrialist, 15},
	{archetypeTrader, 10},
	{archetypePvPer, 15},
	{archetypeCasual, 15},
}

// Ships typical for each archetype.
var archetypeShips = map[memberArchetype][]struct {
	typeID int32
	name   string
}{
	archetypeMiner:         {{17480, "Hulk"}, {22544, "Covetor"}, {17478, "Procurer"}, {33697, "Venture"}},
	archetypeRatter:        {{12005, "Ishtar"}, {24690, "Drake"}, {17703, "Hurricane"}, {17480, "Apocalypse"}},
	archetypeIndustrialist: {{655, "Iteron Mark V"}, {29984, "Noctis"}, {648, "Badger"}, {2998, "Tayra"}},
	archetypeTrader:        {{670, "Capsule"}, {596, "Impairor"}, {601, "Ibis"}},
	archetypePvPer:         {{22456, "Sabre"}, {11993, "Muninn"}, {11978, "Scimitar"}, {587, "Rifter"}},
	archetypeCasual:        {{670, "Capsule"}, {587, "Rifter"}, {596, "Impairor"}},
}

// Staging systems.
var demoSystems = []struct {
	systemID int32
	name     string
}{
	{30004759, "Y-2ANO"},   // main staging
	{30004608, "J5A-IX"},
	{30004762, "3-DMQT"},
	{30004775, "7X-02R"},
	{30002718, "PNQY-Y"},
}

func (d *DemoCorpProvider) generateMembers() []CorpMember {
	rng := rand.New(rand.NewSource(424242))
	totalWeight := 0
	for _, a := range archetypeDistribution {
		totalWeight += a.weight
	}

	pickArchetype := func() memberArchetype {
		roll := rng.Intn(totalWeight)
		cumul := 0
		for _, a := range archetypeDistribution {
			cumul += a.weight
			if roll < cumul {
				return a.archetype
			}
		}
		return archetypeCasual
	}

	members := make([]CorpMember, 0, len(demoNames))
	for i, name := range demoNames {
		arch := pickArchetype()
		charID := int64(2100000000 + i)

		// Last login: most members active within 7 days, some inactive
		daysAgo := rng.Intn(3) // most are recent
		if rng.Float64() < 0.15 {
			daysAgo = 7 + rng.Intn(30) // some inactive
		}
		if rng.Float64() < 0.05 {
			daysAgo = 30 + rng.Intn(60) // very inactive
		}
		lastLogin := d.now.AddDate(0, 0, -daysAgo).Format(time.RFC3339)

		// Pick a ship for the archetype
		ships := archetypeShips[arch]
		ship := ships[rng.Intn(len(ships))]

		// Pick a system
		sys := demoSystems[rng.Intn(len(demoSystems))]

		var roles []string
		if i == 0 {
			roles = []string{"CEO"}
		} else if i <= 3 {
			roles = []string{"Director"}
		} else if rng.Float64() < 0.1 {
			roles = []string{"Hangar_Take_1", "Hangar_Query_1"}
		}

		members = append(members, CorpMember{
			CharacterID: charID,
			Name:        name,
			LastLogin:   lastLogin,
			LogoffDate:  lastLogin,
			ShipTypeID:  ship.typeID,
			ShipName:    ship.name,
			LocationID:  60003760, // simplified
			SystemID:    sys.systemID,
			SystemName:  sys.name,
			Roles:       roles,
		})
	}
	return members
}

func (d *DemoCorpProvider) GetMembers() ([]CorpMember, error) {
	return d.members, nil
}

// ============================================================
// Industry Jobs
// ============================================================

var industryProducts = []struct {
	bpTypeID    int32
	productID   int32
	productName string
	activity    string
}{
	{24691, 24690, "Drake", "manufacturing"},
	{11994, 11993, "Muninn", "manufacturing"},
	{22457, 22456, "Sabre", "manufacturing"},
	{11979, 11978, "Scimitar", "manufacturing"},
	{588, 587, "Rifter", "manufacturing"},
	{17704, 17703, "Hurricane", "manufacturing"},
	{29985, 29984, "Noctis", "manufacturing"},
	{24691, 24690, "Drake", "researching_time_efficiency"},
	{11994, 11993, "Muninn", "researching_material_efficiency"},
	{693, 693, "Antimatter Charge S", "manufacturing"},
	{240, 240, "Scourge Heavy Missile", "manufacturing"},
	{2488, 2488, "100MN Afterburner II", "manufacturing"},
}

func (d *DemoCorpProvider) GetIndustryJobs() ([]CorpIndustryJob, error) {
	rng := rand.New(rand.NewSource(424242 + 5000))

	// Get only industrialist members for installer
	var installers []CorpMember
	for _, m := range d.members {
		// approximate: first 15% are miners, next industrialists region
		if m.CharacterID%5 == 0 {
			installers = append(installers, m)
		}
	}
	if len(installers) == 0 {
		installers = d.members[:5]
	}

	var jobs []CorpIndustryJob
	jobID := int32(1000)

	for day := 59; day >= 0; day-- {
		date := d.now.AddDate(0, 0, -day)
		jobsPerDay := 2 + rng.Intn(6)

		for j := 0; j < jobsPerDay; j++ {
			prod := industryProducts[rng.Intn(len(industryProducts))]
			installer := installers[rng.Intn(len(installers))]
			runs := int32(1 + rng.Intn(10))
			if prod.activity != "manufacturing" {
				runs = int32(1 + rng.Intn(3))
			}

			durationHours := 2 + rng.Intn(72)
			startDate := date.Add(time.Duration(rng.Intn(24)) * time.Hour)
			endDate := startDate.Add(time.Duration(durationHours) * time.Hour)

			status := "delivered"
			if day < 3 && rng.Float64() < 0.4 {
				status = "active"
			}
			if rng.Float64() < 0.03 {
				status = "cancelled"
			}

			jobID++
			jobs = append(jobs, CorpIndustryJob{
				JobID:           jobID,
				InstallerID:     installer.CharacterID,
				InstallerName:   installer.Name,
				Activity:        prod.activity,
				BlueprintTypeID: prod.bpTypeID,
				ProductTypeID:   prod.productID,
				ProductName:     prod.productName,
				Status:          status,
				Runs:            runs,
				StartDate:       startDate.Format(time.RFC3339),
				EndDate:         endDate.Format(time.RFC3339),
				LocationID:      60003760,
				LocationName:    "Y-2ANO - Void Horizons Production Facility",
			})
		}
	}

	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].StartDate < jobs[j].StartDate
	})

	return jobs, nil
}

// ============================================================
// Mining Ledger
// ============================================================

var miningOres = []struct {
	typeID   int32
	name     string
	iskPerUnit float64 // approximate adjusted price for demo ISK estimation
}{
	{1230, "Veldspar", 4.5},
	{1228, "Scordite", 8.0},
	{1224, "Kernite", 45.0},
	{1232, "Omber", 35.0},
	{1227, "Dark Ochre", 80.0},
	{1226, "Spodumain", 120.0},
	{1223, "Bistot", 150.0},
	{1229, "Crokite", 180.0},
	{11396, "Mercoxit", 14000.0},
	{46676, "Rakovene", 200.0},
	{46678, "Bezdnacine", 250.0},
}

func (d *DemoCorpProvider) GetMiningLedger() ([]CorpMiningEntry, error) {
	rng := rand.New(rand.NewSource(424242 + 6000))

	// Filter for miner-archetype members
	var miners []CorpMember
	for _, m := range d.members {
		if m.CharacterID%4 == 0 { // ~25% are miners
			miners = append(miners, m)
		}
	}
	if len(miners) == 0 {
		miners = d.members[:10]
	}

	var entries []CorpMiningEntry

	for day := 29; day >= 0; day-- {
		date := d.now.AddDate(0, 0, -day)
		dateStr := date.Format("2006-01-02")

		// Each miner mines 0-3 ore types per day
		for _, miner := range miners {
			if rng.Float64() < 0.3 { // 30% chance they don't mine today
				continue
			}
			oreCount := 1 + rng.Intn(3)
			for k := 0; k < oreCount; k++ {
				ore := miningOres[rng.Intn(len(miningOres))]
				qty := int64(500 + rng.Intn(15000))

				entries = append(entries, CorpMiningEntry{
					CharacterID:   miner.CharacterID,
					CharacterName: miner.Name,
					Date:          dateStr,
					TypeID:        ore.typeID,
					TypeName:      ore.name,
					Quantity:      qty,
				})
			}
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Date < entries[j].Date
	})

	return entries, nil
}

// ============================================================
// Market Orders
// ============================================================

func (d *DemoCorpProvider) GetOrders() ([]CorpMarketOrder, error) {
	rng := rand.New(rand.NewSource(424242 + 7000))

	// Filter for trader-archetype members
	var traders []CorpMember
	for _, m := range d.members {
		if m.CharacterID%10 == 0 { // ~10% are traders
			traders = append(traders, m)
		}
	}
	if len(traders) == 0 {
		traders = d.members[:5]
	}

	var orders []CorpMarketOrder
	orderID := int64(5000000)

	for _, trader := range traders {
		// Each trader has 5-20 active orders
		numOrders := 5 + rng.Intn(16)
		for j := 0; j < numOrders; j++ {
			item := tradeItems[rng.Intn(len(tradeItems))]
			price := item.minPrice + rng.Float64()*(item.maxPrice-item.minPrice)
			isBuy := rng.Float64() < 0.4
			volTotal := int32(10 + rng.Intn(500))
			if price > 1_000_000 {
				volTotal = int32(1 + rng.Intn(20))
			}
			volRemain := int32(1 + rng.Intn(int(volTotal)))

			daysAgo := rng.Intn(30)
			issued := d.now.AddDate(0, 0, -daysAgo).Format(time.RFC3339)

			orderID++
			orders = append(orders, CorpMarketOrder{
				OrderID:       orderID,
				CharacterID:   trader.CharacterID,
				CharacterName: trader.Name,
				TypeID:        item.typeID,
				TypeName:      item.name,
				Price:         math.Round(price*100) / 100,
				VolumeRemain:  volRemain,
				VolumeTotal:   volTotal,
				IsBuyOrder:    isBuy,
				LocationID:    60003760,
				LocationName:  "Jita IV - Moon 4 - Caldari Navy Assembly Plant",
				Issued:        issued,
				Duration:      90,
				RegionID:      10000002, // The Forge
			})
		}
	}

	return orders, nil
}
